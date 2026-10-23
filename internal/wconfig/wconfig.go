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
	"strings"
)

// Config is what the tasks need from a repo's wrangler config.
type Config struct {
	Path    string         `json:"path"`    // the file wrangler read
	Name    string         `json:"name"`    // the Worker's name, as wrangler resolves it (CLOUDFLARE_ENV included)
	Secrets []string       `json:"secrets"` // secrets.required
	Kinds   []string       `json:"kinds"`   // the kinds of binding the Worker uses: d1_databases, queues, routes, …
	Raw     map[string]any `json:"raw"`     // the file as written, every environment in it
}

// Kinds are the binding kinds wrangler's config can hold, by their config key (and routes, for route and routes).
var Kinds = []string{"d1_databases", "kv_namespaces", "r2_buckets", "queues", "durable_objects", "vectorize", "hyperdrive",
	"ai", "pipelines", "secrets_store_secrets", "workflows", "containers", "browser", "send_email",
	"analytics_engine_datasets", "services", "dispatch_namespaces", "images", "ratelimits", "routes"}

// script reads the config with the repo's own wrangler: unstable_readConfig is what wrangler deploy resolves,
// experimental_readRawConfig the file as written. One marked line, so nothing else wrangler prints gets in the way.
// A kind is used when its list (or its producers, consumers or bindings) has something in it, or its object is set.
const script = `const w = await import('wrangler');
const c = w.unstable_readConfig({ env: process.env.CLOUDFLARE_ENV || undefined });
const r = await w.experimental_readRawConfig({});
const used = (v) => Array.isArray(v) ? v.length > 0
  : v && typeof v === 'object' ? (v.bindings ?? [...(v.producers ?? []), ...(v.consumers ?? [])]).length > 0 || (!v.bindings && !v.producers && !v.consumers && Object.keys(v).length > 0)
  : !!v;
const kinds = KINDS.filter((k) => k === 'routes' ? used(c.routes) || used(c.route) : used(c[k]));
console.log('WCONFIG ' + JSON.stringify({ path: c.configPath, name: c.name, secrets: c.secrets?.required ?? [], kinds, raw: r.rawConfig }));`

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
	kinds, _ := json.Marshal(Kinds)
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", strings.Replace(script, "KINDS", string(kinds), 1))
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
