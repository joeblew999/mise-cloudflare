# mise-cloudflare

[![ci](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml/badge.svg)](https://github.com/joeblew999/mise-cloudflare/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/joeblew999/mise-cloudflare)](https://github.com/joeblew999/mise-cloudflare/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/joeblew999/mise-cloudflare)](go.mod)
[![license](https://img.shields.io/github/license/joeblew999/mise-cloudflare)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/joeblew999/mise-cloudflare.svg)](https://pkg.go.dev/github.com/joeblew999/mise-cloudflare)

Cloudflare Workers for any repo, from mise: run it locally, deploy it, then a preview for every pull request, staging
and production. The same on your machine and in Claude Code. No API tokens to make; the first time, you click **Allow**.

This repo is two things, kept apart:

| | What | Where |
|---|---|---|
| **For other repos** | The `mise-cloudflare` binary, and everything `init` writes into a repo: its tasks, its check workflow, its Claude Code hook and skill, and its docs | [templates/](templates/), [cmd/mise-cloudflare/](cmd/mise-cloudflare/), [internal/](internal/) |
| **For this repo** | The tasks for working on it, the steps that set a repo up, the examples, the tests, its CI and releases. This README | [mise.toml](mise.toml), [example.mise.toml](example.mise.toml), [examples/](examples/), [root/](root/), [.github/workflows/](.github/workflows/) |

Nothing for this repo is ever in `templates/`, and nothing in `templates/` is this repo's own. The Go says it too:
`cmd/mise-cloudflare` is the one entry point that ships (the binary other repos run), with `internal/` and
`templates/`; everything this repo runs for itself in Go is under `root/`.

## Use it in your repo

In a repo that has a Worker (a `wrangler.jsonc`, and wrangler in its `package.json`):

```sh
mise use github:joeblew999/mise-cloudflare
mise-cloudflare init
mise run cloudflare
```

`init` writes everything your repo gets, its own docs among them: `docs/mise-cloudflare/README.md`, with every
command, the logins, previews, staging and production. That page is [templates/docs/README.md](templates/docs/README.md)
here, and [examples/reference/docs/mise-cloudflare/](examples/reference/docs/mise-cloudflare/README.md) shows it as a
repo has it.

## Working on mise-cloudflare

Everything is driven from this repo's [mise.toml](mise.toml), run at the top, never inside an example, and each task
runs what it needs first: run the furthest you need, here, never waiting on GitHub.

### The steps: a repo, set up

[example.mise.toml](example.mise.toml), which mise.toml includes, is the steps: what a person does to a repo, in order.
One name all the way through: from 04 on, step `example:nn-x` runs the repo's task `cloudflare:x`, which runs
`mise-cloudflare x`, which is `main.go`'s `case "x"` calling `r.x` in `x.go` (`TestChain`).

| Step | Runs in the repo |
|---|---|
| `example:00-git-init` | `git init`: a git repo of its own |
| `example:01-mise-install` | `mise install`: what it pins, mise-cloudflare at `local` among them |
| `example:02-setup` | its own `setup` task: wrangler from its `package.json` |
| `example:03-init` | `mise-cloudflare init`: everything from `templates/` written into it |
| `example:04-check` | `cloudflare:04-check` |
| `example:05-show` | `cloudflare:05-show` |
| `example:06-serve` | `cloudflare:06-serve` |
| `example:07-set-secrets` | `cloudflare:07-set-secrets` |
| `example:08-deploy` | `cloudflare:08-deploy` |
| `example:09-connect-github` | `cloudflare:09-connect-github`: it needs the repo on GitHub, which an example's own git has not, so it stops and says so |
| `example:10-set-preview-secrets` | `cloudflare:10-set-preview-secrets` |
| `example:11-preview` | `cloudflare:11-preview` |
| `example:12-login` | `cloudflare:12-login` |
| `example:13-delete` | `cloudflare:13-delete`: 08 and 11 undone |
| `example:14-logout` | `cloudflare:14-logout` |
| `example:15-clean` | `cloudflare:15-clean`: 06's local state undone |

`example:verify` runs in the repo what its own GitHub workflow runs there: the workflow only checks out and installs mise,
then runs `mise run cloudflare:verify`, so `mise run example:verify` here does on this machine exactly what GitHub does in
that repo. `mise tasks` lists each with what it does. A step that asks you something (`06-dev`, `07-secret`, `10-secret`) runs on
your terminal, and no step depends on one, so nothing asks you twice.

The steps run on a repo in [examples/](examples/), two ways, as a person meets them:

- **An existing repo:** [examples/reference/](examples/reference/), committed, the default: `mise run example:05-show`.
- **A new repo:** `mise run example:create` makes `examples/new/` from only what a person brings to a new repo, copied from
  the reference: the Worker's code, its `package.json` and lockfile, and the repo's `mise.toml`, with a Worker name of
  its own (`mise-cloudflare-new`). The steps then do the rest to it: `EXAMPLE=new mise run example:05-show`
  gives it its git, its tools, its wrangler and everything `init` writes. It is never committed, and each
  `mise run example:create` starts it again.

### The examples

Each folder in [examples/](examples/) is another repo, a user's, inside this one ([examples/README.md](examples/README.md)):
its own git, its own `mise.toml` pinning mise-cloudflare at `local` (this checkout's build, which `mise run tool:install`
links), and what `init` writes, all inside it. mise there reads only the example's own `mise.toml`
(`MISE_CEILING_PATHS`), and nothing of an example's goes in this repo's own `.github/` or `.claude/`
(`mise run root:check`); the reference has what this version's `init` writes (`mise run example:04-check`). This repo
tracks the reference's files; `mise run root:deploy-every-push` runs in a checkout of this repo, where the reference is a subfolder,
as Cloudflare builds it from GitHub.

### This repo's tasks

Every task is named noun:verb, what it acts on and then what it does, as a repo's own are (`cloudflare:08-deploy`):
`example:` acts on a repo in `examples/` (the steps, `example:create`, and the end-to-end tests on it), `tool:` on
mise-cloudflare itself, and `root:` on this repo itself.

| Task | What | When | Cloudflare |
|---|---|---|---|
| `mise run example:create` | A new repo, `examples/new/`, from only what a user brings, pinning this checkout's build (`local`), or a release with `--version`; the steps do the rest | To try the steps on a new repo | No |
| `mise run example:try` | The steps that need no account, as a person takes them, on `examples/<EXAMPLE>`: `04-check`'s rules, `05-show`'s next step, `12-login` stopping at once in CI, the Worker locally with `06-serve` | After every change | No |
| `mise run example:deploy` | The steps on Cloudflare, on `examples/<EXAMPLE>`, a few minutes: `12-login`, `07-set-secrets`, `08-deploy`, the live D1, KV, R2 and secret, stray tokens refused by `08-deploy`, `13-delete`; everything deleted after | After a change to how it deploys | Yes |
| `mise run example:clean` | `examples/<EXAMPLE>` back to the start, whatever a stopped `example:deploy` or `root:deploy-every-push` left: nothing of its Worker on Cloudflare (its Workers Builds setup, the root's `e2e-*` branches, then `13-delete`), and none of its local state (`15-clean`) | After a stopped run | Yes |
| `mise run tool:build` | This checkout's build, `bin/mise-cloudflare` | Any task needs it | No |
| `mise run tool:install` | That build as the mise version `local`, for the examples and any repo on this machine: `mise use github:joeblew999/mise-cloudflare@local` there | To try a change in another repo, never by waiting on a release | No |
| `mise run tool:test` | The tool's own checks, run for real: `go vet`, its Go tests, the release config. They need no example | After every change | No |
| `mise run tool:release` | A release from the commit checked out, as `release.yml` runs it: the commit's `v*` tag or the next patch version, goreleaser's binaries on the tag's release, a new repo pinning it and tried, the version on proxy.golang.org. `--dry-run` tags and publishes nothing | Never by hand: a merge to main runs it | GitHub |
| `mise run root:check` | The root against what it gives other repos, once the reference is set up: the examples and the root keep to themselves, the steps line up with a repo's tasks, the tool reads the reference's config and scopes through its own wrangler | After every change | No |
| `mise run root:deploy-every-push` | Workers Builds deploying every push of the root, through GitHub, about 25 minutes: production from a throwaway branch, a branch's preview with its own D1, KV, R2 and secret, gone with its branch | After a change to `connect-github` | Yes, and GitHub |

Every task here runs the Go of its name, by one rule (`mise run root:check` checks it): `noun:verb` runs the test
`TestNounVerb` in `root/e2e/noun_verb_test.go`, or the program `root/noun/verb`. So `mise run example:try` is
`TestExampleTry` in [root/e2e/example_try_test.go](root/e2e/example_try_test.go), `root:deploy-every-push` is
`TestRootDeployEveryPush` in [root/e2e/root_deploy_every_push_test.go](root/e2e/root_deploy_every_push_test.go),
and `example:create` is [root/example/create](root/example/create/main.go); what the tests share is in
[root/e2e/e2e_test.go](root/e2e/e2e_test.go).

A test does what a person does, in their order: the root's numbered steps, one at a time from 00
(`rootStep("07-set-secrets")` is `mise run example:07-set-secrets` at the root), and each step runs the repo's own task
of its number (`mise run cloudflare:07-set-secrets`). The one thing a test runs in the repo itself is
`repoTask("run-wrangler")`, to look at what is on the account; the rest it looks at as a person does: the Worker's
URL, git, Cloudflare's builds. `root:deploy-every-push` runs in a checkout of the root, as Cloudflare builds it from
GitHub, where the reference is a folder of the root's own git: its steps there are 01, 02, then 12 on (00, 03 and 04
would make it a repo of its own, or put its workflow in the root's). `example:deploy` and `root:deploy-every-push`
start by deleting whatever a stopped run left of the Worker on Cloudflare (as `example:clean` does), and delete what
they made with `13-delete` even when they fail. Previews, staging and production are shown working by Cloudflare's own checks on the pull requests:
never build a harness for those.

### Layout

| Folder | Whose | What |
|---|---|---|
| [templates/](templates/) | other repos' | every file `init` writes into a repo: the tasks, the workflow, the Claude Code hook, the agent skill, the docs |
| [cmd/mise-cloudflare/](cmd/mise-cloudflare/) | other repos' | the command: a file per command, `x.go` holding `r.x`, which `main.go`'s `case "x"` calls |
| [internal/](internal/) | other repos' | the login (`wrangler`), Cloudflare's cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [root/example/create/](root/example/create/) | this repo's | `mise run example:create` |
| [root/tool/release/](root/tool/release/) | this repo's | `mise run tool:release` |
| [examples/](examples/) | this repo's | repos of their own that use it: the reference, and the new one |
| [.claude/](.claude/), [.github/workflows/](.github/workflows/) | this repo's | its own Claude Code hook and its own workflows (`ci`, `release`); other repos get theirs from `templates/` |
| [root/e2e/](root/e2e/) | this repo's | the end-to-end tests, a file each, named for its task: `example_try`, `example_deploy`, `example_clean`, `root_check`, `root_deploy_every_push` |

### CI and releases

Shell steps only, no JavaScript actions, and no Cloudflare credentials. Each workflow checks out and installs mise,
then only runs the root mise, as on any machine:

- [ci.yml](.github/workflows/ci.yml), on every push and pull request: `mise run root:check`; on the reference,
  `mise run example:try` and `mise run example:verify` (what its own workflow runs); then a new repo,
  `mise run example:create`, and the same on it, with `EXAMPLE=new`.
- [release.yml](.github/workflows/release.yml), on every change to the tool merged to main: `mise run tool:release`. It
  tags the next patch version, [goreleaser](.goreleaser.yaml) puts the `mise-cloudflare` binary for macOS, Linux and
  Windows on the release, then a new repo pins that release and is tried as a person tries it
  (`mise run example:create --version`, then `EXAMPLE=new mise run example:try`). So the release is always main.
  `mise run tool:release --dry-run` does it here without tagging or publishing anything.

Before merging a change to how it deploys, run `mise run example:deploy`.

### Rules

- Never create, ask for or store a Cloudflare API token for a person: the repo's login is a wrangler auth profile, and
  Workers Builds deploys with Cloudflare's own build token. CI only checks.
- A login needs a person to click Allow: never try to do it for them. With `CLOUDFLARE_LOGIN=device`, or in a Claude
  Code cloud session, the tool prints a link with the code already in it; send it to the person.
- Never parse a wrangler config here: wrangler reads it (`internal/wconfig`).
- The repo's login asks for wrangler's own login's scopes, read from the repo's wrangler (`wrangler login
  --scopes-list`): never a list of binding kinds or a binding-to-scope table here. There's no settings file: every
  choice is a task's flag, kept by Cloudflare's Workers Builds setup.
- Work with the cloud session through pull request comments, never through the owner copying text between devices.

This README is the one place for all of it, for people and agents alike: CLAUDE.md only loads it (`@README.md`).
