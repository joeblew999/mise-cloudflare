---
name: cloudflare
description: Use when working on this repo's Cloudflare Worker with wrangler (deploy, delete, D1, KV, R2, secrets, logs) or its Cloudflare login, in a Claude Code cloud session or on a machine, with github.com/joeblew999/mise-cloudflare (go tool cloudflare, mise run cloudflare:*). Never create or ask for a Cloudflare API token.
---

# Cloudflare from this repo

The repo has its own Cloudflare login: a wrangler auth profile named in `cloudflare.toml` (default: the Worker's name),
with exactly the OAuth scopes listed there. Every wrangler command runs on that login through `go tool cloudflare`, never
on the machine's own `wrangler login`, and never with an API token. Run everything from the repo's root.

## Run wrangler

```sh
mise run cloudflare:deploy                       # wrangler deploy; what it deployed goes to .wrangler/output.json
mise run cloudflare:wrangler -- d1 list --json   # any wrangler command: secret put, delete, kv, r2, tail, …
```

Both log in first if needed. Never run `npx wrangler`, `wrangler login` or a global wrangler: they use another login.
`mise run cloudflare:dev` runs the Worker locally (wrangler dev), with no login.

## Deploys from GitHub: previews, staging, production

`mise run cloudflare:builds` sets Cloudflare's Workers Builds up for the Worker, once per repo: a push to the branch
deploys (`wrangler deploy`), and every other branch gets a preview (`wrangler preview`) with its URL on the pull
request. Cloudflare does both with its own build token, so GitHub never holds a Cloudflare credential. Deploy once
first (`mise run cloudflare:deploy`): Workers Builds attaches to a deployed Worker. The first time it needs a cf login
(Cloudflare's own CLI, whose login can have Workers Builds' scope): give the person its link and code, as below. For
staging, a wrangler environment (`env.staging`) is its own Worker: run it again with `CLOUDFLARE_ENV=staging` and
`--branch main`, and the production Worker with its own branch. `--off` removes the setup. If Cloudflare can't see the
repository, the person gives Cloudflare's GitHub app access to it on GitHub.

## GitHub Actions

`mise run cloudflare:workflow` writes the repo's workflow (checks on every push and pull request, no Cloudflare
credential). With `[workflow] deploy = true` in `cloudflare.toml` it also deploys: production from its branch, a preview
`pr-<number>` per pull request, its URL commented, deleted when the pull request closes. Then
`mise run cloudflare:ci-token` makes the token it deploys with and stores it as the repository's GitHub secrets: never
ask the person for a token, and never print one. A binding beyond Workers Scripts, KV and R2 (D1, Queues, …) needs its
permission group in `[workflow] permissions` (`"D1 Write"`), then `ci-token` again.

## Files mise-cloudflare writes

`.mise/tasks/cloudflare.toml`, `.claude/skills/cloudflare/SKILL.md`, the workflow and
`.claude/hooks/mise-cloudflare-session-start.sh` are stamped from mise-cloudflare at its version: never edit them. After
upgrading, run `mise run cloudflare:tasks`, `cloudflare:skill`, `cloudflare:workflow` and `cloudflare:claude`;
`cloudflare:check` names any that are out of date. For Worker Previews, a wrangler config needs a `previews` block with each storage binding in it, every KV, D1 and R2 one
pointed at an existing, preview-safe resource such as staging's: Cloudflare makes previews none of their own.

## Logging in

`mise run cloudflare:login` logs in once. On a machine with a browser, it opens; in a Claude Code cloud session (or with
`CLOUDFLARE_LOGIN=device`) wrangler prints a link and a code instead. Give the person the link: they open it on any
device, their phone included, click Allow and pick the account. You can't click Allow for them, and must never try.

When `cloudflare.toml`'s scopes change, the next task logs in once more, so the login always holds exactly those
scopes. A task that fails with a permission error needs a scope: add it to `cloudflare.toml` (the names come from
`wrangler login --scopes-list`), then run the task again, and tell the person a login is coming.

## When something is refused

| Message | What to do |
|---|---|
| `refusing: CLOUDFLARE_API_TOKEN is set in this shell` | Unset it. A token overrides every login; the repo doesn't use tokens |
| `refusing: .env sets CLOUDFLARE_API_TOKEN` | Remove that line from the file |
| `wrangler isn't installed here` | Install the repo's dependencies: package.json pins wrangler |
| `… still don't match the repo's` | The person approved different scopes or another account: run `mise run cloudflare:logout`, then log in again |

## The rules

`mise run cloudflare:check --json` fails (`ok: false`, one line per problem) when the repo would leak an account or
its login: an account or resource ID in the wrangler config (any environment), a token or the machine-wide
`wrangler login` in its files, `.dev.vars` or `.env` not git-ignored, `.dev.vars.example` not listing exactly
`secrets.required`, or wrangler not pinned to an exact version. Fix the repo; never weaken the rule.

`mise run cloudflare:status --json` says which login the repo has, its account and scopes, and any stray token. It
never logs in.
