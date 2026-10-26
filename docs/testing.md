# Testing

| Task | What | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests, after every change: the rules, the login's scope check against the repo's own wrangler, the device link, the tasks, skill and workflow init writes, example/ current. CI runs them | No |
| `mise run smoke` | A new repo as a user sets it up: mise-cloudflare from mise, `init`, then `check` passes, the status screen's next step, and the Worker running locally with D1, KV, R2 and its secret. This checkout's build; `--version <v>` tests a release from GitHub instead. CI runs it on every push, and the release on itself | No |
| `mise run prove` | Before merging a change to how it deploys, about 30 minutes, in [e2e/](../e2e/): example/ as a developer uses it (the secret, a deploy, the live D1, KV, R2 and secret, stray tokens refused), then Cloudflare deploying every push (production from a throwaway branch, a pull request's preview with its own D1, KV, R2 and secret, gone with its branch). Everything deleted after | Yes |

## Layout

| Folder | What |
|---|---|
| [cmd/mise-cloudflare/](../cmd/mise-cloudflare/) | the command |
| [mise/](../mise/) | the tasks, workflow and Claude Code hook `init` writes into a repo |
| [skills/](../skills/) | the agent skill `init` installs |
| [internal/](../internal/) | the login (`wrangler`), cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](../example/) | a Worker repo that uses it, on this checkout's build (`mise run example` writes init's files into it) |
| [e2e/](../e2e/) | `mise run smoke` and `mise run prove` |
