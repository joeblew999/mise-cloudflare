// Package e2e proves cloudflare-zero on a real Cloudflare account: mise run prove, prove-other-repo and prove-basin
// (CLOUDFLARE_E2E=1). Without CLOUDFLARE_E2E these tests skip, so go test ./... never touches Cloudflare.
//
// Every Cloudflare step goes through go tool cloudflare, so the repo's own login and wrangler do it. Logging in, the
// first time only, opens a browser, or with CLOUDFLARE_LOGIN=device prints a link to open on any device.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func needAccount(t *testing.T) {
	if os.Getenv("CLOUDFLARE_E2E") != "1" {
		t.Skip("needs a Cloudflare account: mise run prove")
	}
}

// repo is a Worker repo that uses cloudflare-zero, and its Worker's name.
type repo struct {
	t      *testing.T
	dir    string
	worker string
}

func (r repo) step(format string, a ...any) { fmt.Printf("\n=== %s\n", fmt.Sprintf(format, a...)) }
func (r repo) pass(format string, a ...any) { fmt.Printf("PASS %s\n", fmt.Sprintf(format, a...)) }

// run runs a command in the repo, its output shown as it goes (a login's link included) and returned.
func (r repo) run(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stdout, &buf)
	err := cmd.Run()
	return buf.String(), err
}

func (r repo) must(env []string, name string, args ...string) string {
	r.t.Helper()
	out, err := r.run(env, name, args...)
	if err != nil {
		r.t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return out
}

// cf is go tool cloudflare; wrangler is the repo's wrangler on its own login.
func (r repo) cf(env []string, args ...string) string {
	r.t.Helper()
	return r.must(env, "go", append([]string{"tool", "cloudflare"}, args...)...)
}

func (r repo) wrangler(args ...string) string {
	r.t.Helper()
	return r.cf(nil, append([]string{"wrangler"}, args...)...)
}

// status is go tool cloudflare status -json (its JSON only: the tool prints nothing else on stdout).
type status struct {
	Profile        string            `json:"profile"`
	LoggedIn       bool              `json:"loggedIn"`
	DefaultLogin   string            `json:"defaultLogin"`
	FolderBindings map[string]string `json:"folderBindings"`
}

func (r repo) status() status {
	r.t.Helper()
	cmd := exec.Command("go", "tool", "cloudflare", "-json", "status")
	cmd.Dir = r.dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	var s status
	if err == nil {
		err = json.Unmarshal(out, &s)
	}
	if err != nil {
		r.t.Fatalf("status: %v\n%s", err, out)
	}
	return s
}

// realDir is the repo's folder by its real path, as wrangler binds it.
func (r repo) realDir() string {
	d, err := filepath.EvalSymlinks(r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	return d
}

// allWork is the example Worker's answer when D1, KV, R2 and the secret all work.
func allWork(body []byte) bool {
	var v struct{ D1, KV, R2, Secret bool }
	return json.Unmarshal(body, &v) == nil && v.D1 && v.KV && v.R2 && v.Secret
}

// get fetches url until it answers with every binding working, for up to a minute: a Worker just deployed can answer
// 500, then 404, before it's live everywhere.
func get(url string) (int, []byte) {
	code, body := 0, []byte(nil)
	for i := 0; i < 12; i++ {
		if resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url); err == nil {
			body, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			if code = resp.StatusCode; code == 200 && allWork(body) {
				return code, body
			}
		}
		time.Sleep(5 * time.Second)
	}
	return code, body
}

// localWorker runs the Worker with wrangler dev, with no Cloudflare login anywhere it could see, and returns its answer.
func (r repo) localWorker() []byte {
	r.t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		r.t.Fatal(err)
	}
	port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	cmd := exec.Command("node", "node_modules/wrangler/bin/wrangler.js", "dev", "--ip", "127.0.0.1", "--port", port)
	cmd.Dir = r.dir
	home := r.t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "CLOUDFLARE_API_TOKEN=", "CI=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	for i := 0; i < 90; i++ {
		if resp, err := http.Get("http://127.0.0.1:" + port + "/"); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return body
		}
		time.Sleep(time.Second)
	}
	r.t.Fatalf("wrangler dev never answered:\n%s", logs.String())
	return nil
}

