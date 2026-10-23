// Package mise holds cloudflare-zero's mise tasks (tasks.toml): go tool cloudflare tasks writes them into a repo, from
// the cloudflare-zero version in its go.mod.
package mise

import _ "embed"

// Tasks is tasks.toml.
//
//go:embed tasks.toml
var Tasks []byte
