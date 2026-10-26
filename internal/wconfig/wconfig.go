// Package wconfig asks the repo's own wrangler for its config, so nothing here parses one: what deploy will use, and the
// file as written.
package wconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
)

// Config is what the tasks need from a repo's wrangler config.
type Config struct {
	Path    string   `json:"path"`    // the file wrangler read
	Name    string   `json:"name"`    // the Worker's name, as wrangler resolves it (CLOUDFLARE_ENV included)
	Secrets []string `json:"secrets"` // secrets.required
	// The Worker's bindings and its previews block's, name → type, as wrangler itself converts them (kv_namespace, d1,
	// queue, workflow, …).
	Bindings        map[string]string `json:"bindings"`
	PreviewBindings map[string]string `json:"previewBindings"`
	Raw             map[string]any    `json:"raw"` // the file as written, every environment in it
}

// Created is the binding kinds wrangler deploy finds or creates by name (KV, D1, R2), so the config needs no IDs for them.
var Created = map[string]struct {
	What    string   // KV
	Key     string   // its list in a wrangler config: kv_namespaces
	Names   string   // the field a binding names its resource by: id
	IDs     []string // the fields that would put the account's resource IDs into the repo: id, preview_id
	BaseKey string   // the field naming its resource in a preview's bindings: namespace_id
}{
	"kv_namespace": {"KV", "kv_namespaces", "id", []string{"id", "preview_id"}, "namespace_id"},
	"d1":           {"D1", "d1_databases", "database_id", []string{"database_id", "preview_database_id"}, "database_id"},
	"r2_bucket":    {"R2", "r2_buckets", "bucket_name", nil, "bucket_name"},
}

// script reads the config with the repo's own wrangler and prints it as one marked line.
const script = `const w = await import('wrangler');
const c = w.unstable_readConfig({ env: process.env.CLOUDFLARE_ENV || undefined });
const r = await w.experimental_readRawConfig({});
const types = (b) => Object.fromEntries(Object.entries(w.unstable_convertConfigBindingsToStartWorkerBindings(b ?? {}) ?? {}).map(([n, v]) => [n, v.type]));
const bindings = types(c), previewBindings = types(c.previews);
console.log('WCONFIG ' + JSON.stringify({ path: c.configPath, name: c.name, secrets: c.secrets?.required ?? [], bindings, previewBindings, raw: r.rawConfig }));`

// Read asks the wrangler installed in dir (node_modules) for dir's config, in CLOUDFLARE_ENV's environment if it's set.
func Read(ctx context.Context, dir string) (Config, error) {
	return ReadEnv(ctx, dir, os.Getenv("CLOUDFLARE_ENV"))
}

// Envs is the environments the config defines (env.<name>), as written.
func (c Config) Envs() []string {
	envs, _ := c.Raw["env"].(map[string]any)
	return slices.Sorted(maps.Keys(envs))
}

// ReadEnv is Read in the wrangler environment env ("" for none): the Worker that environment deploys.
func ReadEnv(ctx context.Context, dir, env string) (Config, error) {
	var c Config
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLOUDFLARE_ENV="+env)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return c, fmt.Errorf("wrangler couldn't read the config here (install the repo's dependencies first: its package.json pins wrangler): %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	for _, line := range bytes.Split(out, []byte("\n")) {
		if b, ok := bytes.CutPrefix(line, []byte("WCONFIG ")); ok {
			if err := json.Unmarshal(b, &c); err != nil {
				return c, err
			}
			if c.Name == "" {
				return c, fmt.Errorf("%s has no name", c.Path)
			}
			return c, nil
		}
	}
	return c, fmt.Errorf("wrangler printed no config here:\n%s", bytes.TrimSpace(stderr.Bytes()))
}
