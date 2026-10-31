package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
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
		".gitignore":        ".dev.vars\n.env*\n.cloudflare/\nnode_modules/\n",
		"package.json":      `{ "devDependencies": { "wrangler": "4.151.3" } }`,
		"README.md":         "Never run wrangler login here.\n",
		"src/worker.js":     "export default {}\n",
		"notes.txt":         "this machine's other wrangler logins\n",
	}
}

// cfg is what wrangler reads from a config with one D1 and one KV that name no resource, and a previews block whose
// D1 and KV name staging's, needing GREETING.
func cfg() wconfig.Config {
	return wconfig.Config{Path: "/r/wrangler.jsonc", Name: "w", Secrets: []string{"GREETING"},
		Bindings: map[string]string{"DB": "d1", "KV": "kv_namespace"}, PreviewBindings: map[string]string{"DB": "d1", "KV": "kv_namespace"},
		PreviewNamed: []string{"DB", "KV"}, Previews: true}
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
	c.Previews, c.PreviewBindings, c.PreviewNamed = false, nil, nil
	c.Warnings = `▲ [WARNING] Processing wrangler.jsonc configuration:
    - Unexpected fields found in top-level field: "kv_namespacez"`
	p, err := Check(repo(t, files), c)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(p, "\n")
	for _, want := range []string{"wrangler warns about wrangler.jsonc:", `"kv_namespacez"`, "deploy.sh calls",
		"notes.txt holds a Cloudflare token", ".dev.vars isn't", ".env isn't", ".cloudflare/ isn't", "secrets.required lists [GREETING KEY]", "isn't pinned"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "README.md") {
		t.Errorf("a .md file may mention wrangler login:\n%s", got)
	}
}

// TestPreviewBindings: a binding the previews block doesn't have is named, but for KV, D1 and R2, which the Previews Base
// gives (cloudflare deploy, preview and github).
func TestPreviewBindings(t *testing.T) {
	c := cfg()
	c.PreviewBindings = map[string]string{"AI": "ai"}
	c.Bindings = map[string]string{"DB": "d1", "KV": "kv_namespace", "BUCKET": "r2_bucket", "INDEX": "vectorize", "JOBS": "queue", "AI": "ai", "ASSETS": "assets"}
	p, err := Check(repo(t, good()), c)
	if err != nil || len(p) != 1 || !strings.Contains(p[0], "lacks the bindings INDEX, JOBS") {
		t.Errorf("problems %q, %v", p, err)
	}
}

// TestPreviewResources: a previews block's KV, D1 or R2 that doesn't name its resource is named: wrangler preview
// creates none.
func TestPreviewResources(t *testing.T) {
	c := cfg()
	c.Bindings["BUCKET"] = "r2_bucket"
	c.PreviewBindings["BUCKET"] = "r2_bucket"
	c.PreviewNamed = []string{"DB"}
	p := Previews(c, false)
	if len(p) != 1 || !strings.Contains(p[0], "has R2 BUCKET, KV KV with no resource named") {
		t.Errorf("problems %q", p)
	}
}

// TestPreviewsNeeded: no previews block is fine for a repo that runs no Worker Previews, a problem for one that does.
func TestPreviewsNeeded(t *testing.T) {
	c := cfg()
	c.Previews = false
	if p := Previews(c, false); len(p) != 0 {
		t.Errorf("problems %q", p)
	}
	if p := Previews(c, true); len(p) != 1 || !strings.Contains(p[0], "add \"previews\": {}") {
		t.Errorf("problems %q", p)
	}
}
