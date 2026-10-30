// Package templates holds every file mise-cloudflare init writes into a repo: the mise tasks, the GitHub Actions check
// workflow, the Claude Code cloud session hook and the agent skill.
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

//go:embed session-start.sh
var SessionStart []byte

//go:embed skills
var skills embed.FS

// Skills is the agent skill init installs into a repo's .claude/skills: cloudflare/SKILL.md.
var Skills, _ = fs.Sub(skills, "skills")
