package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestBuilds proves go tool cloudflare builds, staging and production as go-htmx4 has them: Cloudflare's Workers Builds
// deploys a Worker from its GitHub repository with its own token, a push to the staging branch deploying staging (the
// wrangler environment env.staging, its own Worker), a push to the production branch deploying production, and any
// other branch getting a preview URL, on the previews' own KV and R2 (go tool cloudflare previews, which builds runs),
// which goes when its branch does. The Worker (testdata/builds) says which branch
// built it, so the proof sees Cloudflare's deploys, not its own. Its branches are throwaway ones, deleted with
// everything else afterwards; main is never touched.
func TestBuilds(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("testdata/builds")
	// The tool from this checkout, built once: testdata/builds is in this module, which has no tool line for itself.
	tool := []string{buildTool(t)}
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-builds", tool: tool}
	st := repo{t: t, dir: dir, worker: "cloudflare-zero-builds-staging", tool: tool, env: "staging"}
	stamp := time.Now().Unix()
	stage, prod, prev := fmt.Sprintf("cfz-builds-staging-%d", stamp), fmt.Sprintf("cfz-builds-prod-%d", stamp), fmt.Sprintf("cfz-builds-preview-%d", stamp)

	r.step("Production and staging, each with KV and R2, deployed once by hand on the repo's own login")
	r.must(nil, "npm", "ci", "--no-audit", "--no-fund")
	r.cf(nil, "login")
	for _, w := range []repo{r, st} {
		args := []string{"wrangler", "deployments", "list", "--json", "--name", w.worker}
		if out, err := w.try(nil, args...); err == nil {
			t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it", w.worker)
		} else if !strings.Contains(out, "10007") {
			t.Fatalf("couldn't tell whether %s is deployed", w.worker)
		}
		if left := w.leftovers(); len(left) > 0 {
			t.Fatalf("%s is left over from another run: delete it first", strings.Join(left, ", "))
		}
	}
	t.Cleanup(func() {
		r.step("Removing everything: the Workers Builds setup, the branches, both Workers, their KV and R2")
		r.try(nil, "-off", "-staging", stage, "builds")
		for _, b := range []string{stage, prod, prev} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		r.deleteAll()
		st.deleteAll()
		r.sweep(r.worker) // anything else named after the Worker
	})
	out := []string{"WRANGLER_OUTPUT_FILE_PATH=.wrangler/output.json"}
	r.cf(out, "wrangler", "deploy")
	url := r.deployedURL()
	r.cf(out, "wrangler", "deploy", "--env", "staging")
	stagingURL := r.deployedURL()
	for _, u := range []string{url, stagingURL} {
		if !built(u, "here", time.Minute) {
			t.Fatalf("%s isn't working", u)
		}
		r.pass("%s: built here, KV and R2 work", u)
	}

	r.step("go tool cloudflare builds: production from %s, staging from %s, previews from any other branch, with their own KV and R2", prod, stage)
	r.cf(nil, "-branch", prod, "-staging", stage, "builds")
	pv := repo{t: t, dir: dir, worker: r.worker + "-previews", tool: tool} // the previews' own <worker>-previews-kv and -bucket
	pvKV := pv.kv()
	if pvKV == "" || !pv.r2() {
		t.Fatalf("builds made no %s-kv and %s-bucket for the previews", pv.worker, pv.worker)
	}
	r.pass("set up; the previews' own KV and R2 are %s-kv and %s-bucket, in the Worker's Previews Base", pv.worker, pv.worker)

	r.step("A push to %s: Workers Builds deploys staging, with Cloudflare's own token", stage)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+stage)
	if !built(stagingURL, stage, 15*time.Minute) {
		r.buildsLog(st.worker)
		t.Fatalf("%s wasn't deployed from %s in 15 minutes", stagingURL, stage)
	}
	r.pass("%s: built from %s by Workers Builds; KV and R2 work", stagingURL, stage)

	r.step("A push to %s: Workers Builds deploys production", prod)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prod)
	if !built(url, prod, 15*time.Minute) {
		r.buildsLog(r.worker)
		t.Fatalf("%s wasn't deployed from %s in 15 minutes", url, prod)
	}
	r.pass("%s: built from %s by Workers Builds; KV and R2 work", url, prod)

	r.step("A pull request's branch %s, with a commit of its own: a preview at its own URL, on its own KV and R2; production and staging untouched", prev)
	// The commit is made in a worktree, so this checkout is untouched.
	wt := filepath.Join(t.TempDir(), "wt")
	r.must(nil, "git", "worktree", "add", "-q", "--detach", wt, "HEAD")
	t.Cleanup(func() { r.run(nil, "git", "worktree", "remove", "--force", wt) })
	os.WriteFile(filepath.Join(wt, "e2e", "testdata", "builds", "preview.txt"), []byte("a pull request's change\n"), 0o644)
	r.must(nil, "git", "-C", wt, "add", "-A")
	r.must(nil, "git", "-C", wt, "commit", "-qm", "prove-builds: a pull request's change")
	r.must(nil, "git", "-C", wt, "push", "-q", "origin", "HEAD:refs/heads/"+prev)
	// A branch's preview is at <alias>-<worker>.<subdomain>.workers.dev, the alias being the branch's name.
	preview := strings.Replace(url, "https://", "https://"+alias(prev)+"-", 1)
	if !built(preview, prev, 15*time.Minute) {
		r.buildsLog(r.worker)
		t.Fatalf("%s wasn't built from %s in 15 minutes", preview, prev)
	}
	if stamp := pv.wrangler("kv", "key", "get", "stamp", "--namespace-id", pvKV, "--remote"); !strings.Contains(stamp, "T") {
		t.Fatalf("the preview didn't write to the previews' own KV %s-kv", pv.worker)
	}
	r.pass("%s: built from %s; its KV and R2 work, and they're the previews' own", preview, prev)
	if !built(url, prod, time.Minute) || !built(stagingURL, stage, time.Minute) {
		t.Fatal("the preview changed production or staging")
	}
	r.pass("production still built from %s, staging from %s", prod, stage)

	r.step("Deleting %s, as merging a pull request does: its preview goes with it", prev)
	r.must(nil, "git", "push", "-q", "origin", "--delete", prev)
	if !gone(preview, prev, 10*time.Minute) {
		t.Fatalf("%s still answers, 10 minutes after its branch went", preview)
	}
	r.pass("%s is gone", preview)

	r.step("go tool cloudflare builds -off removes the setup and the previews' own KV and R2; then both Workers, their KV and R2 go")
	r.cf(nil, "-off", "-staging", stage, "builds")
	r.pass("removed")
	r.deleteAll()
	st.deleteAll()
	if left := r.sweep(r.worker); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	for _, w := range []repo{r, st} {
		if left := w.leftovers(); len(left) > 0 {
			t.Fatalf("left on the account: %s", strings.Join(left, ", "))
		}
	}
	r.pass("nothing left on the account")
	fmt.Println("\nALL PASSED")
}

