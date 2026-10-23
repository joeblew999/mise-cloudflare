// Package mise holds what go tool cloudflare init writes into a repo, from the mise-cloudflare version it runs: the
// mise tasks (tasks.toml), the GitHub Actions workflow (workflow.yml, workflow-deploy.yml) and the Claude Code cloud
// session hook (session-start.sh).
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

// WorkflowDeploy is the jobs workflow.yml gets when GitHub Actions deploys (cloudflare actions):
// production from BRANCH, a preview per pull request, deleted when it closes.
//
//go:embed workflow-deploy.yml
var WorkflowDeploy []byte

// SessionStart is session-start.sh, the Claude Code cloud session hook go tool cloudflare claude writes into a repo's
// .claude/hooks: mise, every pinned tool, and each named folder's setup task, when a cloud session starts.
//
//go:embed session-start.sh
var SessionStart []byte
