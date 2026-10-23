# Reference

The details behind the [README](../README.md): what each task does, what a repo needs and what checks it, the files
mise-cloudflare writes, how the logins work, Workers Builds and GitHub Actions in full, and why it's safe.

## What a repo needs, and what checks it

| Needs | Checked by |
|---|---|
| A wrangler config with no account or resource IDs, in any environment; its secrets in `"secrets": { "required": [...] }` | `cloudflare:check` |
| For Worker Previews (`cloudflare:builds`, or `[workflow] deploy`), a `previews` block in it (`"previews": {}` will do). Previews inherit nothing from production, and Cloudflare creates no KV, D1 or R2 for them (only Durable Objects and Containers), so `cloudflare:previews` makes their own and the block leaves KV, D1 and R2 out (one it names, it must name an existing resource of). Every other storage binding (Queues, Vectorize, …) goes in it, pointed at a preview-safe resource. A repo with no previews needs no block | `cloudflare:check`, `cloudflare:builds` |
| `.dev.vars.example` with a dummy for each required secret; `.dev.vars` and `.env` git-ignored | `cloudflare:check` |
| wrangler pinned exactly in `package.json`; a lockfile, if any, committed (Workers Builds installs from git) | `cloudflare:check`, `cloudflare:builds` |
| The tasks, skill, workflow and Claude Code hook mise-cloudflare wrote, current for its version | `cloudflare:check` |
| For Workers Builds: Cloudflare's GitHub app on the repository, a build token on the account | `cloudflare:builds` says which is missing and how to add it |

## What mise-cloudflare writes into the repo

