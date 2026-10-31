package wconfig

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// read reads testdata/<name>.jsonc (or any config file) in environment env with the template's own wrangler, in a
// folder of its own as a repo would be.
func read(t *testing.T, config, env string) (Config, error) {
	t.Helper()
	modules, _ := filepath.Abs(filepath.Join("..", "..", "templates", "node_modules"))
	if _, err := os.Stat(filepath.Join(modules, "wrangler")); err != nil {
		t.Skip("the template's wrangler isn't installed: mise run template:setup")
	}
	dir := t.TempDir()
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "index.js"), []byte("export default {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), b, 0o644)
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skipf("no symlink to the template's node_modules here: %v", err)
	}
	return ReadEnv(context.Background(), dir, env)
}

// TestAdopted: a project mise-cloudflare didn't make, read as wrangler reads it: production in env.production under its
// own name, its resources named by ID (the developer's), a Durable Object and a Workflow, and no warnings.
func TestAdopted(t *testing.T) {
	for _, c := range []struct{ env, name string }{{"", "adopted-dev"}, {"production", "adopted"}} {
		w, err := read(t, filepath.Join("testdata", "adopted.jsonc"), c.env)
		if err != nil {
			t.Fatal(err)
		}
		if w.Name != c.name || !w.HasEnv("production") || w.HasEnv("staging") || w.Previews || w.Warnings != "" {
			t.Errorf("env %q: name %q, envs %q, previews %v, warnings %q", c.env, w.Name, w.Envs, w.Previews, w.Warnings)
		}
		want := map[string]string{"DB": "hyperdrive", "CACHE": "kv_namespace", "LIMITER": "durable_object_namespace", "INDEX": "workflow"}
		for n, typ := range want {
			if w.Bindings[n] != typ {
				t.Errorf("env %q: binding %s is %q, want %q (all: %v)", c.env, n, w.Bindings[n], typ, w.Bindings)
			}
		}
		if !w.Names("CACHE") || !w.Names("DB") || !slices.Equal(w.Secrets, []string{"API_KEY", "SIGNING_KEY"}) {
			t.Errorf("env %q: named %q, secrets %q", c.env, w.Named, w.Secrets)
		}
	}
}

// TestTemplate: the template's own config: its KV, D1 and R2 name nothing, so wrangler deploy makes them.
func TestTemplate(t *testing.T) {
	w, err := read(t, filepath.Join("..", "..", "templates", "wrangler.jsonc"), "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "mise-cloudflare-template" || len(w.Named) != 0 || !w.Previews || len(w.Envs) != 0 || w.Warnings != "" {
		t.Errorf("name %q, named %q, previews %v, envs %q, warnings %q", w.Name, w.Named, w.Previews, w.Envs, w.Warnings)
	}
	for n, typ := range map[string]string{"DB": "d1", "KV": "kv_namespace", "BUCKET": "r2_bucket"} {
		if w.Bindings[n] != typ {
			t.Errorf("binding %s is %q, want %q", n, w.Bindings[n], typ)
		}
	}
}

// TestBroken: wrangler's own validation comes through: an unexpected field as its warning, a wrong type as its error.
func TestBroken(t *testing.T) {
	w, err := read(t, filepath.Join("testdata", "broken-field.jsonc"), "")
	if err != nil || !strings.Contains(w.Warnings, "kv_namespacez") {
		t.Errorf("an unexpected field: warnings %q, err %v", w.Warnings, err)
	}
	if _, err := read(t, filepath.Join("testdata", "broken-type.jsonc"), ""); err == nil || !strings.Contains(err.Error(), "wrangler can't read the config") || !strings.Contains(err.Error(), "minify") {
		t.Errorf("a wrong type: err %v", err)
	}
	w, err = read(t, filepath.Join("testdata", "bare.jsonc"), "")
	if err != nil || w.Previews || len(w.Bindings) != 0 || w.Warnings != "" {
		t.Errorf("bare: %+v, %v", w, err)
	}
}
