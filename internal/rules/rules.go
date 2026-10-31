// Package rules is mise-cloudflare check: what a repo must be so that its Cloudflare login and its secrets stay out
// of it. Each problem is one line naming the file.
package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
)

var (
	token    = regexp.MustCompile(`CLOUDFLARE_API_TOKEN\s*[=:]\s*["']?[A-Za-z0-9_-]{30,}`)
	login    = regexp.MustCompile(`\bwrangler\s+login\b`)
	varName  = regexp.MustCompile(`(?m)^([A-Z_][A-Z0-9_]*)=`)
	exactPin = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Check returns the repo's problems, one line each.
func Check(dir string, w wconfig.Config) ([]string, error) {
	problems := Previews(w, false)
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	// wrangler's own validation of the config: what it warned about reading it (an unexpected field, …).
	if w.Warnings != "" {
		bad("wrangler warns about %s:\n%s", filepath.Base(w.Path), w.Warnings)
	}

	files, err := gitFiles(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			continue // gone, a directory, or binary
		}
		if token.Match(b) {
			bad("%s holds a Cloudflare token", f)
		}
		if !strings.HasSuffix(f, ".md") && login.Match(b) {
			bad("%s calls the machine-wide wrangler login", f)
		}
	}
	// Only the repo's own ignore files count: another developer doesn't have this machine's global gitignore.
	// .cloudflare/ is where Cloudflare's cf CLI caches the account it logged into (its ID and name).
	for _, f := range []string{".dev.vars", ".env", ".cloudflare/"} {
		if exec.Command("git", "-C", dir, "-c", "core.excludesFile=", "check-ignore", "-q", f).Run() != nil {
			bad("%s isn't git-ignored", f)
		}
	}

	want := slices.Sorted(slices.Values(w.Secrets))
	var have []string
	if b, err := os.ReadFile(filepath.Join(dir, ".dev.vars.example")); err == nil {
		for _, m := range varName.FindAllSubmatch(b, -1) {
			have = append(have, string(m[1]))
		}
	}
	slices.Sort(have)
	if !slices.Equal(want, have) {
		bad(".dev.vars.example lists %v, secrets.required lists %v", have, want)
	}

	var pkg struct{ Dependencies, DevDependencies map[string]string }
	pin := "missing"
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && json.Unmarshal(b, &pkg) == nil {
		pin = cmpOr(pkg.DevDependencies["wrangler"], pkg.Dependencies["wrangler"], "missing")
	}
	if !exactPin.MatchString(pin) {
		bad("wrangler isn't pinned to an exact version in package.json (%s)", pin)
	}
	return problems, nil
}

// Previews returns the problems with the previews block (none without one, unless need): every binding but KV, D1 and
// R2 must be in it, and a KV, D1 or R2 in it must name an existing resource.
func Previews(w wconfig.Config, need bool) []string {
	if !w.Previews {
		if need {
			return []string{fmt.Sprintf("add \"previews\": {} to %s: a preview needs it (its KV, D1 and R2 it gets of its own)", filepath.Base(w.Path))}
		}
		return nil
	}
	var problems []string
	if missing := previewBindings(w); len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("%s's previews block lacks the bindings %s: a preview inherits none from production (only KV, D1 and R2 come from its Previews Base), so it would fail on its first use of one; add them there, pointed at preview-safe resources (vars with preview values)", filepath.Base(w.Path), strings.Join(missing, ", ")))
	}
	if unnamed := unnamedResources(w); len(unnamed) > 0 {
		problems = append(problems, fmt.Sprintf("%s's previews block has %s with no resource named, which wrangler preview sends as it is and Cloudflare refuses: leave it out, and mise run cloudflare:08-deploy gives previews their own, or name an existing one (a KV namespace's id, a D1's database_id, an R2 bucket's bucket_name)", filepath.Base(w.Path), strings.Join(unnamed, ", ")))
	}
	return problems
}

// unnamedResources names the previews block's KV, D1 and R2 (wconfig.PreviewsOwn) that don't name the resource they use:
// wrangler deploy makes those for production, wrangler preview makes none.
func unnamedResources(w wconfig.Config) []string {
	var out []string
	for _, n := range slices.Sorted(maps.Keys(w.PreviewBindings)) {
		if kind, own := wconfig.PreviewsOwn[w.PreviewBindings[n]]; own && !slices.Contains(w.PreviewNamed, n) {
			out = append(out, kind+" "+n)
		}
	}
	return out
}

// previewBindings names the bindings the previews block lacks, but for KV, D1, R2 and static assets.
func previewBindings(w wconfig.Config) []string {
	var missing []string
	for name, typ := range w.Bindings {
		_, own := wconfig.PreviewsOwn[typ]
		if _, ok := w.PreviewBindings[name]; !ok && !own && typ != "assets" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

// gitFiles is the repo's files under dir, tracked or not, without the git-ignored ones.
func gitFiles(dir string) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", ".").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w (the rules need a git repo)", dir, err)
	}
	var files []string
	for _, f := range strings.Fields(string(out)) {
		if !strings.HasPrefix(f, "node_modules/") && !strings.Contains(f, "/node_modules/") {
			files = append(files, f) // installed packages are not the repo's, git-ignored or not
		}
	}
	return files, nil
}

func cmpOr(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
