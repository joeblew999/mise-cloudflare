package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/joeblew999/cloudflare-zero/mise"
)

var commands = []string{"login", "wrangler", "status", "check", "logout", "init", "tasks", "skill"}

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
