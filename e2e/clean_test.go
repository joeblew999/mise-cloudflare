package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestClean deletes whatever a stopped prove or builds left (mise run clean): Cloudflare's Workers Builds setup for the
// Worker, prove's throwaway e2e-* branches on GitHub (a pull request from or onto one closes with it), then the Worker,
// its D1, KV and R2 and its previews'. It ends as prove starts: nothing of the Worker on the account.
func TestClean(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("../example")
	r := repo{t: t, dir: dir, worker: "mise-cloudflare-example"}
	r.must(nil, "mise", "run", "setup")
	r.tool(nil, "login")
	// Only on cf's login if this machine has one: github --off would otherwise wait for someone to log cf in.
	if out, _ := r.run(nil, "cf", "auth", "whoami", "--profile", r.worker); strings.Contains(out, `"authenticated": true`) {
		r.try(nil, "github", "--off")
	}
	// Only builds pushes branches; with GitHub out of reach there's nothing of it to delete from here.
	top, _ := filepath.Abs("..") // this repo: example/ is a git repo of its own, with no origin
	heads, err := r.run(nil, "git", "-C", top, "ls-remote", "--heads", "origin")
	if err != nil {
		r.pass("GitHub isn't reachable: no e2e-* branches looked for")
	}
	for _, line := range strings.Split(heads, "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/heads/e2e-") {
			r.must(nil, "git", "-C", top, "push", "-q", "origin", "--delete", strings.TrimPrefix(f[1], "refs/heads/"))
			r.pass("deleted the branch %s", strings.TrimPrefix(f[1], "refs/heads/"))
		}
	}
	r.deleteAll()
	r.notDeployed()
}
