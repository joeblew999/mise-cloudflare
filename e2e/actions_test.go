package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestActions proves mise run cloudflare actions: on a throwaway branch of this commit, it writes the deploying
// workflow and stores CI's token as this repository's GitHub secrets; a push deploys the Worker by GitHub Actions; a
// pull request gets its preview, its URL commented, with its own D1, KV, R2 and secret; closing it deletes the preview.
// Then everything goes. main is never touched.
func TestActions(t *testing.T) {
	needAccount(t)
	src, _ := filepath.Abs("../example")
	r := repo{t: t, dir: src, worker: "cloudflare-zero-example"}
	stamp := time.Now().Unix()
	prod, pr := fmt.Sprintf("cfz-actions-%d", stamp), fmt.Sprintf("cfz-pr-%d", stamp)
	const workflow = ".github/workflows/cloudflare-example.yml"

	r.step("The Worker isn't on the account yet; a worktree of this commit, on the throwaway branch %s", prod)
	r.must(nil, "mise", "run", "setup")
	r.cf(nil, "login")
	r.notDeployed()
	wt := filepath.Join(t.TempDir(), "wt")
	r.must(nil, "git", "worktree", "add", "-q", "-b", prod, wt, "HEAD")
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", wt)
	w := repo{t: t, dir: filepath.Join(wt, "example"), worker: r.worker}
	t.Cleanup(func() {
		r.step("Removing everything: the pull request, the branches, CI's secrets, the Worker and everything of it")
		if n := prNumber(pr); n != "" {
			r.run(nil, "gh", "pr", "close", n)
		}
		for _, b := range []string{prod, pr} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		w.run(nil, "mise", "run", "cloudflare", "actions", "--off")
		w.run(nil, "npx", "--no-install", "wrangler", "auth", "deactivate", w.realDir())
		r.run(nil, "git", "worktree", "remove", "--force", wt)
		r.run(nil, "git", "branch", "-D", prod, pr)
		r.deleteAll()
	})
	w.must(nil, "mise", "run", "setup")
	// What the Worker needs before GitHub deploys it: its secret, and the same for its previews.
	w.putSecret("secret", "put", "GREETING")
	w.putSecret("preview", "base-config", "secret", "put", "GREETING")

	r.step("mise run cloudflare actions --branch %s: the deploying workflow, and CI's token as GitHub secrets", prod)
	// CI's token, for the proof: the repo login's own, good for about an hour; a repo makes one from the link it gives.
	var tok struct{ Token string }
	json.Unmarshal([]byte(jsonPart(r.wrangler("auth", "token", "--json"))), &tok)
	w.must([]string{"CLOUDFLARE_CI_TOKEN=" + tok.Token}, "mise", "run", "cloudflare", "actions", "--branch", prod)
	if s := w.status(); s.GitHub != "GitHub Actions" {
		t.Fatalf("the guide says %q deploys it", s.GitHub)
	}
	r.pass("the guide says GitHub Actions deploys it")
	w.must(nil, "git", "add", "-A")
	w.must(nil, "git", "commit", "-qm", "prove: GitHub Actions deploys "+r.worker+" from "+prod)

	r.step("A push to %s: GitHub Actions checks the repo and deploys the Worker", prod)
	w.must(nil, "git", "push", "-q", "origin", prod)
	run := waitRun(t, w, workflow, prod, "push")
	url := regexp.MustCompile(`https://` + r.worker + `\.[a-z0-9-]+\.workers\.dev`).FindString(w.must(nil, "gh", "run", "view", run, "--log"))
	if url == "" {
		t.Fatal("the deploy's log has no workers.dev URL")
	}
	waitWork(t, url, 3*time.Minute)
	r.pass("%s works, deployed by GitHub Actions", url)

	r.step("A pull request: its preview pr-<number>, with its own D1, KV, R2 and secret, its URL commented")
	w.must(nil, "git", "checkout", "-q", "-b", pr)
	os.WriteFile(filepath.Join(w.dir, "pr.txt"), []byte("a pull request's change\n"), 0o644)
	w.must(nil, "git", "add", "-A")
	w.must(nil, "git", "commit", "-qm", "prove: a pull request's change")
	w.must(nil, "git", "push", "-q", "origin", pr)
	w.must(nil, "gh", "pr", "create", "--base", prod, "--head", pr, "--title", "prove: a preview", "--body", "A throwaway pull request from mise run prove; it closes itself.")
	n := prNumber(pr)
	waitRun(t, w, workflow, pr, "pull_request")
	preview := ""
	for end := time.Now().Add(2 * time.Minute); preview == "" && time.Now().Before(end); time.Sleep(10 * time.Second) {
		comments := w.must(nil, "gh", "pr", "view", n, "--json", "comments", "--jq", ".comments[].body")
		preview = regexp.MustCompile(`https://[a-z0-9.-]+\.workers\.dev`).FindString(comments)
	}
	if preview == "" {
		t.Fatal("no preview URL commented on the pull request")
	}
	waitWork(t, preview, 3*time.Minute)
	r.pass("pull request #%s: %s, its D1, KV, R2 and secret its own", n, preview)

	r.step("Closing the pull request: GitHub Actions deletes the preview")
	w.must(nil, "gh", "pr", "close", n)
	if !gone(preview, 6*time.Minute) {
		t.Fatalf("%s still answers after the pull request closed", preview)
	}
	r.pass("%s is gone", preview)

	r.step("mise run cloudflare actions --off, then the Worker and everything of it deleted")
	w.must(nil, "mise", "run", "cloudflare", "actions", "--off")
	r.deleteAll()
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("nothing left on the account")
	fmt.Println("\nALL PASSED")
}

// waitRun waits for the newest run of workflow on branch, for event, and fails unless it succeeded. It asks for the
// branch's runs and picks the workflow by its file: gh run list --workflow finds only workflows on the default branch.
func waitRun(t *testing.T, r repo, workflow, branch, event string) string {
	t.Helper()
	runs := "repos/{owner}/{repo}/actions/runs?branch=" + branch + "&event=" + event + "&per_page=20"
	pick := `[.workflow_runs[] | select(.path == "` + workflow + `")][0].id // empty`
	id := ""
	for end := time.Now().Add(3 * time.Minute); id == "" && time.Now().Before(end); time.Sleep(10 * time.Second) {
		id = strings.TrimSpace(r.must(nil, "gh", "api", runs, "--jq", pick))
	}
	if id == "" {
		t.Fatalf("no %s run of %s on %s", event, workflow, branch)
	}
	if _, err := r.run(nil, "gh", "run", "watch", id, "--exit-status", "--interval", "15"); err != nil {
		r.run(nil, "gh", "run", "view", id, "--log-failed")
		t.Fatalf("run %s of %s failed", id, workflow)
	}
	r.pass("GitHub Actions run %s (%s on %s) succeeded", id, event, branch)
	return id
}

func prNumber(branch string) string {
	out, _ := exec.Command("gh", "pr", "list", "--head", branch, "--state", "all", "--json", "number", "--jq", ".[0].number // empty").Output()
	return strings.TrimSpace(string(out))
}
