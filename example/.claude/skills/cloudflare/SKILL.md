---
name: cloudflare
description: Use when working on this repo's Cloudflare Worker (deploy, delete, D1, KV, R2, secrets, logs, previews, staging, production) or its Cloudflare login, with github.com/joeblew999/mise-cloudflare (mise run cloudflare:*). Never create or ask for a Cloudflare API token.
---

# Cloudflare from this repo

The repo has its own Cloudflare login, kept on the machine, never in the repo. Every Cloudflare task runs on it:
never run `npx wrangler`, `wrangler login` or a global wrangler, which use another login, and never use an API token.
`mise tasks` lists the tasks; run them from the repo's root.

| To | Run |
|---|---|
| Run the Worker locally (no account) | `mise run cloudflare:dev` |
| Deploy | `mise run cloudflare:deploy` |
| Any other wrangler command | `mise run cloudflare:wrangler -- d1 list` (secret put, delete, kv, r2, tail, …) |
| See the login, its account and permissions | `mise run cloudflare:status` |
| Check the repo | `mise run cloudflare:check` |
| Deploy every push from GitHub: previews, staging, production | `mise run cloudflare:builds`, once per repo |
| After adding a KV, D1 or R2 binding, give previews theirs | `mise run cloudflare:previews` |
| Update the tasks, this skill and the workflow after upgrading | `mise run cloudflare:update` |

## Logging in

A task that needs the login makes it, the first time, and again only when wrangler asks for more permissions after an
upgrade. On a machine with a browser, Cloudflare's page opens; in a Claude Code cloud session (or with
`CLOUDFLARE_LOGIN=device`) it prints a link with the code already in it. Give the person that link as the first line
of a message: they open it on any device, click Allow and pick the account. Never try to click Allow for them.

`cloudflare:builds` also needs Cloudflare's own CLI's login (cf), the first time: the same, a link to give the person.
Deploy once first: Workers Builds attaches to a deployed Worker. If Cloudflare can't see the repository, the person
gives Cloudflare's GitHub app access to it on GitHub.

## Settings

`cloudflare.toml` is the repo's own settings, every line optional: staging is `staging = "main"` under `[builds]`
(with the wrangler environment `env.staging`); GitHub Actions deploying instead of Cloudflare is `deploy = true` under
`[workflow]`, and then the person runs `mise run cloudflare:ci-token` in their own terminal, which gives them a link to
make CI's token and stores it as GitHub secrets. Never ask for that token or print it.

Previews get no KV, D1 or R2 from Cloudflare, so `cloudflare:previews` gives them their own; leave KV, D1 and R2 out
of the wrangler config's `previews` block. Every other binding a preview uses (vars, Queues, Workers AI, …) goes in
that block, as Cloudflare's docs say: `cloudflare:check` names any that are missing.

`.mise/tasks/mise-cloudflare.toml`, this skill, `.github/workflows/cloudflare*.yml` and
`.claude/hooks/mise-cloudflare-session-start.sh` are written by mise-cloudflare: never edit them.

## When something is refused

| Message | What to do |
|---|---|
| `refusing: CLOUDFLARE_API_TOKEN is set in this shell` | Unset it: a token overrides every login, and the repo uses none |
| `refusing: .env sets CLOUDFLARE_API_TOKEN` | Remove that line from the file |
| `wrangler isn't installed here` | Install the repo's dependencies: package.json pins wrangler |
| `… still don't match the repo's` | The person approved other permissions or another account: `mise run cloudflare:logout`, then log in again |
| A `cloudflare:check` problem | Fix the repo; never weaken the rule |
