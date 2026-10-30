package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmoke is what a user does in a new repo, with no Cloudflare account: mise-cloudflare from mise (this
// checkout's build, or the release SMOKE_RELEASE as mise use installs it), init, then status, check and a
// local Worker. Every step goes through mise, as in the README.
func TestSmoke(t *testing.T) {
	if os.Getenv("SMOKE") != "1" {
		t.Skip("mise run smoke")
	}
	src, _ := filepath.Abs("../example")
	r := repo{t: t, dir: filepath.Join(t.TempDir(), "new-repo"), worker: "mise-cloudflare-smoke", toolCmd: []string{"mise", "exec", "--", "mise-cloudflare"}}
	os.MkdirAll(filepath.Join(r.dir, "src"), 0o755)
	for _, f := range []string{"src/worker.js", ".dev.vars.example"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(r.dir, f), b, 0o644)
	}
	b, _ := os.ReadFile(filepath.Join(src, "wrangler.jsonc"))
	os.WriteFile(filepath.Join(r.dir, "wrangler.jsonc"), bytes.ReplaceAll(b, []byte(`"mise-cloudflare-example"`), []byte(`"`+r.worker+`"`)), 0o644)
	// A range, as most repos have it: init pins it.
	os.WriteFile(filepath.Join(r.dir, "package.json"), []byte(`{ "name": "new-repo", "private": true, "devDependencies": { "wrangler": "^4.151.3" } }`+"\n"), 0o644)
	r.must(nil, "git", "init", "-q")
	if v := os.Getenv("SMOKE_RELEASE"); v != "" {
		r.step("mise-cloudflare %s from GitHub, as mise use installs it", v)
		r.must(nil, "mise", "use", "-q", "node@26.12.1", "github:joeblew999/mise-cloudflare@"+strings.TrimPrefix(v, "v"))
	} else {
		bin, _ := filepath.Abs("../bin")
		r.step("this checkout's mise-cloudflare, from %s", bin)
		os.WriteFile(filepath.Join(r.dir, "mise.toml"), []byte("[env]\n_.path = [\""+bin+"\"]\n\n[tools]\nnode = \"26.12.1\"\n"), 0o644)
	}
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	r.must(nil, "mise", "exec", "--", "npm", "install", "--silent")

	r.step("init, then the repo passes check, and the status screen says what is next")
	r.tool(nil, "init")
	r.must(nil, "mise", "exec", "--", "npm", "install", "--silent") // init pinned wrangler: the lockfile follows
	r.must(nil, "mise", "trust", "-q", "mise.toml")
	if out := r.must(nil, "mise", "run", "cloudflare", "check"); !strings.Contains(out, "rules: pass") {
		t.Fatalf("check: %s", out)
	}
	if out := r.must(nil, "mise", "run", "cloudflare"); !strings.Contains(out, "next: mise run cloudflare:secret (it asks for GREETING)") {
		t.Fatalf("status: %s", out)
	}
	r.pass("init fixed what check asks for, and status says to set the Worker's secret next")
	if out, err := r.run([]string{"CI=true"}, "mise", "run", "cloudflare", "login"); err == nil || !strings.Contains(out, "in CI there's no one to log in") {
		t.Fatalf("in CI, a login didn't stop at once: %s", out)
	}
	r.pass("in CI, a login stops at once: no one is there to click Allow")

	r.step("A local Worker with mise-cloudflare dev, with no Cloudflare login")
	r.toolCmd = []string{strings.TrimSpace(r.must(nil, "mise", "exec", "--", "which", "mise-cloudflare"))} // dev runs with a bare HOME
	if body := r.localWorker(); !allWork(body) {
		t.Fatalf("the local Worker: %s", body)
	}
	r.pass("the local Worker's D1, KV, R2 and secret work")
	fmt.Println("\nALL PASSED")
}
