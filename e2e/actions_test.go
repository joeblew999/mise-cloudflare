package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestActions proves the GitHub Actions deploy path (cloudflare.toml's [workflow] deploy): go tool cloudflare ci-token
// stores CI's token as this repository's GitHub secrets; on a throwaway branch carrying the stamped workflow, GitHub
// Actions deploys testdata/actions, a Worker nothing else ever deploys; a pull request gets a preview pr-<number>, its
// URL commented by the workflow; closing it deletes the preview. Then the branches, the Worker, the token and the
// secrets go. main is never touched.
func TestActions(t *testing.T) {
	needAccount(t)
	src, _ := filepath.Abs("testdata/actions")
	bin := buildTool(t)
	r := repo{t: t, dir: src, worker: "cloudflare-zero-actions", tool: []string{bin}}
	stamp := time.Now().Unix()
	prod, pr := fmt.Sprintf("cfz-actions-prod-%d", stamp), fmt.Sprintf("cfz-actions-pr-%d", stamp)
	const workflow = "cloudflare-e2e-testdata-actions.yml"

	r.step("The Worker isn't deployed, and CI's token goes into this repository's GitHub secrets (cloudflare ci-token)")
	r.must(nil, "npm", "ci", "--no-audit", "--no-fund")
	if out, err := r.try(nil, "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	// CI's token, for the proof: the repo login's own OAuth token, good for about an hour, which is long enough; a repo
	// stores one made in Cloudflare's dashboard instead. Its value is never printed.
	var tok struct{ Token string }
	json.Unmarshal([]byte(jsonPart(r.wrangler("auth", "token", "--json"))), &tok)
	if tok.Token == "" {
		t.Fatal("no token from the repo's login")
	}
	r.cf([]string{"CLOUDFLARE_CI_TOKEN=" + tok.Token}, "ci-token")
	r.pass("the secrets are set")

	// A worktree of this commit, on the throwaway branch, its workflow deploying from that branch.
	wt := filepath.Join(t.TempDir(), "wt")
	r.must(nil, "git", "worktree", "add", "-q", "-b", prod, wt, "HEAD")
	t.Cleanup(func() {
		r.step("Removing everything: the pull request's preview, the branches, the Worker, CI's token and its secrets")
		r.run(nil, "gh", "pr", "close", pr, "--delete-branch")
		for _, b := range []string{prod, pr} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		r.try(nil, "wrangler", "preview", "delete", "--name", "pr-"+prNumber(r, pr), "--skip-confirmation")
		r.try(nil, "wrangler", "delete", "--force")
		r.try(nil, "-off", "ci-token")
		r.run(nil, "git", "worktree", "remove", "--force", wt)
		r.run(nil, "git", "branch", "-D", prod, pr)
	})
	w := repo{t: t, dir: filepath.Join(wt, "e2e", "testdata", "actions"), worker: r.worker, tool: []string{bin}}
	toml := filepath.Join(w.dir, "cloudflare.toml")
	b, _ := os.ReadFile(toml)
	os.WriteFile(toml, []byte(strings.Replace(string(b), `branch = "main"`, `branch = "`+prod+`"`, 1)), 0o644)
	w.cf(nil, "workflow")
	w.must(nil, "git", "add", "-A")
	w.must(nil, "git", "commit", "-qm", "prove-actions: deploy "+r.worker+" from "+prod)

	r.step("A push to %s: GitHub Actions checks it and deploys %s", prod, r.worker)
	w.must(nil, "git", "push", "-q", "origin", prod)
	run := waitRun(t, w, workflow, prod, "push")
	log := w.must(nil, "gh", "run", "view", run, "--log")
	url := regexp.MustCompile(`https://` + r.worker + `\.[a-z0-9-]+\.workers\.dev`).FindString(log)
	if url == "" {
		t.Fatal("the deploy's log has no workers.dev URL")
	}
	if !answers(url, 2*time.Minute) {
		t.Fatalf("%s doesn't answer", url)
	}
	r.pass("%s answers, deployed by GitHub Actions (run %s)", url, run)

	r.step("A pull request: GitHub Actions deploys the preview pr-<number> and comments its URL")
	w.must(nil, "git", "checkout", "-q", "-b", pr)
	os.WriteFile(filepath.Join(w.dir, "worker.js"), []byte(strings.Replace(mustRead(t, filepath.Join(w.dir, "worker.js")),
		`worker: "cloudflare-zero-actions"`, `worker: "cloudflare-zero-actions", pr: true`, 1)), 0o644)
	w.must(nil, "git", "commit", "-qam", "prove-actions: a change for a pull request")
	w.must(nil, "git", "push", "-q", "origin", pr)
	w.must(nil, "gh", "pr", "create", "--base", prod, "--head", pr, "--title", "prove-actions: a preview", "--body", "A throwaway pull request from mise run prove-actions; it closes itself.")
	n := prNumber(w, pr)
	waitRun(t, w, workflow, pr, "pull_request")
	preview := ""
	for end := time.Now().Add(2 * time.Minute); preview == "" && time.Now().Before(end); time.Sleep(10 * time.Second) {
		comments := w.must(nil, "gh", "pr", "view", n, "--json", "comments", "--jq", ".comments[].body")
		preview = regexp.MustCompile(`https://[a-z0-9.-]+\.workers\.dev`).FindString(comments)
	}
	if preview == "" {
		t.Fatal("no preview URL commented on the pull request")
	}
	if !answersWith(preview, `"pr":true`, 2*time.Minute) {
		t.Fatalf("%s doesn't answer with the pull request's change", preview)
	}
	r.pass("pull request #%s: preview pr-%s at %s, with its change", n, n, preview)

	r.step("Closing the pull request: GitHub Actions deletes the preview")
	w.must(nil, "gh", "pr", "close", n)
	waitRun(t, w, workflow, pr, "pull_request")
	if answersWith(preview, `"pr":true`, 2*time.Minute) && !goneWith(preview, `"pr":true`, 5*time.Minute) {
		t.Fatalf("%s still answers after the pull request closed", preview)
	}
	r.pass("%s is gone", preview)
	fmt.Println("\nALL PASSED")
}

// waitRun waits for the newest run of workflow on branch, for event, to finish, and fails unless it succeeded.
func waitRun(t *testing.T, r repo, workflow, branch, event string) string {
	t.Helper()
	var id string
	for end := time.Now().Add(3 * time.Minute); id == "" && time.Now().Before(end); time.Sleep(10 * time.Second) {
		id = strings.TrimSpace(r.must(nil, "gh", "run", "list", "--workflow", workflow, "--branch", branch, "--event", event,
			"--limit", "1", "--json", "databaseId", "--jq", ".[0].databaseId // empty"))
	}
	if id == "" {
		t.Fatalf("no %s run of %s on %s", event, workflow, branch)
	}
	if _, err := r.run(nil, "gh", "run", "watch", id, "--exit-status", "--interval", "15"); err != nil {
		r.run(nil, "gh", "run", "view", id, "--log-failed")
		t.Fatalf("run %s of %s failed", id, workflow)
	}
	return id
}

func prNumber(r repo, branch string) string {
	out, _ := exec.Command("gh", "pr", "list", "--head", branch, "--state", "all", "--json", "number", "--jq", ".[0].number // empty").Output()
	return strings.TrimSpace(string(out))
}

func mustRead(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// answers fetches url until it answers 200 with JSON, or time runs out.
func answers(url string, limit time.Duration) bool { return answersWith(url, "", limit) }

// answersWith fetches url until it answers 200 with a body containing want, or time runs out.
func answersWith(url, want string, limit time.Duration) bool {
	for end := time.Now().Add(limit); ; time.Sleep(10 * time.Second) {
		if code, body := fetch(url); code == 200 && json.Valid(body) && strings.Contains(string(body), want) {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

// goneWith waits until url no longer answers with want.
func goneWith(url, want string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(15 * time.Second) {
		if code, body := fetch(url); code != 200 || !strings.Contains(string(body), want) {
			return true
		}
	}
	return false
}

// fetch is one GET of url.
func fetch(url string) (int, []byte) {
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}
