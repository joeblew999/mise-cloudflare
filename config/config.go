// Package config reads a repo's cloudflare.toml: its own Cloudflare login and what that login may do. go tool cloudflare
// finds it by walking up from the working directory, so every cloudflare:* task runs with no arguments; a flag still
// overrides each setting. Every setting has a default, so the file only says what differs:
//
//	# cloudflare.toml
//	profile = "my-worker"                                   # the login, a wrangler auth profile; default the Worker's name
//	scopes = ["account:read", "user:read", "workers:write"] # exactly what it may do; default DefaultScopes
//
// The repo never holds an account ID, a resource ID, a token or a secret value: the login picks the account, wrangler
// creates the resources, and Cloudflare keeps the secrets.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// File is the config file's name.
const File = "cloudflare.toml"

// DefaultScopes is what a Worker with D1, KV and R2 needs (R2 comes with workers:write).
var DefaultScopes = []string{"account:read", "user:read", "workers:write", "workers_kv:write", "workers_scripts:write", "d1:write"}

// Config is a repo's cloudflare.toml, with its defaults filled in.
type Config struct {
	Profile string   `toml:"profile"` // "" until the caller fills in the Worker's name
	Scopes  []string `toml:"scopes"`
	Path    string   `toml:"-"` // the file read, "" without one
}

// Find reads the nearest cloudflare.toml at or above dir, and fills in the defaults; without one, it is every default.
func Find(dir string) (Config, error) {
	var c Config
	dir, err := filepath.Abs(dir)
	if err != nil {
		return c, err
	}
	for {
		path := filepath.Join(dir, File)
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, &c); err != nil {
				return c, err
			}
			c.Path = path
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if c.Scopes == nil {
		c.Scopes = DefaultScopes
	}
	return c, nil
}

// Template is the cloudflare.toml go tool cloudflare init writes: every setting, commented out at its default.
const Template = `# cloudflare.toml: this repo on Cloudflare, for go tool cloudflare and the cloudflare:* tasks
# (github.com/joeblew999/cloudflare-zero). The tasks read it, so none needs an argument; a flag still overrides it.
# Every line is optional: uncomment what differs from its default. Never put an account ID, a resource ID, a token or
# a secret value here: the login picks the account, wrangler creates the resources, Cloudflare keeps the secrets.

# The repo's own Cloudflare login, a wrangler auth profile. Default: the Worker's name (its wrangler config's name).
# profile = "my-worker"

# What the login may do: OAuth scopes, named as wrangler lists them (its login command, with --scopes-list). The login
# holds exactly these; change them and the next task logs in once more. Default: what a Worker with D1, KV and R2 needs.
# scopes = ["account:read", "user:read", "workers:write", "workers_kv:write", "workers_scripts:write", "d1:write"]
`

// TemplateFor is Template with profile set, for go tool cloudflare init -profile.
func TemplateFor(profile string) string {
	return strings.Replace(Template, `# profile = "my-worker"`, `profile = "`+profile+`"`, 1)
}
