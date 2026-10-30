package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/joeblew999/mise-cloudflare/templates"
)

// commands is every mise-cloudflare command, in the order of a repo's tasks: the numbered steps 03 to 14, then the rest.
var commands = []string{"init", "check", "show", "serve", "set-secrets", "deploy", "connect-github", "set-preview-secrets", "preview", "login", "delete", "logout", "clean", "verify", "run-wrangler", "show-version", "show-help"}

// taskVerb is the command a task's name names: cloudflare:08-deploy names deploy, cloudflare:verify names verify.
var taskVerb = regexp.MustCompile(`^cloudflare:(?:\d\d-)?([a-z][a-z-]*)$`)

// TestSkill: the skill installs where Claude Code finds it, and names its tasks right.
func TestSkill(t *testing.T) {
	b, err := fs.ReadFile(templates.Tool, ".claude/skills/cloudflare/SKILL.md")
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

// TestInit: init writes the template's tool half into a repo exactly as the template has it, at the same paths, the
// hook executable; a .gitignore of the repo's own it leaves as it is. Then nothing is stale.
func TestInit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0o644)
	initRepo(dir)
	fs.WalkDir(templates.Tool, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || theirs[p] {
			return err
		}
		want, _ := fs.ReadFile(templates.Tool, p)
		if b, err := os.ReadFile(filepath.Join(dir, p)); err != nil || !bytes.Equal(b, want) {
			t.Errorf("%s isn't the template's: %v", p, err)
		}
		return nil
	})
	if st, err := os.Stat(filepath.Join(dir, ".claude", "hooks", templates.HookFile)); err != nil || st.Mode()&0o100 == 0 {
		t.Errorf("the hook isn't executable: %v %v", st, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(b) != "dist/\n" {
		t.Errorf("the repo's own .gitignore was written: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".claude", "settings.json")); !bytes.Contains(b, []byte(templates.HookFile)) {
		t.Errorf("settings.json doesn't run the hook:\n%s", b)
	}
	if got := (repo{dir: dir}).stale(context.Background()); len(got) > 0 {
		t.Errorf("stale right after init: %q", got)
	}
}
