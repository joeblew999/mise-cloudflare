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

// commands is every mise-cloudflare command, in the order of a repo's tasks: the numbered steps 03 to 14, then the rest.
var commands = []string{"init", "check", "show", "serve", "set-secrets", "deploy", "connect-github", "set-preview-secrets", "preview", "login", "delete", "logout", "verify", "run-wrangler", "show-version", "show-help"}

// taskVerb is the command a task's name names: cloudflare:08-deploy names deploy, cloudflare:verify names verify.
var taskVerb = regexp.MustCompile(`^cloudflare:(?:\d\d-)?([a-z][a-z-]*)$`)

// TestTasks: one name all the way through. Each task cloudflare:nn-x (or cloudflare:x) runs mise-cloudflare x, a
// command it has, and every command has its task, in commands' order; cloudflare alone runs it with none (show).
func TestTasks(t *testing.T) {
	// mise-cloudflare, its flags, then its command: none is show.
	for _, m := range regexp.MustCompile(`mise-cloudflare(?: -\S+)*(?: ([a-z][\w-]*))?`).FindAllStringSubmatch(string(templates.Tasks), -1) {
		if m[1] != "" && !slices.Contains(commands, m[1]) {
			t.Errorf("templates/tasks.toml runs mise-cloudflare %s, which it doesn't have", m[1])
		}
	}
	tasks := map[string]string{}
	var order []string
	for _, m := range regexp.MustCompile(`(?m)^\["(cloudflare:[a-z0-9-]+)"\][^\[]*?^run = "mise-cloudflare ?([a-z-]*)"$`).FindAllStringSubmatch(string(templates.Tasks), -1) {
		v := taskVerb.FindStringSubmatch(m[1])
		if v == nil || v[1] != m[2] {
			t.Errorf("the task %s runs mise-cloudflare %s", m[1], m[2])
			continue
		}
		tasks[v[1]] = m[1]
		order = append(order, v[1])
	}
	for _, c := range commands {
		if _, ok := tasks[c]; !ok {
			t.Errorf("mise-cloudflare %s has no task cloudflare:%s", c, c)
		}
	}
	// In the steps' order, which commands has: a repo's tasks file reads as the steps do, after cloudflare itself.
	if !slices.Equal(order, commands) {
		t.Errorf("templates/tasks.toml has its tasks in the order %v, not the steps' %v", order, commands)
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
	writeTree(templates.Skills, dir)
	b, err := os.ReadFile(filepath.Join(dir, "cloudflare", "SKILL.md"))
	if err != nil || !regexp.MustCompile(`(?s)^---\nname: cloudflare\ndescription: .+\n---\n`).Match(b) {
		t.Fatalf("SKILL.md: %v\n%.200s", err, b)
	}
	for _, m := range regexp.MustCompile(`mise run (cloudflare:[a-z0-9][a-z0-9-]*)`).FindAllStringSubmatch(string(b), -1) {
		if v := taskVerb.FindStringSubmatch(m[1]); v == nil || !slices.Contains(commands, v[1]) {
			t.Errorf("the skill names %s, which isn't a command's task", m[1])
		}
	}
	if m := regexp.MustCompile(`mise run cloudflare [a-z]\w*`).FindString(string(b)); m != "" {
		t.Errorf("the skill names %q: each command is its task, mise run cloudflare:<command>", m)
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
		if inCI() != want {
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
	tasks, runs := len(regexp.MustCompile(`(?m)^\["cloudflare`).FindAll(b, -1)), len(regexp.MustCompile(`(?m)^run = "mise-cloudflare( [a-z][a-z-]*)?"$`).FindAll(b, -1))
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

// TestChain: one name all the way through, from a command to its code: main.go's case "x" calls r.x, which is in
// x.go. init, which runs before there's a repo to open, is initRepo in init.go; help and version are a line each in
// main.go. That the steps run the tasks in this order is root:check's.
func TestChain(t *testing.T) {
	main, _ := os.ReadFile("main.go")
	for _, c := range commands {
		if c == "show-help" || c == "show-version" {
			continue
		}
		// set-secrets is r.setSecrets, in set_secrets.go.
		parts := strings.Split(c, "-")
		method := parts[0]
		for _, p := range parts[1:] {
			method += strings.ToUpper(p[:1]) + p[1:]
		}
		call, def, file := "r."+method+"(", "func (r repo) "+method+"(", strings.Join(parts, "_")+".go"
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
