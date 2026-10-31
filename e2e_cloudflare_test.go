// What the walk looks at besides, as a person does: the Worker's URL, the account's D1, KV and R2, Cloudflare's
// builds.

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/mise-cloudflare/internal/cfapi"
)

// allWork is the template's Worker's answer when D1, KV, R2 and the secret all work.
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

// waitTillItWorks waits until url answers with the template's Worker's D1, KV, R2 and secret all working.
func waitTillItWorks(t *testing.T, url string, limit time.Duration) {
	t.Helper()
	var body []byte
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(15 * time.Second) {
		var code int
		if code, body = get(url); code == 200 && allWork(body) {
			return
		}
	}
	t.Fatalf("%s doesn't work: %s", url, body)
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

// d1ID is the uuid of the Worker's D1 (wrangler names it <worker>-db), "" if it doesn't exist.
func (r repo) d1ID() string {
	r.t.Helper()
	var dbs []struct{ UUID, Name string }
	if out := r.miseWranglerStdout("d1", "list", "--json"); json.Unmarshal([]byte(jsonPart(out)), &dbs) != nil {
		r.t.Fatalf("wrangler d1 list --json printed no list of D1 databases:\n%s", out)
	}
	for _, d := range dbs {
		if d.Name == r.worker+"-db" {
			return d.UUID
		}
	}
	return ""
}

// kvID is the id of the Worker's KV namespace (<worker>-kv), "" if it doesn't exist.
func (r repo) kvID() string {
	r.t.Helper()
	var ns []struct{ ID, Title string }
	if out := r.miseWranglerStdout("kv", "namespace", "list"); json.Unmarshal([]byte(jsonPart(out)), &ns) != nil {
		r.t.Fatalf("wrangler kv namespace list printed no list of KV namespaces:\n%s", out)
	}
	for _, n := range ns {
		if n.Title == r.worker+"-kv" {
			return n.ID
		}
	}
	return ""
}

// hasR2 reports whether the Worker's bucket (<worker>-bucket) exists. wrangler lists each as "name:" and the name,
// padded.
func (r repo) hasR2(token, account string) bool {
	got, err := cfapi.Call(context.Background(), token, http.MethodGet, "/accounts/"+account+"/r2/buckets/"+r.worker+"-bucket", nil, "")
	if err != nil || (!got.OK() && got.Status != http.StatusNotFound) {
		r.t.Fatalf("Cloudflare's R2 bucket %s-bucket: %v %s", r.worker, err, got.Said())
	}
	return got.OK()
}

// api is the repo's login's OAuth token, as wrangler gives it (cloudflare:run-wrangler), and its account, as
// Cloudflare's API gives it, for Cloudflare's API. The token is never printed.
func (r repo) api() (token, account string) {
	var t struct {
		Token string `json:"token"`
	}
	if json.Unmarshal([]byte(jsonPart(r.miseWranglerStdout("auth", "token", "--json"))), &t) != nil || t.Token == "" {
		r.t.Fatal("wrangler auth token --json gave no token")
	}
	var accounts []struct {
		ID string `json:"id"`
	}
	got, err := cfapi.Call(context.Background(), t.Token, http.MethodGet, "/accounts", nil, "")
	if err != nil || !got.OK() || json.Unmarshal(got.Result, &accounts) != nil || len(accounts) != 1 {
		r.t.Fatalf("the login's accounts, from Cloudflare's API: %d, want 1: %v %s", len(accounts), err, got.Said())
	}
	return t.Token, accounts[0].ID
}

// leftOnCloudflare names the Worker's D1, KV and R2 that exist, and its previews', and with the extras theirs, which a
// walk that deploys must start without and end without.
func (r repo) leftOnCloudflare() []string {
	return r.leftWith(r.api())
}

func (r repo) leftWith(token, account string) []string {
	left := r.left(token, account)
	if withExtras() {
		left = append(left, r.extrasOn(token, account)...)
	}
	return left
}

// left names the Worker's D1, KV and R2 that exist, and its previews'.
func (r repo) left(token, account string) []string {
	var left []string
	if r.d1ID() != "" {
		left = append(left, "D1 "+r.worker+"-db")
	}
	if r.kvID() != "" {
		left = append(left, "KV "+r.worker+"-kv")
	}
	if r.hasR2(token, account) {
		left = append(left, "R2 "+r.worker+"-bucket")
	}
	if !strings.HasSuffix(r.worker, "-previews") {
		p := r
		p.worker = r.worker + "-previews"
		left = append(left, p.left(token, account)...)
	}
	return left
}

// nothingOnCloudflare fails unless the Worker and its D1, KV and R2 (and its previews') aren't on the account.
func (r repo) nothingOnCloudflare() {
	r.t.Helper()
	// Whether the Worker is there is Cloudflare's API's to say, by its status (404 not there), as the tool decides.
	token, account := r.api()
	got, err := cfapi.Call(context.Background(), token, http.MethodGet, "/accounts/"+account+"/workers/scripts/"+r.worker+"/settings", nil, "")
	switch {
	case err != nil:
		r.t.Fatal(err)
	case got.OK():
		r.t.Fatalf("%s is still deployed", r.worker)
	case got.Status != http.StatusNotFound:
		r.t.Fatalf("couldn't tell whether %s is deployed: %s", r.worker, got.Said())
	}
	if left := r.leftWith(token, account); len(left) > 0 {
		r.t.Fatalf("%s is still on the account", strings.Join(left, ", "))
	}
	r.pass("nothing of %s is on the account", r.worker)
}

// buildOutcome waits for Cloudflare's own record of a Workers Builds build of the Worker from branch, and returns its
// outcome (success, failure), or what it waited for.
func (r repo) buildOutcome(branch string, limit time.Duration) string {
	r.t.Helper()
	cfJSON := func(v any, args ...string) error {
		cmd := exec.Command("cf", append(args, "--profile", r.worker)...)
		cmd.Dir = r.dir
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		var env struct{ Result json.RawMessage }
		if json.Unmarshal(out, &env) == nil && len(env.Result) > 0 {
			out = env.Result
		}
		return json.Unmarshal(out, v)
	}
	var found []struct {
		ID   string `json:"id"`
		Name string `json:"script_name"`
	}
	cfJSON(&found, "workers", "scripts", "search", "--name", r.worker)
	tag := ""
	for _, f := range found {
		if f.Name == r.worker {
			tag = f.ID
		}
	}
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(20 * time.Second) {
		var builds []struct {
			UUID    string `json:"build_uuid"`
			Outcome string `json:"build_outcome"`
			Trigger struct {
				Branch string `json:"branch"`
			} `json:"build_trigger_metadata"`
		}
		cfJSON(&builds, "builds", "list", "--external-script-id", tag, "--per-page", "10")
		for _, b := range builds {
			if b.Trigger.Branch == branch && b.Outcome != "" {
				if b.Outcome != "success" {
					r.run(nil, "cf", "builds", "logs", "get", b.UUID, "--profile", r.worker)
				}
				return b.Outcome
			}
		}
	}
	return "no build in " + limit.String()
}

// previewAlias is a branch name as Cloudflare names its preview: lowercase letters, digits and dashes.
func previewAlias(branch string) string {
	return strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(branch), "-"), "-")
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

// TestJSONPart: wrangler's JSON is found past a warning that has brackets in it.
func TestJSONPart(t *testing.T) {
	out := "▲ [WARNING] auth activate is experimental\n\n[\n  {\"uuid\": \"x\"}\n]\n"
	if got := jsonPart(out); !strings.HasPrefix(got, "[\n  {") {
		t.Errorf("got %q", got)
	}
}
