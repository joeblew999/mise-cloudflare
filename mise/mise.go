// Package mise holds the files mise-cloudflare init writes into a repo: the mise tasks, the GitHub Actions check
// workflow and the Claude Code cloud session hook.
package mise

import _ "embed"

//go:embed tasks.toml
var Tasks []byte

// Workflow has NAME and DIR filled in by init.
//
//go:embed workflow.yml
var Workflow []byte

//go:embed session-start.sh
var SessionStart []byte
