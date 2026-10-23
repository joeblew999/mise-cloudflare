// Package config reads a repo's cloudflare.toml: its own Cloudflare login and what that login may do. go tool cloudflare
// finds it by walking up from the working directory, so every cloudflare:* task runs with no arguments; a flag still
// overrides each setting. Every setting has a default, so the file only says what differs:
//
//	# cloudflare.toml
//	profile = "my-worker"                                   # the login, a wrangler auth profile; default the Worker's name
//	scopes = ["pipelines:write"]                            # what it may do beyond what the Worker's bindings need
//
// The repo never holds an account ID, a resource ID, a token or a secret value: the login picks the account, wrangler
// creates the resources, and Cloudflare keeps the secrets.
package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// File is the config file's name.
const File = "cloudflare.toml"

// BaseScopes are what every Worker's login needs: the account, the user, and Workers and their scripts.
var BaseScopes = []string{"account:read", "user:read", "workers:write", "workers_scripts:write"}

// BindingScope is what one kind of binding needs of a repo's login beyond BaseScopes, and the proof that showed it on a
// real account ("" if none has yet).
type BindingScope struct {
	Scopes []string
	Proven string
}

// BindingScopes is each kind of binding, by the type wrangler itself gives it (unstable_convertConfigBindingsToStartWorkerBindings),
// and routes, containers and k2, which aren't bindings but need scopes. Neither wrangler nor Cloudflare publishes which
// scope a kind needs, so each is the scope whose description in wrangler's own login names the product
// ("workers_kv:write: See and change Cloudflare Workers KV Storage data…"), or none where no scope does, as with
// wrangler's own login. TestBindingTypes fails when the installed wrangler has a type this doesn't cover, or names a
// scope wrangler doesn't have; a Worker with a kind that isn't here is told so (Unknown), never given nothing silently.
var BindingScopes = map[string]BindingScope{
	"kv_namespace":             {[]string{"workers_kv:write"}, "prove, prove-builds"},
	"d1":                       {[]string{"d1:write"}, "prove"},
	"r2_bucket":                {nil, "prove, prove-builds"},
	"queue":                    {[]string{"queues:write"}, "prove-bindings"},
	"ai":                       {[]string{"ai:write"}, "prove-bindings"},
	"browser":                  {[]string{"browser:write"}, "prove-bindings"},
	"durable_object_namespace": {nil, "prove-bindings"},
	"workflow":                 {nil, "prove-bindings"},
	"vectorize":                {nil, "prove-bindings"},
	"analytics_engine":         {nil, "prove-bindings"},
	"images":                   {nil, "prove-bindings"},
	"ratelimit":                {nil, "prove-bindings"},
	"plain_text":               {nil, "prove-builds"},
	"pipeline":                 {[]string{"pipelines:write"}, ""},
	"secrets_store_secret":     {[]string{"secrets_store:write"}, ""},
	"agent_memory":             {[]string{"agent-memory:write"}, ""},
	"ai_search":                {[]string{"ai-search:write"}, ""},
	"ai_search_namespace":      {[]string{"ai-search:write"}, ""},
	"artifacts":                {[]string{"artifacts:write"}, ""},
	"flagship":                 {[]string{"flagship:write"}, ""},
	"mtls_certificate":         {[]string{"ssl_certs:write"}, ""},
	"vpc_service":              {[]string{"connectivity:admin"}, ""},
	"vpc_network":              {[]string{"connectivity:admin"}, ""},
	"routes":                   {[]string{"workers_routes:write", "zone:read"}, ""},
	"containers":               {[]string{"containers:write", "cloudchamber:write"}, ""},
	"k2":                       {[]string{"k2.write"}, ""},
	"json":                     {nil, ""},
	"version_metadata":         {nil, ""},
	"assets":                   {nil, ""},
	"data_blob":                {nil, ""},
	"text_blob":                {nil, ""},
	"wasm_module":              {nil, ""},
	"service":                  {nil, ""},
	"worker_loader":            {nil, ""},
	"dispatch_namespace":       {nil, ""},
	"hyperdrive":               {nil, ""},
	"media":                    {nil, ""},
	"stream":                   {nil, ""},
	"logfwdr":                  {nil, ""},
	"analytics":                {nil, ""},
	"unsafe_hello_world":       {nil, ""},
}

// NoScopeKnown are kinds of binding wrangler has whose scope can't be told from its descriptions: send_email could be
// Email Routing's or Email Sending's. A Worker with one is told, as for any kind BindingScopes lacks.
var NoScopeKnown = []string{"send_email"}

