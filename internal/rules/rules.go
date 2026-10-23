// Package rules is go tool cloudflare check: what a repo must be so that its Cloudflare login and its secrets stay out
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

// Check returns the repo at dir's problems: IDs in its wrangler config (any environment), its previews block's (see
// Previews; previews says the repo runs Worker Previews, so it needs one), tokens or the machine-wide wrangler login in
// its files, .dev.vars or .env not git-ignored, .dev.vars.example not matching secrets.required, wrangler not pinned to
// an exact version.
func Check(dir string, w wconfig.Config, previews bool) ([]string, error) {
	problems := Previews(w, previews)
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	for _, p := range IDs(w.Raw) {
		bad("%s holds %s: the login picks the account, and deploy finds or creates resources by name", filepath.Base(w.Path), p)
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
	for _, f := range []string{".dev.vars", ".env"} {
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

// IDs names every account or resource ID written into a raw wrangler config, at its top level and in each of its
// environments (env.<name>): account_id, a D1's database_id or preview_database_id, a KV namespace's id or preview_id.
func IDs(raw map[string]any) []string {
	var out []string
	section := func(where string, s map[string]any) {
		if v, _ := s["account_id"].(string); v != "" {
			out = append(out, where+"an account ID (account_id)")
		}
		for _, d := range list(s["d1_databases"]) {
			if d["database_id"] != nil || d["preview_database_id"] != nil {
				out = append(out, fmt.Sprintf("%sa resource ID (D1 %v)", where, d["binding"]))
			}
		}
		for _, k := range list(s["kv_namespaces"]) {
			if k["id"] != nil || k["preview_id"] != nil {
				out = append(out, fmt.Sprintf("%sa resource ID (KV %v)", where, k["binding"]))
			}
		}
	}
	section("", raw)
	envs, _ := raw["env"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(envs)) {
		if e, ok := envs[name].(map[string]any); ok {
			section("env."+name+": ", e)
		}
	}
	return out
}

// Previews returns the problems with the Worker's previews block, which Worker Previews (wrangler preview) run on; with
// no block, none unless need. Previews inherit nothing from production, and Cloudflare makes them no KV, D1 or R2 of
// their own (only Durable Objects and Containers). A KV, D1 or R2 binding left out of the block comes from the Worker's
// Previews Base, where go tool cloudflare previews puts the previews' own; one in it names an existing resource, since
// wrangler preview sends it as it is. Every other binding (vars, Queues, Workers AI, …) is declared there again.
func Previews(w wconfig.Config, need bool) []string {
	previews, ok := w.Raw["previews"].(map[string]any)
	if !ok {
		if need {
			return []string{fmt.Sprintf("%s has no previews block: Worker Previews need one; add \"previews\": {}, with the Worker's KV, D1, R2 and other storage bindings in it, pointed at preview-safe ones", filepath.Base(w.Path))}
		}
		return nil
	}
	var problems []string
	if missing := previewBindings(w); len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("%s's previews block lacks the bindings %s: a preview inherits none from production (only KV, D1 and R2 come from its Previews Base), so it would fail on its first use of one; add them there, pointed at preview-safe resources (vars with preview values)", filepath.Base(w.Path), strings.Join(missing, ", ")))
	}
	if unnamed := unnamedResources(previews); len(unnamed) > 0 {
		problems = append(problems, fmt.Sprintf("%s's previews block has %s with no resource named, which wrangler preview sends as it is and Cloudflare refuses: leave it out, and mise run cloudflare:previews gives previews their own, or name an existing one (a KV namespace's id, a D1's database_id, an R2 bucket's bucket_name)", filepath.Base(w.Path), strings.Join(unnamed, ", ")))
	}
	return problems
}

// unnamedResources names the previews block's KV, D1 and R2 bindings that don't say which resource they use: wrangler
// deploy creates those for production, wrangler preview doesn't.
func unnamedResources(previews map[string]any) []string {
	var out []string
	for _, k := range []struct{ kind, what, field string }{
		{"kv_namespaces", "KV", "id"}, {"d1_databases", "D1", "database_id"}, {"r2_buckets", "R2", "bucket_name"},
	} {
		for _, b := range list(previews[k.kind]) {
			if v, _ := b[k.field].(string); v == "" {
				out = append(out, fmt.Sprintf("%s %v", k.what, b["binding"]))
			}
		}
	}
	return out
}

// fromBase are the binding types a preview gets from the Worker's Previews Base (go tool cloudflare previews) when its
// previews block leaves them out, and assets, which stay at the top level.
var fromBase = []string{"kv_namespace", "d1", "r2_bucket", "assets"}

// previewBindings names the Worker's bindings, as its wrangler converts them, that its previews block doesn't have: a
// preview inherits none (Cloudflare's docs: vars, storage and API bindings alike go in the block), but for fromBase.
func previewBindings(w wconfig.Config) []string {
	var missing []string
	for name, typ := range w.Bindings {
		if _, ok := w.PreviewBindings[name]; !ok && !slices.Contains(fromBase, typ) {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

func list(v any) []map[string]any {
	items, _ := v.([]any)
	var out []map[string]any
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// gitFiles is the repo's files under dir, tracked or not, without the git-ignored ones.
func gitFiles(dir string) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", ".").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w (the rules need a git repo)", dir, err)
	}
	return strings.Fields(string(out)), nil
}

func cmpOr(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
