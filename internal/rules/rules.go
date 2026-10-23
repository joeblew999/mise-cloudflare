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

// Check returns the repo at dir's problems: IDs in its wrangler config (any environment), tokens or the machine-wide
// wrangler login in its files, .dev.vars or .env not git-ignored, .dev.vars.example not matching secrets.required,
// wrangler not pinned to an exact version.
func Check(dir string, w wconfig.Config) ([]string, error) {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	if previews, ok := w.Raw["previews"].(map[string]any); !ok {
		bad("%s has no previews block: Worker Previews (Workers Builds' and GitHub Actions') need one; add \"previews\": {} (it can be empty)", filepath.Base(w.Path))
	} else if missing := previewBindings(w.Raw, previews); len(missing) > 0 {
		bad("%s's previews block lacks the bindings %s: Worker Previews don't inherit them from production (a preview would fail on its first use of one); add them there, pointed at preview-safe resources", filepath.Base(w.Path), strings.Join(missing, ", "))
	}
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

// storageKinds are the bindings to resources that Worker Previews don't inherit: each needs its own in the previews block.
var storageKinds = []string{"d1_databases", "kv_namespaces", "r2_buckets", "vectorize", "hyperdrive", "analytics_engine_datasets",
	"pipelines", "secrets_store_secrets"}

// previewBindings names the storage bindings the Worker has that its previews block doesn't declare.
func previewBindings(raw, previews map[string]any) []string {
	var missing []string
	names := func(v any) []string {
		var out []string
		for _, b := range list(v) {
			if n, _ := b["binding"].(string); n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	for _, k := range storageKinds {
		have := names(previews[k])
		for _, n := range names(raw[k]) {
			if !slices.Contains(have, n) {
				missing = append(missing, n)
			}
		}
	}
	// Queue producers too.
	q, _ := raw["queues"].(map[string]any)
	pq, _ := previews["queues"].(map[string]any)
	for _, p := range list(q["producers"]) {
		n, _ := p["binding"].(string)
		found := false
		for _, pp := range list(pq["producers"]) {
			found = found || pp["binding"] == n
		}
		if n != "" && !found {
			missing = append(missing, n)
		}
	}
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
