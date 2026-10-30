package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prove is the one real deploy of a Worker repo: local, deploy on its own login, live, stray tokens, deleting
// everything, and with clean the login logged out.
func prove(t *testing.T, r repo, clean bool) {
	r.step("1. A developer with no Cloudflare login at all: setup, the rules, a local Worker")
	r.must(nil, "mise", "run", "setup")
	r.must(nil, "mise", "run", "cloudflare", "check")
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker answered %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work, with no Cloudflare login")

	r.step("2. Deploy on the repo's own login, %s (logs in the first time only)", r.worker)
	r.tool(nil, "login")
	if out, err := r.try(nil, "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed on this account: another machine may be proving it now. Wait for it, or delete it (mise run clean)", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("%s is left over from another run: delete it first (mise run clean)", strings.Join(left, ", "))
	}
	r.pass("neither the Worker nor its D1, KV or R2 is on this account yet, so nothing else is using them")
	t.Cleanup(func() {
		if t.Failed() {
			r.step("Cleaning up after the failure")
			r.deleteAll()
		}
	})
	before := r.must(nil, "git", "status", "--porcelain", ".")
	// As a developer does it: what status says, each time.
	r.next("mise run cloudflare:secret")
	r.putSecret("secret", "put", "GREETING")
	r.next("mise run cloudflare:deploy")
	r.must(nil, "mise", "run", "cloudflare:deploy")
	s := r.status()
	if !s.LoggedIn || !s.Deployed {
		t.Fatalf("not deployed on the login %s: %+v", r.worker, s)
	}
	r.pass("deployed on the repo's own login, %s", s.Profile)

	r.step("3. The live Worker: D1, KV, R2 and the secret")
	url := r.deployedURL()
	if code, body := get(url); code != 200 || !allWork(body) {
		t.Fatalf("%s: HTTP %d %s", url, code, body)
	}
	r.pass("%s: D1, KV, R2 and the secret all work", url)
	if after := r.must(nil, "git", "status", "--porcelain", "."); after != before {
		t.Fatalf("the repo changed:\n%s", after)
	}
	r.pass("deploying wrote nothing into the repo: no IDs")
	r.next("mise run cloudflare:github")

	r.step("4. A stray CLOUDFLARE_API_TOKEN is refused, in the shell and in a .env")
	if out, err := r.try([]string{"CLOUDFLARE_API_TOKEN=dummy-not-a-real-token"}, "wrangler", "deploy"); err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in the shell")
	}
	r.pass("a token in the shell is refused")
	env := filepath.Join(r.dir, ".env")
	os.WriteFile(env, []byte("CLOUDFLARE_API_TOKEN=dummy-not-a-real-token\n"), 0o600)
	out, err := r.try(nil, "wrangler", "deploy")
	os.Remove(env)
	if err == nil || !strings.Contains(out, "refusing:") {
		t.Fatal("deployed with a token in .env")
	}
	r.pass("a token in .env is refused")

	r.step("5. wrangler, on the repo's login, deletes the Worker, its D1, KV and R2")
	r.deleteAll()
	if !gone(url, time.Minute) {
		t.Fatalf("%s still answers", url)
	}
	r.pass("%s no longer answers", url)
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("no D1, KV or R2 left")

	if clean {
		r.step("6. Logout removes the login")
		r.tool(nil, "logout")
		if r.status().LoggedIn {
			t.Fatal("the login is still there")
		}
		r.pass("the login is gone")
	}
	fmt.Println("\nALL PASSED")
}

// TestProve proves it all in example/.
func TestProve(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("../example")
	prove(t, repo{t: t, dir: dir, worker: "mise-cloudflare-example"}, os.Getenv("PROVE_CLEAN") == "true")
}
