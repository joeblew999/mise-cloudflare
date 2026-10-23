// Package wconfig asks a repo's own pinned wrangler for its config. wrangler reads it (wrangler.json, wrangler.jsonc or
// wrangler.toml, every setting, environments, CLOUDFLARE_ENV), so nothing here parses a wrangler config: it takes
// wrangler's resolved config for what deploy will use, and the file as written, every environment in it, for the rules.
package wconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Config is what the tasks need from a repo's wrangler config.
type Config struct {
	Path    string         `json:"path"`    // the file wrangler read
	Name    string         `json:"name"`    // the Worker's name, as wrangler resolves it (CLOUDFLARE_ENV included)
	Secrets []string       `json:"secrets"` // secrets.required
	Raw     map[string]any `json:"raw"`     // the file as written, every environment in it
}

// script reads the config with the repo's own wrangler: unstable_readConfig is what wrangler deploy resolves,
// experimental_readRawConfig the file as written. One marked line, so nothing else wrangler prints gets in the way.
const script = `const w = await import('wrangler');
const c = w.unstable_readConfig({ env: process.env.CLOUDFLARE_ENV || undefined });
const r = await w.experimental_readRawConfig({});
console.log('WCONFIG ' + JSON.stringify({ path: c.configPath, name: c.name, secrets: c.secrets?.required ?? [], raw: r.rawConfig }));`

// Read asks the wrangler installed in dir (node_modules) for dir's config.
func Read(ctx context.Context, dir string) (Config, error) {
	var c Config
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return c, fmt.Errorf("wrangler couldn't read the config here (run go tool cloudflare setup first): %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
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
