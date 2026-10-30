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

// TestDeployEveryPush proves mise run cloudflare:09-connect-github on the root (mise run root:deploy-every-push), as a
// person comes to it: what cloudflare:05-show says, each time (the secret, a deploy), then Cloudflare deploying every push. A push to a throwaway branch is built and deployed as production by Cloudflare;
// a pull request's branch gets a preview, with its own D1, KV, R2 and secret; deleting the branch deletes the preview.
// Then --off, and everything deleted. main is never touched.
// It runs in a checkout of this repo at HEAD, what Cloudflare builds from GitHub: the reference is a subfolder of it
// there, not the repo of its own example:00-git-init makes it here, so it runs the reference's own tasks, not the
// root's steps.
func TestDeployEveryPush(t *testing.T) {
	needAccount(t)
	top, _ := filepath.Abs("../..")
	checkout := filepath.Join(t.TempDir(), "checkout")
	if out, err := exec.Command("git", "-C", top, "worktree", "add", "-q", "--detach", checkout, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	t.Cleanup(func() { exec.Command("git", "-C", top, "worktree", "remove", "--force", checkout).Run() })
	r := repo{t: t, dir: filepath.Join(checkout, "examples", "reference"), worker: "mise-cloudflare-example"}
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	stamp := time.Now().Unix()
	prod, prev := fmt.Sprintf("e2e-prod-%d", stamp), fmt.Sprintf("e2e-preview-%d", stamp)

	r.heading("The Worker deployed as a developer does: what 05-show says, each time")
	r.must(nil, "mise", "run", "setup")
	r.mustTask("", "12-login")
	r.notDeployed()
	t.Cleanup(func() {
		r.heading("Removing everything: Cloudflare's setup, the branches, the Worker, its D1, KV and R2, the previews'")
		r.run(nil, "mise", "run", "cloudflare:09-connect-github", "--off")
		for _, b := range []string{prod, prev} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		r.deleteAll()
	})
	r.next("mise run cloudflare:07-set-secrets")
	r.mustTask("set-by-deploy\n", "07-set-secrets")
	r.next("mise run cloudflare:08-deploy")
	r.must(nil, "mise", "run", "cloudflare:08-deploy")
	url := r.show().URL
	waitWork(t, url, 2*time.Minute)
	r.pass("%s: D1, KV, R2 and the secret work", url)
	r.next("mise run cloudflare:09-connect-github")

	r.heading("mise run cloudflare:09-connect-github --branch %s: Cloudflare deploys every push, production from %s", prod, prod)
	r.must(nil, "mise", "run", "cloudflare:09-connect-github", "--branch", prod)
	r.next("mise run cloudflare:10-set-preview-secrets")
	r.mustTask("set-by-deploy\n", "10-set-preview-secrets")
	r.next("push a branch")

	r.heading("A push to %s: Cloudflare builds and deploys production", prod)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prod)
	if outcome := r.built(prod, 15*time.Minute); outcome != "success" {
		t.Fatalf("Cloudflare's build of %s: %s", prod, outcome)
	}
	waitWork(t, url, 2*time.Minute)
	r.pass("Cloudflare built and deployed production from %s; it works", prod)

	r.heading("A pull request's branch %s: its preview, with its own D1, KV, R2 and secret", prev)
	wt := filepath.Join(t.TempDir(), "wt")
	r.must(nil, "git", "worktree", "add", "-q", "--detach", wt, "HEAD")
	t.Cleanup(func() { r.run(nil, "git", "worktree", "remove", "--force", wt) })
	os.WriteFile(filepath.Join(wt, "examples", "reference", "preview.txt"), []byte("a pull request's change\n"), 0o644)
	r.must(nil, "git", "-C", wt, "add", "-A")
	r.must(nil, "git", "-C", wt, "commit", "-qm", "prove: a pull request's change")
	r.must(nil, "git", "-C", wt, "push", "-q", "origin", "HEAD:refs/heads/"+prev)
	preview := strings.Replace(url, "https://", "https://"+alias(prev)+"-", 1)
	waitWork(t, preview, 15*time.Minute)
	r.pass("%s: built by Cloudflare from %s; its D1, KV, R2 and secret work", preview, prev)

	r.heading("Deleting %s, as merging a pull request does: its preview goes with it", prev)
	r.must(nil, "git", "push", "-q", "origin", "--delete", prev)
	if !gone(preview, 10*time.Minute) {
		t.Fatalf("%s still answers, 10 minutes after its branch went", preview)
	}
	r.pass("%s is gone", preview)

	r.heading("mise run cloudflare:09-connect-github --off, then the Worker and everything of it deleted")
	r.must(nil, "mise", "run", "cloudflare:09-connect-github", "--off")
	r.deleteAll()
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("nothing left on the account")
	fmt.Println("\nALL PASSED")
}

// notDeployed fails unless the Worker and its D1, KV and R2 (and its previews') aren't on the account: another run may
// be using them.
func (r repo) notDeployed() {
	r.t.Helper()
	if out, err := r.wranglerTry("deployments", "list", "--json"); err == nil {
		r.t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it (mise run example:clean)", r.worker)
	} else if !strings.Contains(out, "10007") {
		r.t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	if left := r.leftovers(); len(left) > 0 {
		r.t.Fatalf("%s is left over from another run: delete it first (mise run example:clean)", strings.Join(left, ", "))
	}
	r.pass("nothing of %s is on the account yet", r.worker)
}

// mustTask runs one of the repo's own tasks, with input typed into it, and stops at its failure.
func (r repo) mustTask(input, task string, args ...string) string {
	r.t.Helper()
	out, err := r.task(input, task, args...)
	if err != nil {
		r.t.Fatalf("cloudflare:%s %s: %v", task, strings.Join(args, " "), err)
	}
	return out
}

// built waits for Cloudflare's own record of a Workers Builds build of the Worker from branch, and returns its outcome
// (success, failure), or what it waited for.
func (r repo) built(branch string, limit time.Duration) string {
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

// waitWork waits until url answers with the example Worker's D1, KV, R2 and secret all working.
func waitWork(t *testing.T, url string, limit time.Duration) {
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

// alias is a branch name as Cloudflare names its preview: lowercase letters, digits and dashes.
func alias(branch string) string {
	return strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(branch), "-"), "-")
}
