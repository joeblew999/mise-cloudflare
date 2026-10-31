// The repo, and the calls into its own mise file, made in the repo as a person there makes them. Never the
// root's mise.toml.

package e2e

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
)

// repo is a Worker repo that uses mise-cloudflare: this repo (top, whose GitHub the throwaway branches go to), the
// repo's name (REPO_PATH, its folder here: templates), its full path, and its Worker's name.
type repo struct {
	t      *testing.T
	top    string
	name   string
	dir    string
	worker string
}

// repoAt is the repo at path, a folder of this repo.
func repoAt(t *testing.T, path string) repo {
	if path == "" {
		t.Fatal("REPO_PATH names the repo to walk, and the root's task sets it: mise run template:e2e-serve")
	}
	top, err := filepath.Abs(".")
	if err == nil {
		top, err = filepath.EvalSymlinks(top) // mise stops only at a ceiling given as the real path (MISE_CEILING_PATHS)
		top = longPath(top)
	}
	if err != nil {
		t.Fatal(err)
	}
	return repo{t: t, top: top, name: path, dir: filepath.Join(top, path)}
}

// checkoutOfHead is the same repo in a checkout of this repo at HEAD, which is what Cloudflare builds from GitHub:
// connect-github's steps push it. The checkout goes when the test ends.
func (r repo) checkoutOfHead() repo {
	r.t.Helper()
	tmp, err := filepath.EvalSymlinks(longPath(r.t.TempDir())) // the real path: macOS's /var is /private/var, and mise stops only there
	if err != nil {
		r.t.Fatal(err)
	}
	checkout := filepath.Join(tmp, "checkout")
	if out, err := exec.Command("git", "-C", r.top, "worktree", "add", "-q", "--detach", checkout, "HEAD").CombinedOutput(); err != nil {
		r.t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	top := r.top
	r.t.Cleanup(func() { exec.Command("git", "-C", top, "worktree", "remove", "--force", checkout).Run() })
	from := r.dir
	r.top, r.dir = checkout, filepath.Join(checkout, r.name)
	// The machine's own override isn't committed (mise run tool:install writes it): without it the checkout would run
	// the release the repo pins, not this checkout's build.
	if b, err := os.ReadFile(filepath.Join(from, ".mise", "config.local.toml")); err == nil {
		if err := os.WriteFile(filepath.Join(r.dir, ".mise", "config.local.toml"), b, 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	return r
}

// The stamped copy's Worker is its own, so its walk never touches the template's: its folder's name, as template
// names a Worker.
const stampedWorker = "mise-cloudflare-new"

// stampedCopy is a new repo, made as a person makes one: an empty folder with its own git, then mise-cloudflare init,
// run at version, a release from GitHub, or this checkout's build (local) when it is "" (init pins mise-cloudflare at
// itself, in .mise/config.toml), then the repo's own cloudflare:template, which writes the template's Worker named for
// the folder. It is in a folder of its own that goes when the test ends; its logins are wrangler's and cf's on this
// machine, so they stay.
func (r repo) stampedCopy(version string) repo {
	r.t.Helper()
	version = strings.TrimPrefix(version, "v")
	tmp, err := filepath.EvalSymlinks(longPath(r.t.TempDir())) // the real path, in long names: mise stops only at a ceiling given as one
	if err != nil {
		r.t.Fatal(err)
	}
	to := filepath.Join(tmp, stampedWorker)
	if err := os.Mkdir(to, 0o755); err != nil {
		r.t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", to, "init", "-q").CombinedOutput(); err != nil {
		r.t.Fatalf("git init: %v\n%s", err, out)
	}
	at := cmp.Or(version, "local")
	cmd := exec.Command("mise", "exec", "github:joeblew999/mise-cloudflare@"+at, "--", "mise-cloudflare", "init")
	cmd.Dir = to
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+tmp, "MISE_TRUSTED_CONFIG_PATHS="+to, "MISE_YES=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		r.t.Fatalf("mise-cloudflare init: %v", err)
	}
	want := `"github:joeblew999/mise-cloudflare" = "` + at + `"`
	if b, _ := os.ReadFile(filepath.Join(to, ".mise", "config.toml")); !bytes.Contains(b, []byte(want)) {
		r.t.Fatalf("mise-cloudflare %s's init didn't pin itself in .mise/config.toml (%s): a release from before it did can't make a new repo", at, want)
	}
	if b, _ := os.ReadFile(filepath.Join(to, ".mise", "tasks", "mise-cloudflare.toml")); !bytes.Contains(b, []byte(`["cloudflare:template"]`)) {
		r.t.Fatalf("mise-cloudflare %s has no cloudflare:template: a release from before it can't make a new repo", at)
	}
	from := r.name
	r.name, r.dir = "a new repo", to
	r.mustRunMiseTask(nil, "cloudflare:template")
	if b, _ := os.ReadFile(filepath.Join(to, "wrangler.jsonc")); !bytes.Contains(b, []byte(`"`+stampedWorker+`"`)) {
		r.t.Fatalf("cloudflare:template didn't name the Worker for its folder, %s: the walk would walk %s's", stampedWorker, from)
	}
	r.pass("%s, %s: an empty folder, its own git, mise-cloudflare init at %s, then cloudflare:template (the template's Worker, named for the folder)", r.name, stampedWorker, at)
	if withExtras() {
		r.addExtras()
	}
	return r
}

// miseSetUp sets the repo up as a person does, with its own mise file: mise install, then mise run setup. Then it reads
// its Worker's name, as wrangler does.
func (r *repo) miseSetUp() {
	r.t.Helper()
	r.must(nil, "mise", "install")
	r.must(nil, "mise", "run", "setup")
	c, err := wconfig.Read(context.Background(), r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	r.worker = c.Name
	r.pass("%s is set up: mise install, mise run setup; its Worker is %s", r.name, r.worker)
	r.pass("%s runs %s", r.name, r.versions())
}

// versions is what the repo runs, each as its own tool reports it: its mise tools (mise ls --local --json), the build
// of mise-cloudflare (cloudflare:show-version) and the wrangler mise run setup installed (its package.json).
func (r repo) versions() string {
	r.t.Helper()
	var tools map[string][]struct {
		Version string
		Active  bool
	}
	if err := json.Unmarshal(r.stdoutOf("mise", "ls", "--local", "--json"), &tools); err != nil || len(tools) == 0 {
		r.t.Fatalf("the repo's mise tools, mise ls --local --json: %v (%d read)", err, len(tools))
	}
	var v []string
	for _, name := range slices.Sorted(maps.Keys(tools)) {
		for _, t := range tools[name] {
			if t.Active {
				v = append(v, name+" "+t.Version)
			}
		}
	}
	build := strings.TrimSpace(string(r.stdoutOf("mise", "run", "cloudflare:show-version")))
	var w struct{ Version string }
	b, err := os.ReadFile(filepath.Join(r.dir, "node_modules", "wrangler", "package.json"))
	if err == nil {
		err = json.Unmarshal(b, &w)
	}
	if err != nil || w.Version == "" || build == "" {
		r.t.Fatalf("the installed wrangler (%q, %v) or mise-cloudflare's build (%q)", w.Version, err, build)
	}
	return fmt.Sprintf("%s (mise.toml); mise-cloudflare's build %s; wrangler %s, installed", strings.Join(v, ", "), build, w.Version)
}

// stdoutOf is what a command prints on stdout in the repo, on its own mise only, without showing it; its stderr shows.
func (r repo) stdoutOf(name string, args ...string) []byte {
	r.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS="+r.dir, "MISE_TASK_OUTPUT=interleave")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return out
}

func (r repo) heading(format string, a ...any) { fmt.Printf("\n=== %s\n", fmt.Sprintf(format, a...)) }
func (r repo) pass(format string, a ...any)    { fmt.Printf("PASS %s\n", fmt.Sprintf(format, a...)) }

// runIn runs a command in dir, with input on its stdin, its output shown as it goes (a login's link included) and
// returned.
func (r repo) runIn(dir string, env []string, input, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stdout, &buf)
	err := cmd.Run()
	if out := buf.String(); out != "" && !strings.HasSuffix(out, "\n") {
		fmt.Println() // a value printed bare (a KV key's), so the next line starts on its own
	}
	return buf.String(), err
}

// run runs a command in the repo, on its own mise only (MISE_CEILING_PATHS), as in any user's repo.
func (r repo) run(env []string, name string, args ...string) (string, error) {
	return r.runIn(r.dir, append([]string{"MISE_CEILING_PATHS=" + filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS=" + r.dir}, env...), "", name, args...)
}

func (r repo) must(env []string, name string, args ...string) string {
	r.t.Helper()
	out, err := r.run(env, name, args...)
	if err != nil {
		r.t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return out
}

func (r repo) mustIn(dir, name string, args ...string) {
	r.t.Helper()
	if _, err := r.runIn(dir, nil, "", name, args...); err != nil {
		r.t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
}

// runMiseTask runs the repo's own task, named in full ("cloudflare:04-check"), in the repo, on its own mise only, with
// input typed into it (a secret's value, when it asks): mise run cloudflare:04-check.
func (r repo) runMiseTask(env []string, input, task string, args ...string) (string, error) {
	fmt.Printf("$ mise run %s %s   (in %s)\n", task, strings.Join(args, " "), r.name)
	env = append([]string{"MISE_CEILING_PATHS=" + filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS=" + r.dir, "MISE_YES=1"}, env...)
	return r.runIn(r.dir, env, input, "mise", append([]string{"run", task}, args...)...)
}

func (r repo) mustRunMiseTask(env []string, task string, args ...string) string {
	r.t.Helper()
	out, err := r.runMiseTask(env, "", task, args...)
	if err != nil {
		r.t.Fatalf("%s %s: %v", task, strings.Join(args, " "), err)
	}
	return out
}

// miseWranglerStdout is what a wrangler command prints on stdout, on the repo's login (cloudflare:run-wrangler): the JSON
// or the list itself. What mise prints around it ("[cloudflare:run-wrangler] $ …", which starts like JSON) goes to
// stderr.
func (r repo) miseWranglerStdout(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("mise", append([]string{"run", "cloudflare:run-wrangler"}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS="+r.dir, "MISE_YES=1", "MISE_TASK_OUTPUT=interleave")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("wrangler %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// shown is what the repo's cloudflare:05-show shows, as JSON (--json).
type shown struct {
	Profile  string   `json:"profile"`
	LoggedIn bool     `json:"loggedIn"`
	Needs    []string `json:"needs"`
	Deployed bool     `json:"deployed"`
	URL      string   `json:"url"`
	GitHub   string   `json:"github"`
	Next     string   `json:"next"`
}

// miseShowJSON is the repo's cloudflare:05-show --json, its output unprefixed (and only its stdout read) so its JSON reads
// as JSON.
func (r repo) miseShowJSON() shown {
	r.t.Helper()
	cmd := exec.Command("mise", "run", "cloudflare:05-show", "--json")
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS="+r.dir, "MISE_TASK_OUTPUT=interleave")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	var s shown
	if err == nil {
		err = json.NewDecoder(strings.NewReader(jsonPart(string(out)))).Decode(&s)
	}
	if err != nil {
		r.t.Fatalf("cloudflare:05-show --json: %v\n%s", err, out)
	}
	return s
}

// miseShowSaysNext fails unless cloudflare:05-show says to run want next.
func (r repo) miseShowSaysNext(want string) {
	r.t.Helper()
	if s := r.miseShowJSON(); !strings.Contains(s.Next, want) {
		r.t.Fatalf("cloudflare:05-show says next: %q, not %q", s.Next, want)
	}
	r.pass("cloudflare:05-show says next: %s", want)
}

// miseServeLocally runs the Worker as a person does, with the repo's cloudflare:06-serve (which makes .dev.vars from
// .dev.vars.example), with no Cloudflare login anywhere it could see: a home of its own, with mise's own folders kept,
// so its tools are still there. It returns the Worker's answer.
func (r repo) miseServeLocally() []byte {
	r.t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		r.t.Fatal(err)
	}
	port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	var doctor struct {
		Dirs struct{ Data, Cache, State string }
	}
	if out, err := exec.Command("mise", "doctor", "--json").Output(); err != nil || json.Unmarshal(out, &doctor) != nil {
		r.t.Fatalf("mise doctor --json: %v", err)
	}
	home := r.t.TempDir()
	cmd := exec.Command("mise", "run", "cloudflare:06-serve", "--ip", "127.0.0.1", "--port", port)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(r.dir), "MISE_YES=1", "MISE_TRUSTED_CONFIG_PATHS="+r.dir,
		"HOME="+home, "XDG_CONFIG_HOME="+home, "CLOUDFLARE_API_TOKEN=", "CI=1",
		"MISE_DATA_DIR="+doctor.Dirs.Data, "MISE_CACHE_DIR="+doctor.Dirs.Cache, "MISE_STATE_DIR="+doctor.Dirs.State)
	ownGroup(cmd)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	defer killGroup(cmd)
	for i := 0; i < 90; i++ {
		if resp, err := http.Get("http://127.0.0.1:" + port + "/"); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return body
		}
		time.Sleep(time.Second)
	}
	r.t.Fatalf("cloudflare:06-serve never answered:\n%s", logs.String())
	return nil
}

// miseDeleteEverythingOnCloudflare deletes everything of the repo's Worker on Cloudflare, with the repo's own tasks, so a
// walk that deploys starts from nothing whatever a stopped one left: Workers Builds' setup for it
// (cloudflare:09-connect-github --off, when cf has a login for it; without one there's none), connect-github's
// throwaway e2e-* branches on this repo's GitHub, and the Worker with its D1, KV and R2 and its previews'
// (cloudflare:13-delete). Then nothing of it may be on the account.
func (r repo) miseDeleteEverythingOnCloudflare() {
	r.t.Helper()
	r.heading("Nothing of %s on Cloudflare: what a stopped walk left, deleted", r.worker)
	if out, _ := r.run(nil, "cf", "auth", "whoami", "--profile", r.worker); strings.Contains(out, `"authenticated": true`) {
		r.mustRunMiseTask(nil, "cloudflare:09-connect-github", "--off")
	}
	heads, err := r.runIn(r.top, nil, "", "git", "ls-remote", "--heads", "origin")
	if err != nil {
		r.t.Fatalf("git ls-remote origin: %v", err)
	}
	for _, line := range strings.Split(heads, "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/e2e-") {
			b := strings.TrimPrefix(f[1], "refs/heads/")
			if _, err := r.runIn(r.top, nil, "", "git", "push", "-q", "origin", "--delete", b); err != nil {
				r.t.Fatalf("deleting the branch %s: %v", b, err)
			}
			r.pass("deleted the branch %s", b)
		}
	}
	r.mustRunMiseTask(nil, "cloudflare:13-delete", "--yes")
	r.nothingOnCloudflare()
}
