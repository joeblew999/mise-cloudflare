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
| **For other repos** | The `mise-cloudflare` binary, and the template: a whole repo you can drop anywhere and run, whose tool half `init` writes into a repo (its tasks, its check workflow, its Claude Code hook and skill, its docs, its `.gitignore`) | [templates/](templates/), [cmd/mise-cloudflare/](cmd/mise-cloudflare/), [internal/](internal/) |
| **For this repo** | The tasks for working on it, the end-to-end walk, the release, its CI. This README | [mise.toml](mise.toml), the `*_test.go` at the top, [.github/workflows/](.github/workflows/) |

Nothing for this repo is ever in `templates/`, and nothing in `templates/` is this repo's own. The Go says it too:
`cmd/mise-cloudflare` is the one entry point that ships (the binary other repos run), with `internal/` and
`templates/`; everything this repo runs for itself in Go is a test at the top (the `*_test.go`, package `e2e`), which
the root's tasks run.

## Use it in your repo

In a repo, new or with a Worker of its own (a `wrangler.jsonc`, and wrangler in its `package.json`):

```sh
mise exec github:joeblew999/mise-cloudflare@<version> -- mise-cloudflare init
mise trust --all && mise install
mise run cloudflare
```

`init` pins mise-cloudflare in the repo's `.mise/config.toml`, at the release that ran it, so `mise install` gets that
release from GitHub. A repo with no Worker yet can take the template's: `mise run cloudflare:template`, which never
writes over a file. In this checkout the template pins a release the same way, and `mise run tool:install` overrides
it on this machine only, with this checkout's build (`templates/.mise/config.local.toml`, never committed).

`init` writes everything your repo gets, its own docs among them: `docs/mise-cloudflare/README.md`, with every
command, the logins, previews, staging and production. That page is [templates/docs/mise-cloudflare/README.md](templates/docs/mise-cloudflare/README.md)
here, in the template, just as a repo has it.

## Working on mise-cloudflare

Everything is driven from this repo's [mise.toml](mise.toml), run at the top, and each task runs what it needs first:
run the furthest you need, here, never waiting on GitHub. `mise tasks` lists each with what it does.

### The walk

A repo's steps are its own numbered tasks, `cloudflare:03-init` to `cloudflare:15-clean`
([templates/.mise/tasks/mise-cloudflare.toml](templates/.mise/tasks/mise-cloudflare.toml)). One name all the way
through: `cloudflare:08-deploy` runs `mise-cloudflare deploy`, which is `main.go`'s `case "deploy"` calling `r.deploy`
in `deploy.go`. Steps 00 to 02 come before them: the repo's git, `mise install`, its own `setup`.

The end-to-end test, `TestWalk`, walks them in their order, through the repo's own mise file, as a person takes them,
each step with its check, as far as a stage:

| Stage | As far as | Needs |
|---|---|---|
| `serve` | `06-serve`: the Worker locally, with local D1, KV and R2; on the way, `04-check`'s rules and what the repo's GitHub workflow runs (`cloudflare:verify`) | Nothing |
| `deploy` | `08-deploy` on the repo's own login, the live D1, KV, R2 and secret, stray tokens refused, then `13-delete` | A Cloudflare account; the first time, an Allow |
| `connect-github` | `09-connect-github`: Workers Builds builds production from a throwaway branch and a preview for a pull request's branch, gone with its branch | GitHub too |

It walks two repos:

- **The template**, [templates/](templates/), in place; for `connect-github`, in a checkout of this repo at HEAD, as
  Cloudflare builds it from GitHub.
- **A new repo**, made as a person makes one: an empty folder (`mise-cloudflare-new`) with its own git,
  `mise-cloudflare init`, then its own `cloudflare:template`, which writes the template's Worker named for the folder.
  The walk makes it in a folder of its own, and it goes when the walk ends. Its `init` runs at this checkout's build
  (`local`), or at a release with `--version`, as the release tries it, and pins itself in `.mise/config.toml`. Not
  `connect-github`: it has no GitHub.

Each walk first says what it runs: the repo's mise tools, mise-cloudflare's build, the installed wrangler. It runs in
the repo on that repo's own mise only (`MISE_CEILING_PATHS`). A walk that deploys starts by deleting whatever a stopped
one left on Cloudflare, and deletes what it made even when it fails. Besides the steps it runs only
`cloudflare:run-wrangler`, to look at the account; the rest it looks at as a person does: the Worker's URL, git,
Cloudflare's builds. Previews, staging and production are shown working by Cloudflare's own checks: never build a
harness for those.

### This repo's tasks

Every task is named noun:verb, what it acts on and then what it does, as a repo's own are (`cloudflare:08-deploy`):
`template:` acts on the template, `example:` on a copy stamped out from it, `tool:` on mise-cloudflare itself, and
`root:` on this repo itself.

