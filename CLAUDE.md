# mise-cloudflare

A repo's own Cloudflare login, and the repo's own wrangler on it: the `mise-cloudflare` binary, which a repo pins with
mise, with its mise tasks and agent skill. README.md is the whole picture; read it first.

## Working on it

Every task runs at the top, never in example/, and each runs the steps before it (mise.toml says the order):
`example`, `test`, `smoke`, `prove`, `builds`. Run the furthest you need, here, never waiting on GitHub:

- `mise run smoke` after every change: example/ set up, every check that needs no account (run for real), and a new
  repo set up as a user does it. GitHub's CI runs the same.
- `mise run install`: this checkout's build as the mise version `local`, for any repo on this machine (`mise use
  github:joeblew999/mise-cloudflare@local` there). Try a change in another repo that way, never by waiting on a release.
- Releases are automatic: a merge to main that changes the tool (cmd/, internal/, templates/, go.mod) tags the
  next patch version and releases it (release.yml), so the release is always main.
- `mise run prove`: one real deploy of example/ from this machine, no GitHub, a few minutes, after a change to how it
  deploys. `mise run builds` is Workers Builds deploying every push, through GitHub, about 25 minutes. Each
  refuses to start while the Worker, its D1, KV or R2 exists and deletes what it made, even when it fails;
  `mise run clean` deletes what a stopped run left. Previews, staging and production are shown working by
  Cloudflare's own checks on the pull requests: never build a harness for those.
- Names match all the way through: `mise run x` runs `TestX` in `e2e/x_test.go`; this repo's tasks are one word. A
  user's repo has the tasks `init` writes (`cloudflare`, `cloudflare:<step>`, from `templates/`); example/ has those and
  its own `setup`, and mise there never sees this repo's (`MISE_CEILING_PATHS`).
- example/ is a user's repo of its own: its own git repo (`example/.git`, local, made by `mise run 00-git-init`), so
  everything init writes for it is inside it. Nothing of example/'s goes in this repo's own `.github/` or `.claude/`;
  this repo's `.claude/settings.json` runs `templates/session-start.sh` for itself.
- A login needs a person to click Allow: never try to do it for them. With `CLOUDFLARE_LOGIN=device`, or in a Claude
  Code cloud session, the tool prints a link with the code already in it; send it to the person.

## Rules

- Never create, ask for or store a Cloudflare API token for a person: the repo's login is a wrangler auth profile, and
  Workers Builds deploys with Cloudflare's own build token. CI only checks.
- Never parse a wrangler config here: wrangler reads it (`internal/wconfig`).
- The repo's login asks for wrangler's own login's scopes, read from the repo's wrangler (`wrangler login
  --scopes-list`): never a list of binding kinds or a binding-to-scope table here. There's no settings file: every choice is a
  task's flag, kept by Cloudflare's Workers Builds setup.
- Work with the cloud session through pull request comments, never through the owner copying text between devices.
