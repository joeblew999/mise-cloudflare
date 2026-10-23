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
- Never guess which permissions a binding needs: there's no complete list, not even Cloudflare's. The repo states them
  (`cloudflare.toml`'s scopes).
- Work with the cloud session through pull request comments, never through the owner copying text between devices.
