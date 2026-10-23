package main

import (
	"bytes"
	"cmp"
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

const hookFile = "mise-cloudflare-session-start.sh"

// gitTop is the repository dir is in, and dir's path in it ("." at its root), both by their real paths.
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

// claude sets the repository dir is in up for Claude Code cloud sessions: the hook script in .claude/hooks, and a
// SessionStart entry in .claude/settings.json that runs it for dir's folder. Settings already there are kept: the
// entry is added to them, or, if one already runs the hook, left as it is.
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
	entry := map[string]any{
		"matcher": "startup",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/` + hookFile + " " + rel,
			"timeout": 600,
		}},
	}
	start, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(slices.Clone(start), entry)
	settings["hooks"] = hooks
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fail(err)
	}
	writeFile(path, append(b, '\n'))
}

// tasksFile is where init writes the mise tasks: mise finds a TOML file in .mise/tasks/ by itself.
var tasksFile = filepath.Join(".mise", "tasks", "mise-cloudflare.toml")

// initRepo sets the repo up, or updates it after an upgrade: the files mise-cloudflare owns, written for this version,
// none of them for editing: the mise tasks, the agent skill, the GitHub Actions workflow (deploying or not, as it is)
// and the Claude Code hook.
func initRepo(ctx context.Context, dir string) {
	if _, err := os.Stat("cloudflare.toml"); err == nil {
		fmt.Println("· cloudflare.toml isn't read any more: its settings are the tasks' flags now (mise run cloudflare github --staging main, mise run cloudflare actions). Delete it")
	}
	b, err := tasksFor(dir)
	if err != nil {
		fail(err)
	}
	writeFile(tasksFile, b)
	// The tasks' old name, before 0.1.3: one mise-cloudflare wrote goes.
	old := filepath.Join(".mise", "tasks", "cloudflare.toml")
	if b, err := os.ReadFile(old); err == nil && bytes.HasPrefix(b, []byte("# mise-cloudflare's mise tasks")) {
		must(os.Remove(old))
		fmt.Println("removed", old, "(now "+tasksFile+")")
	}
	writeSkill(filepath.Join(".claude", "skills"))
	deploys, branch := workflowMode(ctx, dir)
	if path, w, err := workflowFor(ctx, dir, deploys, branch); err == nil {
		writeFile(path, w)
	}
	claude(ctx, dir)
}

// writeSkill installs the agent skill into dir (a repo's .claude/skills).
func writeSkill(dir string) {
	err := fs.WalkDir(skills.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skills.FS.ReadFile(path)
		if err == nil {
			writeFile(filepath.Join(dir, path), b)
		}
		return err
	})
	if err != nil {
		fail(err)
	}
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

// stale names the files init wrote into the repo that this version would write differently; init (cloudflare update)
// writes them again.
func (r repo) stale(ctx context.Context) []string {
	var out []string
	differs := func(path string, want []byte) bool {
		b, err := os.ReadFile(path)
		return err == nil && !bytes.Equal(b, want)
	}
	if want, err := tasksFor(r.dir); err == nil && differs(filepath.Join(r.dir, tasksFile), want) {
		out = append(out, tasksFile+" is out of date for mise-cloudflare "+buildVersion()+": run mise run cloudflare update")
	}
	if want, err := skills.FS.ReadFile("cloudflare/SKILL.md"); err == nil && differs(filepath.Join(r.dir, ".claude", "skills", "cloudflare", "SKILL.md"), want) {
		out = append(out, ".claude/skills/cloudflare/SKILL.md is out of date: run mise run cloudflare update")
	}
	if top, _, err := gitTop(ctx, r.dir); err == nil && differs(filepath.Join(top, ".claude", "hooks", hookFile), mise.SessionStart) {
		out = append(out, ".claude/hooks/"+hookFile+" is out of date: run mise run cloudflare update")
	}
	deploys, branch := workflowMode(ctx, r.dir)
	if path, want, err := workflowFor(ctx, r.dir, deploys, branch); err == nil && differs(path, want) {
		rel, _ := filepath.Rel(r.dir, path)
		out = append(out, rel+" is out of date: run mise run cloudflare update")
	}
	return out
}

const toolPath = "github.com/joeblew999/mise-cloudflare/cmd/cloudflare"

// goTool reports whether the repo at dir runs mise-cloudflare with go tool: its go.mod has the tool line.
func goTool(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && regexp.MustCompile(`(?m)^\s*tool\s+(\(\s*)?`+regexp.QuoteMeta(toolPath)+`\s*$|^\s+`+regexp.QuoteMeta(toolPath)+`\s*$`).Match(b)
}

var (
	taskStart = regexp.MustCompile(`(?m)^\["cloudflare(?::[^"]+)?"\]\n`)
	toolsLine = regexp.MustCompile(`(?m)^tools = \{ `)
)

// tasksFor is the tasks file for the repo at dir. A repo with go.mod's tool line runs go tool cloudflare, at go.mod's
// version, as the file is embedded. Any other repo needs no Go: its tasks run the cloudflare binary, each pinning this
// release (mise's github backend downloads it from the repository's releases), as a task pins any tool it runs.
func tasksFor(dir string) ([]byte, error) {
	if goTool(dir) {
		return mise.Tasks, nil
	}
	v := buildVersion()
	if v == "(devel)" || strings.Contains(v, "-0.") {
		return nil, errors.New("this is a development build, which no release matches: in a repo without go.mod's tool line, use a released cloudflare (mise use github:joeblew999/mise-cloudflare)")
	}
	pin := `"github:joeblew999/mise-cloudflare" = "` + v + `"`
	b := bytes.ReplaceAll(mise.Tasks, []byte("go tool cloudflare"), []byte("cloudflare"))
	// Each task pins the release: in its tools table if it has one, else in a new one after its name.
	heads := taskStart.FindAllIndex(b, -1)
	var out []byte
	for i, h := range heads {
		if i == 0 {
			out = append(out, b[:h[0]]...)
		}
		end := len(b)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		task := b[h[1]:end]
		if loc := toolsLine.FindIndex(task); loc != nil {
			task = append(slices.Clone(task[:loc[1]]), append([]byte(pin+", "), task[loc[1]:]...)...)
			out = append(append(out, b[h[0]:h[1]]...), task...)
		} else {
			out = append(append(append(out, b[h[0]:h[1]]...), "tools = { "+pin+" }\n"...), task...)
		}
	}
	b = out
	return b, nil
}

// workflowPath is the repo's GitHub Actions workflow for the Worker in dir: .github/workflows/cloudflare.yml at the
// repository's root, cloudflare-<folder>.yml for a Worker in a subfolder; name and rel are what it's written with.
func workflowPath(ctx context.Context, dir string) (path, name, rel string, err error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", "", err
	}
	top, _ := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	real, _ := filepath.EvalSymlinks(dir)
	if rel, err = filepath.Rel(top, real); err != nil {
		return "", "", "", err
	}
	rel = filepath.ToSlash(rel)
	name = "cloudflare"
	if rel != "." {
		name += "-" + strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(rel), "-"), "-")
	}
	return filepath.Join(top, ".github", "workflows", name+".yml"), name, rel, nil
}

