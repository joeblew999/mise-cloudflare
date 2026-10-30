// Package templates is the template: a whole repo with mise-cloudflare, which runs wherever it is put. It has
// two halves. The tool's half, listed below, is what mise-cloudflare init writes into any repo, at the same paths: the
// mise tasks, the GitHub workflow, the Claude Code hook and its settings, the agent skill, the docs and the
// .gitignore. The rest is a sample Worker, the repo's own (src/, wrangler.jsonc, package.json and its lockfile,
// mise.toml, .dev.vars.example), there so the template runs: init never writes it. What is for this repo itself (its
// own mise.toml, tests, CI and README) is never here.
package templates

import (
	"embed"
	"io/fs"
)

//go:embed .mise/tasks/mise-cloudflare.toml
//go:embed .github/workflows/cloudflare.yml
//go:embed .claude/hooks/mise-cloudflare-session-start.sh .claude/settings.json .claude/skills
//go:embed docs/mise-cloudflare
//go:embed .gitignore
var tool embed.FS

// Tool is the template's tool half: the files init writes into a repo, at their paths there.
var Tool fs.FS = tool

// Tasks is the repo's mise tasks, .mise/tasks/mise-cloudflare.toml.
var Tasks, _ = fs.ReadFile(Tool, ".mise/tasks/mise-cloudflare.toml")

// HookFile is the Claude Code hook's name in a repo's .claude/hooks.
const HookFile = "mise-cloudflare-session-start.sh"
