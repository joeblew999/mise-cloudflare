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

// TestBuilds proves go tool cloudflare builds: Cloudflare's Workers Builds deploys a Worker from its GitHub repository
// with its own token, a push to the deploy branch going live and any other branch getting a preview URL. The Worker
// (testdata/builds) says which branch built it, so the proof sees Cloudflare's deploys, not its own. Its branches are
// throwaway ones, deleted with everything else afterwards; main is never touched.
func TestBuilds(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("testdata/builds")
	// The tool from this checkout: testdata/builds is in this module, which has no tool line for itself.
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-builds", tool: []string{"go", "run", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare"}}
	stamp := time.Now().Unix()
	prod, prev := fmt.Sprintf("cfz-builds-prod-%d", stamp), fmt.Sprintf("cfz-builds-preview-%d", stamp)

	r.step("The Worker, with KV and R2, deployed once by hand on the repo's own login")
	r.must(nil, "npm", "ci", "--no-audit", "--no-fund")
	r.cf(nil, "login")
	if out, err := r.try(nil, "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("%s is left over from another run: delete it first", strings.Join(left, ", "))
	}
	t.Cleanup(func() {
		r.step("Removing everything: the Workers Builds setup, the branches, the Worker, its KV and R2")
		r.try(nil, "-off", "builds")
		for _, b := range []string{prod, prev} {
			r.run(nil, "git", "push", "-q", "origin", "--delete", b)
		}
		r.deleteAll()
	})
	r.cf([]string{"WRANGLER_OUTPUT_FILE_PATH=.wrangler/output.json"}, "wrangler", "deploy")
	url := r.deployedURL()
	if !built(url, "here", time.Minute) {
		t.Fatalf("%s isn't working", url)
	}
	r.pass("%s: built here, KV and R2 work", url)

	r.step("go tool cloudflare builds: Workers Builds, deploying from %s", prod)
	r.cf(nil, "-branch", prod, "builds")
	r.pass("set up")

	r.step("A push to %s: Workers Builds builds it and deploys it, with Cloudflare's own token", prod)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prod)
	if !built(url, prod, 15*time.Minute) {
		r.buildsLog()
		t.Fatalf("%s wasn't deployed from %s in 15 minutes", url, prod)
	}
	r.pass("%s: built from %s by Workers Builds; KV and R2 work", url, prod)

	r.step("A push to %s: a preview at its own URL, the live Worker untouched", prev)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prev)
	// A branch's preview is at <alias>-<worker>.<subdomain>.workers.dev, the alias being the branch's name.
	preview := strings.Replace(url, "https://", "https://"+alias(prev)+"-", 1)
	if !built(preview, prev, 15*time.Minute) {
		r.buildsLog()
		t.Fatalf("%s wasn't built from %s in 15 minutes", preview, prev)
	}
	r.pass("%s: built from %s; KV and R2 work", preview, prev)
	if !built(url, prod, time.Minute) {
		t.Fatalf("the preview changed the live Worker at %s", url)
	}
	r.pass("%s: still built from %s", url, prod)

	r.step("go tool cloudflare builds -off removes the setup; then the Worker, its KV and R2 go")
	r.cf(nil, "-off", "builds")
	r.pass("removed")
	r.deleteAll()
	if left := r.leftovers(); len(left) > 0 {
		t.Fatalf("left on the account: %s", strings.Join(left, ", "))
	}
	r.pass("nothing left on the account")
	fmt.Println("\nALL PASSED")
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

// buildsLog prints the Worker's latest Workers Builds, to see why a deploy didn't come.
func (r repo) buildsLog() {
	tag := ""
	if b, err := os.ReadFile(filepath.Join(r.dir, ".wrangler", "output.json")); err == nil {
		if m := regexp.MustCompile(`"worker_tag":"([0-9a-f]+)"`).FindAllSubmatch(b, -1); len(m) > 0 {
			tag = string(m[len(m)-1][1])
		}
	}
	if tag != "" {
		r.run(nil, "cf", "builds", "list", tag, "--profile", "cloudflare-zero-example")
	}
}
