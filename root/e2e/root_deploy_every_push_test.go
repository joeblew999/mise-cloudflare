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

// TestRootDeployEveryPush is Cloudflare's Workers Builds deploying every push of this repo (mise run
// root:deploy-every-push): the root's steps on the reference, as a person takes them, what 05-show says each time (the
// secret, a deploy, 09-connect-github), then a push to a throwaway branch built and deployed as production by
// Cloudflare, a pull request's branch with a preview of its own D1, KV, R2 and secret, and the preview gone with its
// branch. Then 09-connect-github --off, and 13-delete. main is never touched.
//
// It runs in a checkout of this repo at HEAD, which is what Cloudflare builds from GitHub: there the reference is a
// folder in this repo's own git, as on GitHub, and init's files are committed in it. So it runs 01 and 02, then from
// 12 on: 00-git-init would make it a repo of its own, and 03-init and 04-check would put its workflow and hook in this
// repo's own .github and .claude, which this repo keeps out (root:check).
func TestRootDeployEveryPush(t *testing.T) {
	needAccount(t)
	top, _ := filepath.Abs("../..")
	checkout := filepath.Join(t.TempDir(), "checkout")
	if out, err := exec.Command("git", "-C", top, "worktree", "add", "-q", "--detach", checkout, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	t.Cleanup(func() { exec.Command("git", "-C", top, "worktree", "remove", "--force", checkout).Run() })
	r := repo{t: t, top: checkout, name: "reference", dir: filepath.Join(checkout, "examples", "reference")}
	stamp := time.Now().Unix()
	prod, prev := fmt.Sprintf("e2e-prod-%d", stamp), fmt.Sprintf("e2e-preview-%d", stamp)

	r.heading("The Worker deployed as a developer does: what 05-show says, each time")
	r.mustRootStep(nil, "01-mise-install")
	r.mustRootStep(nil, "02-setup")
	r.readWorker()
	r.mustRootStep(nil, "12-login")
	r.cleanCloudflare()
	t.Cleanup(func() {
		r.heading("Removing everything: Cloudflare's setup, the branches, the Worker, its D1, KV and R2, the previews'")
		r.rootStep(nil, "", "09-connect-github", "--off")
		for _, b := range []string{prod, prev} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		r.rootStep(nil, "", "13-delete")
	})
	r.next("mise run cloudflare:07-set-secrets")
	if _, err := r.rootStep(nil, "set-by-deploy\n", "07-set-secrets"); err != nil {
		t.Fatalf("example:07-set-secrets: %v", err)
	}
	r.next("mise run cloudflare:08-deploy")
	r.mustRootStep(nil, "08-deploy")
	url := r.show().URL
	waitWork(t, url, 2*time.Minute)
	r.pass("%s: D1, KV, R2 and the secret work", url)
	r.next("mise run cloudflare:09-connect-github")

	r.heading("example:09-connect-github --branch %s: Cloudflare deploys every push, production from %s", prod, prod)
	r.mustRootStep(nil, "09-connect-github", "--branch", prod)
	r.next("mise run cloudflare:10-set-preview-secrets")
	if _, err := r.rootStep(nil, "set-by-deploy\n", "10-set-preview-secrets"); err != nil {
		t.Fatalf("example:10-set-preview-secrets: %v", err)
	}
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

	r.heading("example:09-connect-github --off, then example:13-delete: the Worker and everything of it")
	r.mustRootStep(nil, "09-connect-github", "--off")
	r.mustRootStep(nil, "13-delete")
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("nothing left on the account")
	fmt.Println("\nALL PASSED")
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
