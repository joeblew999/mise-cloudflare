# Reference

The details behind the [README](../README.md).

## The commands

Each is `mise run cloudflare <command>` (it runs `cloudflare <command>`, or `go tool cloudflare <command>` in a Go repo).

| Command | Does | Asks you |
|---|---|---|
| (none) | Where the Worker is, step by step, and what to run next; `--json` for a program | Nothing |
| `dev` | `wrangler dev`: the Worker locally, with local D1, KV and R2 and `.dev.vars` | Nothing |
| `deploy` | `wrangler deploy` on the repo's login (wrangler's flags follow: `--env staging`); previews get their own KV, D1 and R2 | Allow, the first time |
| `github` | Cloudflare deploys every push (Workers Builds): production from the repository's default branch, a preview for every other; `--staging main` for staging; `--branch`, `--build` to change those; `--off` to stop. Run again with no flags, it keeps what's set | Allow for cf, the first time |
| `actions` | Instead of `github`: the repo's workflow deploys, with a Cloudflare token you make once from the link it gives, kept as GitHub secrets; `--branch` for production's branch; `--off` to stop | Making the token, once |
| `preview` | A preview of what's here now (`wrangler preview`: `--name my-test`), with its own KV, D1 and R2 | Allow, the first time |
| `wrangler <...>` | Any wrangler command on the repo's login: `wrangler d1 list`, `secret put`, `delete`, `tail`, … | Allow, the first time |
| `check` | The repo's rules (below), and whether init's files are current | Nothing |
| `login` / `logout` | The repo's login (the others make it when they need it); logout deletes it | Allow, to log in |
| `update` | After upgrading mise-cloudflare: init's files written again | Nothing |
| `help` / `version` | Every command; this build's version | Nothing |

## What init adds

| File | What |
|---|---|
| `.mise/tasks/mise-cloudflare.toml` | The one task, `mise run cloudflare` (mise finds it there by itself) |
| `.github/workflows/cloudflare.yml` (`cloudflare-<folder>.yml` for a Worker in a subfolder) | Checks the repo on every push and pull request; after `cloudflare actions` it deploys too |
| `.claude/skills/cloudflare/SKILL.md` | The agent skill: how Claude works on the Worker |
| `.claude/hooks/mise-cloudflare-session-start.sh`, and its entry in `.claude/settings.json` | Sets a Claude Code cloud session up when it starts |

