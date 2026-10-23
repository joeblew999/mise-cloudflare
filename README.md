# mise-cloudflare

[![ci](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml/badge.svg)](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/joeblew999/mise-cloudflare)](https://github.com/joeblew999/mise-cloudflare/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/joeblew999/mise-cloudflare)](go.mod)
[![license](https://img.shields.io/github/license/joeblew999/mise-cloudflare)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/joeblew999/mise-cloudflare.svg)](https://pkg.go.dev/github.com/joeblew999/mise-cloudflare)

Cloudflare without the token mess. Anyone who pulls a repo that uses this gets their own Cloudflare login for it, with
exactly the scopes the repo says, made with one click on Allow; the repo's own wrangler then runs on that login. No API
tokens to make, nothing about Cloudflare in the repo, and the same on a Mac and in a Claude Code cloud session.

One command, `cloudflare`, with its mise tasks and agent skill, built the way
[go-htmx4](https://github.com/joeblew999/go-htmx4)'s `seo` and `intl` are.

## In a repo

**With Go** (go.mod pins it, as `seo` and `intl` are pinned):

```sh
go get -tool github.com/joeblew999/mise-cloudflare/cmd/cloudflare@latest
go tool cloudflare init    # cloudflare.toml, its mise tasks, agent skill, GitHub Actions workflow and Claude Code hook
```

**Without Go** (the released binary, through mise):

```sh
mise use github:joeblew999/mise-cloudflare
cloudflare init            # the same files; each task pins this release, so the repo needs only mise
```

Then the repo's `mise.toml` includes the tasks:

```toml
[tools]
node = "26.11.1"   # runs wrangler
aube = "2.8.1"     # or whichever package manager installs the repo's package.json

[task_config]
includes = [".mise/tasks/cloudflare.toml"]
```

(With Go, add `[settings] idiomatic_version_file_enable_tools = ["go"]` so mise installs go.mod's Go.)
[example/](example/) is a repo set up exactly so.

### What a repo needs, and what checks it

| Needs | Checked by |
|---|---|
| A wrangler config with no account or resource IDs, in any environment; its secrets in `"secrets": { "required": [...] }` | `cloudflare:check` |
| A `previews` block in it, declaring every storage binding the Worker has (D1, KV, R2, …): Worker Previews inherit nothing from production, so a preview without them fails on its first use of one. Bindings with no IDs get their own preview resources | `cloudflare:check` |
| `.dev.vars.example` with a dummy for each required secret; `.dev.vars` and `.env` git-ignored | `cloudflare:check` |
| wrangler pinned exactly in `package.json`; a lockfile, if any, committed (Workers Builds installs from git) | `cloudflare:check`, `cloudflare:builds` |
| The tasks, skill, workflow and Claude Code hook mise-cloudflare wrote, current for its version | `cloudflare:check` |
| For Workers Builds: Cloudflare's GitHub app on the repository, a build token on the account | `cloudflare:builds` says which is missing and how to add it |

### What mise-cloudflare writes into the repo

Like charter's and go-htmx4's own, these are stamped from the copy inside mise-cloudflare, at its version: don't edit
them; after upgrading, write them again (`cloudflare:check` fails while they're out of date).

| File | Written by |
|---|---|
| `.mise/tasks/cloudflare.toml` | `mise run cloudflare:tasks` |
| `.claude/skills/cloudflare/SKILL.md` | `mise run cloudflare:skill` |
| `.github/workflows/cloudflare.yml` (`cloudflare-<folder>.yml` for a Worker in a subfolder) | `mise run cloudflare:workflow` |
| `.claude/hooks/mise-cloudflare-session-start.sh`, and its entry in `.claude/settings.json` | `mise run cloudflare:claude` |

## Where it builds and deploys

Pick one per repo; they don't mix up.

| Where | When you want it | How |
|---|---|---|
| **Locally**: a Mac, or a Claude Code cloud session | Trying something now, by hand | `mise run cloudflare:deploy`, on the repo's own login |
| **Cloudflare**: Workers Builds | Every push deploys by itself; a preview for every other branch, its URL on the pull request; GitHub holds no Cloudflare credential | `mise run cloudflare:builds`, once per repo |
| **GitHub Actions** | GitHub builds and tests, and deploys | `[workflow] deploy = true`, then `mise run cloudflare:workflow` and `mise run cloudflare:ci-token`: the branch deploys production, each pull request gets a preview `pr-<number>`, its URL commented, deleted when it closes |

Whichever deploys, the repo's workflow checks every push and pull request (its `setup` and `test` tasks, then
`cloudflare:check`), with no Cloudflare credential unless GitHub Actions deploys.

## The tasks

| Task | Does | Logs in |
|---|---|---|
| `mise run cloudflare:login` | The repo's own login, with exactly `cloudflare.toml`'s scopes | The first time, and when the scopes change |
| `mise run cloudflare:deploy` | `wrangler deploy` on that login; what it deployed, URLs included, goes to `.wrangler/output.json` | If needed |
| `mise run cloudflare:wrangler -- <args>` | Any wrangler command on that login: `d1 list`, `secret put`, `delete`, `tail`, `preview`, … | If needed |
| `mise run cloudflare:builds` | Workers Builds for the Worker: a push to the branch deploys production, any other branch gets a preview; with `[builds] staging`, its branch deploys staging; `--off` removes it | A cf login, the first time |
| `mise run cloudflare:ci-token` | For GitHub Actions deploying: CI's Cloudflare token, made and stored as the repository's GitHub secrets, never shown; `--off` deletes it | A cf login, the first time |
| `mise run cloudflare:workflow` | Writes the GitHub Actions workflow | Never |
| `mise run cloudflare:claude` | Writes the Claude Code cloud session hook | Never |
| `mise run cloudflare:status` | The repo's login, its account and scopes, stray tokens, this machine's other wrangler logins | Never |
| `mise run cloudflare:check` | The repo's rules: no account or resource IDs (in any environment), no tokens, `.dev.vars` and `.env` git-ignored, `.dev.vars.example` matching `secrets.required`, wrangler pinned | Never |
| `mise run cloudflare:logout` | Logs the repo's logins out on Cloudflare and deletes them | Never |

Each runs `cloudflare …` (or `go tool cloudflare …`); `--json` gives one JSON document. Deploying and deleting are
wrangler's own job, done by the repo's own wrangler; `wrangler dev` needs no login at all.

## The login: where it comes from, where it lives

- **What it may do:** `scopes` in the repo's [cloudflare.toml](example/cloudflare.toml), OAuth scope names as
  `wrangler login --scopes-list` gives them. Without that line: `account:read user:read workers:write workers_kv:write
  workers_scripts:write d1:write`, enough for a Worker with D1, KV and R2.
- **Its name:** `profile` in `cloudflare.toml`, else the Worker's name. It's a
  [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/).
- **Where it lives:** wrangler's folder in the person's home directory, never the repo:
  `~/Library/Preferences/.wrangler/config/<profile>.toml` on a Mac, `~/.config/.wrangler/config/<profile>.toml` on
  Linux. An OAuth token that lasts an hour, and a refresh token wrangler renews it with.
- **The first time:** the first task that needs it asks. On a machine with a browser, Cloudflare's consent page opens,
  listing exactly the repo's scopes: click Allow and pick your account. In a Claude Code cloud session, or with
  `CLOUDFLARE_LOGIN=device`, it prints a link with the code already in it: open it on any device, your phone included.
- **Every time after:** each task compares the scopes the login holds with the repo's. Equal: nothing is asked. A scope
  added or removed: it logs in once more, so the login always holds exactly the repo's scopes.

### Bindings and their permissions

There's no complete list of which permission each binding needs, not even Cloudflare's, so nothing here guesses. The
repo states what its login may do (`scopes`); a binding that needs more fails with wrangler naming the Cloudflare API call
that was refused, and you add the scope. R2 works with `workers:write`: wrangler has no R2 scope.

## Workers Builds: production, staging, previews and pull requests

`mise run cloudflare:builds` connects the Worker to its GitHub repository on Cloudflare: the repository's ids, the
folder (a monorepo's subfolder works), the install command from the lockfile committed to git, `npx wrangler deploy` for
the deploy branch and `npx wrangler preview` for every other branch (`cloudflare.toml`'s `[builds]` overrides any). With
`[builds] staging = "main"`, as go-htmx4 has it, a push to `main` deploys **staging** (the wrangler environment
`env.staging`, its own Worker with its own bindings), a push to `production` deploys **production**, and every other
branch gets a preview. Then:

- **A push to the deploy branch** is built and deployed by Cloudflare, with Cloudflare's own build token.
- **Any other branch** gets a [Worker Preview](https://developers.cloudflare.com/workers/previews/) at
  `https://<branch>-<worker>.<subdomain>.workers.dev`, its own logs and metrics, and Cloudflare's GitHub app posts its
  URL on the pull request and keeps the comment current.
- **A merged pull request's preview** goes when its branch does: `builds` switches on GitHub's "Automatically delete head
  branches" (with `GITHUB_TOKEN` or gh's login).

It needs, once: Cloudflare's GitHub app on the repository (a click on GitHub, by its owner), and a build token on the
account, which Cloudflare makes the first time any repository is connected in its dashboard. Its permissions are
Cloudflare's: Workers Scripts, KV, R2 and Workers Routes. A Worker binding more (D1, Queues, …) has those added to the
token in the dashboard, as Cloudflare documents. Workers Builds' own scope isn't one wrangler's login can have, so
`builds` uses Cloudflare's own CLI, [cf](https://github.com/cloudflare/cf), on a cf login for the repo; the task brings
cf.

## GitHub Actions

`mise run cloudflare:workflow` writes the repo's workflow: on every push and pull request, mise and its pinned tools, the
repo's own `setup` and `test` tasks, then `cloudflare:check`. GitHub Actions never asks anyone anything: the checks need
no Cloudflare account, and in CI (`CI=true`) a task that would log in uses the `CLOUDFLARE_API_TOKEN` secret instead, or
stops at once saying so.

With `[workflow] deploy = true` it deploys too: production from `[workflow] branch` (default `main`), a preview
`pr-<number>` for each pull request from the repository, its URL commented on the pull request (one comment, kept
current), deleted when the pull request closes. It deploys with a Cloudflare API token, the one token in the whole
setup, because GitHub has no one to click Allow. Make it once at
[dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) from the "Edit Cloudflare
Workers" template (adding, say, D1 Edit for a Worker with D1; `[workflow] permissions` notes which), with no expiry
unless you want one; Cloudflare lets no login make API tokens (tried: it leaves `account_api_tokens:create` out of every
grant). `mise run cloudflare:ci-token` then takes it, hidden, checks it against the account's Workers, and stores it and
the account id as the repository's GitHub secrets with gh. It's never shown or written into the repo; `--off` deletes
the secrets. This repo's own [ci.yml](.github/workflows/ci.yml) runs its Go tests and checks its release config;
example/'s [workflow](.github/workflows/cloudflare-example.yml) is stamped by the tool, as any repo's is.

## Why it's safe

- Every wrangler command gets `--profile <the repo's login>`. `wrangler whoami`, which takes no `--profile`, runs on the
  folder's binding, made by its real path (a symlinked folder once fell back to the machine's default login), and the
  task stops unless wrangler names the repo's login as the active one. The machine's own `wrangler login` is never used
  or changed; the proofs check its file is byte for byte the same before and after.
- A `CLOUDFLARE_API_TOKEN` in the shell or in a `.env` would override the login, so every task refuses one (outside CI).
- The repo never holds account IDs, resource IDs, tokens or secret values: `cloudflare:check` fails if it does.
- The Worker's config is wrangler's own reading of it (`unstable_readConfig`, `experimental_readRawConfig`): nothing
  here parses a wrangler config.

## Claude Code cloud sessions

A cloud session starts on a bare Linux machine with only the repo. `mise run cloudflare:claude` writes the hook
[.claude/hooks/mise-cloudflare-session-start.sh](.claude/hooks/mise-cloudflare-session-start.sh) and adds it to
`.claude/settings.json` (keeping what's there): when a session starts, it installs mise and every pinned tool, then runs
the Worker folder's own `setup` task, so the tasks work straight away. This repo's settings name `. example`. On your
own machine it does nothing. After a repository is renamed, attach it to a cloud session by its new name: GitHub
redirects the old one through paths the cloud session's GitHub access refuses. The cloud environment's network settings must allow mise.run,
GitHub, nodejs.org, the npm registry, proxy.golang.org and sum.golang.org.

## Releases

A `v*` tag runs [release.yml](.github/workflows/release.yml): the tests, then
[goreleaser](.goreleaser.yaml) puts the `cloudflare` binary for macOS, Linux and Windows (Intel and ARM) on the
release, and the module is fetched through proxy.golang.org so pkg.go.dev and `go get` have it at once.

## Proofs

| Task | Proves | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests: the rules, the scope comparison, the login's guard, the device link, the tasks for repos with and without Go, the skill, example/ current | No |
| `mise run prove` | In example/: a local Worker with no login; deploy on its own login, bound by its real path; the live D1, KV, R2 and secret; a second deploy that asks nothing and writes nothing into the repo; stray tokens refused; everything deleted again; the machine's default login untouched | Yes |
| `mise run prove-other-repo` | A new repo gets mise-cloudflare from GitHub (`go get -tool`, ref `MISE_CLOUDFLARE_REF`), runs `init`, then the same proof on a login of its own | Yes |
| `mise run prove-builds` | Workers Builds, with Cloudflare's own token: staging from one throwaway branch, production from another, both with KV and R2 working; a preview of a third; the preview gone once its branch is deleted; then all of it removed. main is never touched | Yes |
| `mise run prove-actions` | GitHub Actions deploying: `ci-token` stores CI's token; a throwaway branch with the stamped workflow deploys a Worker nothing else deploys; a pull request gets its `pr-<number>` preview, URL commented; closing it deletes the preview; then all of it removed | Yes |
| `mise run prove-basin` | Basin on a repo's own login: works up to a Basin Pipeline's catalog sink, which needs a stored API token Cloudflare keeps ("The sink token is expired or invalid") | Yes |

They're Go tests in [e2e/](e2e/), run only with `CLOUDFLARE_E2E=1` (the tasks set it). Each refuses to start while its
Worker or its D1, KV or R2 exists, and deletes what it made even when it fails.

## Layout

| Folder | What |
|---|---|
| [cmd/cloudflare/](cmd/cloudflare/) | the command |
| [config/](config/) | `cloudflare.toml` |
| [mise/](mise/) | its tasks, written into a repo by `cloudflare tasks` |
| [skills/](skills/) | its agent skill, installed by `cloudflare skill` |
| [internal/](internal/) | the login (`wrangler`), cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](example/) | a Worker repo that uses it, exactly as any other repo would (`mise run example` stamps it) |
| [e2e/](e2e/) | the proofs, and their Workers in [e2e/testdata/](e2e/testdata/) |
