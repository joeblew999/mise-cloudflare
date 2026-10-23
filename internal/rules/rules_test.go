package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeblew999/cloudflare-zero/config"
	"github.com/joeblew999/cloudflare-zero/internal/wconfig"
)

func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skip("no git")
	}
	for name, body := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	return dir
}

func good() map[string]string {
	return map[string]string{
		".dev.vars.example": "GREETING=dummy\n",
		".gitignore":        ".dev.vars\n.env*\nnode_modules/\n",
		"package.json":      `{ "devDependencies": { "wrangler": "4.151.3" } }`,
		"README.md":         "Never run wrangler login here.\n",
		"src/worker.js":     "export default {}\n",
		"notes.txt":         "this machine's other wrangler logins\n",
	}
}

// cfg is what wrangler reads from a config with one D1 and one KV, no IDs, needing GREETING.
func cfg() wconfig.Config {
	return wconfig.Config{Path: "/r/wrangler.jsonc", Name: "w", Secrets: []string{"GREETING"}, Raw: map[string]any{
		"name":          "w",
		"d1_databases":  []any{map[string]any{"binding": "DB"}},
		"kv_namespaces": []any{map[string]any{"binding": "KV"}},
	}}
}

func TestCheckPasses(t *testing.T) {
	if p, err := Check(repo(t, good()), cfg()); err != nil || len(p) != 0 {
		t.Errorf("problems %q, %v", p, err)
	}
}

func TestCheckFinds(t *testing.T) {
	files := good()
	files["deploy.sh"] = "npx wrangler login\n"
	files["notes.txt"] = "CLOUDFLARE_API_TOKEN=abcdefghijklmnopqrstuvwxyz0123456789\n"
	files["package.json"] = `{ "devDependencies": { "wrangler": "^4.0.0" } }`
	files[".gitignore"] = "node_modules/\n"
	c := cfg()
	c.Secrets = []string{"GREETING", "KEY"}
	c.Raw["account_id"] = "a"
	c.Raw["d1_databases"] = []any{map[string]any{"binding": "DB", "database_id": "x"}}
	c.Raw["env"] = map[string]any{"production": map[string]any{"kv_namespaces": []any{map[string]any{"binding": "KV", "id": "y"}}}}
	p, err := Check(repo(t, files), c)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(p, "\n")
	for _, want := range []string{"an account ID", "(D1 DB)", "env.production: a resource ID (KV KV)", "deploy.sh calls",
		"notes.txt holds a Cloudflare token", ".dev.vars isn't", ".env isn't", "secrets.required lists [GREETING KEY]", "isn't pinned"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "README.md") {
		t.Errorf("a .md file may mention wrangler login:\n%s", got)
	}
}

// TestInitPasses: what go tool cloudflare init writes into a repo passes the rules.
func TestInitPasses(t *testing.T) {
	files := good()
	files["cloudflare.toml"] = config.Template
	if p, err := Check(repo(t, files), cfg()); err != nil || len(p) != 0 {
		t.Errorf("problems %q, %v", p, err)
	}
}
