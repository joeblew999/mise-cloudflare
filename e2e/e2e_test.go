// Package e2e is mise run smoke (a new repo as a user sets it up, no account: MISE_CLOUDFLARE_SMOKE=1) and mise run
// prove (real deploys on a Cloudflare account: CLOUDFLARE_E2E=1). Without those, the tests skip, so go test ./...
// never touches Cloudflare. Every step runs the tool, so the repo's own login and wrangler do it.
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
	"regexp"
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

// repo is a Worker repo that uses mise-cloudflare, its Worker's name, and how it runs the tool (this checkout's
// bin/mise-cloudflare unless tool says otherwise).
type repo struct {
	t      *testing.T
	dir    string
	worker string
	tool   []string
	env    string // a wrangler environment's Worker (staging), "" for the Worker itself
}

func (r repo) toolArgs(args ...string) (string, []string) {
	tool := r.tool
	if tool == nil {
		bin, _ := filepath.Abs("../bin/mise-cloudflare")
		tool = []string{bin}
	}
	return tool[0], append(append([]string{}, tool[1:]...), args...)
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
	if out := buf.String(); out != "" && !strings.HasSuffix(out, "\n") {
		fmt.Println() // a value printed bare (a KV key's), so the next line starts on its own
	}
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

// cf runs the tool; wrangler is the repo's wrangler on its own login.
func (r repo) cf(env []string, args ...string) string {
	r.t.Helper()
	name, a := r.toolArgs(args...)
	return r.must(env, name, a...)
}

// try runs the tool, its failure returned rather than fatal.
func (r repo) try(env []string, args ...string) (string, error) {
	name, a := r.toolArgs(args...)
	return r.run(env, name, a...)
}

func (r repo) wrangler(args ...string) string {
	r.t.Helper()
	return r.cf(nil, append([]string{"wrangler"}, args...)...)
}

// status is cloudflare -json, the guide (its JSON only: the tool prints nothing else on stdout).
type status struct {
	Profile  string   `json:"profile"`
	LoggedIn bool     `json:"loggedIn"`
	Needs    []string `json:"needs"`
	Deployed bool     `json:"deployed"`
	URL      string   `json:"url"`
	GitHub   string   `json:"github"`
	Next     string   `json:"next"`
}

// next is what the guide (mise run cloudflare) says to run next, failing unless it says want.
func (r repo) next(want string) {
	r.t.Helper()
	if s := r.status(); !strings.Contains(s.Next, want) {
		r.t.Fatalf("the guide says next: %q, not %q", s.Next, want)
	}
	r.pass("the guide says next: %s", want)
}

func (r repo) status() status {
	r.t.Helper()
	name, a := r.toolArgs("-json")
	cmd := exec.Command(name, a...)
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

// r2 reports whether the Worker's bucket (<worker>-bucket) exists. wrangler lists each as "name:" and the name, padded.
func (r repo) r2() bool {
	for _, line := range strings.Split(r.wrangler("r2", "bucket", "list"), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "name:" && f[1] == r.worker+"-bucket" {
			return true
		}
	}
	return false
}

// jsonStart is where JSON starts in wrangler's output: the first line that begins with [ or {. A line like
// "▲ [WARNING] …", which wrangler prints first on some machines, isn't it.
var jsonStart = regexp.MustCompile(`(?m)^[\[{]`)

// jsonPart is wrangler's JSON, past anything it prints before it.
func jsonPart(out string) string {
	if loc := jsonStart.FindStringIndex(out); loc != nil {
		return out[loc[0]:]
	}
	return ""
}

// leftovers names the Worker's D1, KV and R2 that exist, which prove must start without and end without.
func (r repo) leftovers() []string {
	var left []string
	if r.d1() != "" {
		left = append(left, "D1 "+r.worker+"-db")
	}
	if r.kv() != "" {
		left = append(left, "KV "+r.worker+"-kv")
	}
	if r.r2() {
		left = append(left, "R2 "+r.worker+"-bucket")
	}
	if !strings.HasSuffix(r.worker, "-previews") {
		left = append(left, repo{t: r.t, dir: r.dir, worker: r.worker + "-previews", tool: r.tool}.leftovers()...)
	}
	return left
}

// putSecret runs a wrangler secret command (secret put, preview base-config secret put) through the task a developer
// would, its value typed in as wrangler asks for it.
func (r repo) putSecret(args ...string) {
	r.t.Helper()
	cmd := exec.Command("mise", append([]string{"run", "cloudflare", "wrangler"}, args...)...)
	cmd.Dir = r.dir
	cmd.Stdin = strings.NewReader("set-by-prove")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("wrangler %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	r.pass("wrangler %s", strings.Join(args, " "))
}

// deleteAll deletes the Worker, its D1, KV and R2, and its previews' own, with wrangler on the repo's login. Errors
// are ignored: it also runs as cleanup after a failure, when some of them may not exist.
func (r repo) deleteAll() {
	if !strings.HasSuffix(r.worker, "-previews") {
		defer repo{t: r.t, dir: r.dir, worker: r.worker + "-previews", tool: r.tool}.deleteAll()
	}
	if r.env != "" {
		r.try(nil, "wrangler", "delete", "--force", "--env", r.env)
	} else {
		r.try(nil, "wrangler", "delete", "--force")
	}
	if id := r.d1(); id != "" {
		r.try(nil, "wrangler", "d1", "delete", r.worker+"-db", "-y")
	}
	if id := r.kv(); id != "" {
		r.try(nil, "wrangler", "kv", "namespace", "delete", "--namespace-id", id, "-y")
	}
	if r.r2() {
		r.try(nil, "wrangler", "r2", "object", "delete", r.worker+"-bucket/stamp.txt", "--remote")
		r.try(nil, "wrangler", "r2", "bucket", "delete", r.worker+"-bucket")
	}
}

// prove is the one real deploy of a Worker repo: local, deploy on its own login, live, stray tokens, deleting
// everything, and with clean the login logged out.
func prove(t *testing.T, r repo, clean bool) {
	r.step("1. A developer with no Cloudflare login at all: setup, the rules, a local Worker")
	r.must(nil, "mise", "run", "setup")
	r.must(nil, "mise", "run", "cloudflare", "check")
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker answered %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work, with no Cloudflare login")

	r.step("2. Deploy on the repo's own login, %s (logs in the first time only)", r.worker)
	r.cf(nil, "login")
	if out, err := r.try(nil, "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed on this account: another machine may be proving it now. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("%s is left over from another run: delete it first", strings.Join(left, ", "))
	}
	r.pass("neither the Worker nor its D1, KV or R2 is on this account yet, so nothing else is using them")
	t.Cleanup(func() {
		if t.Failed() {
			r.step("Cleaning up after the failure")
			r.deleteAll()
		}
	})
	before := r.must(nil, "git", "status", "--porcelain", ".")
	// As a developer does it: what the guide says, each time.
	r.next("wrangler secret put GREETING")
	r.putSecret("secret", "put", "GREETING")
	r.next("mise run cloudflare:deploy")
	r.must(nil, "mise", "run", "cloudflare:deploy")
	s := r.status()
	if !s.LoggedIn || !s.Deployed {
		t.Fatalf("not deployed on the login %s: %+v", r.worker, s)
	}
	r.pass("deployed on the repo's own login, %s", s.Profile)

	r.step("3. The live Worker: D1, KV, R2 and the secret")
	url := r.deployedURL()
	if code, body := get(url); code != 200 || !allWork(body) {
		t.Fatalf("%s: HTTP %d %s", url, code, body)
	}
	r.pass("%s: D1, KV, R2 and the secret all work", url)
	if after := r.must(nil, "git", "status", "--porcelain", "."); after != before {
		t.Fatalf("the repo changed:\n%s", after)
	}
	r.pass("deploying wrote nothing into the repo: no IDs")
	r.next("mise run cloudflare:github")

	r.step("4. A stray CLOUDFLARE_API_TOKEN is refused, in the shell and in a .env")
	if out, err := r.try([]string{"CLOUDFLARE_API_TOKEN=dummy-not-a-real-token"}, "wrangler", "deploy"); err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in the shell")
	}
	r.pass("a token in the shell is refused")
	env := filepath.Join(r.dir, ".env")
	os.WriteFile(env, []byte("CLOUDFLARE_API_TOKEN=dummy-not-a-real-token\n"), 0o600)
	out, err := r.try(nil, "wrangler", "deploy")
	os.Remove(env)
	if err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in .env")
	}
	r.pass("a token in .env is refused")

	r.step("5. wrangler, on the repo's login, deletes the Worker, its D1, KV and R2")
	r.deleteAll()
	if !gone(url, time.Minute) {
		t.Fatalf("%s still answers", url)
	}
	r.pass("%s no longer answers", url)
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("no D1, KV or R2 left")

	if clean {
		r.step("6. Logout removes the login")
		r.cf(nil, "logout")
		if r.status().LoggedIn {
			t.Fatal("the login is still there")
		}
		r.pass("the login is gone")
	}
	fmt.Println("\nALL PASSED")
}

// TestProve proves it all in example/.
func TestProve(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("../example")
	prove(t, repo{t: t, dir: dir, worker: "cloudflare-zero-example"}, os.Getenv("PROVE_CLEAN") == "true")
}

// TestNewRepo is what a user does in a new repo, with no Cloudflare account: mise-cloudflare from mise (this
// checkout's build, or the release MISE_CLOUDFLARE_VERSION as mise use installs it), init, then status, check and a
// local Worker. Every step goes through mise, as in the README.
func TestNewRepo(t *testing.T) {
	if os.Getenv("MISE_CLOUDFLARE_SMOKE") != "1" {
		t.Skip("mise run smoke")
	}
	src, _ := filepath.Abs("../example")
	r := repo{t: t, dir: filepath.Join(t.TempDir(), "new-repo"), worker: "cloudflare-zero-new", tool: []string{"mise", "exec", "--", "mise-cloudflare"}}
	os.MkdirAll(filepath.Join(r.dir, "src"), 0o755)
	for _, f := range []string{"src/worker.js", ".dev.vars.example"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(r.dir, f), b, 0o644)
	}
	b, _ := os.ReadFile(filepath.Join(src, "wrangler.jsonc"))
	os.WriteFile(filepath.Join(r.dir, "wrangler.jsonc"), bytes.ReplaceAll(b, []byte(`"cloudflare-zero-example"`), []byte(`"`+r.worker+`"`)), 0o644)
	// A range, as most repos have it: init pins it.
	os.WriteFile(filepath.Join(r.dir, "package.json"), []byte(`{ "name": "new-repo", "private": true, "devDependencies": { "wrangler": "^4.151.3" } }`+"\n"), 0o644)
	r.must(nil, "git", "init", "-q")
	if v := os.Getenv("MISE_CLOUDFLARE_VERSION"); v != "" {
		r.step("mise-cloudflare %s from GitHub, as mise use installs it", v)
		r.must(nil, "mise", "use", "-q", "node@26.12.1", "github:joeblew999/mise-cloudflare@"+strings.TrimPrefix(v, "v"))
	} else {
		bin, _ := filepath.Abs("../bin")
		r.step("this checkout's mise-cloudflare, from %s", bin)
		os.WriteFile(filepath.Join(r.dir, "mise.toml"), []byte("[env]\n_.path = [\""+bin+"\"]\n\n[tools]\nnode = \"26.12.1\"\n"), 0o644)
	}
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	r.must(nil, "mise", "exec", "--", "npm", "install", "--silent")

	r.step("init, then the repo passes check, and the status screen says what is next")
	r.cf(nil, "init")
	r.must(nil, "mise", "exec", "--", "npm", "install", "--silent") // init pinned wrangler: the lockfile follows
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	if out := r.must(nil, "mise", "run", "cloudflare", "check"); !strings.Contains(out, "rules: pass") {
		t.Fatalf("check: %s", out)
	}
	if out := r.must(nil, "mise", "run", "cloudflare"); !strings.Contains(out, "next: mise run cloudflare wrangler secret put GREETING") {
		t.Fatalf("status: %s", out)
	}
	r.pass("init fixed what check asks for, and the guide says to set the Worker's secret next")

	r.step("A local Worker, with no Cloudflare login")
	os.WriteFile(filepath.Join(r.dir, ".dev.vars"), []byte("GREETING=hello\n"), 0o600) // the secret's local value
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker: %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work")
	fmt.Println("\nALL PASSED")
}

// TestJSONPart: wrangler's JSON is found past a warning that has brackets in it.
func TestJSONPart(t *testing.T) {
	out := "\u25b2 [WARNING] auth activate is experimental\n\n[\n  {\"uuid\": \"x\"}\n]\n"
	if got := jsonPart(out); !strings.HasPrefix(got, "[\n  {") {
		t.Errorf("got %q", got)
	}
}

// gone waits, for up to limit, until url no longer answers 200.
func gone(url string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(5 * time.Second) {
		if resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url); err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			return true
		} else {
			resp.Body.Close()
		}
	}
	return false
}
