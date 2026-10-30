package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestTry is a repo's steps that need no Cloudflare account, as a person takes them from the root (mise run
// example:try), on the repo EXAMPLE names: mise run example:create first for a new one, which is how a release is tried
// too (example:create --version). example:04-check's rules pass, example:05-show says what to do next,
// example:12-login stops at once in CI (no one is there to click Allow), and example:06-serve runs the Worker locally,
// with its D1, KV, R2 and secret.
func TestTry(t *testing.T) {
	if os.Getenv("TRY") != "1" {
		t.Skip("mise run example:try")
	}
	r := example(t)
	r.next("mise run cloudflare:07-set-secrets")

	r.heading("example:12-login in CI stops at once: no one is there to click Allow")
	if out, err := r.runStep([]string{"CI=true"}, "", "12-login"); err == nil || !strings.Contains(out, "in CI there's no one to log in") {
		t.Fatalf("in CI, a login didn't stop at once: %s", out)
	}
	r.pass("in CI, example:12-login stops at once")

	r.heading("example:06-serve: the Worker locally, with no Cloudflare login")
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker: %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work")
	fmt.Println("\nALL PASSED")
}
