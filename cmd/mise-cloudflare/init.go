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

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
	"github.com/joeblew999/mise-cloudflare/templates"
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
	writeFile(tasksFile, templates.Tasks)
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
			out = append(out, name+" is out of date: run mise exec -- mise-cloudflare init")
		}
	}
	check(filepath.Join(r.dir, tasksFile), tasksFile, templates.Tasks)
	if want, err := fs.ReadFile(templates.Skills, "cloudflare/SKILL.md"); err == nil {
		check(filepath.Join(r.dir, ".claude", "skills", "cloudflare", "SKILL.md"), ".claude/skills/cloudflare/SKILL.md", want)
	}
	if top, _, err := gitTop(ctx, r.dir); err == nil {
		check(filepath.Join(top, ".claude", "hooks", hookFile), ".claude/hooks/"+hookFile, templates.SessionStart)
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
		name += "-" + slug(rel)
	}
	b := strings.NewReplacer("NAME", name, "DIR", rel).Replace(string(templates.Workflow))
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
	writeFile(script, templates.SessionStart)
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
	must(fs.WalkDir(templates.Skills, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(templates.Skills, path)
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

var wranglerDep = regexp.MustCompile(`("wrangler"\s*:\s*")([^"]+)(")`)

// pinWrangler pins package.json's wrangler to the exact version installed (node_modules/wrangler), when it's a range.
func pinWrangler(dir string) {
	path := filepath.Join(dir, "package.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	m := wranglerDep.FindSubmatch(b)
	if m == nil || regexp.MustCompile(`^\d+\.\d+\.\d+$`).Match(m[2]) {
		return
	}
	var installed struct{ Version string }
	ib, err := os.ReadFile(filepath.Join(dir, "node_modules", "wrangler", "package.json"))
	if err != nil || json.Unmarshal(ib, &installed) != nil || installed.Version == "" {
		fmt.Printf("· package.json's wrangler is %s, not an exact version: install the dependencies, then run init again to pin the installed one\n", m[2])
		return
	}
	loc := wranglerDep.FindSubmatchIndex(b)
	out := append(append(append([]byte{}, b[:loc[4]]...), installed.Version...), b[loc[5]:]...)
	must(os.WriteFile(path, out, 0o644))
	fmt.Printf("pinned wrangler %s (was %s) in package.json: install the dependencies again to update the lockfile\n", installed.Version, m[2])
}

// fixRepo makes the changes cloudflare check asks for that need no decision, so init leaves the repo passing them:
// .dev.vars, .env, .cloudflare/ and node_modules/ git-ignored, wrangler pinned to the exact version installed, and a .dev.vars.example
// listing secrets.required when there's none. What needs a person (an ID in the wrangler config) check still names.
func fixRepo(ctx context.Context, dir string) {
	if exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil {
		var add []string
		for _, f := range []string{".dev.vars", ".env", ".cloudflare/", "node_modules/"} {
			if exec.Command("git", "-C", dir, "-c", "core.excludesFile=", "check-ignore", "-q", f).Run() != nil {
				add = append(add, f)
			}
		}
		if len(add) > 0 {
			path := filepath.Join(dir, ".gitignore")
			old, _ := os.ReadFile(path)
			s := string(old)
			if s != "" && !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			s += "# The repo's Cloudflare secrets and login cache, and installed packages (mise-cloudflare)\n" + strings.Join(add, "\n") + "\n"
			must(os.WriteFile(path, []byte(s), 0o644))
			fmt.Println("git-ignored", strings.Join(add, ", "), "in .gitignore")
		}
	}
	pinWrangler(dir)
	if _, err := os.Stat(filepath.Join(dir, ".dev.vars.example")); os.IsNotExist(err) && (wrangler.W{Dir: dir}).Installed() {
		if wc, err := wconfig.Read(ctx, dir); err == nil && len(wc.Secrets) > 0 {
			var b strings.Builder
			b.WriteString("# The Worker's secrets.required, for wrangler dev: cloudflare dev copies this to .dev.vars (git-ignored), where you fill them in.\n")
			for _, s := range wc.Secrets {
				b.WriteString(s + "=\n")
			}
			writeFile(filepath.Join(dir, ".dev.vars.example"), []byte(b.String()))
		}
	}
}
