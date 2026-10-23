# mise-cloudflare

A repo's own Cloudflare login, and the repo's own wrangler on it, as a Go command (`go tool cloudflare`, or the released
`cloudflare` binary for repos without Go) with its mise tasks and agent skill, built the way go-htmx4's `seo` and `intl`
are. README.md is the whole picture; read it first.

## Working on it

- `mise run test`: the Go tests, no Cloudflare account. Run them after every change.
- `mise run example`: runs `go tool cloudflare init` in `example/`, as any repo is set up. The tests fail while it's
  stale.
- `mise run prove`: one real deploy of example/, before a release, a few minutes. It refuses to start while its Worker,
  D1, KV or R2 exists and deletes what it made, even when it fails. Previews, staging and production are shown working
  by Cloudflare's own checks on the pull requests, and GitHub Actions by its runs: never build a harness for those.
- A login needs a person to click Allow: never try to do it for them. With `CLOUDFLARE_LOGIN=device`, or in a Claude
  Code cloud session, the tool prints a link with the code already in it; send it to the person.

## Rules

- Never create, ask for or store a Cloudflare API token for a person: the repo's login is a wrangler auth profile, and
  Workers Builds deploys with Cloudflare's own build token. The one token is CI's, as a GitHub secret.
- Never parse a wrangler config here: wrangler reads it (`internal/wconfig`).
- The repo's login asks for wrangler's own login's scopes, read from the repo's wrangler (`wrangler login
  --scopes-list`): never a list of binding kinds or a binding-to-scope table here. `cloudflare.toml`'s scopes replace
  them exactly, for a narrower login.
- Work with the cloud session through pull request comments, never through the owner copying text between devices.
