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

	"github.com/joeblew999/mise-cloudflare/templates"
)

// commands is every mise-cloudflare command, in the order of example.mise.toml's steps.
var commands = []string{"init", "check", "status", "dev", "secret", "deploy", "github", "preview", "login", "delete", "logout", "wrangler", "version", "help"}

// TestTasks: one name all the way through. Each task cloudflare:x runs mise-cloudflare x, a command it has, and every
// command has its task; cloudflare alone runs it with none (status).
func TestTasks(t *testing.T) {
	// mise-cloudflare, its flags, then its command: none is status.
	for _, m := range regexp.MustCompile(`mise-cloudflare(?: -\S+)*(?: ([a-z][\w-]*))?`).FindAllStringSubmatch(string(templates.Tasks), -1) {
		if m[1] != "" && !slices.Contains(commands, m[1]) {
			t.Errorf("templates/tasks.toml runs mise-cloudflare %s, which it doesn't have", m[1])
		}
	}
	tasks := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\["(cloudflare(?::([a-z]+))?)"\][^\[]*?^run = "mise-cloudflare ?([a-z]*)"$`).FindAllStringSubmatch(string(templates.Tasks), -1) {
		if m[2] != m[3] {
			t.Errorf("the task %s runs mise-cloudflare %s", m[1], m[3])
		}
		tasks[m[2]] = m[1]
	}
	for _, c := range commands {
		if _, ok := tasks[c]; !ok {
			t.Errorf("mise-cloudflare %s has no task cloudflare:%s", c, c)
		}
	}
	path := filepath.Join(t.TempDir(), tasksFile)
	writeFile(path, templates.Tasks)
	if b, err := os.ReadFile(path); err != nil || string(b) != string(templates.Tasks) {
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
	for _, m := range regexp.MustCompile(`mise run cloudflare:([a-z][\w-]*)`).FindAllStringSubmatch(string(b), -1) {
		if !slices.Contains(commands, m[1]) {
			t.Errorf("the skill names cloudflare:%s, which isn't a command", m[1])
		}
	}
	if m := regexp.MustCompile(`mise run cloudflare [a-z]\w*`).FindString(string(b)); m != "" {
		t.Errorf("the skill names %q: each command is its task, mise run cloudflare:<command>", m)
	}
}

// TestExampleIsCurrent: example/ has what this version's init writes, as any repo would after it.
func TestExampleIsCurrent(t *testing.T) {
	for _, d := range []string{"example"} {
		if b, err := os.ReadFile(filepath.Join("../..", d, tasksFile)); err != nil || string(b) != string(templates.Tasks) {
			t.Errorf("%s/%s is stale: run mise run 03-init (%v)", d, tasksFile, err)
		}
	}
	want, _ := os.ReadFile("../../templates/skills/cloudflare/SKILL.md")
	if b, err := os.ReadFile("../../example/.claude/skills/cloudflare/SKILL.md"); err != nil || string(b) != string(want) {
		t.Errorf("example/.claude/skills/cloudflare/SKILL.md is stale: run mise run 03-init (%v)", err)
	}
	if b, err := os.ReadFile("../../example/.claude/hooks/" + hookFile); err != nil || string(b) != string(templates.SessionStart) {
		t.Errorf("example/.claude/hooks/%s is stale: run mise run 03-init (%v)", hookFile, err)
	}
	if _, err := os.Stat("../../example/.github/workflows/cloudflare.yml"); err != nil {
		t.Errorf("example/.github/workflows/cloudflare.yml is missing: run mise run 03-init (%v)", err)
	}
}

// TestExampleStaysInExample: everything init writes for example/ is in example/, a repo of its own; none of it is in
// this repo's own .github/ or .claude/.
func TestExampleStaysInExample(t *testing.T) {
	for _, p := range []string{"../../.github/workflows/cloudflare-example.yml", "../../.claude/hooks/" + hookFile} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s is example/'s, in this repo: delete it, and run mise run 03-init", strings.TrimPrefix(p, "../../"))
		}
	}
	if b, _ := os.ReadFile("../../.claude/settings.json"); strings.Contains(string(b), ".claude/hooks/"+hookFile) {
		t.Errorf(".claude/settings.json runs example/'s hook: this repo's own runs templates/session-start.sh")
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
	b := templates.Tasks
	if strings.Contains(string(b), "go tool") || strings.Contains(string(b), `"github:joeblew999/mise-cloudflare"`) {
		t.Errorf("tasks run go tool or pin the tool:\n%s", b)
	}
	tasks, runs := len(regexp.MustCompile(`(?m)^\["cloudflare`).FindAll(b, -1)), len(regexp.MustCompile(`(?m)^run = "mise-cloudflare( \w+)?"$`).FindAll(b, -1))
	if tasks == 0 || runs != tasks {
		t.Errorf("%d of %d tasks run mise-cloudflare:\n%s", runs, tasks, b)
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

// TestChain: one name all the way through, from a step to its code. Step nn-x from 04 on (example.mise.toml) runs the
// task cloudflare:x, which runs mise-cloudflare x (TestTasks); main.go's case "x" calls r.x, which is in x.go. init,
// which runs before there's a repo to open, is initRepo in init.go; help and version are a line each in main.go.
func TestChain(t *testing.T) {
	steps, err := os.ReadFile("../../example.mise.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`(?m)^\["(\d\d)-([a-z-]+)"\]\n(?:.*\n)*?run = "([^"]*)"`).FindAllStringSubmatch(string(steps), -1) {
		if want := "mise run cloudflare:" + m[2]; m[1] >= "04" && m[3] != want && !strings.HasPrefix(m[3], want+" ") {
			t.Errorf("step %s-%s runs %q, not %s", m[1], m[2], m[3], want)
		}
	}
	main, _ := os.ReadFile("main.go")
	for _, c := range commands {
		if c == "help" || c == "version" {
			continue
		}
		call, def, file := "r."+c+"(", "func (r repo) "+c+"(", c+".go"
		if c == "init" {
			call, def = "initRepo(", "func initRepo("
		}
		m := regexp.MustCompile(`(?m)^\tcase [^\n]*"` + c + `"[^\n]*:(?: //[^\n]*)?\n\t+(\S[^\n]*)`).FindSubmatch(main)
		if m == nil || !strings.HasPrefix(string(m[1]), call) {
			t.Errorf("main.go's case %q doesn't call %s…)", c, call)
		}
		if b, err := os.ReadFile(file); err != nil || !strings.Contains(string(b), "\n"+def) {
			t.Errorf("%s isn't in %s", def, file)
		}
	}
}
