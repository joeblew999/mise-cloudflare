// Package skills holds the agent skill that ships with cloudflare-zero; go tool cloudflare skill installs it into a
// repo's .claude/skills.
package skills

import "embed"

// FS is every skill directory: cloudflare/SKILL.md.
//
//go:embed cloudflare
var FS embed.FS
