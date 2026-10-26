// Package skills holds the agent skill that ships with mise-cloudflare; mise-cloudflare init installs it into a
// repo's .claude/skills.
package skills

import "embed"

// FS is every skill directory: cloudflare/SKILL.md.
//
//go:embed cloudflare
var FS embed.FS
