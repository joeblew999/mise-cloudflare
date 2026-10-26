package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/joeblew999/mise-cloudflare/mise"
)

var commands = []string{"init", "update", "help", "dev", "deploy", "preview", "wrangler", "github", "check", "login", "logout", "version"}

// TestTasks: every command the tasks run is one cloudflare has, and tasks writes them.
func TestTasks(t *testing.T) {
	// mise-cloudflare, its flags, then its command: none is the guide.
	for _, m := range regexp.MustCompile(`mise-cloudflare(?: -\S+)*(?: ([a-z][\w-]*))?`).FindAllStringSubmatch(string(mise.Tasks), -1) {
		if m[1] != "" && !slices.Contains(commands, m[1]) {
			t.Errorf("mise/tasks.toml runs mise-cloudflare %s, which it doesn't have", m[1])
		}
	}
	path := filepath.Join(t.TempDir(), tasksFile)
	writeFile(path, mise.Tasks)
	if b, err := os.ReadFile(path); err != nil || string(b) != string(mise.Tasks) {
		t.Fatalf("wrote %q, %v", b, err)
	}
}

// TestSkill: the skill installs where Claude Code finds it, and names its tasks right.
func TestSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkill(dir)
	b, err := os.ReadFile(filepath.Join(dir, "cloudflare", "SKILL.md"))
	if err != nil || !regexp.MustCompile(`(?s)^---\nname: cloudflare\ndescription: .+\n---\n`).Match(b) {
		t.Fatalf("SKILL.md: %v\n%.200s", err, b)
	}
	for _, m := range regexp.MustCompile(`mise run cloudflare ([a-z][\w-]*)`).FindAllStringSubmatch(string(b), -1) {
		if !slices.Contains(commands, m[1]) && m[1] != "help" {
			t.Errorf("the skill names cloudflare %s, which isn't a command", m[1])
		}
	}
}

// TestExampleIsCurrent: example/ has what this version's init writes, as any repo would after it.
func TestExampleIsCurrent(t *testing.T) {
	for _, d := range []string{"example"} {
		if b, err := os.ReadFile(filepath.Join("../..", d, tasksFile)); err != nil || string(b) != string(mise.Tasks) {
			t.Errorf("%s/%s is stale: run mise run example (%v)", d, tasksFile, err)
		}
	}
	want, _ := os.ReadFile("../../skills/cloudflare/SKILL.md")
	if b, err := os.ReadFile("../../example/.claude/skills/cloudflare/SKILL.md"); err != nil || string(b) != string(want) {
		t.Errorf("example/.claude/skills/cloudflare/SKILL.md is stale: run mise run example (%v)", err)
	}
}

// TestTokenEnvFiles: a .env or .env.* with CLOUDFLARE_API_TOKEN set is found; an empty one or a comment isn't.
func TestTokenEnvFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\nCLOUDFLARE_API_TOKEN=abc\n"), 0o600)
	os.WriteFile(filepath.Join(dir, ".env.local"), []byte("CLOUDFLARE_API_TOKEN=\n# CLOUDFLARE_API_TOKEN=x\n"), 0o600)
	if got := tokenEnvFiles(dir); !slices.Equal(got, []string{".env"}) {
		t.Errorf("got %q", got)
	}
}

// TestCI: CI=true is CI; unset, false or 0 isn't.
func TestCI(t *testing.T) {
	for v, want := range map[string]bool{"true": true, "1": true, "": false, "false": false, "0": false} {
		t.Setenv("CI", v)
		if ci() != want {
			t.Errorf("CI=%q: %v", v, !want)
		}
	}
}

// TestInstallCommand: only a lockfile committed to git counts, since Workers Builds checks the repository out.
func TestInstallCommand(t *testing.T) {
	dir := t.TempDir()
	if exec.Command("git", "-C", dir, "init", "-q").Run() != nil {
		t.Skip("no git")
	}
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644)
	if got := installCommand(dir); got != "npm install" {
		t.Errorf("an uncommitted package-lock.json: %q", got)
	}
	exec.Command("git", "-C", dir, "add", "package-lock.json").Run()
	if got := installCommand(dir); got != "npm ci" {
		t.Errorf("a committed package-lock.json: %q", got)
	}
}

// TestTasksRunTheBinary: every task runs mise-cloudflare; the repo's mise.toml pins its version, not the tasks.
func TestTasksRunTheBinary(t *testing.T) {
	b := mise.Tasks
	if strings.Contains(string(b), "go tool") || strings.Contains(string(b), `"github:joeblew999/mise-cloudflare"`) {
		t.Errorf("tasks run go tool or pin the tool:\n%s", b)
	}
	if n := len(regexp.MustCompile(`(?m)^run = "mise-cloudflare( \w+)?"$`).FindAll(b, -1)); n != 4 {
		t.Errorf("%d of 4 tasks run mise-cloudflare:\n%s", n, b)
	}
}

// TestWorkflowFor: a Worker at a repository's root gets cloudflare.yml, one in a subfolder cloudflare-<folder>.yml,
// working there, with no placeholders left and no Cloudflare token.
func TestWorkflowFor(t *testing.T) {
	top := t.TempDir()
	if exec.Command("git", "-C", top, "init", "-q").Run() != nil {
		t.Skip("no git")
	}
	sub := filepath.Join(top, "workers", "api")
	os.MkdirAll(sub, 0o755)
	for dir, want := range map[string]string{top: "cloudflare.yml", sub: "cloudflare-workers-api.yml"} {
		path, b, err := workflowFor(context.Background(), dir)
		if err != nil || filepath.Base(path) != want || filepath.Dir(path) != filepath.Join(mustReal(t, top), ".github", "workflows") {
			t.Errorf("%s: %s, %v", dir, path, err)
		}
		if regexp.MustCompile(`\b(NAME|DIR)\b`).Match(b) || strings.Contains(string(b), "CLOUDFLARE_API_TOKEN") {
			t.Errorf("%s: placeholders or a token:\n%s", want, b)
		}
	}
	_, b, _ := workflowFor(context.Background(), sub)
	if !strings.Contains(string(b), "working-directory: workers/api") || !strings.Contains(string(b), "name: cloudflare-workers-api") {
		t.Errorf("the subfolder's workflow:\n%s", b)
	}
}

func mustReal(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestClaude: the hook is written and made executable, and its SessionStart entry added to settings already there,
// once.
func TestClaude(t *testing.T) {
	top := t.TempDir()
	if exec.Command("git", "-C", top, "init", "-q").Run() != nil {
		t.Skip("no git")
	}
	sub := filepath.Join(top, "worker")
	os.MkdirAll(filepath.Join(top, ".claude"), 0o755)
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(top, ".claude", "settings.json"), []byte(`{"permissions": {"allow": ["Bash(ls)"]}}`), 0o644)
	claude(context.Background(), sub)
	claude(context.Background(), sub)
	st, err := os.Stat(filepath.Join(top, ".claude", "hooks", hookFile))
	if err != nil || st.Mode()&0o100 == 0 {
		t.Fatalf("hook: %v %v", st, err)
	}
	b, _ := os.ReadFile(filepath.Join(top, ".claude", "settings.json"))
	if !strings.Contains(string(b), `"Bash(ls)"`) || strings.Count(string(b), hookFile) != 1 || !strings.Contains(string(b), hookFile+" worker") {
		t.Errorf("settings:\n%s", b)
	}
}
