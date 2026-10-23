// Package mise holds mise-cloudflare's mise tasks (tasks.toml): go tool cloudflare tasks writes them into a repo, from
// the mise-cloudflare version in its go.mod.
package mise

import _ "embed"

// Tasks is tasks.toml.
//
//go:embed tasks.toml
var Tasks []byte
