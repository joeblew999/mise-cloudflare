package main

import (
	"bytes"
	"context"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/joeblew999/mise-cloudflare/templates"
)

// commands is every mise-cloudflare command, in the order of a repo's tasks: the numbered steps 03 to 14, then the rest.
var commands = []string{"init", "check", "show", "serve", "set-secrets", "deploy", "connect-github", "set-preview-secrets", "preview", "login", "delete", "logout", "clean", "verify", "update", "template", "run-wrangler", "show-version", "show-help"}

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

// TestMoved: update ends with every pin it read, the moved ones before → after, in the order of their file and name.
func TestMoved(t *testing.T) {
	before := map[string]string{"mise.toml node": "26.12.1", "mise.toml aube": "2.8.4", "package.json wrangler": "4.151.3", "go.mod example.com/old": "v1.0.0"}
	after := map[string]string{"mise.toml node": "26.13.0", "mise.toml aube": "2.8.4", "package.json wrangler": "4.155.0", "go.mod example.com/new": "v2.0.0"}
	lines, n := moved(before, after)
	want := []string{
		"example.com/new none → v2.0.0 (go.mod)",
		"example.com/old v1.0.0 → none (go.mod)",
		"aube 2.8.4 (mise.toml)",
		"node 26.12.1 → 26.13.0 (mise.toml)",
		"wrangler 4.151.3 → 4.155.0 (package.json)",
	}
	if !slices.Equal(lines, want) || n != 4 {
		t.Errorf("moved: %d\n%q\nwant 4\n%q", n, lines, want)
	}
}

// TestOwnMise: update reads the repo's own pins and no parent folder's, also when the repo's path goes through a link
// (/tmp and /var are /private/… on macOS: mise only stops at a ceiling given as the real path), and a pin that isn't
// installed yet is a pin too.
func TestOwnMise(t *testing.T) {
	if _, err := exec.LookPath("mise"); err != nil {
		t.Skip("no mise")
	}
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(real, "mise.toml"), []byte("[tools]\njq = \"1.8.2\"\n"), 0o644)
	os.Mkdir(filepath.Join(real, "repo"), 0o755)
	os.WriteFile(filepath.Join(real, "repo", "mise.toml"), []byte("[tools]\nyq = \"4.44.1\"\n"), 0o644) // not installed here
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", strings.Join([]string{real, longPath(real), link}, string(os.PathListSeparator)))
	got := repo{dir: filepath.Join(link, "repo")}.pins(context.Background())
	if want := map[string]string{"mise.toml yq": "4.44.1"}; !maps.Equal(got, want) {
		t.Errorf("a repo through a link, its yq not installed: pins %v, want %v (its own, not its parent folder's jq)", got, want)
	}
}

// TestTemplate: template writes the template's Worker, named for the folder, into a repo with none, and never writes
// over a file: a repo with a Worker of its own gets none of it (a README, if it has none), and a mise.toml or
// README.md of its own is left as it is.
func TestTemplate(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "My_New Repo")
	os.Mkdir(empty, 0o755)
	templateRepo(empty)
	for _, f := range []string{"src/worker.js", "wrangler.jsonc", "package.json", "pnpm-lock.yaml", ".dev.vars.example", "mise.toml", "README.md"} {
		if _, err := os.Stat(filepath.Join(empty, f)); err != nil {
			t.Errorf("%s not written into an empty repo: %v", f, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(empty, "README.md")); !bytes.HasPrefix(b, []byte("# my-new-repo\n")) {
		t.Errorf("README.md isn't named for its folder, my-new-repo:\n%s", b)
	}
	for _, f := range []string{"wrangler.jsonc", "package.json"} {
		if b, _ := os.ReadFile(filepath.Join(empty, f)); !bytes.Contains(b, []byte(`"my-new-repo"`)) || bytes.Contains(b, []byte(templates.WorkerName)) {
			t.Errorf("%s isn't named for its folder, my-new-repo:\n%s", f, b)
		}
	}
	own := t.TempDir()
	os.WriteFile(filepath.Join(own, "package.json"), []byte(`{"name":"mine"}`), 0o644)
	templateRepo(own)
	if entries, _ := os.ReadDir(own); len(entries) != 2 {
		t.Errorf("a repo with a package.json of its own got more than a README: %v", entries)
	}
	if b, _ := os.ReadFile(filepath.Join(own, "package.json")); string(b) != `{"name":"mine"}` {
		t.Errorf("the repo's own package.json was written over: %q", b)
	}
	os.WriteFile(filepath.Join(own, "README.md"), []byte("mine\n"), 0o644)
	templateRepo(own)
	if b, _ := os.ReadFile(filepath.Join(own, "README.md")); string(b) != "mine\n" {
		t.Errorf("the repo's own README.md was written over: %q", b)
	}
	tools := t.TempDir()
	mine := "[tools]\njq = \"1.8.2\"\n"
	os.WriteFile(filepath.Join(tools, "mise.toml"), []byte(mine), 0o644)
	templateRepo(tools)
	if b, _ := os.ReadFile(filepath.Join(tools, "mise.toml")); string(b) != mine {
		t.Errorf("the repo's own mise.toml was written over: %q", b)
	}
	if _, err := os.Stat(filepath.Join(tools, "wrangler.jsonc")); err != nil {
		t.Errorf("no Worker written beside a mise.toml of the repo's own: %v", err)
	}
	for _, bad := range []string{"___", strings.Repeat("a", 64)} {
		if _, err := workerName(bad); err == nil {
			t.Errorf("%q makes a Worker name", bad)
		}
	}
}

// TestInit: init writes the template's tool half into a repo exactly as the template has it, at the same paths, the
// hook executable; a .gitignore of the repo's own it leaves as it is. Then nothing is stale. mise-cloudflare's pin it
// writes once, for the build that ran it.
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
	if st, err := os.Stat(filepath.Join(dir, ".claude", "hooks", templates.HookFile)); err != nil || runtime.GOOS != "windows" && st.Mode()&0o100 == 0 { // Windows has no executable bit
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
	// .mise/config.toml pins the build that ran init (this test's, a checkout's: local), and is the repo's from then on.
	config := filepath.Join(dir, ".mise", "config.toml")
	want, _ := fs.ReadFile(templates.Tool, ".mise/config.toml")
	want = pinLine.ReplaceAll(want, []byte(`"`+self+`" = "local"`))
	if b, _ := os.ReadFile(config); !bytes.Equal(b, want) || !bytes.Contains(b, []byte(`"`+self+`" = "local"`)) {
		t.Errorf(".mise/config.toml doesn't pin this build, local:\n%s", b)
	}
	os.WriteFile(config, []byte("[tools]\n\"github:joeblew999/mise-cloudflare\" = \"0.0.1\"\n"), 0o644)
	initRepo(dir)
	if b, _ := os.ReadFile(config); !bytes.Contains(b, []byte(`= "0.0.1"`)) {
		t.Errorf("init wrote over the repo's own pin:\n%s", b)
	}
}
