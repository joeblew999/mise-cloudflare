# mise-cloudflare in this repo

This repo's Cloudflare Worker, with [mise-cloudflare](https://github.com/joeblew999/mise-cloudflare): run it
locally, deploy it, then a preview for every pull request, staging and production. The same on your machine and in
Claude Code. No API tokens to make; the first time, you click **Allow**.

Written by `mise-cloudflare init`, for the version this repo's `.mise/config.toml` pins: don't edit it. After upgrading,
`mise run cloudflare:03-init` writes it again, and `mise run cloudflare:04-check` fails until it has.

## Set up and upgrade

Step by step, at the repository's root (GitHub and Claude Code read `.github/` and `.claude/` only there), in a repo
that's new or has a Worker of its own (a `wrangler.jsonc`, and wrangler in its `package.json`):

1. **Install mise**, once on each machine: [mise's getting started](https://mise.jdx.dev/getting-started.html)
   (`curl https://mise.run | sh`, or `brew install mise`).
2. **mise-cloudflare's files, always**, at a release (the newest is on
   [its releases](https://github.com/joeblew999/mise-cloudflare/releases)):

   ```sh
   mise exec github:joeblew999/mise-cloudflare@<version> -- mise-cloudflare init
   ```

   It writes the repo's `.mise/` folder (its tasks, and mise-cloudflare pinned at that release, which mise gets from
   GitHub), its GitHub workflow, its Claude Code hook and skill, its docs and its `.gitignore`.
3. **The template's Worker, only in a repo with none yet:**

   ```sh
   mise run cloudflare:template
   ```

   It writes the template's Worker, named for the repo's folder, with the `mise.toml` that installs its wrangler, and
   installs it. It never writes over a file: a repo with a Worker of its own gets none of it, and installs its wrangler
   its own way.
4. **Then:**

   ```sh
   mise run cloudflare
   ```

   It shows what's done, and what to run next.

Commit what steps 2 and 3 wrote.

mise-cloudflare's pin, `.mise/config.toml`, is the release that ran `init`, so `mise install` gets that release from
GitHub, here and on GitHub's machines alike. There's nothing to edit, then or ever. `init` writes only its own files: a `.gitignore` or `.claude/settings.json` the repo has already it leaves as it is, and `check` names what
they lack (the secrets git-ignored, the hook's entry). Pin wrangler to an exact version in `package.json` and list the
Worker's secrets in `.dev.vars.example` with local values: `check` and `serve` ask for them.

To upgrade: `mise run cloudflare:update` moves the pin (with every other), then `mise run cloudflare:03-init` writes
`init`'s files again; or name the release: `mise use --path .mise/config.toml github:joeblew999/mise-cloudflare@<version>`.
mise holds back a release for a while after it's published (its minimum release age), so name the version to get one
at once.

## Use it

One task does everything, and tells you what to run:

```sh
mise run cloudflare
```

```
my-worker
  ✓ 06. Run it locally     mise run cloudflare:06-serve
  ✓ 08. Deploy it          mise run cloudflare:08-deploy   https://my-worker.example.workers.dev
  · 09. Deploy every push  mise run cloudflare:09-connect-github   a preview for every pull request, then staging and production
  login: my-worker, on My Account
next: mise run cloudflare:09-connect-github
```

| Step | Run | You're asked |
|---|---|---|
| 06. Run it locally, with local D1, KV and R2 | `mise run cloudflare:06-serve` | Nothing: no account needed |
| 08. Deploy it yourself | `mise run cloudflare:08-deploy` | **Allow**, the first time |
| 09. Have Cloudflare deploy every push, once | `mise run cloudflare:09-connect-github` | **Allow** for Cloudflare's own CLI, the first time |

After step 09 there's nothing to run: push a branch and open a pull request, and Cloudflare builds a preview and puts
its link on the pull request; merge it, and Cloudflare deploys production. For staging too,
`mise run cloudflare:09-connect-github --staging main`: then merging into `main` deploys staging, and merging `main` into
`production` deploys production.

## The commands

One name all the way through: each command is a task, `mise run cloudflare:<command>`, which runs
`mise-cloudflare <command>`. `mise run cloudflare` alone is `show`, and `mise run cloudflare:show-help` lists them all.

| Task | Does | Asks you |
|---|---|---|
| `cloudflare:05-show` | Where the Worker is, step by step, and what to run next; `--json` for a program | Nothing |
| `cloudflare:04-check` | The repo's rules (below), and whether init's files are current | Nothing |
| `cloudflare:verify` | What the repo's GitHub workflow runs: its own `setup` and `test` tasks, if it has them, then `check`. The same on your machine as on GitHub | Nothing |
| `cloudflare:07-set-secrets` | The secrets the Worker requires (`secrets.required`) that Cloudflare doesn't have yet: it asks for each, unseen; `--previews` sets the ones the Worker's Previews Base lacks, which previews don't inherit | Each value |
| `cloudflare:06-serve` | `wrangler dev`: the Worker locally, with local D1, KV and R2 and `.dev.vars` | Nothing |
| `cloudflare:08-deploy` | `wrangler deploy` on the repo's login (wrangler's flags follow: `--env staging`) | Allow, the first time |
| `cloudflare:09-connect-github` | Cloudflare deploys every push (Workers Builds): production from the repository's default branch, a preview for every other; `--staging main` for staging; `--branch`, `--build` to change those; `--off` to stop. Run again with no flags, it keeps what's set | Allow for cf, the first time |
| `cloudflare:11-preview` | A preview of what's here now (`wrangler preview`: `--name my-test`), with its own KV, D1 and R2 | Allow, the first time |
| `cloudflare:run-wrangler <...>` | Any wrangler command on the repo's login: `wrangler d1 list`, `secret put`, `tail`, … | Allow, the first time |
| `cloudflare:13-delete` | Deploy undone: the Worker deleted, then its own KV, D1 and R2 and its previews' own. An R2 bucket that still holds objects stays: it never deletes what's in one. It asks first; `--yes` doesn't | The Worker's name, to be sure |
| `cloudflare:12-login` / `cloudflare:14-logout` | The repo's login (the others make it when they need it); logout deletes it, and cf's | Allow, to log in |
| `cloudflare:15-clean` | The local state undone, as `13-delete` undoes Cloudflare's: `.wrangler/` (the local D1, KV and R2, and what deploys wrote) and `.dev.vars`, so the next `06-serve` starts from nothing. It asks first; `--yes` doesn't | y, to be sure |
| `cloudflare:template` | The template's own Worker, in a repo with none yet, named for the repo's folder, with the `mise.toml` that installs its wrangler. It never writes over a file | Nothing |
| `cloudflare:update` | The repo's own pins at their newest releases, nothing outside it: its mise tools (`mise upgrade --bump`), its packages (aube, which writes whichever lockfile the repo has) and Go modules (`go get -u`, `go mod tidy`), then `03-init` if mise-cloudflare moved (at `local` it stays). mise and aube skip a release younger than their minimum age. What changed is in git: the steps prove it | Nothing |
| `cloudflare:03-init` | After upgrading mise-cloudflare: init's files written again. The first time, before there are tasks: `mise exec github:joeblew999/mise-cloudflare@<version> -- mise-cloudflare init` | Nothing |
| `cloudflare:show-version` | This build's version | Nothing |
| `cloudflare:show-help` | Every command | Nothing |

## What init adds

| File | What |
|---|---|
| `.mise/tasks/mise-cloudflare.toml` | The tasks, `mise run cloudflare` and `cloudflare:<command>` (mise finds them there by itself) |
| `.mise/config.toml` | mise-cloudflare's pin, at the release that ran `init`: written once, then the repo's (`cloudflare:update` moves it) |
| `docs/mise-cloudflare/README.md` | This page |
| `.github/workflows/cloudflare.yml` | Checks the repo on every push and pull request |
| `.claude/skills/cloudflare/SKILL.md` | The agent skill: how Claude works on the Worker |
| `.claude/hooks/mise-cloudflare-session-start.sh`, and `.claude/settings.json` that runs it | Sets a Claude Code cloud session up when it starts |
| `.gitignore` | The Worker's local secrets, a stray token, cf's login cache, installed packages and wrangler's local data, never committed |

None is for editing. They come from inside mise-cloudflare, for the version `.mise/config.toml` pins. There's no settings
file: each choice is a command's flag, kept by Cloudflare's Workers Builds setup.

## The rules (`check`)

| The repo needs | Because |
|---|---|
| No account or resource IDs in its wrangler config, in any environment | The login picks the account; wrangler finds or creates KV, D1 and R2 by name |
| No Cloudflare token in its files, and nothing calling `wrangler login` | A token in the repo leaks; `wrangler login` is the machine's, not the repo's |
| `.dev.vars`, `.env` and `.cloudflare/` git-ignored (init does it) | Secrets, and Cloudflare's `cf` caches the account it logged into in `.cloudflare/` |
| `.dev.vars.example` listing exactly the config's `secrets.required` | So `wrangler dev` has every secret |
| wrangler pinned to an exact version in `package.json` (init does it) | So everyone, CI and Cloudflare's builds run the same wrangler |
| A `previews` block, if there is one, with every binding a preview uses but KV, D1 and R2 | A preview inherits nothing from production; KV, D1 and R2 it gets its own (below) |

## Logins and tokens

| | What it is | Where |
|---|---|---|
| **The repo's login** | What you get by clicking Allow: kept on your machine, never in the repo | Your machine, Claude Code |
| **Cloudflare's build token** | Cloudflare's own, which never leaves Cloudflare | Cloudflare deploying every push |

There's no API token anywhere, and the Cloudflare Global API Key is never used. A running Worker uses none of these:
Cloudflare connects its bindings itself. A `CLOUDFLARE_API_TOKEN` in your shell or a `.env` would beat every login
(wrangler uses it, silently, instead), so the commands refuse one.

The repo's login:

- **Its name:** the Worker's: a [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/),
  bound to the repo's folder.
- **What it may do:** what wrangler's own login asks for, read from the repo's wrangler (`wrangler login
  --scopes-list` shows the list), so it can do whatever wrangler can with any kind of binding.
- **The first time:** the first command that needs it asks. With a browser, Cloudflare's page opens: click Allow and
  pick your account. It lasts about an hour, and wrangler renews it by itself. In a Claude Code cloud session, or with
  `CLOUDFLARE_LOGIN=device`, it prints a link with the code already in it: open it on any device, your phone included.
- **After that:** each command compares what the login holds with what it needs, and asks again only when an upgraded
  wrangler needs more.
- **Safe:** every wrangler command gets `--profile` with the repo's login. `wrangler whoami`, which takes none, runs on
  the folder's binding, made by the folder's real path, and the command stops unless wrangler names the repo's login
  as the active one. The machine's own `wrangler login` is never used or changed.

## Workers Builds: previews, staging and production (`connect-github`)

`connect-github` connects the Worker to its GitHub repository on Cloudflare: the folder (a monorepo's subfolder works), a
build command (installing from the lockfile committed to git, or `--build`), `npx wrangler deploy` for the deploy
branch and `npx wrangler preview` for every other. With `--staging main`, a push to `main` deploys **staging** (the
wrangler environment `env.staging`, its own Worker with its own bindings, deployed once first with
`deploy --env staging`), a push to `production` deploys **production**, and every other branch gets a preview.
Cloudflare keeps these settings; run `connect-github` again with no flags and it keeps them.

- **A push to the deploy branch** is built and deployed by Cloudflare, with its own build token.
- **Any other branch** gets a [Worker Preview](https://developers.cloudflare.com/workers/previews/) at
  `https://<branch>-<worker>.<subdomain>.workers.dev`, and Cloudflare's GitHub app posts its URL on the pull request.
- **A merged pull request's preview** goes when its branch does: `connect-github` switches on GitHub's "Automatically delete
  head branches".

It needs, once: Cloudflare's GitHub app on the repository, and a build token on the account, which Cloudflare makes the
first time any repository is connected in its dashboard. That token covers Workers Scripts, KV, R2 and Workers Routes;
a Worker using more (D1, say) has that permission added to it in the dashboard. Workers Builds' own scope isn't one
wrangler's login can have, so `connect-github` uses Cloudflare's own CLI, [cf](https://github.com/cloudflare/cf), on its own
login; the task brings cf.

### The previews' own KV, D1 and R2

Cloudflare makes previews no KV, D1 or R2 (only Durable Objects and Containers), and `wrangler preview` creates none.
So `connect-github` and `preview` make `<worker>-previews-<binding>` for each KV, D1 and R2 binding, once, and put them in the
Worker's Previews Base on Cloudflare, the settings every new preview starts from. Nothing goes into the repo. All
previews share them; production's and staging's stay theirs. After adding a KV, D1 or R2 binding, `mise run cloudflare` says to run
`connect-github` again, which gives previews the new one. `connect-github --off` deletes them (an R2 bucket only once it's empty).

## CI

init's workflow runs on every push and pull request: it checks the commit out, installs mise and the pinned tools,
then runs `mise run cloudflare:verify` (the repo's own `setup` and `test` tasks, then `check`). Run that task on your
machine and it does what GitHub does. It needs no Cloudflare account. In CI the commands that need a login stop at once: no one is
there to click Allow, and Cloudflare's Workers Builds deploys.

## When something is refused

| Message | What to do |
|---|---|
| `refusing: CLOUDFLARE_API_TOKEN is set in this shell` | Unset it: a token overrides every login, and the repo uses none |
| `refusing: .env sets CLOUDFLARE_API_TOKEN` | Remove that line from the file |
| `wrangler isn't installed here` | Install the repo's dependencies: `package.json` pins wrangler |
| `… still don't match the repo's` | The person approved other permissions or another account: `mise run cloudflare:14-logout`, then log in again |
| A `mise run cloudflare:04-check` problem | Fix the repo; never weaken the rule |

## Claude Code

The skill teaches Claude these commands. A cloud session starts on a bare Linux machine with only the repo: init's
hook, run when a session starts, installs mise and every pinned tool, then the Worker folder's own `setup` task, so the
commands work at once; on your own machine it does nothing. A login comes as a link you open on your phone. The cloud
environment's network settings must allow mise.run, GitHub, nodejs.org, the npm registry, proxy.golang.org and
sum.golang.org. After a repository is renamed, attach it to a cloud session by its new name.
