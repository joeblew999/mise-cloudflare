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

var commands = []string{"login", "wrangler", "builds", "status", "check", "logout", "init", "tasks", "skill", "workflow", "version"}

// TestTasks: every command the tasks run is one cloudflare has, and tasks writes them.
func TestTasks(t *testing.T) {
	for _, m := range regexp.MustCompile(`go tool cloudflare (?:-\S+ )*(\w+)`).FindAllStringSubmatch(string(mise.Tasks), -1) {
		if !slices.Contains(commands, m[1]) {
			t.Errorf("mise/tasks.toml runs cloudflare %s, which it doesn't have", m[1])
		}
	}
	path := filepath.Join(t.TempDir(), ".mise", "tasks", "cloudflare.toml")
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
	for _, m := range regexp.MustCompile(`cloudflare:(\w+)`).FindAllStringSubmatch(string(b), -1) {
		if !regexp.MustCompile(`\["cloudflare:` + m[1] + `"\]`).Match(mise.Tasks) {
			t.Errorf("the skill names cloudflare:%s, which the tasks don't have", m[1])
		}
	}
}

// TestExampleIsCurrent: example/ has the tasks and the skill this version writes, as any repo would after
// go tool cloudflare tasks and skill.
func TestExampleIsCurrent(t *testing.T) {
	if b, err := os.ReadFile("../../example/.mise/tasks/cloudflare.toml"); err != nil || string(b) != string(mise.Tasks) {
		t.Errorf("example/.mise/tasks/cloudflare.toml is stale: run go tool cloudflare tasks in example/ (%v)", err)
	}
	want, _ := os.ReadFile("../../skills/cloudflare/SKILL.md")
	if b, err := os.ReadFile("../../example/.claude/skills/cloudflare/SKILL.md"); err != nil || string(b) != string(want) {
		t.Errorf("example/.claude/skills/cloudflare/SKILL.md is stale: run go tool cloudflare skill in example/ (%v)", err)
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

// TestTasksFor: a repo with go.mod's tool line gets the tasks as embedded; any other repo gets tasks that run the
// cloudflare binary, each pinning this release, and needs no Go.
func TestTasksFor(t *testing.T) {
	goRepo := t.TempDir()
	os.WriteFile(filepath.Join(goRepo, "go.mod"), []byte("module x\n\ngo 1.26.0\n\ntool "+toolPath+"\n"), 0o644)
	if b, err := tasksFor(goRepo); err != nil || string(b) != string(mise.Tasks) {
		t.Errorf("a Go repo: %v", err)
	}
	other := t.TempDir()
	if _, err := tasksFor(other); err == nil {
		t.Error("a development build wrote tasks pinning no release")
	}
	defer func(v string) { version = v }(version)
	version = "v0.1.0"
	b, err := tasksFor(other)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "go tool cloudflare") {
		t.Error("tasks for a repo without Go run go tool")
	}
	pin := `"github:joeblew999/mise-cloudflare" = "0.1.0"`
	if n, tasks := strings.Count(string(b), pin), strings.Count(string(b), `["cloudflare:`); n != tasks {
		t.Errorf("%d of %d tasks pin the release:\n%s", n, tasks, b)
	}
	if !strings.Contains(string(b), `tools = { `+pin+`, "npm:cf" = "`) {
		t.Errorf("cloudflare:builds pins both cloudflare and cf in one tools table:\n%s", b)
	}
	if !regexp.MustCompile(`(?m)^run = "cloudflare -off=`).Match(b) {
		t.Errorf("builds runs the binary:\n%s", b)
	}
}

// TestWorkflowFor: a Worker at a repository's root gets cloudflare.yml, one in a subfolder cloudflare-<folder>.yml,
// working there; nothing of the template's placeholders is left.
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
		if regexp.MustCompile(`\b(VERSION|NAME|DIR)\b`).Match(b) {
			t.Errorf("%s: placeholders left:\n%s", want, b)
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
