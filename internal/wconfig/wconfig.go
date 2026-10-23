// Package wconfig asks a repo's own pinned wrangler for its config. wrangler reads it (wrangler.json, wrangler.jsonc or
// wrangler.toml, every setting, environments, CLOUDFLARE_ENV), so nothing here parses a wrangler config: it takes
// wrangler's resolved config for what deploy will use, and the file as written, every environment in it, for the rules.
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
	// queue, workflow, …): no list of kinds here, so a kind wrangler adds is there too.
	Bindings        map[string]string `json:"bindings"`
	PreviewBindings map[string]string `json:"previewBindings"`
	// Kinds is the binding types the Worker has, and routes, containers and k2 if it has them (settings wrangler doesn't
	// convert to bindings, and a login needs scopes for).
	Kinds []string       `json:"kinds"`
	Raw   map[string]any `json:"raw"` // the file as written, every environment in it
}

// script reads the config with the repo's own wrangler: unstable_readConfig is what wrangler deploy resolves,
// experimental_readRawConfig the file as written, unstable_convertConfigBindingsToStartWorkerBindings its bindings, as
// wrangler sends them. One marked line, so nothing else wrangler prints gets in the way.
const script = `const w = await import('wrangler');
const c = w.unstable_readConfig({ env: process.env.CLOUDFLARE_ENV || undefined });
const r = await w.experimental_readRawConfig({});
const types = (b) => Object.fromEntries(Object.entries(w.unstable_convertConfigBindingsToStartWorkerBindings(b ?? {}) ?? {}).map(([n, v]) => [n, v.type]));
const bindings = types(c), previewBindings = types(c.previews);
const kinds = [...new Set(Object.values(bindings))];
if ((c.routes?.length ?? 0) > 0 || c.route) kinds.push('routes');
if ((c.containers?.length ?? 0) > 0) kinds.push('containers');
if ((c.k2?.length ?? 0) > 0) kinds.push('k2');
kinds.sort();
console.log('WCONFIG ' + JSON.stringify({ path: c.configPath, name: c.name, secrets: c.secrets?.required ?? [], bindings, previewBindings, kinds, raw: r.rawConfig }));`

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
