// Package templates holds every file mise-cloudflare init writes into a repo: everything that is for other repos. The
// mise tasks, the GitHub Actions check workflow, the Claude Code cloud session hook, the agent skill and the repo's
// docs. What is for this repo itself (its own mise.toml, tests, CI and README) is never here.
package templates

import (
	"embed"
	"io/fs"
)

//go:embed tasks.toml
var Tasks []byte

// Workflow has NAME and DIR filled in by init.
//
//go:embed workflow.yml
var Workflow []byte

// SessionStart is the Claude Code cloud session hook, which init writes into a repo's .claude/hooks as HookFile.
//
//go:embed session-start.sh
var SessionStart []byte

// HookFile is the hook's name in a repo's .claude/hooks.
const HookFile = "mise-cloudflare-session-start.sh"

//go:embed skills
var skills embed.FS

// Skills is the agent skill init installs into a repo's .claude/skills: cloudflare/SKILL.md.
var Skills, _ = fs.Sub(skills, "skills")

//go:embed docs
var docs embed.FS

// Docs is what init writes into a repo's docs/mise-cloudflare: how mise-cloudflare works there, for the repo's people
// and agents alike.
var Docs, _ = fs.Sub(docs, "docs")
