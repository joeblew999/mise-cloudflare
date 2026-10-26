# mise-cloudflare

[![ci](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml/badge.svg)](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/joeblew999/mise-cloudflare)](https://github.com/joeblew999/mise-cloudflare/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/joeblew999/mise-cloudflare)](go.mod)
[![license](https://img.shields.io/github/license/joeblew999/mise-cloudflare)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/joeblew999/mise-cloudflare.svg)](https://pkg.go.dev/github.com/joeblew999/mise-cloudflare)

Cloudflare Workers for any repo, from mise: run it locally, deploy it, then a preview for every pull request, staging
and production. The same on your machine and in Claude Code. No API tokens to make; the first time, you click **Allow**.

## Set up

In a repo that has a Worker (a `wrangler.jsonc`, and a `package.json` that pins wrangler):

```sh
mise use github:joeblew999/mise-cloudflare   # or, in a Go repo: go get -tool github.com/joeblew999/mise-cloudflare/cmd/cloudflare
mise-cloudflare init                         # or: go tool cloudflare init
```

Commit what `init` wrote. There's nothing to edit, then or ever. `init` also fixes what it can for `check`: it
git-ignores `.dev.vars`, `.env` and `.cloudflare/`, and pins wrangler to the version installed.

## Use it

One task does everything, and tells you what to run:

```sh
mise run cloudflare
```

```
my-worker
  ✓ 1. Run it locally     mise run cloudflare:dev
  ✓ 2. Deploy it          mise run cloudflare:deploy   https://my-worker.example.workers.dev
  · 3. Deploy every push  mise run cloudflare:github   a preview for every pull request, then staging and production
  login: my-worker, on My Account
next: mise run cloudflare:github
```

| Step | Run | You're asked |
|---|---|---|
| 1. Run it locally, with local D1, KV and R2 | `mise run cloudflare:dev` | Nothing: no account needed |
| 2. Deploy it yourself | `mise run cloudflare:deploy` | **Allow**, the first time |
| 3. Have Cloudflare deploy every push, once | `mise run cloudflare:github` | **Allow** for Cloudflare's own CLI, the first time |

After step 3 there's nothing to run: push a branch and open a pull request, and Cloudflare builds a preview and puts
its link on the pull request; merge it, and Cloudflare deploys production. For staging too,
`mise run cloudflare:github --staging main`: then merging into `main` deploys staging, and merging `main` into
`production` deploys production. To have GitHub Actions deploy instead of Cloudflare, `mise run cloudflare actions`.

`mise run cloudflare help` lists every command: a preview by hand, any wrangler command on the repo's login, checking
the repo, and updating after an upgrade.

## What init adds

| File | What |
|---|---|
| `.mise/tasks/mise-cloudflare.toml` | The one task, `mise run cloudflare` |
| `.github/workflows/cloudflare.yml` | Checks the repo on every push and pull request; it deploys only after `mise run cloudflare actions` |
| `.claude/skills/cloudflare/SKILL.md` | How Claude works on the Worker |
| `.claude/hooks/mise-cloudflare-session-start.sh` | Sets a Claude Code cloud session up when it starts |

None is for editing: `mise run cloudflare update` writes them again after upgrading. [example/](example/) is a repo set
up this way.

## Logins and tokens

| | What it is | Where |
|---|---|---|
| **The repo's login** | What you get by clicking Allow: kept on your machine, never in the repo | Your machine, Claude Code |
| **Cloudflare's build token** | Cloudflare's own, which never leaves Cloudflare | Cloudflare deploying every push |
| **A GitHub Actions token** | Made once in Cloudflare's dashboard, kept as a GitHub secret | Only if GitHub Actions deploys |

A `CLOUDFLARE_API_TOKEN` in your shell or a `.env` would override the repo's login, so the commands refuse one.

## Claude Code

The skill teaches Claude these commands. In a Claude Code cloud session, the hook installs everything when the session
starts, and a login comes as a link you open on your phone.

## More

- [Reference](docs/reference.md): every command, what `check` checks, how the login works, previews, Workers Builds and
  GitHub Actions in full.
- [Testing](docs/testing.md): the Go tests after every change, and one real deploy before a release.
