---
name: cloudflare
description: Use when working on this repo's Cloudflare Worker (run it, deploy it, previews, staging, production, D1, KV, R2, secrets, logs) or its Cloudflare login, with github.com/joeblew999/mise-cloudflare (mise run cloudflare:*). Never create or ask for a Cloudflare API token.
---

# Cloudflare from this repo

Everything is a task, one per command: `mise run cloudflare:<command>`. Start with `mise run cloudflare:status` (or
just `mise run cloudflare`): it shows the steps, which are done, and what to run next.

| Step | Task |
|---|---|
| 1. Run the Worker locally (no account) | `mise run cloudflare:dev` |
| 2. Deploy it | `mise run cloudflare:deploy` |
| 3. Have Cloudflare deploy every push: a preview for every pull request, then production | `mise run cloudflare:github` (`-- --staging main` for staging) |
| A preview of what's here now | `mise run cloudflare:preview --name my-test` |
| Any other wrangler command | `mise run cloudflare:wrangler d1 list` (secret put, kv, r2, tail, …) |
| Check the repo | `mise run cloudflare:check` |
| Set the secrets the Worker requires (status names them) | `mise run cloudflare:secret` (`--previews` for its previews) |
| Deploy undone: the Worker, its KV, D1 and R2, its previews' own | `mise run cloudflare:delete` (it asks first) |
| After upgrading mise-cloudflare | `mise run cloudflare:init` |

Never run `npx wrangler`, `wrangler login` or a global wrangler: they use another login than the repo's. Never use a
Cloudflare API token.

## Logging in

A task that needs the repo's login makes it the first time. In a Claude Code cloud session it prints a link with the
code already in it: give the person that link as the first line of a message, so they open it on any device, click
Allow and pick the account. Never try to click Allow for them. `mise run cloudflare:github` also needs Cloudflare's own
CLI (cf) logged in the first time, the same way.

## Previews

`mise run cloudflare:github` and `preview` give previews their own KV, D1 and R2: leave those out of the wrangler
config's `previews` block. Every other binding a preview uses (vars, Queues, Workers AI, …) goes in that block;
`mise run cloudflare:check` names any that are missing.

## When something is refused

| Message | What to do |
|---|---|
| `refusing: CLOUDFLARE_API_TOKEN is set in this shell` | Unset it: a token overrides every login, and the repo uses none |
| `refusing: .env sets CLOUDFLARE_API_TOKEN` | Remove that line from the file |
| `wrangler isn't installed here` | Install the repo's dependencies: package.json pins wrangler |
| `… still don't match the repo's` | The person approved other permissions or another account: `mise run cloudflare:logout`, then log in again |
| A `mise run cloudflare:check` problem | Fix the repo; never weaken the rule |