// Unknown is the kinds of binding a Worker has that BindingScopes doesn't cover: whatever they need, the login has it
// only if cloudflare.toml's scopes adds it.
func Unknown(kinds []string) []string {
	var out []string
	for _, k := range kinds {
		if _, ok := BindingScopes[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// TokenPermissions are the permissions CI's token needs for a kind of binding (BindingScopes' kinds) beyond Workers Scripts, which deploying
// needs ("You do not need separate permissions on the bound resources to deploy the Worker", Cloudflare's Workers
// authorization docs): for what wrangler does to the resources themselves, creating KV, D1 and R2 on a first deploy and
// routes on a zone. Each is key:type in Cloudflare's token links (fundamentals/api/how-to/account-owned-token-template).
var TokenPermissions = map[string][]string{
	"kv_namespace": {"workers_kv_storage:edit"},
	"r2_bucket":    {"workers_r2:edit"},
	"d1":           {"d1:edit"},
	"routes":       {"workers_routes:edit", "zone:read"},
}

// TokenByHand are the permissions CI's token needs for a kind of binding that Cloudflare's token links have no key for,
// by their names in the dashboard: Secrets Store's, which Cloudflare says deploying a Worker bound to it needs.
var TokenByHand = map[string][]string{
	"secrets_store_secret": {"Secrets Store Edit"},
}

// Scopes is what a repo's login holds: BaseScopes, what each kind of binding its Worker has needs (BindingScopes), and
// extra, cloudflare.toml's scopes, for what isn't a binding.
func Scopes(kinds, extra []string) []string {
	out := slices.Clone(BaseScopes)
	add := func(ss []string) {
		for _, s := range ss {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	for _, k := range kinds {
		add(BindingScopes[k].Scopes)
	}
	add(extra)
	return out
}

// Config is a repo's cloudflare.toml, with its defaults filled in.
type Config struct {
	Profile  string   `toml:"profile"` // "" until the caller fills in the Worker's name
	Scopes   []string `toml:"scopes"`  // what the login may do beyond what the Worker's bindings need (see Scopes)
	Builds   Builds   `toml:"builds"`
	Workflow Workflow `toml:"workflow"`
	Path     string   `toml:"-"` // the file read, "" without one
}

// Workflow is the GitHub Actions workflow go tool cloudflare workflow writes. Without deploy it checks the repo on every
// push and pull request, and Workers Builds deploys. With deploy, GitHub Actions deploys too: production from Branch,
// a preview per pull request, with the CLOUDFLARE_API_TOKEN go tool cloudflare ci-token stores as a GitHub secret.
// The token's permissions come from the Worker's bindings (TokenPermissions).
type Workflow struct {
	Deploy bool   `toml:"deploy"`
	Branch string `toml:"branch"` // default: the repository's default branch, else main
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
	return c, nil
}

// Template is the cloudflare.toml go tool cloudflare init writes: every setting, commented out at its default.
const Template = `# cloudflare.toml: this repo on Cloudflare, for go tool cloudflare and the cloudflare:* tasks
# (github.com/joeblew999/mise-cloudflare). The tasks read it, so none needs an argument; a flag still overrides it.
# Every line is optional: uncomment what differs from its default. Never put an account ID, a resource ID, a token or
# a secret value here: the login picks the account, wrangler creates the resources, Cloudflare keeps the secrets.

# The repo's own Cloudflare login, a wrangler auth profile. Default: the Worker's name (its wrangler config's name).
# profile = "my-worker"

# What the login may do beyond what the Worker's bindings need. It holds exactly what they need, worked out from the
# wrangler config (KV: workers_kv:write, D1: d1:write, Queues: queues:write, …), and these: OAuth scopes as wrangler
# names them (its login command, with --scopes-list), for what isn't a binding, such as Basin's pipelines:write.
# Add a binding or a scope and the next task logs in once more.
# scopes = ["pipelines:write"]

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

# The GitHub Actions workflow (go tool cloudflare workflow). It checks the repo on every push and pull request. With
# deploy, GitHub Actions also deploys, instead of Workers Builds: production from branch, a preview pr-<number> per pull
# request, deleted when it closes, with a token made once in Cloudflare's dashboard and stored by go tool cloudflare
# ci-token as a GitHub secret. ci-token gives the link that makes it, with what the Worker's bindings need picked.
[workflow]
# deploy = true
# branch = "main"
`

// TemplateFor is Template with profile set, for go tool cloudflare init -profile.
func TemplateFor(profile string) string {
	return strings.Replace(Template, `# profile = "my-worker"`, `profile = "`+profile+`"`, 1)
}
