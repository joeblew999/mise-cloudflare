# Reference

The details behind the [README](../README.md).

## What init adds to a repo

| File | What | Edit it? |
|---|---|---|
| `cloudflare.toml` | The repo's settings, every one commented at its default (written only if there's none) | Yes |
| `.mise/tasks/mise-cloudflare.toml` | The `mise run cloudflare:…` tasks (mise finds it there by itself) | No |
| `.claude/skills/cloudflare/SKILL.md` | The agent skill: how Claude works on the Worker | No |
| `.github/workflows/cloudflare.yml` (`cloudflare-<folder>.yml` for a Worker in a subfolder) | Checks the repo on every push and pull request; deploys too, with `[workflow] deploy` | No |
| `.claude/hooks/mise-cloudflare-session-start.sh`, and its entry in `.claude/settings.json` | Sets a Claude Code cloud session up when it starts | No |

The files you don't edit come from inside mise-cloudflare, at its version. After upgrading, `mise run cloudflare:update`
writes them again; `cloudflare:check` fails while they're out of date.

## The tasks

| Task | Does | Asks you |
|---|---|---|
| `cloudflare:dev` | `wrangler dev`: the Worker locally, with local D1, KV and R2 and `.dev.vars` | Nothing |
| `cloudflare:deploy` | `wrangler deploy` on the repo's login; what it deployed goes to `.wrangler/output.json` | Allow, the first time |
| `cloudflare:wrangler -- <args>` | Any wrangler command on that login: `d1 list`, `secret put`, `delete`, `tail`, … | Allow, the first time |
| `cloudflare:login` / `logout` | Log the repo in, or out (deleting its logins) | Allow, to log in |
| `cloudflare:status` | The repo's login: its account, the scopes it holds and needs, any stray token | Nothing |
| `cloudflare:check` | The repo's rules (below), and whether init's files are current | Nothing |
| `cloudflare:builds` | Workers Builds: a push to the branch deploys production, every other branch gets a preview; with `[builds] staging`, its branch deploys staging; `--off` removes it | Allow for cf, the first time |
| `cloudflare:previews` | The previews' own KV, D1 and R2 (below); `--off` deletes them | Nothing more |
| `cloudflare:ci-token` | Only if GitHub Actions deploys: the link that makes CI's token, then the token stored as GitHub secrets; `--off` deletes them | Making the token, once |
| `cloudflare:update` | After upgrading: init's files written again | Nothing |

Each runs `cloudflare …` (`go tool cloudflare …` in a Go repo); `--json` prints one JSON document.

## The rules (`cloudflare:check`)

| The repo needs | Because |
|---|---|
| No account or resource IDs in its wrangler config, in any environment | The login picks the account; wrangler finds or creates KV, D1 and R2 by name |
| No Cloudflare token in its files, and nothing calling `wrangler login` | A token in the repo leaks; `wrangler login` is the machine's, not the repo's |
| `.dev.vars`, `.env` and `.cloudflare/` git-ignored | Secrets, and Cloudflare's `cf` caches the account it logged into in `.cloudflare/` |
| `.dev.vars.example` listing exactly the config's `secrets.required` | So `wrangler dev` has every secret |
| wrangler pinned to an exact version in `package.json` | So everyone, CI and Cloudflare's builds run the same wrangler |
| Where previews run (`cloudflare:builds`, `[workflow] deploy`): a `previews` block with every binding a preview uses but KV, D1 and R2 | A preview inherits nothing from production; KV, D1 and R2 come from `cloudflare:previews` |

## Cloudflare's three kinds of credential

- **A login (OAuth).** wrangler opens Cloudflare's page, you click Allow and pick your account, and wrangler gets a
  token that can do what the page listed, its scopes. It lasts about an hour, and wrangler renews it by itself. It lives
  in wrangler's folder on your machine, never in the repo. Each repo has its own: the repo's login, below.
- **An API token.** A secret string made by hand in Cloudflare's dashboard, its permissions picked there. Using one
  needs no click, so it's for GitHub Actions, where no one is there to click; it's kept as a GitHub secret. Used here
  only if GitHub Actions deploys.
- **Cloudflare's build token.** Workers Builds deploys with a token Cloudflare made and keeps, so GitHub holds no
  Cloudflare credential at all.

A running Worker uses none of them: Cloudflare connects its bindings itself. A `CLOUDFLARE_API_TOKEN` in a shell or a
`.env` would beat every login (wrangler uses it, silently, instead), so outside CI the tasks refuse one. Cloudflare's
Global API Key is never used.

## The repo's login

- **Its name:** `profile` in `cloudflare.toml`, else the Worker's name: a
  [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/), bound to the repo's folder.