None is for editing. They come from inside mise-cloudflare, at its version: after upgrading, `update` writes them
again, and `check` fails until it has. There's no settings file: each choice is a command's flag, kept where it lives
(Cloudflare's Workers Builds setup, the workflow file).

## The rules (`check`)

| The repo needs | Because |
|---|---|
| No account or resource IDs in its wrangler config, in any environment | The login picks the account; wrangler finds or creates KV, D1 and R2 by name |
| No Cloudflare token in its files, and nothing calling `wrangler login` | A token in the repo leaks; `wrangler login` is the machine's, not the repo's |
| `.dev.vars`, `.env` and `.cloudflare/` git-ignored | Secrets, and Cloudflare's `cf` caches the account it logged into in `.cloudflare/` |
| `.dev.vars.example` listing exactly the config's `secrets.required` | So `wrangler dev` has every secret |
| wrangler pinned to an exact version in `package.json` | So everyone, CI and Cloudflare's builds run the same wrangler |
| Where previews run (`github`, `actions`): a `previews` block with every binding a preview uses but KV, D1 and R2 | A preview inherits nothing from production; KV, D1 and R2 it gets its own (below) |

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
`.env` would beat every login (wrangler uses it, silently, instead), so outside CI the commands refuse one.
Cloudflare's Global API Key is never used.

## The repo's login

- **Its name:** the Worker's: a [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/),
  bound to the repo's folder.
- **What it may do:** what wrangler's own login asks for, read from the repo's wrangler (`wrangler login
  --scopes-list` shows the list), so it can do whatever wrangler can with any kind of binding.
- **The first time:** the first command that needs it asks. With a browser, Cloudflare's page opens: click Allow and
  pick your account. In a Claude Code cloud session, or with `CLOUDFLARE_LOGIN=device`, it prints a link with the code
  already in it: open it on any device, your phone included.
- **After that:** each command compares what the login holds with what it needs, and asks again only when an upgraded
  wrangler needs more.
- **Safe:** every wrangler command gets `--profile` with the repo's login. `wrangler whoami`, which takes none, runs on
  the folder's binding, made by the folder's real path, and the command stops unless wrangler names the repo's login as
  the active one. The machine's own `wrangler login` is never used or changed.

## Workers Builds: previews, staging and production (`github`)

`github` connects the Worker to its GitHub repository on Cloudflare: the folder (a monorepo's subfolder works), a
build command (installing from the lockfile committed to git, or `--build`), `npx wrangler deploy` for the deploy
branch and `npx wrangler preview` for every other. With `--staging main`, a push to `main` deploys **staging** (the
wrangler environment `env.staging`, its own Worker with its own bindings, deployed once first with
`deploy --env staging`), a push to `production` deploys **production**, and every other branch gets a preview.
Cloudflare keeps these settings; run `github` again with no flags and it keeps them.

- **A push to the deploy branch** is built and deployed by Cloudflare, with its own build token.
- **Any other branch** gets a [Worker Preview](https://developers.cloudflare.com/workers/previews/) at
  `https://<branch>-<worker>.<subdomain>.workers.dev`, and Cloudflare's GitHub app posts its URL on the pull request.
- **A merged pull request's preview** goes when its branch does: `github` switches on GitHub's "Automatically delete
  head branches".

It needs, once: Cloudflare's GitHub app on the repository, and a build token on the account, which Cloudflare makes the
first time any repository is connected in its dashboard. That token covers Workers Scripts, KV, R2 and Workers Routes;
a Worker using more (D1, say) has that permission added to it in the dashboard. Workers Builds' own scope isn't one
wrangler's login can have, so `github` uses Cloudflare's own CLI, [cf](https://github.com/cloudflare/cf), on its own
login; the task brings cf.

### The previews' own KV, D1 and R2

Cloudflare makes previews no KV, D1 or R2 (only Durable Objects and Containers), and `wrangler preview` creates none.
So `deploy`, `preview` and `github` make `<worker>-previews-<binding>` for each KV, D1 and R2 binding, once, and put
them in the Worker's Previews Base on Cloudflare, the settings every new preview starts from. Nothing goes into the
repo. All previews share them; production's and staging's stay theirs. After adding a KV, D1 or R2 binding, the guide
says to run `github` again, which gives previews the new one. `github --off` deletes them (an R2 bucket only once it's
empty).

## GitHub Actions (`actions`)

init's workflow runs on every push and pull request: mise and its pinned tools, the repo's own `setup` and `test`
tasks, then `check`. It needs no Cloudflare account.

`actions` turns deploying on in it, instead of Workers Builds: production from the repository's default branch (or
`--branch`), a preview `pr-<number>` for each pull request, its URL commented, deleted when the pull request closes. It
deploys with an API token, since GitHub has no one to click Allow, and Cloudflare lets no login make one, so you make
it once in the dashboard: `actions` prints the link, with Workers Scripts, KV, R2, D1, Queues, Workers Routes and Zone
Read already picked (a Worker using more needs that added there), takes the token hidden, checks it, and stores it and
the account's ID as the repository's GitHub secrets. It's never shown or written into the repo. Commit the workflow it
rewrote. `actions --off` takes the deploy jobs out again and deletes the secrets.

## Claude Code cloud sessions

A cloud session starts on a bare Linux machine with only the repo. init's hook, run when a session starts, installs
mise and every pinned tool, then the Worker folder's own `setup` task, so the commands work at once; on your own
machine it does nothing. The cloud environment's network settings must allow mise.run, GitHub, nodejs.org, the npm
registry, proxy.golang.org and sum.golang.org. After a repository is renamed, attach it to a cloud session by its new
name.

## Releases

Before a release, `mise run prove` deploys example/ for real (see [testing](testing.md)). A `v*` tag runs
[release.yml](../.github/workflows/release.yml): the tests, then [goreleaser](../.goreleaser.yaml) puts the
`cloudflare` binary for macOS, Linux and Windows on the release with the commits since the last tag as its notes, and
proxy.golang.org fetches the module so `go get` and pkg.go.dev have it at once.
