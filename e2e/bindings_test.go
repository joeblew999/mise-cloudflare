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

// TestBindings proves a repo's login, with wrangler's own login's scopes, does what every kind of binding needs: a
// Worker with one of each kind Cloudflare makes for nothing (testdata/bindings), on its own login, makes its queue and
// Vectorize index, deploys (its Durable Object, Workflow, Workers AI, Browser, Images, Analytics Engine and rate limit
// with it), and uses every binding live. Then the Worker, the queue, the index and the Workflow go.
func TestBindings(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("testdata/bindings")
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-bindings", tool: []string{buildTool(t)}}
	const queue, index, workflow = "cloudflare-zero-bindings-q", "cloudflare-zero-bindings-idx", "cloudflare-zero-bindings-wf"

	r.step("Its own login, with wrangler's own login's scopes")
	r.must(nil, "npm", "ci", "--no-audit", "--no-fund")
	s := r.status()
	if !slices.Contains(s.Needs, "workers_scripts:write") || !slices.Contains(s.Needs, "queues:write") {
		t.Fatalf("its login needs %q: not wrangler's own login's scopes", s.Needs)
	}
	r.cf(nil, "login") // fails unless the login holds exactly those
	r.pass("the login %s holds wrangler's own login's scopes: %s", s.Profile, strings.Join(s.Needs, " "))

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
	r.cf(nil, "wrangler", "vectorize", "create", index, "--dimensions", "32", "--metric", "cosine")
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