Like charter's and go-htmx4's own, these are stamped from the copy inside mise-cloudflare, at its version: don't edit
them; after upgrading, write them again (`cloudflare:check` fails while they're out of date).

| File | Written by |
|---|---|
| `.mise/tasks/cloudflare.toml` | `mise run cloudflare:tasks` |
| `.claude/skills/cloudflare/SKILL.md` | `mise run cloudflare:skill` |
| `.github/workflows/cloudflare.yml` (`cloudflare-<folder>.yml` for a Worker in a subfolder) | `mise run cloudflare:workflow` |
| `.claude/hooks/mise-cloudflare-session-start.sh`, and its entry in `.claude/settings.json` | `mise run cloudflare:claude` |

## The tasks

| Task | Does | Logs in |
|---|---|---|
| `mise run cloudflare:dev` | Develop locally: `wrangler dev`, with local D1, KV and R2 and `.dev.vars` | Never |
| `mise run cloudflare:login` | The repo's own login, with exactly `cloudflare.toml`'s scopes | The first time, and when the scopes change |
| `mise run cloudflare:deploy` | `wrangler deploy` on that login; what it deployed, URLs included, goes to `.wrangler/output.json` | If needed |
| `mise run cloudflare:wrangler -- <args>` | Any wrangler command on that login: `d1 list`, `secret put`, `delete`, `tail`, `preview`, … | If needed |
| `mise run cloudflare:builds` | Workers Builds for the Worker: a push to the branch deploys production, any other branch gets a preview; with `[builds] staging`, its branch deploys staging; `--off` removes it | A cf login, the first time |
| `mise run cloudflare:previews` | The previews' own KV, D1 and R2 (`<worker>-previews-<binding>`), made once and put in the Worker's Previews Base on Cloudflare; `cloudflare:builds` and the deploy workflow run it; `--off` deletes them | If needed |
| `mise run cloudflare:ci-token` | For GitHub Actions deploying: the dashboard link that makes CI's Cloudflare token with the Worker's permissions picked, then the token stored as the repository's GitHub secrets, never shown; `--off` deletes them | Making the token, once, in the dashboard |
| `mise run cloudflare:workflow` | Writes the GitHub Actions workflow | Never |
| `mise run cloudflare:claude` | Writes the Claude Code cloud session hook | Never |
| `mise run cloudflare:status` | The repo's login, its account and scopes, stray tokens, this machine's other wrangler logins | Never |
| `mise run cloudflare:check` | The repo's rules: no account or resource IDs (in any environment), no tokens, `.dev.vars` and `.env` git-ignored, `.dev.vars.example` matching `secrets.required`, wrangler pinned | Never |
| `mise run cloudflare:logout` | Logs the repo's logins out on Cloudflare and deletes them | Never |

Each runs `cloudflare …` (or `go tool cloudflare …`); `--json` gives one JSON document. Deploying and deleting are
wrangler's own job, done by the repo's own wrangler; `wrangler dev` needs no login at all.

## Cloudflare's three kinds of credential

- **A login (OAuth).** wrangler opens Cloudflare's page, you click Allow and pick your account, and wrangler gets a
  token that can do only what the page listed: its scopes, such as `workers_kv:write`. It lasts about an hour, and
  wrangler renews it by itself. It lives in wrangler's folder on your machine, never in the repo. Each repo gets its own,
  with exactly the scopes its bindings need: that's the repo's login, below.
- **An API token.** A long secret string made by hand in Cloudflare's dashboard, its permissions picked there ("Workers
  Scripts Edit", "D1 Edit": the same idea as scopes, by other names). Using one needs no click, so it's for machines with
  no one there to click: GitHub Actions. Whoever has the string has its power, so it's kept as a GitHub secret. It can
  belong to a person or to the account; CI's should be the account's, so it outlives anyone leaving. Used here only if
  GitHub Actions deploys ([below](#github-actions)).
- **Cloudflare's build token.** Workers Builds deploys with a token Cloudflare made and keeps itself, so it never leaves
  Cloudflare and GitHub holds no Cloudflare credential at all ([below](#workers-builds-production-staging-previews-and-pull-requests)).

A credential is only needed to change things on Cloudflare: creating a KV namespace, deploying, deleting. A running
Worker uses none: Cloudflare connects its bindings itself. That's why deploying needs only Workers Scripts, and a
product's own scope or permission is for creating and managing its resources. And a `CLOUDFLARE_API_TOKEN` in a shell
or a `.env` beats every login (wrangler uses it, silently, instead), so outside CI the tasks refuse one. Cloudflare's
old Global API Key, which can do everything, is never used.

## The login: where it comes from, where it lives

- **What it may do:** what wrangler's own login asks for ([below](#what-the-login-may-do)), unless `scopes` in the
  repo's [cloudflare.toml](../example/cloudflare.toml) says exactly which. `cloudflare:status` shows what it holds and
  what it needs.
- **Its name:** `profile` in `cloudflare.toml`, else the Worker's name. It's a
  [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/).
- **Where it lives:** wrangler's folder in the person's home directory, never the repo:
  `~/Library/Preferences/.wrangler/config/<profile>.toml` on a Mac, `~/.config/.wrangler/config/<profile>.toml` on
  Linux. An OAuth token that lasts an hour, and a refresh token wrangler renews it with.
- **The first time:** the first task that needs it asks. On a machine with a browser, Cloudflare's consent page opens,
  listing exactly the repo's scopes: click Allow and pick your account. In a Claude Code cloud session, or with
  `CLOUDFLARE_LOGIN=device`, it prints a link with the code already in it: open it on any device, your phone included.
- **Every time after:** each task compares the scopes the login holds with what it needs. Equal: nothing is asked.
  Different (wrangler upgraded, or `scopes` changed): it logs in once more, so the login always holds exactly that.

### What the login may do

It asks for what wrangler's own login asks for: the list wrangler keeps, for everything it does, read from the repo's
own wrangler (`wrangler login --scopes-list` shows it). So whatever wrangler can do with any kind of binding, the
repo's login can, and nothing here lists kinds of binding or which scope each needs (neither wrangler nor Cloudflare
publishes that). When an upgraded wrangler asks for more, the next task logs in once more. `scopes` in
`cloudflare.toml` gives the login exactly those instead, for a narrower one. Proven on a real account: every kind of
binding Cloudflare makes for nothing (`prove-bindings`: Queues, Vectorize, Workers AI, Durable Objects, Workflows,
Analytics Engine, rate limits, Browser, Images), and D1, KV and R2 (`prove`).

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
  URL on the pull request and keeps the comment current. Previews have their own KV, D1 and R2, which `builds` makes
  (`cloudflare:previews`, below), so a preview never touches production's or staging's data.
- **A merged pull request's preview** goes when its branch does: `builds` switches on GitHub's "Automatically delete head
  branches" (with `GITHUB_TOKEN` or gh's login).

### The previews' own KV, D1 and R2

Cloudflare makes previews no KV, D1 or R2 (only Durable Objects and Containers), and `wrangler preview` creates none,
as `wrangler deploy` does for production. So `mise run cloudflare:previews` makes `<worker>-previews-<binding>` for each
KV, D1 and R2 binding, once, with the repo's own wrangler, and puts them in the Worker's Previews Base on Cloudflare,
the settings every new preview starts from. Nothing goes into the repo, which is why the `previews` block leaves KV, D1
and R2 out. All previews share them, as they share staging's settings; production's and staging's stay theirs.
`cloudflare:builds` runs it, and so does the deploy workflow's preview job; after adding a KV, D1 or R2 binding, run it
again (it makes only what's new). `--off` takes them out and deletes them (an R2 bucket only once it's empty). Proven
on the account: a preview on its own KV and R2 from Workers Builds (`prove-builds`) and on its own KV from GitHub
Actions (`prove-actions`).

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
setup, because GitHub has no one to click Allow. You make it once in Cloudflare's dashboard, since Cloudflare lets no
login make API tokens (tried: it leaves `account_api_tokens:create` out of every grant). `mise run cloudflare:ci-token`
prints the link: an account's token (it outlives anyone leaving), no expiry unless you want one, with the permissions
already picked: Workers Scripts, KV, R2, D1, Queues, Workers Routes and Zone Read, every Workers one Cloudflare's token
links can pick. A Worker using more (Secrets Store, Pipelines, …) needs that permission added there too. Proven so far:
the deploy path, on the repo login's own token; a token made from the link, not yet. `ci-token` then takes the token, hidden, checks it against the account's Workers,
and stores it and the account id as the repository's GitHub secrets with gh. It's never shown or written into the repo; `--off` deletes
the secrets. This repo's own [ci.yml](../.github/workflows/ci.yml) runs its Go tests and checks its release config;
example/'s [workflow](../.github/workflows/cloudflare-example.yml) is stamped by the tool, as any repo's is.

## Claude Code cloud sessions

A cloud session starts on a bare Linux machine with only the repo. `mise run cloudflare:claude` writes the hook
[.claude/hooks/mise-cloudflare-session-start.sh](../.claude/hooks/mise-cloudflare-session-start.sh) and adds it to
`.claude/settings.json` (keeping what's there): when a session starts, it installs mise and every pinned tool, then runs
the Worker folder's own `setup` task, so the tasks work straight away. This repo's settings name `. example`. On your
own machine it does nothing. After a repository is renamed, attach it to a cloud session by its new name: GitHub
redirects the old one through paths the cloud session's GitHub access refuses. The cloud environment's network settings must allow mise.run,
GitHub, nodejs.org, the npm registry, proxy.golang.org and sum.golang.org.

## Why it's safe

- Every wrangler command gets `--profile <the repo's login>`. `wrangler whoami`, which takes no `--profile`, runs on the
  folder's binding, made by its real path (a symlinked folder once fell back to the machine's default login), and the
  task stops unless wrangler names the repo's login as the active one. The machine's own `wrangler login` is never used
  or changed; the proofs check its file is byte for byte the same before and after.
- A `CLOUDFLARE_API_TOKEN` in the shell or in a `.env` would override the login, so every task refuses one (outside CI).
- The repo never holds account IDs, resource IDs, tokens or secret values: `cloudflare:check` fails if it does.
- The Worker's config is wrangler's own reading of it (`unstable_readConfig`, `experimental_readRawConfig`): nothing
  here parses a wrangler config.

## Releases

A `v*` tag runs [release.yml](../.github/workflows/release.yml): the tests, then
[goreleaser](../.goreleaser.yaml) puts the `cloudflare` binary for macOS, Linux and Windows (Intel and ARM) on the
release, and the module is fetched through proxy.golang.org so pkg.go.dev and `go get` have it at once.