// deployBranch is the branch a deploying workflow deploys production from, as it's written in it.
var deployBranch = regexp.MustCompile(`github\.ref == 'refs/heads/([^']+)'`)

// workflowMode is how the repo's workflow is now: whether GitHub Actions deploys with it (cloudflare actions), and the
// branch it deploys production from.
func workflowMode(ctx context.Context, dir string) (deploys bool, branch string) {
	path, _, _, err := workflowPath(ctx, dir)
	if err != nil {
		return false, ""
	}
	b, _ := os.ReadFile(path)
	if m := deployBranch.FindSubmatch(b); m != nil {
		return true, string(m[1])
	}
	return false, ""
}

// workflowFor is the repo's GitHub Actions workflow, and where it goes: it checks the repo on every push and pull
// request; deploying, it also deploys production from branch and a preview per pull request.
func workflowFor(ctx context.Context, dir string, deploys bool, branch string) (path string, b []byte, err error) {
	path, name, rel, err := workflowPath(ctx, dir)
	if err != nil {
		return "", nil, err
	}
	tmpl := string(mise.Workflow)
	if deploys {
		tmpl += string(mise.WorkflowDeploy)
	}
	b = []byte(strings.NewReplacer("VERSION", buildVersion(), "NAME", name, "DIR", rel, "BRANCH", cmp.Or(branch, "main")).Replace(tmpl))
	return path, b, nil
}