// deployedURL is the last deploy's workers.dev URL, from what wrangler wrote to .wrangler/output.json.
func (r repo) deployedURL() string {
	r.t.Helper()
	f, err := os.Open(filepath.Join(r.dir, ".wrangler", "output.json"))
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	url := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var e struct {
			Type    string   `json:"type"`
			Targets []string `json:"targets"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Type == "deploy" {
			for _, u := range e.Targets {
				if strings.HasSuffix(u, ".workers.dev") {
					url = u
				}
			}
		}
	}
	if url == "" {
		r.t.Fatal("the deploy wrote no workers.dev URL to .wrangler/output.json")
	}
	return url
}

// d1 is the uuid of the Worker's D1 (wrangler names it <worker>-db), "" if it doesn't exist.
func (r repo) d1() string {
	var dbs []struct{ UUID, Name string }
	json.Unmarshal([]byte(jsonPart(r.wrangler("d1", "list", "--json"))), &dbs)
	for _, d := range dbs {
		if d.Name == r.worker+"-db" {
			return d.UUID
		}
	}
	return ""
}

// kv is the id of the Worker's KV namespace (<worker>-kv), "" if it doesn't exist.
func (r repo) kv() string {
	var ns []struct{ ID, Title string }
	json.Unmarshal([]byte(jsonPart(r.wrangler("kv", "namespace", "list"))), &ns)
	for _, n := range ns {
		if n.Title == r.worker+"-kv" {
			return n.ID
		}
	}
	return ""
}

// r2 reports whether the Worker's bucket (<worker>-bucket) exists.
func (r repo) r2() bool {
	for _, line := range strings.Split(r.wrangler("r2", "bucket", "list"), "\n") {
		if strings.TrimSpace(line) == "name: "+r.worker+"-bucket" {
			return true
		}
	}
	return false
}

// jsonPart is wrangler's output from its first [ or {, past the banner it prints first.
func jsonPart(out string) string {
	if i := strings.IndexAny(out, "[{"); i >= 0 {
		return out[i:]
	}
	return ""
}

// deleteAll deletes the Worker, its D1, KV and R2, with wrangler on the repo's login. Errors are ignored: it also runs
// as cleanup after a failure, when some of them may not exist.
func (r repo) deleteAll() {
	r.run(nil, "go", "tool", "cloudflare", "wrangler", "delete", "--force")
	if id := r.d1(); id != "" {
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "d1", "delete", r.worker+"-db", "-y")
	}
	if id := r.kv(); id != "" {
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "kv", "namespace", "delete", "--namespace-id", id, "-y")
	}
	if r.r2() {
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "r2", "object", "delete", r.worker+"-bucket/stamp.txt", "--remote")
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "r2", "bucket", "delete", r.worker+"-bucket")
	}
}

// prove is the whole proof on a Worker repo (example/, or a new one): local, deploy on its own login, live, a second
// deploy, stray tokens, deleting everything, and with clean the login logged out.
func prove(t *testing.T, r repo, clean bool) {
	defaultLogin := r.status().DefaultLogin
	fmt.Printf("machine's default login: config/default.toml %s (a checksum; its contents are never read)\n", defaultLogin)

	r.step("1. A developer with no Cloudflare login at all: setup, the rules, a local Worker")
	r.must(nil, "mise", "run", "setup")
	r.must(nil, "mise", "run", "cloudflare:check")
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker answered %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work, with no Cloudflare login")

	r.step("2. Deploy on the repo's own login, %s (logs in the first time only)", r.worker)
	r.cf(nil, "login")
	if out, err := r.run(nil, "go", "tool", "cloudflare", "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed on this account: another machine may be proving it now. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	r.pass("the Worker isn't deployed on this account yet, so nothing else is using it")
	t.Cleanup(func() {
		if t.Failed() {
			r.step("Cleaning up after the failure")
			r.deleteAll()
		}
	})
	before := r.must(nil, "git", "status", "--porcelain", ".")
	secrets := filepath.Join(t.TempDir(), "secrets.json")
	os.WriteFile(secrets, []byte(`{"GREETING":"set-by-prove"}`), 0o600)
	r.cf([]string{"WRANGLER_OUTPUT_FILE_PATH=.wrangler/output.json"}, "wrangler", "deploy", "--secrets-file", secrets)
	s := r.status()
	if !s.LoggedIn {
		t.Fatalf("no login %s", r.worker)
	}
	r.pass("login %s exists", s.Profile)
	if s.FolderBindings[r.realDir()] != s.Profile {
		t.Fatalf("login not bound to %s: %v", r.realDir(), s.FolderBindings)
	}
	r.pass("login %s is bound to %s, by its real path", s.Profile, r.realDir())
	if s.DefaultLogin != defaultLogin {
		t.Fatal("the machine's default login changed")
	}
	r.pass("the machine's default login is untouched")

	r.step("3. The live Worker: D1, KV, R2 and the secret")
	url := r.deployedURL()
	if code, body := get(url); code != 200 || !allWork(body) {
		t.Fatalf("%s: HTTP %d %s", url, code, body)
	}
	r.pass("%s: D1, KV, R2 and the secret all work", url)
	db := r.d1()

	r.step("4. Deploy again: no login, nothing asked, the same resources, the repo unchanged")
	out := r.must(nil, "mise", "run", "cloudflare:deploy")
	if strings.Contains(out, "Logging in") || strings.Contains(out, "scopes changed") {
		t.Fatal("the second deploy logged in again")
	}
	r.pass("no login")
	if r.d1() != db || db == "" {
		t.Fatalf("D1 %s became %s", db, r.d1())
	}
	r.pass("the same D1 (%s), KV and R2", db)
	if after := r.must(nil, "git", "status", "--porcelain", "."); after != before {
		t.Fatalf("the repo changed:\n%s", after)
	}
	r.pass("deploying changed nothing in the repo: no IDs written into it")

	r.step("5. A stray CLOUDFLARE_API_TOKEN is refused, in the shell and in a .env")
	if out, err := r.run([]string{"CLOUDFLARE_API_TOKEN=dummy-not-a-real-token"}, "go", "tool", "cloudflare", "wrangler", "deploy"); err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in the shell")
	}
	r.pass("a token in the shell is refused")
	env := filepath.Join(r.dir, ".env")
	os.WriteFile(env, []byte("CLOUDFLARE_API_TOKEN=dummy-not-a-real-token\n"), 0o600)
	out, err := r.run(nil, "go", "tool", "cloudflare", "wrangler", "deploy")
	os.Remove(env)
	if err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in .env")
	}
	r.pass("a token in .env is refused")

	r.step("6. wrangler, on the repo's login, deletes the Worker, its D1, KV and R2")
	r.deleteAll()
	if code, _ := get(url); code == 200 {
		t.Fatalf("%s still answers", url)
	}
	r.pass("%s no longer answers", url)
	if r.d1() != "" || r.kv() != "" || r.r2() {
		t.Fatal("D1, KV or R2 left")
	}
	r.pass("no D1, KV or R2 left")

	if clean {
		r.step("7. Logout removes the login and its folder binding")
		r.cf(nil, "logout")
		s := r.status()
		if s.LoggedIn {
			t.Fatal("the login is still there")
		}
		if _, ok := s.FolderBindings[r.realDir()]; ok {
			t.Fatal("the folder binding is still there")
		}
		r.pass("login and folder binding gone")
	}
	if r.status().DefaultLogin != defaultLogin {
		t.Fatal("the machine's default login changed")
	}
	r.pass("the machine's default login was untouched throughout")
	fmt.Println("\nALL PASSED")
}

// TestProve proves it all in example/.
func TestProve(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("../example")
	prove(t, repo{t: t, dir: dir, worker: "cloudflare-zero-example"}, os.Getenv("PROVE_CLEAN") == "true")
}

// TestOtherRepo proves a brand-new repo gets the same from config alone: cloudflare-zero from GitHub with go get -tool,
// go tool cloudflare init, and nothing else of cloudflare-zero's.
func TestOtherRepo(t *testing.T) {
	needAccount(t)
	ref := os.Getenv("CF_ZERO_REF")
	if ref == "" {
		ref = "main"
	}
	src, _ := filepath.Abs("../example")
	r := repo{t: t, dir: filepath.Join(t.TempDir(), "other-repo"), worker: "cloudflare-zero-other"}
	r.step("A new repo at %s: a Worker, and cloudflare-zero from GitHub at %s", r.dir, ref)
	os.MkdirAll(filepath.Join(r.dir, "src"), 0o755)
	for _, f := range []string{"src/worker.js", "package.json", "aube-lock.yaml", ".dev.vars.example", ".gitignore", "mise.toml"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(r.dir, f), b, 0o644)
	}
	b, _ := os.ReadFile(filepath.Join(src, "wrangler.jsonc"))
	os.WriteFile(filepath.Join(r.dir, "wrangler.jsonc"), bytes.ReplaceAll(b, []byte(`"cloudflare-zero-example"`), []byte(`"`+r.worker+`"`)), 0o644)
	r.must(nil, "git", "init", "-q")
	r.must(nil, "go", "mod", "init", "example.com/other-repo")
	// Straight from GitHub, whatever the ref: not from the module proxy, which may not have a new commit yet.
	r.must([]string{"GOPRIVATE=github.com/joeblew999/cloudflare-zero"}, "go", "get", "-tool", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare@"+ref)
	r.cf(nil, "init")
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	r.must(nil, "git", "add", "-A")
	r.must(nil, "wc", "-l", "go.mod", "cloudflare.toml", "mise.toml", "wrangler.jsonc", "package.json", ".dev.vars.example", ".gitignore", "src/worker.js")
	if s := r.status(); s.Profile != r.worker {
		t.Fatalf("its login is %q, not the Worker's name", s.Profile)
	}
	r.pass("cloudflare.toml's defaults: the login is named after the Worker, %s", r.worker)
	prove(t, r, os.Getenv("PROVE_CLEAN") == "true")
}
