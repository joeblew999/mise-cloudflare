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
	Builds  Builds   `toml:"builds"`
	Path    string   `toml:"-"` // the file read, "" without one
}

// Builds is how Cloudflare's Workers Builds builds and deploys the Worker from its GitHub repository (go tool cloudflare
// builds). Every field has a default, filled in by the command: it knows the repository and the lockfile.
type Builds struct {
	Branch  string `toml:"branch"`  // the branch whose pushes deploy: default the repository's default branch
	Root    string `toml:"root"`    // the folder, in the repository, its commands run in: default this one
	Build   string `toml:"build"`   // installs the Worker's dependencies: default from its lockfile
	Deploy  string `toml:"deploy"`  // default npx wrangler deploy (with --env when CLOUDFLARE_ENV is set)
	Preview string `toml:"preview"` // every other branch's preview: default npx wrangler preview (with --env)
	Token   string `toml:"token"`   // the name of the account's build token it deploys with: default the first
	// Staging, if set, is the branch that deploys the wrangler environment staging: its own Worker (env.staging, with
	// its own bindings), deployed with --env staging. Production's branch then defaults to "production".
	Staging string `toml:"staging"`
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
# (github.com/joeblew999/mise-cloudflare). The tasks read it, so none needs an argument; a flag still overrides it.
# Every line is optional: uncomment what differs from its default. Never put an account ID, a resource ID, a token or
# a secret value here: the login picks the account, wrangler creates the resources, Cloudflare keeps the secrets.

# The repo's own Cloudflare login, a wrangler auth profile. Default: the Worker's name (its wrangler config's name).
# profile = "my-worker"

# What the login may do: OAuth scopes, named as wrangler lists them (its login command, with --scopes-list). The login
# holds exactly these; change them and the next task logs in once more. Default: what a Worker with D1, KV and R2 needs.
# scopes = ["account:read", "user:read", "workers:write", "workers_kv:write", "workers_scripts:write", "d1:write"]

# Workers Builds (go tool cloudflare builds): Cloudflare builds and deploys the Worker from its GitHub repository, with
# its own token, so GitHub never holds a Cloudflare credential. A push to branch deploys; every other branch gets a
# preview, its URL on the pull request.
[builds]
# branch = "main"                              # default: the repository's default branch
# root = "."                                   # the folder its commands run in; default: this one
# build = "npm ci"                             # default: from the lockfile
# deploy = "npx wrangler deploy"
# preview = "npx wrangler preview"
# token = "My Build Token"                     # which of the account's build tokens; default: the first
# For staging and production: a push to main deploys staging (the wrangler environment env.staging, its own Worker),
# a push to production deploys production, and every other branch gets a preview.
# staging = "main"
# branch = "production"
`

// TemplateFor is Template with profile set, for go tool cloudflare init -profile.
func TemplateFor(profile string) string {
	return strings.Replace(Template, `# profile = "my-worker"`, `profile = "`+profile+`"`, 1)
}
