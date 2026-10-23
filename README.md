# mise-cloudflare

[![ci](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml/badge.svg)](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/joeblew999/mise-cloudflare)](https://github.com/joeblew999/mise-cloudflare/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/joeblew999/mise-cloudflare)](go.mod)
[![license](https://img.shields.io/github/license/joeblew999/mise-cloudflare)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/joeblew999/mise-cloudflare.svg)](https://pkg.go.dev/github.com/joeblew999/mise-cloudflare)

Cloudflare Workers for any repo, from mise: local development, a preview for every pull request, staging and
production, the same on your machine and in Claude Code. No API tokens to make (unless you have GitHub Actions deploy).
The first time, you click **Allow** once.

## Set up

In a repo that has a Worker (a `wrangler.jsonc` and a `package.json` that pins wrangler):

```sh
mise use github:joeblew999/mise-cloudflare   # or, in a Go repo: go get -tool github.com/joeblew999/mise-cloudflare/cmd/cloudflare
cloudflare init                              # or: go tool cloudflare init
mise run cloudflare:check                    # says exactly what, if anything, the repo still needs
```

`init` adds these to the repo ([example/](example/) is a repo set up this way):

| File | What | Edit it? |
|---|---|---|
| `cloudflare.toml` | The repo's settings, every one commented at its default | Yes |
| `.mise/tasks/mise-cloudflare.toml` | The tasks: `mise tasks` lists them | No |
| `.claude/skills/cloudflare/SKILL.md` | How Claude works on the Worker | No |
| `.github/workflows/cloudflare.yml` | Checks the repo on every push and pull request | No |
| `.claude/hooks/mise-cloudflare-session-start.sh` | Sets a Claude Code cloud session up | No |

After upgrading mise-cloudflare, `mise run cloudflare:update` writes the ones you don't edit again.

## Develop locally

```sh
mise run cloudflare:dev     # wrangler dev, on http://localhost:8787, with local D1, KV and R2
```

No Cloudflare account needed.

## Deploy by hand

```sh
mise run cloudflare:deploy
```

The first time, your browser opens Cloudflare's page: click **Allow** and pick your account. That's the repo's own
login, kept on your machine (in a Claude Code cloud session, you get a link to tap on your phone instead). After that,
it asks again only when an upgraded wrangler needs more.

## Preview every pull request, then staging and production

Set it up once:

```sh
mise run cloudflare:builds
```

From then on, Cloudflare builds and deploys every push by itself:

| You | Cloudflare |
|---|---|
| Push a branch, open a pull request | Builds a **preview** at its own URL, and posts the link on the pull request |
| Merge the pull request into `main` | Deploys **staging**, and deletes the pull request's preview |
| Merge `main` into `production` | Deploys **production** |

For staging, add `staging = "main"` under `[builds]` in `cloudflare.toml`; without it, `main` deploys production
directly.

- **Previews' data:** previews get a KV, D1 and R2 of their own, shared by all previews, never production's or
  staging's. Cloudflare gives previews none, so `builds` makes them (`<worker>-previews-…`), and nothing goes into your
  repo. Add a KV, D1 or R2 binding later? Run `mise run cloudflare:previews` once.
- **Cloudflare's GitHub app** needs to be on the repository: if it isn't, `builds` says so.

Rather have GitHub Actions deploy instead of Cloudflare? Set `[workflow] deploy = true`; the
[reference](docs/reference.md#github-actions) has the one-time steps.

## What you'll be asked, and when

| When | What |
|---|---|
| First deploy on your machine | **Allow**, in your browser |
| `cloudflare:builds`, once per repo | **Allow**, once more (Cloudflare's own CLI asks for Workers Builds) |
| wrangler is upgraded and asks for more, or you change `scopes` in `cloudflare.toml` | **Allow**, once |
| GitHub Actions deploying instead, once per repo | Make CI's token from the link `cloudflare:ci-token` prints |
| Anything else: deploys, previews, merges, GitHub Actions | Nothing |

## Logins and tokens

| | What it is | Where | What you do |
|---|---|---|---|
| **The repo's login** | What you get by clicking Allow: a token on your machine that can do what wrangler's own login can | Your machine, Claude Code | Allow, once |
| **Cloudflare's build token** | Cloudflare's own, which never leaves Cloudflare | Workers Builds: previews, staging, production | Nothing |
| **CI's API token** | A secret made in Cloudflare's dashboard | GitHub Actions, only if you have it deploy | Make it once, from the link `cloudflare:ci-token` gives |

A `CLOUDFLARE_API_TOKEN` in your shell or a `.env` would override every login, so outside CI the tasks refuse one.
The [reference](docs/reference.md#cloudflares-three-kinds-of-credential) has the whole story.

## Claude Code

The Claude skill (`.claude/skills/cloudflare`) teaches Claude to use these tasks. In a Claude Code cloud session, the
setup hook installs everything when the session starts, and any login comes as a link you open on your phone.

## More

- [Reference](docs/reference.md): every task and setting, what `cloudflare:check` checks, the files `init` writes, how
  the logins work and why they're safe, Workers Builds and GitHub Actions in full.
- [Testing](docs/testing.md): the Go tests after every change, and one real deploy before a release.
