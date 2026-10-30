package e2e

import (
	"strings"
	"testing"
)

// TestClean deletes whatever a stopped example:deploy or root:deploy-every-push left of the Worker of the repo EXAMPLE
// names (mise run example:clean): Cloudflare's Workers Builds setup for it (example:09-connect-github --off),
// deploy-every-push's throwaway e2e-* branches on the root's GitHub (a pull request from or onto one closes with it),
// then the Worker, its D1, KV and R2 and its previews'. It ends as example:deploy starts: nothing of the Worker on the
// account.
func TestClean(t *testing.T) {
	needAccount(t)
	r := example(t)
	r.mustStep(nil, "12-login")
	// Only on cf's login if this machine has one: connect-github --off would otherwise wait for someone to log cf in.
	if out, _ := r.run(nil, "cf", "auth", "whoami", "--profile", r.worker); strings.Contains(out, `"authenticated": true`) {
		r.runStep(nil, "", "09-connect-github", "--off")
	}
	// Only deploy-every-push pushes branches; with GitHub out of reach there's nothing of it to delete from here.
	heads, err := r.runIn(r.top, nil, "", "git", "ls-remote", "--heads", "origin")
	if err != nil {
		r.pass("GitHub isn't reachable: no e2e-* branches looked for")
	}
	for _, line := range strings.Split(heads, "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/e2e-") {
			if _, err := r.runIn(r.top, nil, "", "git", "push", "-q", "origin", "--delete", strings.TrimPrefix(f[1], "refs/heads/")); err != nil {
				t.Fatalf("deleting the branch %s: %v", f[1], err)
			}
			r.pass("deleted the branch %s", strings.TrimPrefix(f[1], "refs/heads/"))
		}
	}
	r.deleteAll()
	r.notDeployed()
}