| Task | What | When | Cloudflare |
|---|---|---|---|
| `mise run template:e2e-serve`, `mise run example:e2e-serve` | The walk as far as `serve`, on the template or on a stamped copy (`--version` pins a release) | After every change | No |
| `mise run template:e2e-deploy`, `mise run example:e2e-deploy` | The walk as far as `deploy`, about 3 minutes | After a change to how it deploys | Yes |
| `mise run template:e2e-connect-github` | The walk on the template as far as `connect-github`, about 7 minutes (it waits on Cloudflare's builds) | After a change to `connect-github` | Yes, and GitHub |
| `mise run template:check-mise` | `TestMise` on the template's mise file and the tool's Go (below) | After a change to a task or a command | No |
| `mise run template:update` | The template's own pins at their newest releases, by its own `cloudflare:update`: every pin listed, the moved ones before → after | To move its pins; then the walks | No |
| `mise run root:update` | This repo's own pins (this `mise.toml`'s tools, `go.mod`'s modules) at their newest, by the tool's own update | To move its pins; then `tool:test` and the walks | No |
| `mise run tool:build` | This checkout's build, `bin/mise-cloudflare` | Any task needs it | No |
| `mise run tool:install` | That build as the mise version `local`, for any repo on this machine (`mise use github:joeblew999/mise-cloudflare@local` there), and the template's here: `templates/.mise/config.local.toml`, never committed, overrides the release its `.mise/config.toml` pins | To try a change in another repo, never by waiting on a release | No |
| `mise run tool:test` | The tool's own checks: `go vet`, its Go tests, the release config | After every change | No |
| `mise run tool:release` | A release from the commit checked out, as `release.yml` runs it: the commit's `v*` tag or the next patch version, goreleaser's binaries on the tag's release, a copy stamped out pinning it and walked (`example:e2e-serve --version`), the version on proxy.golang.org. `--dry-run` tags and publishes nothing | Never by hand: a merge to main runs it | GitHub |

[mise_test.go](mise_test.go), `TestMise`, checks a mise file against the Go it runs, both ways, for any repo (the
caller says which: `template:check-mise` points it at the template's). Each task's command is a command's file in
`cmd/mise-cloudflare`, and each command a task's; each task the walk names is one; every task declares its flags and
arguments with mise's `usage` and passes them by name. The root's `mise.toml` is the control plane, and isn't checked.

### Layout

| Path | Whose | What |
|---|---|---|
| [templates/](templates/) | other repos' | the template: a whole repo you can drop anywhere and run, a sample Worker plus the tool's half, which `init` writes into a repo at the same paths (the tasks, the workflow, the Claude Code hook and settings, the agent skill, the docs, the `.gitignore`); [templates.go](templates/templates.go) lists that half |
| [cmd/mise-cloudflare/](cmd/mise-cloudflare/) | other repos' | the command: a file per command, `x.go` holding `r.x`, which `main.go`'s `case "x"` calls |
| [internal/](internal/) | other repos' | the login (`wrangler`), Cloudflare's cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [e2e_walk_test.go](e2e_walk_test.go), [e2e_steps_test.go](e2e_steps_test.go), [e2e_mise_test.go](e2e_mise_test.go), [e2e_cloudflare_test.go](e2e_cloudflare_test.go) | this repo's | the walk: `TestWalk` and its stages; the steps, each with its check; the repo and the calls into its own mise file (the stamped copy and the checkout made here); what it looks at on Cloudflare |
| [tool_test.go](tool_test.go) | this repo's | `TestToolRelease`, the release: only `mise run tool:release` runs it |
| [mise_test.go](mise_test.go) | this repo's, for now | the mise ↔ Go checker, `TestMise` |
| [.claude/](.claude/), [.github/workflows/](.github/workflows/) | this repo's | its own Claude Code hook and its own workflows (`ci`, `release`); other repos get theirs from `templates/` |

### CI and releases

Shell steps only, no JavaScript actions, and no Cloudflare credentials. Each workflow checks out and installs mise,
then only runs the root mise, as on any machine:

- [ci.yml](.github/workflows/ci.yml), on every push and pull request (not the walk's throwaway `e2e-*` branches):
  `mise run tool:test`, `mise run template:check-mise`, then the walk as far as `serve` on the template
  (`mise run template:e2e-serve`) and on a stamped copy (`mise run example:e2e-serve`).
- [release.yml](.github/workflows/release.yml), on every change to the tool merged to main: `mise run tool:release`. It
  tags the next patch version, [goreleaser](.goreleaser.yaml) puts the `mise-cloudflare` binary for macOS, Linux and
  Windows on the release, then a copy stamped out pinning that release is walked (`mise run example:e2e-serve
  --version`). So the release is always main. `mise run tool:release --dry-run` does it here without tagging or
  publishing anything.

Before merging a change to how it deploys, run `mise run template:e2e-deploy`, and `mise run
template:e2e-connect-github` for a change to `connect-github`.

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
