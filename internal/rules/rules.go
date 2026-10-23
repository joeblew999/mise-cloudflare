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