- **What it may do:** what wrangler's own login asks for, read from the repo's wrangler (`wrangler login
  --scopes-list` shows the list), so it can do whatever wrangler can with any kind of binding. `scopes` in
  `cloudflare.toml` gives it exactly those instead, for a narrower login.
- **The first time:** the first task that needs it asks. With a browser, Cloudflare's page opens: click Allow and pick
  your account. In a Claude Code cloud session, or with `CLOUDFLARE_LOGIN=device`, it prints a link with the code
  already in it: open it on any device, your phone included.
- **After that:** each task compares what the login holds with what it needs, and asks again only when that changes
  (wrangler upgraded, or `scopes` changed).
- **Safe:** every wrangler command gets `--profile` with the repo's login. `wrangler whoami`, which takes none, runs on
  the folder's binding, made by the folder's real path, and the task stops unless wrangler names the repo's login as
  the active one. The machine's own `wrangler login` is never used or changed.

## Workers Builds: previews, staging and production

`cloudflare:builds` connects the Worker to its GitHub repository on Cloudflare: the folder (a monorepo's subfolder
works), a build command (installing from the lockfile committed to git, or `[builds] build`), `npx wrangler deploy` for
the deploy branch and `npx wrangler preview` for every other. With `[builds] staging = "main"`, a push to `main` deploys
**staging** (the wrangler environment `env.staging`, its own Worker with its own bindings), a push to `production`
deploys **production**, and every other branch gets a preview.

- **A push to the deploy branch** is built and deployed by Cloudflare, with its own build token.
- **Any other branch** gets a [Worker Preview](https://developers.cloudflare.com/workers/previews/) at
  `https://<branch>-<worker>.<subdomain>.workers.dev`, and Cloudflare's GitHub app posts its URL on the pull request.
- **A merged pull request's preview** goes when its branch does: `builds` switches on GitHub's "Automatically delete head
  branches".

It needs, once: Cloudflare's GitHub app on the repository, and a build token on the account, which Cloudflare makes the
first time any repository is connected in its dashboard. That token covers Workers Scripts, KV, R2 and Workers Routes;
a Worker using more (D1, say) has that permission added to it in the dashboard. Workers Builds' own scope isn't one
wrangler's login can have, so `builds` uses Cloudflare's own CLI, [cf](https://github.com/cloudflare/cf), on its own
login; the task brings cf.

### The previews' own KV, D1 and R2

Cloudflare makes previews no KV, D1 or R2 (only Durable Objects and Containers), and `wrangler preview` creates none.
So `cloudflare:previews` makes `<worker>-previews-<binding>` for each KV, D1 and R2 binding, once, and puts them in the
Worker's Previews Base on Cloudflare, the settings every new preview starts from. Nothing goes into the repo. All
previews share them; production's and staging's stay theirs. `cloudflare:builds` runs it, and so does the deploy
workflow's preview job; after adding a KV, D1 or R2 binding, run it again. `--off` deletes them (an R2 bucket only once
it's empty).

## GitHub Actions

init's workflow runs on every push and pull request: mise and its pinned tools, the repo's own `setup` and `test`
tasks, then `cloudflare:check`. It needs no Cloudflare account.

With `[workflow] deploy = true` it deploys too, instead of Workers Builds: production from `[workflow] branch` (default
`main`), a preview `pr-<number>` for each pull request, its URL commented, deleted when the pull request closes. It
deploys with an API token, since GitHub has no one to click Allow, and Cloudflare lets no login make one, so you make
it once in the dashboard: `cloudflare:ci-token` prints the link, with Workers Scripts, KV, R2, D1, Queues, Workers Routes
and Zone Read already picked (a Worker using more needs that added there), takes the token hidden, checks it, and
stores it and the account's ID as the repository's GitHub secrets. It's never shown or written into the repo.

## Claude Code cloud sessions

A cloud session starts on a bare Linux machine with only the repo. init's hook, run when a session starts, installs
mise and every pinned tool, then the Worker folder's own `setup` task, so the tasks work at once; on your own machine it
does nothing. The cloud environment's network settings must allow mise.run, GitHub, nodejs.org, the npm registry,
proxy.golang.org and sum.golang.org. After a repository is renamed, attach it to a cloud session by its new name.

## Releases

Before a release, `mise run prove` deploys example/ for real (see [testing](testing.md)). A `v*` tag runs
[release.yml](../.github/workflows/release.yml): the tests, then [goreleaser](../.goreleaser.yaml) puts the
`cloudflare` binary for macOS, Linux and Windows on the release with the commits since the last tag as its notes, and
proxy.golang.org fetches the module so `go get` and pkg.go.dev have it at once.
