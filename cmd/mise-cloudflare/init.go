package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/mise"
	"github.com/joeblew999/mise-cloudflare/skills"
)

const (
	hookFile = "mise-cloudflare-session-start.sh"
	toolName = "github:joeblew999/mise-cloudflare"
)

// tasksFile is where init writes the mise tasks; mise finds files in .mise/tasks/ by itself.
var tasksFile = filepath.Join(".mise", "tasks", "mise-cloudflare.toml")

// initRepo writes the files mise-cloudflare owns into the repo (again, after an upgrade), then fixes what it can for check.
func initRepo(ctx context.Context, dir string) {
	if _, err := os.Stat("cloudflare.toml"); err == nil {
		fmt.Println("· cloudflare.toml isn't read any more (its settings are task flags now): delete it")
	}
	writeFile(tasksFile, mise.Tasks)
	old := filepath.Join(".mise", "tasks", "cloudflare.toml") // the name before 0.1.3
	if b, err := os.ReadFile(old); err == nil && bytes.HasPrefix(b, []byte("# mise-cloudflare's mise tasks")) {
		must(os.Remove(old))
		fmt.Println("removed", old)
	}
	writeSkill(filepath.Join(".claude", "skills"))
	if path, b, err := workflowFor(ctx, dir); err == nil {
		writeFile(path, b)
	}
	claude(ctx, dir)
	fixRepo(ctx, dir)
	if v := buildVersion(); !pinned(ctx, dir) && regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		fmt.Printf("· pin mise-cloudflare in the repo, so every developer gets this version: mise use %s@%s\n", toolName, v)
	}
}

var pinLine = regexp.MustCompile(`(?m)^\s*"` + regexp.QuoteMeta(toolName) + `"\s*=`)

// pinned reports whether a mise.toml from dir up to the repository's root pins mise-cloudflare.
func pinned(ctx context.Context, dir string) bool {
	top, _, err := gitTop(ctx, dir)
	if err != nil {
		top = dir
	}
	for d, _ := filepath.EvalSymlinks(dir); ; d = filepath.Dir(d) {
		for _, f := range []string{"mise.toml", ".mise.toml"} {
			if b, err := os.ReadFile(filepath.Join(d, f)); err == nil && pinLine.Match(b) {
				return true
			}
		}
		if d == top || d == filepath.Dir(d) {
			return false
		}
	}
}

// stale names the files init wrote that this version would write differently.
func (r repo) stale(ctx context.Context) []string {
	var out []string
	check := func(path, name string, want []byte) {
		if b, err := os.ReadFile(path); err == nil && !bytes.Equal(b, want) {
			out = append(out, name+" is out of date: run mise run cloudflare update")
		}
	}
	check(filepath.Join(r.dir, tasksFile), tasksFile, mise.Tasks)
	if want, err := skills.FS.ReadFile("cloudflare/SKILL.md"); err == nil {
		check(filepath.Join(r.dir, ".claude", "skills", "cloudflare", "SKILL.md"), ".claude/skills/cloudflare/SKILL.md", want)
	}
	if top, _, err := gitTop(ctx, r.dir); err == nil {
		check(filepath.Join(top, ".claude", "hooks", hookFile), ".claude/hooks/"+hookFile, mise.SessionStart)
	}
	if path, want, err := workflowFor(ctx, r.dir); err == nil {
		rel, _ := filepath.Rel(r.dir, path)
		check(path, rel, want)
	}
	return out
}

// workflowFor is the repo's check workflow and where it goes: .github/workflows/cloudflare.yml for a Worker at the
// repository's root, cloudflare-<folder>.yml for one in a subfolder.
func workflowFor(ctx context.Context, dir string) (string, []byte, error) {
	top, rel, err := gitTop(ctx, dir)
	if err != nil {
		return "", nil, err
	}
	name := "cloudflare"
	if rel != "." {
		name += "-" + strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(rel), "-"), "-")
	}
	b := strings.NewReplacer("NAME", name, "DIR", rel).Replace(string(mise.Workflow))
	return filepath.Join(top, ".github", "workflows", name+".yml"), []byte(b), nil
}

// gitTop is the repository dir is in, and dir's path in it ("." at its root), by their real paths.
func gitTop(ctx context.Context, dir string) (top, rel string, err error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", errors.New("not in a git repository")
	}
	top, _ = filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	real, _ := filepath.EvalSymlinks(dir)
	rel, err = filepath.Rel(top, real)
	return top, filepath.ToSlash(rel), err
}

// claude adds the Claude Code cloud session hook to the repository: the script, and a SessionStart entry in
// .claude/settings.json that runs it for dir, keeping the settings already there.
func claude(ctx context.Context, dir string) {
	top, rel, err := gitTop(ctx, dir)
	if err != nil {
		fail(err)
	}
	script := filepath.Join(top, ".claude", "hooks", hookFile)
	writeFile(script, mise.SessionStart)
	must(os.Chmod(script, 0o755))

	path := filepath.Join(top, ".claude", "settings.json")
	settings := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &settings); err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
	}
	if b, _ := json.Marshal(settings); strings.Contains(string(b), hookFile) {
		fmt.Printf("✓ %s runs the hook already\n", path)
		return
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	start, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(slices.Clone(start), map[string]any{
		"matcher": "startup",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/` + hookFile + " " + rel,
			"timeout": 600,
		}},
	})
	settings["hooks"] = hooks
	b, err := json.MarshalIndent(settings, "", "  ")
	must(err)
	writeFile(path, append(b, '\n'))
}

func writeSkill(dir string) {
	must(fs.WalkDir(skills.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skills.FS.ReadFile(path)
		if err == nil {
			writeFile(filepath.Join(dir, path), b)
		}
		return err
	}))
}

func writeFile(path string, b []byte) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		fmt.Printf("✓ %s is up to date\n", path)
		return
	}
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, b, 0o644))
	fmt.Println("wrote", path)
}
