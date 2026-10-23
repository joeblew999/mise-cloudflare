// Package mise holds mise-cloudflare's mise tasks (tasks.toml) and GitHub Actions workflow (workflow.yml): go tool
// cloudflare tasks and workflow write them into a repo, from the mise-cloudflare version it runs.
package mise

import _ "embed"

// Tasks is tasks.toml.
//
//go:embed tasks.toml
var Tasks []byte

// Workflow is workflow.yml, the GitHub Actions workflow go tool cloudflare workflow writes into a repo: VERSION, NAME
// and DIR are filled in.
//
//go:embed workflow.yml
var Workflow []byte

// SessionStart is session-start.sh, the Claude Code cloud session hook go tool cloudflare claude writes into a repo's
// .claude/hooks: mise, every pinned tool, and each named folder's setup task, when a cloud session starts.
//
//go:embed session-start.sh
var SessionStart []byte
