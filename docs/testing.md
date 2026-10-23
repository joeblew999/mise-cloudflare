# Testing

| Task | What | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests, after every change: the rules, the login's scope check against the repo's own wrangler, the device link, the tasks for repos with and without Go, the skill and the workflow it writes, example/ current. CI runs them | No |
| `mise run prove` | Before a release, a few minutes: in example/, a local Worker with no login; a deploy on its own login; the live D1, KV, R2 and secret; stray tokens refused; everything deleted again; and a brand-new repo from GitHub set up by `init` alone | Yes |

Previews, staging and production aren't tested here: Cloudflare's own check on each pull request shows a preview's
build and its link, and each deploy's result. The deploy workflow's runs show GitHub Actions'.

## Layout

| Folder | What |
|---|---|
| [cmd/cloudflare/](../cmd/cloudflare/) | the command |
| [mise/](../mise/) | the tasks, workflow and Claude Code hook `cloudflare init` writes into a repo |
| [skills/](../skills/) | the agent skill `cloudflare init` installs |
| [internal/](../internal/) | the login (`wrangler`), cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](../example/) | a Worker repo that uses it, exactly as any other repo would (`mise run example` stamps it) |
| [e2e/](../e2e/) | `mise run prove` |
