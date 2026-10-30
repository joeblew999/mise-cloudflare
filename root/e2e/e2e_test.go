// Package e2e is the root's end-to-end tests. Each is a task of the root's mise, named for what it acts on, and runs the
// Go test of its name, in the file of its name: mise run example:try is TestTry in try_test.go, and root:check is
// TestCheck. A test on an example does what a person does, in their order: the root's numbered steps, mise run
// example:nn-x, on the repo EXAMPLE names in examples/ (the reference by default); it looks at the repo, and cleans up,
// with the repo's own tasks. try and check need no account (TRY=1, CHECK=1); deploy, deploy-every-push and clean are
// real, on a Cloudflare account (CLOUDFLARE_E2E=1). Without those the tests skip, so go test ./... never touches
// Cloudflare. This file is what they share.
package e2e

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
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

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
)

func needAccount(t *testing.T) {
	if os.Getenv("CLOUDFLARE_E2E") != "1" {
		t.Skip("needs a Cloudflare account: mise run example:deploy")
	}
}

// repo is a Worker repo that uses mise-cloudflare: its folder, its Worker's name, and, for one in examples/, the root
// whose steps run on it (top) and its name there (EXAMPLE).
type repo struct {
	t      *testing.T
	top    string
	name   string
	dir    string
	worker string
	env    string // a wrangler environment's Worker (staging), "" for the Worker itself
}

// example is the repo EXAMPLE names in this repo's examples/ (the reference by default), set up by the root's steps up
// to example:04-check, whose rules must pass, with its Worker's name as wrangler reads it.
func example(t *testing.T) repo {
	top, _ := filepath.Abs("../..")
	r := repo{t: t, top: top, name: cmp.Or(os.Getenv("EXAMPLE"), "reference")}
	r.dir = filepath.Join(top, "examples", r.name)
	if out := r.mustStep(nil, "04-check"); !strings.Contains(out, "rules: pass") {
		t.Fatalf("example:04-check on examples/%s: %s", r.name, out)
	}
	c, err := wconfig.Read(context.Background(), r.dir)
	if err != nil {
		t.Fatal(err)
	}
	r.worker = c.Name
	r.pass("example:04-check on examples/%s: the rules pass", r.name)
	return r
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

// runStep runs the root's step example:<step> on the repo, as a person runs it at the root, with input typed into it
// (a secret's value, when it asks).
func (r repo) runStep(env []string, input, step string, args ...string) (string, error) {
	fmt.Printf("$ EXAMPLE=%s mise run example:%s %s\n", r.name, step, strings.Join(args, " "))
	env = append([]string{"EXAMPLE=" + r.name, "MISE_YES=1", "MISE_TRUSTED_CONFIG_PATHS=" + r.top}, env...)
	return r.runIn(r.top, env, input, "mise", append([]string{"run", "example:" + step}, args...)...)
}

func (r repo) mustStep(env []string, step string, args ...string) string {
	r.t.Helper()
	out, err := r.runStep(env, "", step, args...)
	if err != nil {
		r.t.Fatalf("example:%s %s: %v", step, strings.Join(args, " "), err)
	}
	return out
}

// task runs one of the repo's own tasks, mise run cloudflare:<task>, in the repo: how a test looks at it and cleans up.
func (r repo) task(input, task string, args ...string) (string, error) {
	env := []string{"MISE_CEILING_PATHS=" + filepath.Dir(r.dir), "MISE_TRUSTED_CONFIG_PATHS=" + r.dir, "MISE_YES=1"}
	return r.runIn(r.dir, env, input, "mise", append([]string{"run", "cloudflare:" + task}, args...)...)
}

// wranglerTry runs a wrangler command on the repo's login, with its own task cloudflare:run-wrangler.
func (r repo) wranglerTry(args ...string) (string, error) { return r.task("", "run-wrangler", args...) }

func (r repo) wrangler(args ...string) string {
	r.t.Helper()
	out, err := r.wranglerTry(args...)
	if err != nil {
		r.t.Fatalf("wrangler %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// shown is what step 05 shows, as JSON: cloudflare:05-show --json.
type shown struct {
	Profile  string   `json:"profile"`
	LoggedIn bool     `json:"loggedIn"`
	Needs    []string `json:"needs"`
	Deployed bool     `json:"deployed"`
	URL      string   `json:"url"`
	GitHub   string   `json:"github"`
	Next     string   `json:"next"`
}

// next is what 05-show says to run next, failing unless it says want.
func (r repo) next(want string) {
	r.t.Helper()
	if s := r.show(); !strings.Contains(s.Next, want) {
		r.t.Fatalf("05-show says next: %q, not %q", s.Next, want)
	}
	r.pass("05-show says next: %s", want)
}

func (r repo) show() shown {
	r.t.Helper()
	// The repo's own task, cloudflare:05-show --json, its output unprefixed so its JSON reads as JSON.
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

// localWorker runs the Worker as a person does, with the root's step example:06-serve (which makes .dev.vars from
// .dev.vars.example), with no Cloudflare login anywhere it could see: a home of its own, with mise's own folders kept,
// so its tools are still there. It returns the Worker's answer.
func (r repo) localWorker() []byte {
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
	cmd := exec.Command("mise", "run", "--skip-deps", "example:06-serve", "--ip", "127.0.0.1", "--port", port)
	cmd.Dir = r.top
	cmd.Env = append(os.Environ(), "EXAMPLE="+r.name, "MISE_YES=1", "MISE_TRUSTED_CONFIG_PATHS="+r.top,
		"HOME="+home, "XDG_CONFIG_HOME="+home, "CLOUDFLARE_API_TOKEN=", "CI=1",
		"MISE_DATA_DIR="+doctor.Dirs.Data, "MISE_CACHE_DIR="+doctor.Dirs.Cache, "MISE_STATE_DIR="+doctor.Dirs.State)
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
	r.t.Fatalf("example:06-serve never answered:\n%s", logs.String())
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

// leftovers names the Worker's D1, KV and R2 that exist, which a real deploy must start without and end without.
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
		p := r
		p.worker = r.worker + "-previews"
		left = append(left, p.leftovers()...)
	}
	return left
}

// deleteAll deletes the Worker, its D1, KV and R2, and its previews' own, with wrangler on the repo's login. Errors
// are ignored: it also runs as cleanup after a failure, when some of them may not exist.
func (r repo) deleteAll() {
	if !strings.HasSuffix(r.worker, "-previews") {
		p := r
		p.worker = r.worker + "-previews"
		defer p.deleteAll()
	}
	if r.env != "" {
		r.wranglerTry("delete", "--force", "--env", r.env)
	} else {
		r.wranglerTry("delete", "--force")
	}
	if id := r.d1(); id != "" {
		r.wranglerTry("d1", "delete", r.worker+"-db", "-y")
	}
	if id := r.kv(); id != "" {
		r.wranglerTry("kv", "namespace", "delete", "--namespace-id", id, "-y")
	}
	if r.r2() {
		r.wranglerTry("r2", "object", "delete", r.worker+"-bucket/stamp.txt", "--remote")
		r.wranglerTry("r2", "bucket", "delete", r.worker+"-bucket")
	}
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
