# cloudflare-zero

Cloudflare without the token mess. Anyone who pulls a repo that uses this gets their own Cloudflare login for it, with
exactly the scopes the repo says, made with one click on Allow; the repo's own wrangler then runs on that login. No API
tokens to make, nothing about Cloudflare in the repo, and the same on a Mac and in a Claude Code cloud session.

One Go command, `go tool cloudflare`, with its mise tasks and agent skill, the way
[go-htmx4](https://github.com/joeblew999/go-htmx4)'s `seo` and `intl` are built.

## In a repo

```sh
go get -tool github.com/joeblew999/cloudflare-zero/cmd/cloudflare@main
go tool cloudflare init    # cloudflare.toml, .mise/tasks/cloudflare.toml, .claude/skills/cloudflare
```

Then the repo's `mise.toml` takes Go from `go.mod` and includes the tasks:

```toml
[settings]
idiomatic_version_file_enable_tools = ["go"]   # go.mod's toolchain line is the Go pin

[tools]
node = "26.11.1"   # runs wrangler
aube = "2.8.1"     # or whichever package manager installs the repo's package.json

[task_config]
includes = [".mise/tasks/cloudflare.toml"]
```

The repo also has, as any Worker repo does:

- a wrangler config (`wrangler.jsonc`) with no `account_id` and no resource IDs: bindings only, plus
  `"secrets": { "required": [...] }`
- `.dev.vars.example`, with a dummy value for each required secret
- wrangler pinned to an exact version in `package.json`
- `.gitignore` covering `.dev.vars`, `.env*`, `.wrangler/` and `node_modules/`

[example/](example/) is exactly that. After upgrading cloudflare-zero, `mise run cloudflare:tasks` and
`mise run cloudflare:skill` write the tasks and the skill again from the new version.

## The tasks

| Task | Does | Logs in |
|---|---|---|
| `mise run cloudflare:login` | The repo's own login, with exactly `cloudflare.toml`'s scopes | The first time, and when the scopes change |
| `mise run cloudflare:deploy` | `wrangler deploy` on that login; what it deployed, URLs included, goes to `.wrangler/output.json` | If needed |
| `mise run cloudflare:wrangler -- <args>` | Any wrangler command on that login: `d1 list`, `secret put`, `delete`, `tail`, … | If needed |
| `mise run cloudflare:status` | The repo's login, its account and scopes, stray tokens, this machine's other wrangler logins | Never |
| `mise run cloudflare:check` | The repo's rules: no account or resource IDs (in any environment), no tokens, `.dev.vars` and `.env` git-ignored, `.dev.vars.example` matching `secrets.required`, wrangler pinned | Never |
| `mise run cloudflare:logout` | Logs the repo's login out on Cloudflare and deletes it | Never |

Each runs `go tool cloudflare …`; `-json` gives one JSON document instead of text. Deploying, deleting and everything
else on Cloudflare is wrangler's own job, done by the repo's own wrangler.

## The login: where it comes from, where it lives

- **What it may do:** `scopes` in the repo's [cloudflare.toml](example/cloudflare.toml), OAuth scopes from
  `wrangler login --scopes-list`. Without that line, the default: `account:read user:read workers:write
  workers_kv:write workers_scripts:write d1:write`, what a Worker with D1, KV and R2 needs.
- **Its name:** `profile` in `cloudflare.toml`, else the Worker's name from its wrangler config. It's a
  [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/).
- **Where it lives:** in wrangler's folder in the person's home directory, never in the repo:
  `~/Library/Preferences/.wrangler/config/<profile>.toml` on a Mac, `~/.config/.wrangler/config/<profile>.toml` on
  Linux. It holds an OAuth token that lasts an hour and a refresh token wrangler renews it with.
- **The first time:** there is no login, so the first task that needs one asks. On a machine with a browser, it opens
  Cloudflare's consent page listing exactly the repo's scopes: click Allow and pick your account. In a Claude Code
  cloud session, or with `CLOUDFLARE_LOGIN=device`, wrangler prints a link and a code instead: open it on any device,
  your phone included.
- **Every time after:** each task compares the scopes the login holds (`wrangler whoami`) with the repo's. Equal:
  nothing is asked. Different either way, a scope added to `cloudflare.toml` or one removed: it logs in once more, so
  the login always holds exactly the repo's scopes.
- **R2** works with `workers:write`: wrangler has no R2 scope.

## Why it's safe

- Every wrangler command gets `--profile <the repo's login>`. `wrangler whoami`, which takes no `--profile`, runs on
  the folder's binding, made by the folder's real path, and the task stops unless wrangler names the repo's login as the
  active one. The machine's own `wrangler login` and other repos' logins are never used or changed; the proof checks
  the default login's file is byte for byte the same before and after.
- A `CLOUDFLARE_API_TOKEN` in the shell or in a `.env` would override the login, so every task refuses to run with one
  (except in CI).
- The repo never holds account IDs, resource IDs, tokens or secret values: `cloudflare:check` fails if it does.

## Claude Code cloud sessions

A cloud session (claude.ai/code, or the Claude iOS app) starts on a bare Linux machine with only the repo.
[.claude/settings.json](.claude/settings.json) runs [.claude/hooks/session-start.sh](.claude/hooks/session-start.sh)
when it starts: it installs mise and every tool the repo pins (Go from `go.mod`), then runs each folder's `setup` task
if it has one, so the tasks work straight away. It sets up the repo's own folder, or the folders named after it (this
repo's settings name `. example`); another repo copies both files. On a machine of your own it does nothing.

The cloud environment's network settings must allow mise.run, GitHub, nodejs.org, the npm registry, proxy.golang.org
and sum.golang.org. The hook hides the session's GitHub token from mise: that token can see only the session's own
repos, and GitHub answers 404 when mise sends it elsewhere.

There's no browser in a cloud session, so its login uses the link: wrangler prints it, you open it on your phone and
click Allow. `wrangler auth create` has no device flow (wrangler 4.151.3 and 4.152.0), so `wrangler login --device`
logs in inside a temporary wrangler folder and its login file becomes the profile's; wrangler writes both the same way.
The login lasts as long as that cloud session's machine.

Don't prove the same repo on two machines at once: both deploy the same Worker to the same account. `mise run prove`
stops if the Worker is already deployed.

## Basin

Tested 23 Oct 2026 with a repo's own login: turning on Basin Catalog, Basin SQL queries (through wrangler and through
Cloudflare's API) and creating a Basin Pipeline stream all work, with no API token; a fake token gets HTTP 403. One
thing needs a stored API token: a Basin Pipeline's sink into Basin Catalog. Cloudflare keeps that token and uses it
itself, and it rejected the login's token: the pipeline went to `failed` with "The sink token is expired or invalid".
`mise run prove-basin` shows this.

## Proofs

| Task | Proves | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests: the rules, the scope comparison, the login's guard, the tasks and the skill, `example/` current | No |
| `mise run prove` | In `example/`: a local Worker with no login at all; deploy on its own login, bound by its real path; the live Worker's D1, KV, R2 and secret; a second deploy that asks nothing and writes nothing into the repo; stray tokens refused, in the shell and in `.env`; everything deleted again with wrangler; the machine's default login untouched. `--clean` also logs the login out | Yes |
| `mise run prove-other-repo` | A brand-new repo in a temporary folder gets cloudflare-zero from GitHub (`go get -tool`, ref `CF_ZERO_REF`, default `main`) and `go tool cloudflare init`, then the same proof on its own login, `cloudflare-zero-other`, made the first time (kept for the next run; `--clean` logs it out) | Yes |
| `mise run prove-basin` | Basin, above | Yes |

The proofs are Go tests in [e2e/](e2e/), run only with `CLOUDFLARE_E2E=1` (the tasks set it), so `go test ./...` never
touches Cloudflare. A proof that fails deletes what it made.

## Layout

| Folder | What |
|---|---|
| [cmd/cloudflare/](cmd/cloudflare/) | the command, `go tool cloudflare` |
| [config/](config/) | `cloudflare.toml` |
| [mise/](mise/) | its tasks, written into a repo by `go tool cloudflare tasks` |
| [skills/](skills/) | its agent skill, installed by `go tool cloudflare skill` |
| [internal/](internal/) | the login (`wrangler`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](example/) | a Worker repo that uses it, exactly as any other repo would |
| [e2e/](e2e/) | the proofs |
