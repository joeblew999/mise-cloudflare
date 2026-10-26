package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// fixRepo makes the changes cloudflare check asks for that need no decision, so init leaves the repo passing them:
// .dev.vars, .env and .cloudflare/ git-ignored, wrangler pinned to the exact version installed, and a .dev.vars.example
// listing secrets.required when there's none. What needs a person (an ID in the wrangler config) check still names.
func fixRepo(ctx context.Context, dir string) {
	if exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil {
		var add []string
		for _, f := range []string{".dev.vars", ".env", ".cloudflare/"} {
			if exec.Command("git", "-C", dir, "-c", "core.excludesFile=", "check-ignore", "-q", f).Run() != nil {
				add = append(add, f)
			}
		}
		if len(add) > 0 {
			path := filepath.Join(dir, ".gitignore")
			old, _ := os.ReadFile(path)
			s := string(old)
			if s != "" && !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			s += "# The repo's Cloudflare secrets and login cache (mise-cloudflare)\n" + strings.Join(add, "\n") + "\n"
			must(os.WriteFile(path, []byte(s), 0o644))
			fmt.Println("git-ignored", strings.Join(add, ", "), "in .gitignore")
		}
	}
	pinWrangler(dir)
	if _, err := os.Stat(filepath.Join(dir, ".dev.vars.example")); os.IsNotExist(err) && (wrangler.W{Dir: dir}).Installed() {
		if wc, err := wconfig.Read(ctx, dir); err == nil && len(wc.Secrets) > 0 {
			var b strings.Builder
			b.WriteString("# The Worker's secrets.required, for wrangler dev: cloudflare dev copies this to .dev.vars (git-ignored), where you fill them in.\n")
			for _, s := range wc.Secrets {
				b.WriteString(s + "=\n")
			}
			writeFile(filepath.Join(dir, ".dev.vars.example"), []byte(b.String()))
		}
	}
}

var wranglerDep = regexp.MustCompile(`("wrangler"\s*:\s*")([^"]+)(")`)

// pinWrangler pins package.json's wrangler to the exact version installed (node_modules/wrangler), when it's a range.
func pinWrangler(dir string) {
	path := filepath.Join(dir, "package.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	m := wranglerDep.FindSubmatch(b)
	if m == nil || regexp.MustCompile(`^\d+\.\d+\.\d+$`).Match(m[2]) {
		return
	}
	var installed struct{ Version string }
	ib, err := os.ReadFile(filepath.Join(dir, "node_modules", "wrangler", "package.json"))
	if err != nil || json.Unmarshal(ib, &installed) != nil || installed.Version == "" {
		fmt.Printf("· package.json's wrangler is %s, not an exact version: install the dependencies, then run init again to pin the installed one\n", m[2])
		return
	}
	loc := wranglerDep.FindSubmatchIndex(b)
	out := append(append(append([]byte{}, b[:loc[4]]...), installed.Version...), b[loc[5]:]...)
	must(os.WriteFile(path, out, 0o644))
	fmt.Printf("pinned wrangler %s (was %s) in package.json: install the dependencies again to update the lockfile\n", installed.Version, m[2])
}

// devVars makes .dev.vars from .dev.vars.example for wrangler dev when there's none, and names the secrets it has no
// value for, which the Worker would run without.
func devVars(dir string, secrets []string) {
	path := filepath.Join(dir, ".dev.vars")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if b, err := os.ReadFile(filepath.Join(dir, ".dev.vars.example")); err == nil {
			must(os.WriteFile(path, b, 0o600))
			fmt.Println("made .dev.vars from .dev.vars.example (git-ignored): put the local values there")
		}
	}
	b, _ := os.ReadFile(path)
	var empty []string
	for _, s := range secrets {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(s) + `=\S`).Match(b) {
			empty = append(empty, s)
		}
	}
	if len(empty) > 0 {
		fmt.Printf("· no local value for %s: set it in .dev.vars, or the Worker runs without it\n", strings.Join(empty, ", "))
	}
}
