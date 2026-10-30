# Testing

Every task runs at the top of the repo. Two files hold them: [mise.toml](../mise.toml), what this repo does itself,
and [example.mise.toml](../example.mise.toml), which it includes: the steps on example/, as on any repo. Each task runs
what it needs first (mise `depends`), so run the furthest you need: `mise run smoke` is build, install, 00-git-init to
04-check, test, then smoke. `mise tasks deps <task>` shows it.

The steps on example/, as on any repo (example.mise.toml), each in example/ on example/'s own mise. One name all
the way through: each step is named after the one thing it runs. From 4 on, step `n-x` runs example/'s task
`cloudflare:x`, which runs `mise-cloudflare x`; steps 0 to 3 come before those tasks exist.

| Step | Runs in example/ | What |
|---|---|---|
| `00-git-init` | `git init` | a git repo of its own (only example/ needs it) |
| `01-mise-install` | `mise install` | what it pins: node, aube, mise-cloudflare at `local` |
| `02-setup` | `mise run setup`, its own task: `aube install` | its dependencies: the wrangler package.json pins |
| `03-init` | `mise exec -- mise-cloudflare init` | the cloudflare tasks, the skill, the workflow, the hook, written into it |
| `04-check` | `mise run cloudflare:check` | its rules pass |
| `05-status` | `mise run cloudflare:status` | where it is, and what to run next |
| `06-dev` | `mise run cloudflare:dev` | the Worker locally, no account |
| `07-secret` | `mise run cloudflare:secret` | the secrets it requires (it asks for each) |
| `08-deploy` | `mise run cloudflare:deploy` | on Cloudflare, on its own login |
| `09-github` | `mise run cloudflare:github` | Cloudflare deploys every push: it needs the repo on GitHub, and example/'s own git has none, so it stops and says so |
| `10-secret` | `mise run cloudflare:secret --previews` | the secrets previews require, which they don't inherit (after `08-deploy`) |
| `11-preview` | `mise run cloudflare:preview --name example` | a preview at its own URL, with its own KV, D1 and R2 (after `08-deploy`) |
| `12-login` | `mise run cloudflare:login` | the repo's login, which the others make when they need it |
| `13-delete` | `mise run cloudflare:delete --yes` | `08-deploy` and `11-preview` undone: the Worker, its KV, D1 and R2, its previews' own |
| `14-logout` | `mise run cloudflare:logout` | the logins deleted: the next `08-deploy` asks for Allow again |

A step that asks you something (`06-dev`, `07-secret`, `10-secret`) runs on your terminal (`raw`), and no step
depends on one, so nothing asks you twice.

This repo's own (mise.toml):

| Task | What | When | Cloudflare |
|---|---|---|---|
| `mise run test` | Every check that needs no account, run for real (no cached results): `go vet`, the Go tests (the rules, the login's scope check against the repo's own wrangler, the device link, the files init writes, example/ current), the release config | After every change | No |
| `mise run smoke` | A new repo as a user sets it up: mise-cloudflare from mise, `init`, `check` passes, status's next step, a login in CI stopping at once, the Worker running locally with D1, KV, R2 and its secret. This checkout's build; `--version <v>` tests a release from GitHub instead. GitHub's CI runs it | After every change | No |
| `mise run prove` | One real deploy of example/ from this machine, no GitHub, a few minutes: the secret, a deploy, the live D1, KV, R2 and secret, stray tokens refused. Everything deleted after | After a change to how it deploys | Yes |
| `mise run builds` | Cloudflare's Workers Builds deploying every push, through GitHub, about 25 minutes: production from a throwaway branch, a branch's preview with its own D1, KV, R2 and secret, gone with its branch. Everything deleted after | After a change to `github` | Yes, and GitHub |
| `mise run install` | This checkout's build as the mise version `local`, for any repo on this machine, no release: `mise use github:joeblew999/mise-cloudflare@local` there. It links `bin/`, so each build updates it. `01-mise-install` runs it first, since example/ pins it | To try a change in another repo | No |
| `mise run clean` | Deletes what a stopped `prove` or `builds` left, which the next one refuses to start with: the Worker, its D1, KV and R2 and its previews', its Workers Builds setup, the `e2e-*` branches | After a stopped run | Yes |

Each e2e task runs the Go test of its name, in the file of its name: `mise run smoke` is `TestSmoke` in
[e2e/smoke_test.go](../e2e/smoke_test.go), and so `prove`, `builds` and `clean`. What they share is in
[e2e/e2e_test.go](../e2e/e2e_test.go).

example/ is a user's repo of its own, inside this one:

- **Its own git repo** (`example/.git`, which `00-git-init` makes): so everything init writes is in example/, its
  workflow in `example/.github/`, its Claude Code hook in `example/.claude/`, and none of it in this repo's own
  `.github/` or `.claude/` (`TestExampleStaysInExample`). This repo still tracks every file in it. `builds` runs in a
  checkout of this repo instead, where example/ is a subfolder, as Cloudflare builds it from GitHub.
- **Its own mise**: what runs in example/ runs with `MISE_CEILING_PATHS`, so mise there reads example/'s own
  `mise.toml` only: its tasks are the ones init writes (`cloudflare`, `cloudflare:<step>`) and its own `setup`, not
  this repo's.

## Layout

| Folder | What |
|---|---|
| [cmd/mise-cloudflare/](../cmd/mise-cloudflare/) | the command: a file per command, `x.go` holding `r.x`, which main.go's `case "x"` calls; `resources.go` the KV, D1 and R2 several of them use |
| [templates/](../templates/) | every file `init` writes into a repo: the tasks, the workflow, the Claude Code hook, the agent skill |
| [internal/](../internal/) | the login (`wrangler`), Cloudflare's cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](../example/) | a Worker repo of its own that uses it, on this checkout's build (its steps are in example.mise.toml) |
| [e2e/](../e2e/) | `smoke`, `prove`, `builds` and `clean`, a file each |
