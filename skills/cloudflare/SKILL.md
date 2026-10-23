---
name: cloudflare
description: Use when working on this repo's Cloudflare Worker (run it, deploy it, previews, staging, production, D1, KV, R2, secrets, logs) or its Cloudflare login, with github.com/joeblew999/mise-cloudflare (mise run cloudflare:*). Never create or ask for a Cloudflare API token.
---

# Cloudflare from this repo

Everything is a task; there's nothing to edit. Start with `mise run cloudflare`: it shows the steps in order, which are
done for this repo, and what to run next. Run tasks from the repo's root.

| Step | Task |
|---|---|
| 1. Run the Worker locally (no account) | `mise run cloudflare:dev` |
| 2. Deploy it | `mise run cloudflare:deploy` |
| 3. Have Cloudflare deploy every push: a preview for every pull request, then staging and production | `mise run cloudflare:github` (`-- --staging main` for staging) |
| Or instead of 3, have GitHub Actions deploy | `mise run cloudflare actions` |
| A preview of what's here now | `mise run cloudflare preview --name my-test` |
| Any other wrangler command | `mise run cloudflare wrangler d1 list` (secret put, delete, kv, r2, tail, …) |
| Check the repo | `mise run cloudflare check` |
| After upgrading mise-cloudflare | `mise run cloudflare update` |

Never run `npx wrangler`, `wrangler login` or a global wrangler: they use another login than the repo's. Never use a
Cloudflare API token.

## Logging in

A task that needs the repo's login makes it the first time, and again only when wrangler needs more permissions after
an upgrade. With a browser, Cloudflare's page opens; in a Claude Code cloud session it prints a link with the code
already in it. Give the person that link as the first line of a message: they open it on any device, click Allow and
pick the account. Never try to click Allow for them. `cloudflare github` also needs Cloudflare's own CLI's login (cf)
the first time: the same, a link to give the person.

`cloudflare actions` needs a Cloudflare token for GitHub Actions: the person runs it in their own terminal, opens the
link it gives, makes the token and pastes it, hidden. Never ask for that token or print it.

## Previews

A preview's KV, D1 and R2 are its own, made by `cloudflare deploy`, `cloudflare preview` and `cloudflare github`: leave
KV, D1 and R2 out of the wrangler config's `previews` block. Every other binding a preview uses (vars, Queues, Workers
AI, …) goes in that block; `cloudflare check` names any that are missing.

## When something is refused

| Message | What to do |
|---|---|
| `refusing: CLOUDFLARE_API_TOKEN is set in this shell` | Unset it: a token overrides every login, and the repo uses none |
| `refusing: .env sets CLOUDFLARE_API_TOKEN` | Remove that line from the file |
| `wrangler isn't installed here` | Install the repo's dependencies: package.json pins wrangler |
| `… still don't match the repo's` | The person approved other permissions or another account: `mise run cloudflare logout`, then log in again |
| A `cloudflare check` problem | Fix the repo; never weaken the rule |
