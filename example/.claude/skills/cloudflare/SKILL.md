---
name: cloudflare
description: Use when working on this repo's Cloudflare Worker with wrangler (deploy, delete, D1, KV, R2, secrets, logs) or its Cloudflare login, in a Claude Code cloud session or on a machine, with github.com/joeblew999/cloudflare-zero (go tool cloudflare, mise run cloudflare:*). Never create or ask for a Cloudflare API token.
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
`wrangler dev` needs no login: run the repo's own wrangler for it.

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
