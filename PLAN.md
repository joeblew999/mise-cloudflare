# Cloudflare without the token mess

Plan, 23 Oct 2026. Built and proven the same day: see README.md (`mise run prove`, `mise run prove-other-repo`). Still to do: a token template link for what a login can't do (Basin SQL), CI, a second account, and a release tag for other repos to pin.

## Goal

Anyone who clones a repo that uses this one gets everything Cloudflare needs set up by the repo itself, as easily as possible:

- **The repo says which Cloudflare permissions it needs.** Getting them is one browser click. On the normal path nobody creates a token or picks scopes.
- **Nothing global, nothing shared.** The login belongs to the repo, not to the machine and not to other repos. Today `wrangler login` is machine-wide, so every repo on a machine shares one login.
- **Local dev needs no Cloudflare account.** `mise install`, then setup and dev.
- **Deploy works on whichever account you log in to**, your own or someone else's. The repo holds no account ID, no resource IDs and no secret values.
- **Where Cloudflare really needs a stored token**, the repo gets it as close to one click as Cloudflare allows, and says why.
- **Scripts prove all of this**, here and on other repos.

This repo holds the shared tasks and a throwaway example Worker that proves them.

## What Cloudflare offers

From Cloudflare's docs and from the code of wrangler 4.151.3, 23 Oct 2026.

