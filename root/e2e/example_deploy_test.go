package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDeploy is a repo's steps on Cloudflare, as a person takes them from the root (mise run example:deploy), on the
// repo EXAMPLE names: the Worker locally with no login, example:12-login, the secret (example:07-set-secrets),
// example:08-deploy, the live Worker, stray tokens refused, example:13-delete, and with DEPLOY_CLEAN=true
// example:14-logout. Everything it made is deleted, even when it fails.
func TestExampleDeploy(t *testing.T) {
	needAccount(t)
	r := example(t)

	r.heading("1. A person with no Cloudflare login at all: the Worker locally (example:06-serve)")
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker answered %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work, with no Cloudflare login")

	r.heading("2. Deploy on the repo's own login, %s (example:12-login logs in the first time only)", r.worker)
	r.mustRootStep(nil, "12-login")
	r.cleanCloudflare()
	t.Cleanup(func() {
		if t.Failed() {
			r.heading("Cleaning up after the failure")
			r.rootStep(nil, "", "13-delete")
		}
	})
	before := r.must(nil, "git", "status", "--porcelain", ".")
	// As a person does it: what example:05-show says, each time.
	r.next("mise run cloudflare:07-set-secrets")
	if _, err := r.rootStep(nil, "set-by-deploy\n", "07-set-secrets"); err != nil {
		t.Fatalf("example:07-set-secrets: %v", err)
	}
	r.pass("example:07-set-secrets set GREETING")
	r.next("mise run cloudflare:08-deploy")
	r.mustRootStep(nil, "08-deploy")
	s := r.show()
	if !s.LoggedIn || !s.Deployed {
		t.Fatalf("not deployed on the login %s: %+v", r.worker, s)
	}
	r.pass("example:08-deploy deployed it on the repo's own login, %s", s.Profile)

	r.heading("3. The live Worker: D1, KV, R2 and the secret")
	url := r.deployedURL()
	if code, body := get(url); code != 200 || !allWork(body) {
		t.Fatalf("%s: HTTP %d %s", url, code, body)
	}
	r.pass("%s: D1, KV, R2 and the secret all work", url)
	if after := r.must(nil, "git", "status", "--porcelain", "."); after != before {
		t.Fatalf("the repo changed:\n%s", after)
	}
	r.pass("deploying wrote nothing into the repo: no IDs")
	r.next("mise run cloudflare:09-connect-github")

	r.heading("4. A stray CLOUDFLARE_API_TOKEN is refused by example:08-deploy, in the shell and in a .env")
	if out, err := r.rootStep([]string{"CLOUDFLARE_API_TOKEN=dummy-not-a-real-token"}, "", "08-deploy"); err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in the shell")
	}
	r.pass("a token in the shell is refused")
	env := filepath.Join(r.dir, ".env")
	os.WriteFile(env, []byte("CLOUDFLARE_API_TOKEN=dummy-not-a-real-token\n"), 0o600)
	out, err := r.rootStep(nil, "", "08-deploy")
	os.Remove(env)
	if err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in .env")
	}
	r.pass("a token in .env is refused")

	r.heading("5. example:13-delete: the Worker, its D1, KV and R2 and its previews' own")
	r.mustRootStep(nil, "13-delete")
	if !gone(url, time.Minute) {
		t.Fatalf("%s still answers", url)
	}
	r.pass("%s no longer answers", url)
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("no D1, KV or R2 left")

	if os.Getenv("DEPLOY_CLEAN") == "true" {
		r.heading("6. example:14-logout removes the login")
		r.mustRootStep(nil, "14-logout")
		if r.show().LoggedIn {
			t.Fatal("the login is still there")
		}
		r.pass("the login is gone")
	}

	r.heading("example:15-clean: the local state undone, .wrangler/ and .dev.vars")
	r.mustRootStep(nil, "15-clean")
	fmt.Println("\nALL PASSED")
}
