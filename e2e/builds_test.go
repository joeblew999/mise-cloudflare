package e2e

import (
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
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-builds"}
	// The tool from this checkout: testdata/builds is in this module, which has no tool line for itself.
	tool := func(env []string, args ...string) string {
		t.Helper()
		return r.must(env, "go", append([]string{"run", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare"}, args...)...)
	}
	stamp := time.Now().Unix()
	prod, prev := fmt.Sprintf("cfz-builds-prod-%d", stamp), fmt.Sprintf("cfz-builds-preview-%d", stamp)

	r.step("The Worker, deployed once by hand on the repo's own login")
	r.must(nil, "npm", "install", "--no-audit", "--no-fund")
	tool(nil, "login")
	if out, err := r.run(nil, "go", "run", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare", "wrangler", "deployments", "list", "--json"); err == nil {
		t.Fatalf("%s is already deployed: another run may be using it. Wait for it, or delete it", r.worker)
	} else if !strings.Contains(out, "10007") {
		t.Fatalf("couldn't tell whether %s is deployed", r.worker)
	}
	t.Cleanup(func() {
		r.step("Removing everything: the Workers Builds setup, the branches, the Worker")
		r.run(nil, "go", "run", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare", "-off", "builds")
		r.run(nil, "git", "push", "-q", "origin", "--delete", prod, prev)
		r.run(nil, "go", "run", "github.com/joeblew999/cloudflare-zero/cmd/cloudflare", "wrangler", "delete", "--force")
	})
	tool([]string{"WRANGLER_OUTPUT_FILE_PATH=.wrangler/output.json"}, "wrangler", "deploy")
	url := r.deployedURL()
	if got := waitText(url, "built from here", time.Minute); got != "built from here" {
		t.Fatalf("%s answered %q", url, got)
	}
	r.pass("%s: built from here", url)

	r.step("go tool cloudflare builds: Workers Builds, deploying from %s (the first time, a cf login approved on any device)", prod)
	tool(nil, "-branch", prod, "builds")
	r.pass("set up")

	r.step("A push to %s: Workers Builds builds it and deploys it, with Cloudflare's own token", prod)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prod)
	if got := waitText(url, "built from "+prod, 15*time.Minute); got != "built from "+prod {
		r.buildsLog()
		t.Fatalf("%s still answers %q after 15 minutes", url, got)
	}
	r.pass("%s: built from %s, deployed by Workers Builds", url, prod)

	r.step("A push to %s: a preview at its own URL, the live Worker untouched", prev)
	r.must(nil, "git", "push", "-q", "origin", "HEAD:refs/heads/"+prev)
	// A preview's URL is <alias>-<worker>.<subdomain>.workers.dev, the alias being the branch's name.
	preview := strings.Replace(url, "https://", "https://"+alias(prev)+"-", 1)
	if got := waitText(preview, "built from "+prev, 15*time.Minute); got != "built from "+prev {
		r.buildsLog()
		t.Fatalf("%s answers %q after 15 minutes", preview, got)
	}
	r.pass("%s: built from %s", preview, prev)
	if got := waitText(url, "built from "+prod, time.Minute); got != "built from "+prod {
		t.Fatalf("the preview changed the live Worker: %s answers %q", url, got)
	}
	r.pass("%s still answers built from %s", url, prod)

	r.step("go tool cloudflare builds -off removes the setup")
	tool(nil, "-off", "builds")
	r.pass("removed")
	fmt.Println("\nALL PASSED")
}

// waitText fetches url until its body is want or time runs out, and returns the last body.
func waitText(url, want string, limit time.Duration) string {
	last := ""
	for end := time.Now().Add(limit); ; {
		if resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if body := strings.TrimSpace(string(b)); body != last {
				last = body
				fmt.Printf("  %s: %.80q\n", url, body)
			}
			if last == want {
				return last
			}
		}
		if time.Now().After(end) {
			return last
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
