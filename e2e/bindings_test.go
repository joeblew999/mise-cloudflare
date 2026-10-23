package e2e

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestBindings proves the scopes table (config.BindingScopes): a Worker with one of each kind of binding Cloudflare
// makes for nothing (testdata/bindings), on its own login holding exactly the scopes worked out from them, makes its
// queue and Vectorize index, deploys (its Durable Object, Workflow, Workers AI, Browser, Images, Analytics Engine and
// rate limit with it), and uses every binding live. Then the Worker, the queue, the index and the Workflow go.
func TestBindings(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("testdata/bindings")
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-bindings", tool: []string{buildTool(t)}}
	const queue, index, workflow = "cloudflare-zero-bindings-q", "cloudflare-zero-bindings-idx", "cloudflare-zero-bindings-wf"

	r.step("The scopes worked out from the Worker's bindings, and its own login with exactly those")
	r.must(nil, "npm", "ci", "--no-audit", "--no-fund")
	s := r.status()
	want := []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "queues:write", "ai:write", "browser:write"}
	if !slices.Equal(s.Needs, want) {
		t.Fatalf("its bindings %q need %q, want %q", s.Bindings, s.Needs, want)
	}
	r.pass("its bindings (%s) need %s", strings.Join(s.Bindings, ", "), strings.Join(s.Needs, " "))
	r.cf(nil, "login") // fails unless the login holds exactly those
	r.pass("the login %s holds exactly those", s.Profile)

	if out, err := r.try(nil, "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	for _, l := range [][]string{{"queues", "list"}, {"vectorize", "list"}, {"workflows", "list"}} {
		for _, name := range []string{queue, index, workflow} {
			if strings.Contains(r.wrangler(l...), name) {
				t.Fatalf("%s is left over from another run: delete it first", name)
			}
		}
	}
	t.Cleanup(func() {
		r.step("Removing everything: the Worker, its Workflow, queue and Vectorize index")
		r.try(nil, "wrangler", "delete", "--force")
		r.try(nil, "wrangler", "workflows", "delete", workflow)
		r.try(nil, "wrangler", "queues", "delete", queue)
		r.try(nil, "wrangler", "vectorize", "delete", index, "--force")
	})

	r.step("Its queue and Vectorize index, made on that login")
	r.cf(nil, "wrangler", "queues", "create", queue)
	r.cf(nil, "wrangler", "vectorize", "create", index, "--dimensions", "3", "--metric", "cosine")
	r.pass("%s and %s", queue, index)

	r.step("Deploy on that login: the Durable Object, Workflow, Workers AI, Browser, Images, Analytics Engine and rate limit with it")
	r.cf([]string{"WRANGLER_OUTPUT_FILE_PATH=.wrangler/output.json"}, "wrangler", "deploy")
	url := r.deployedURL()
	r.pass("deployed: %s", url)

	r.step("The live Worker uses every binding")
	var got map[string]any
	allWork := func() bool {
		for _, v := range got {
			if v != true {
				return false
			}
		}
		return len(got) > 0
	}
	for end := time.Now().Add(2 * time.Minute); ; time.Sleep(10 * time.Second) {
		if code, body := fetch(url); code == 200 && json.Unmarshal(body, &got) == nil {
			fmt.Printf("  %s\n", body)
			if allWork() {
				break
			}
		}
		if time.Now().After(end) {
			t.Fatalf("%s: not every binding works: %v", url, got)
		}
	}
	var kinds []string
	for k := range got {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	r.pass("%s: %s all work", url, strings.Join(kinds, ", "))
	fmt.Println("\nALL PASSED")
}
