# mise-cloudflare

A repo's own Cloudflare login, and the repo's own wrangler on it, as a Go command (`go tool cloudflare`, or the released
`cloudflare` binary for repos without Go) with its mise tasks and agent skill, built the way go-htmx4's `seo` and `intl`
are. README.md is the whole picture; read it first.

## Working on it

- `mise run test`: the Go tests, no Cloudflare account. Run them after every change.
- `mise run example`: stamps `example/` through the tool (its tasks and skill), as any repo gets them. The tests fail
  while it's stale.
- The proofs deploy to a real account: `mise run prove`, `prove-other-repo`, `prove-builds`, `prove-basin`. Each refuses
  to start while its Worker, D1, KV or R2 exists (another machine may be proving it) and deletes what it made, even when
  it fails. Never run the same one on two machines at once.
- A login needs a person to click Allow: never try to do it for them. With `CLOUDFLARE_LOGIN=device`, or in a Claude
  Code cloud session, the tool prints a link with the code already in it; send it to the person.

## Rules

- Never create, ask for or store a Cloudflare API token for a person: the repo's login is a wrangler auth profile, and
  Workers Builds deploys with Cloudflare's own build token. The one token is CI's, as a GitHub secret.
- Never parse a wrangler config here: wrangler reads it (`internal/wconfig`).
- The login's scopes come from the Worker's bindings, which its own wrangler lists at run time, with their types: no
  list of kinds here. `config.BindingScopes` maps each type to the scope whose description in wrangler's own login
  names its product; `go test` fails when wrangler has a type it lacks, or it names a scope wrangler can't ask for, or
  docs/reference.md's table differs. Never add an entry from a guess; set `Proven` only when a proof shows it.
  `cloudflare.toml`'s scopes are only for what isn't a binding.
- Work with the cloud session through pull request comments, never through the owner copying text between devices.