| Need | Cloudflare's way | What matters for us |
|---|---|---|
| A login per repo | **Auth profiles** (beta since July 2026). `wrangler auth create <name> --scopes …` makes a named login, and `wrangler auth activate <name> <dir>` binds it to a folder and everything under it. [Docs](https://developers.cloudflare.com/workers/wrangler/profiles/) | The login lives in wrangler's own folder under its name (`~/Library/Preferences/.wrangler/config/<name>.toml`), never in the repo. The machine's default `wrangler login` stays untouched. Any tool run inside the folder gets the same login without changes. |
| What the login may do | `--scopes`. A plain `wrangler login` asks for all of them, and since Aug 2026 the consent page lets you untick optional ones. [Changelog](https://developers.cloudflare.com/changelog/post/2026-08-22-wrangler-mcp-optional-oauth-scopes/) | `wrangler login --scopes-list` prints 30 scopes (`d1:write`, `pipelines:write`, `workers_scripts:write` and more), none of them for R2, Basin Catalog or Basin SQL. Logins can't be limited to single resources. [Docs](https://developers.cloudflare.com/workers/authorization/) |
| Resources (KV, D1, R2, Queues and more) | **Automatic provisioning**: write bindings without IDs, and deploy creates the resources, named after the Worker. [Docs](https://developers.cloudflare.com/workers/wrangler/configuration/#automatic-provisioning) | Later deploys find them again without IDs. wrangler writes the IDs into the config only on an interactive deploy; its code skips that under `CI=1`. Not covered: Hyperdrive, Vectorize, Secrets Store, Pipelines, Basin. |
| Secret names | `secrets.required` in the wrangler config, plus `.dev.vars.example` with dummy values (what Deploy Buttons read). [Docs](https://developers.cloudflare.com/workers/platform/deploy-buttons/) | A new Worker missing a required secret won't deploy, so the first deploy passes its secrets with `--secrets-file`. |
| A token for other tools | `wrangler auth token` prints the login's current token, refreshed. [Changelog](https://developers.cloudflare.com/changelog/post/2025-12-18-wrangler-auth-token/) | For API calls outside wrangler. Never stored. |
| A token that has to exist | **Token template links**: a dashboard link with the permissions filled in. You click Create and copy the token. [Docs](https://developers.cloudflare.com/fundamentals/api/how-to/account-owned-token-template/) | The closest Cloudflare gets to one click for a stored token. |
| Basin SQL | Needs an API token with Basin SQL Read, Basin Catalog and R2 storage. No Worker binding queries it without one. [Docs](https://developers.cloudflare.com/basin-sql/query-data/#authentication) | So a repo that queries Basin SQL needs a stored token. We can't avoid it, only make it easy (a token template link). |
| Deploys from CI | GitHub Actions with `CLOUDFLARE_API_TOKEN` as a secret ([docs](https://developers.cloudflare.com/workers/ci-cd/external-cicd/github-actions/)), or **Workers Builds**: connect the repo once in the dashboard and Cloudflare makes and keeps the token itself ([docs](https://developers.cloudflare.com/workers/ci-cd/builds/configuration/#api-token)) | Cloudflare offers no GitHub OIDC login, so CI with no token at all isn't possible. Workers Builds' own token gets no D1 permission by default. |
| A new CLI | `cf` (beta since Sep 2026) has the same profiles and token rules, with logins of its own. [Docs](https://developers.cloudflare.com/cf/get-started/) | Cloudflare says not to run `cf deploy` in wrangler projects, so wrangler projects stay on wrangler. |

Traps found on the way:

- **A stray token wins over everything.** `CLOUDFLARE_API_TOKEN` beats every profile, and wrangler also reads it from a `.env` in whatever folder it runs in ([docs](https://developers.cloudflare.com/workers/wrangler/profiles/#profiles-and-environment-variables); nimbalyst hit the same: [wranglerPaths.ts](https://github.com/nimbalyst/nimbalyst/blob/HEAD/packages/electron/src/main/services/cloudflareSandbox/wranglerPaths.ts)). A token left in your shell, or in a `.env`, silently takes over. Our tasks refuse it outside CI.
- **Remote bindings ignore `--profile`.** They find the profile from the working folder (same file, wrangler 4.125). So we bind with `auth activate` rather than pass `--profile`.
- **`XDG_CONFIG_HOME` also isolates wrangler**, by moving its whole folder into the repo. Tested 23 Oct: pointed at an empty folder, wrangler saw no login while the machine's login stayed in place, and it wrote only logs, metrics and a cache there. Profiles are Cloudflare's documented way, though, and Gerard chose them (23 Oct).

## The design

What a developer does in a repo that uses this one, on any machine:

1. `mise install`: every tool, pinned.
2. `cloudflare:setup`: dependencies, `.dev.vars` from `.dev.vars.example`, the local database. No account.
3. `cloudflare:dev`: runs locally. No account.
4. `cloudflare:deploy`: if the repo's login isn't there yet, it opens the browser once with the repo's scopes, and you click Allow and pick the account. Then it creates any missing resources, asks once for each secret not yet set on the Worker, runs migrations, deploys and prints the URL. Running it again changes nothing.
5. Only for repos that need a stored token (Basin SQL, for example): the first deploy prints a token link with the permissions filled in. You click Create and paste the token once.

The repo holds the scopes (one list in mise.toml), bindings without IDs, secret names and `.dev.vars.example`. It never holds account IDs, resource IDs, tokens or secret values. The login is a profile named after the repo (`CF_PROFILE`), bound to the repo's folder.

## Minimal code in each repo

Cloudflare's tools already do the heavy lifting: profiles, provisioning, `secrets.required`. What's left is a little glue: log the repo in if it isn't, refuse stray tokens, ask for missing secrets and then deploy, and the rules check. That glue is written **once, here**, as one folder of mise tasks. Other repos don't copy it. mise pulls it from this repo by git URL, pinned to a tag ([mise docs: remote git includes](https://mise.jdx.dev/tasks/task-discovery.html#remote-git-includes); our mise is 2026.10.17).

What each repo then carries is configuration only, no scripts:

```toml
# mise.toml
[settings]
experimental = true                                              # mise needs this for git includes

[env]
CF_PROFILE = "my-repo"                                           # the login's name
CF_SCOPES = "account:read user:read workers_scripts:write d1:write"  # what the repo may do

[task_config]
includes = [
  "git::https://github.com/joeblew999/cloudflare-zero.git//shared?ref=<tag>",  # the shared tasks
  "mise-tasks",                                                  # the repo's own; same name overrides shared
]
```

Plus `wrangler.jsonc` without IDs and with `secrets.required`, `.dev.vars.example`, and wrangler in `package.json`.

`example/` is built as exactly that kind of repo: config and a tiny Worker, using the shared tasks the way any other repo would. So how small it is proves how small every repo's share is.

Checked 23 Oct with mise 2026.10.17, in throwaway folders:
- **Task names.** An included folder's own name is dropped: including `mise-tasks/tools` gave a task called `test`, not `tools:test`. Subfolders keep theirs: including `shared/`, which contains `cloudflare/status`, gave `cloudflare:status`. Hence `shared/cloudflare/`.
- **Git includes work, but are experimental.** A git include of a tasks folder from a repo on GitHub loaded its tasks, but only once the including repo's own `mise.toml` set `experimental = true`; without it mise skips the include with a warning. A `git::file://` URL to a repo on disk loaded nothing and gave no error. So before a change is pushed, we test with a plain folder path in `includes`, which works.
- **Shared tasks must be self-contained.** Including tasks whose descriptions use a setting the including repo doesn't define (`{{env.X}}`) failed: mise refuses to load them. Shared tasks may only use settings the including repo is told to set (`CF_PROFILE`, `CF_SCOPES`).
- **Your own tasks.** `includes` replaces mise's default task folders, so a repo must list its own folder too (as above).

## Layout

```
mise.toml            this repo's own tools, profile (cloudflare-zero) and the shared tasks
shared/cloudflare/   the shared tasks, with their helpers; other repos include shared/
example/             a minimal repo: its own mise.toml (includes ../shared), package.json,
                     wrangler.jsonc, src/worker.ts, .dev.vars.example, .gitignore
```

The example Worker uses the bindings most Workers need: D1, KV, R2 and one required secret.

One leak to handle: mise also reads the config files of every folder above the one you're in, so in `example/` it loads this repo's root `mise.toml` as well. The runs that prove anything (steps 4 and 11) use a copy outside the repo. The other runs set `MISE_CEILING_PATHS` so mise stops at `example/`.

The tasks, as they run in `example/`:

| Task | Does | Needs an account |
|---|---|---|
| `cloudflare:setup` | aube install, `.dev.vars` from the example if missing, local D1 migrations | No |
| `cloudflare:dev` | `wrangler dev` | No |
| `cloudflare:test` | The repo-rules check, then a temporary local Worker shows D1, KV, R2 and the dummy secret working | No |
| `cloudflare:test [repo]` | The rules check alone, on any repo | No |
| `cloudflare:test --prod` | The same checks against the deployed Worker | Yes |
| `cloudflare:deploy` | Login if needed, then resources, secrets, migrations, deploy, URL | Yes |
| `cloudflare:status` | Which profile, whether it's logged in, its account and scopes, the URL and whether it answers, the resources. Built so far: the logins and stray tokens, with `--baseline` | Yes |
| `cloudflare:teardown` | Deletes the Worker and the resources it created | Yes |
| `cloudflare:clean` | Removes local state and the repo's profile (`wrangler auth delete`) | No |

The rules check, the part other repos reuse, fails when:

- a tracked wrangler config has `account_id` or a resource ID (`database_id`, a KV `id` or `preview_id`)
- a tracked file holds a Cloudflare token
- `.dev.vars` or `.env` isn't git-ignored, or `.dev.vars.example` doesn't list exactly the names in `secrets.required`
- wrangler isn't pinned in the repo, or a task calls the global `wrangler login`

## Proofs

Each one is a task run, with its output recorded in `FINDINGS.md`.

| # | Proves | How |
|---|---|---|
| 1 | A fresh developer needs no account | Clone into a temp folder with a bare environment (only mise, no wrangler login visible). Then `mise install`, `cloudflare:setup`, `cloudflare:test`. |
| 2 | The check catches real problems | It passes on `example/` and fails on a copy of it with an `account_id` and a `database_id` added. |
| 3 | Deploying takes one click | With no profile yet, `cloudflare:deploy` opens the browser once. You click Allow, the resources get created, the secret is asked for once, and the URL answers with D1, KV, R2 and the secret working. |
| 4 | It uses the fewest scopes | Start with the fewest scopes, and add one per failure, writing each down. That ends with the exact list. |
| 5 | Re-running is safe | A second `cloudflare:deploy` asks nothing and keeps the same resource IDs. |
| 6 | The repo stays clean | `git status` is empty after deploying, and the rules check still passes. |
| 7 | The login is isolated | The machine's default login is unchanged (its file and `wrangler whoami` outside the repo). A `CLOUDFLARE_API_TOKEN` in the shell or in a `.env` is refused with a clear message. |
| 8 | Nothing is left behind | After `cloudflare:teardown`, `cloudflare:status` shows no Worker and no resources. `cloudflare:clean` removes the profile. |
| 9 | It works on another account | The same deploy on a second Cloudflare account. A free account covers D1 and KV; R2 must be turned on first. Needs someone with a second account. |
| 10 | It works for other repos | The rules check on some of our other repos says what each one must change. |

## Steps, with you watching

One step at a time. After each one I show you the real output and stop. The next step starts only when you say so. If something fails, I show the failure and stop; I don't route around it.

| Step | What I do | It works when | You |
|---|---|---|---|
| 1. Baseline | Record the machine as it is: `cloudflare:status --baseline` (wrangler's logins as checksums, profiles, no `CLOUDFLARE_API_TOKEN`). Changes nothing | The output is saved, so step 9 can compare against it | Look |
| 2. Example, local | Write the rest of `example/` (D1, KV, R2 with no IDs, one required secret, `.dev.vars.example`) and the shared tasks; in `example/` run `cloudflare:setup` and `cloudflare:test` | `cloudflare:test` shows D1, KV, R2 and the dummy secret all working, with no account | Review the files and output |
| 3. Rules check | Run the check on `example/`, then on a copy with IDs added | It passes on `example/` and fails on the copy, naming the `account_id` and `database_id` | Look |
| 4. Fresh developer | Clone into a temp folder with a bare environment; `mise install`, `cloudflare:setup`, `cloudflare:test` | Same result as step 2, and wrangler there sees no login | Look |
| 5. Login | `cloudflare:deploy` creates the `cloudflare-zero-example` profile with the fewest scopes and opens the browser | `wrangler auth list` shows `cloudflare-zero-example` bound to the example folder; the default login file is unchanged | **Click Allow in the browser** |
| 6. First deploy | The same run carries on: resources, secret, migrations, deploy. A missing scope shows as an error; I add that one scope and log in again | `cloudflare:test --prod` shows D1, KV, R2 and the secret working on the live URL. Every scope added is written down | Click Allow again for each scope added |
| 7. Re-run, clean repo | `cloudflare:deploy` again, then `git status` and the rules check | No prompts, the same resource IDs, `git status` empty, the check passes | Look |
| 8. Stray tokens | Deploy with a dummy `CLOUDFLARE_API_TOKEN` in the shell, then in a `.env` | Both refused, with a message saying why | Look |
| 9. Teardown | `cloudflare:teardown`, `cloudflare:status`, `cloudflare:clean` | No Worker and no resources left, no `cloudflare-zero-example` profile, the default login file matches step 1 | Look |
| 10. You, from zero | You run `cloudflare:deploy` in your own terminal, as a new developer would. This also tries the secret prompt, which I can't type into | One click, one secret typed, and it's live | **Run it yourself** |
| 11. Another repo, minimal | A new repo in a temp folder holding only the config above and a tiny Worker, with the shared tasks pulled in by git URL. Run setup, test, deploy, teardown | It works the same as `example/`; its line count shows what each repo carries | Look; click Allow once |
| 12. Other repos | The rules check on a few of our other repos. Read-only | It lists what each repo must change | Look |
| 13. Write up | `FINDINGS.md` with the outputs, then tag a release other repos can pin | Every output in it is real | Decide whether to tag |

After that, still with you: a second Cloudflare account (proof 9).

## Decisions for you

1. **Deploys from CI.** Options: none for now (my pick), GitHub Actions with one account token from a template link, or Workers Builds (no token in GitHub at all, but no D1 by default, and we'd have to check that mise runs in its build).
2. **Where a stored token lives** (Basin SQL and the like): in the keychain via fnox, under a key only that repo uses (my pick).
3. **Proof 9 needs a second Cloudflare account.**