// gone waits until url no longer serves what branch built, or time runs out.
func gone(url, branch string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(15 * time.Second) {
		if !built(url, branch, time.Second) {
			fmt.Printf("  %s: no longer built from %s\n", url, branch)
			return true
		}
	}
	return false
}

// built fetches url until it says it was built from branch with its KV and R2 working, or time runs out.
func built(url, branch string, limit time.Duration) bool {
	last := ""
	for end := time.Now().Add(limit); ; {
		if resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if body := strings.TrimSpace(string(b)); body != last {
				last = body
				fmt.Printf("  %s: %.120s\n", url, body)
			}
			var v struct {
				Built  string
				KV, R2 bool
			}
			if json.Unmarshal(b, &v) == nil && v.Built == branch && v.KV && v.R2 {
				return true
			}
		}
		if time.Now().After(end) {
			return false
		}
		time.Sleep(10 * time.Second)
	}
}

// alias is a branch name as a preview alias: lowercase letters, digits and dashes.
func alias(branch string) string {
	return strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(branch), "-"), "-")
}

// buildsLog prints worker's latest Workers Builds, to see why a deploy didn't come.
func (r repo) buildsLog(worker string) {
	out, _ := r.run(nil, "cf", "workers", "scripts", "search", "--name", worker, "--profile", "cloudflare-zero-example")
	var found []struct{ ID, Script_name string }
	json.Unmarshal([]byte(jsonPart(out)), &found)
	for _, w := range found {
		if w.Script_name == worker {
			r.run(nil, "cf", "builds", "list", "--external-script-id", w.ID, "--per-page", "5", "--profile", "cloudflare-zero-example")
		}
	}
}
