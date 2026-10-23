# Testing

| Task | What | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests, after every change: the rules, the login's scope check against the repo's own wrangler, the device link, the tasks for repos with and without Go, the skill and the workflow it writes, example/ current. CI runs them | No |
| `mise run prove` | Before a release, about 40 minutes, in [e2e/](../e2e/): example/ as a developer uses it, following what the guide says (the secret, a deploy, the live D1, KV, R2 and secret, stray tokens refused); a brand-new repo from GitHub set up by `init` alone; Cloudflare deploying every push (production from a throwaway branch, a pull request's preview with its own D1, KV, R2 and secret, gone with its branch); GitHub Actions deploying (a push, a pull request's preview, gone when it closes). Everything deleted after | Yes |


## Layout

| Folder | What |
|---|---|
| [cmd/cloudflare/](../cmd/cloudflare/) | the command |
| [mise/](../mise/) | the tasks, workflow and Claude Code hook `cloudflare init` writes into a repo |
| [skills/](../skills/) | the agent skill `cloudflare init` installs |
| [internal/](../internal/) | the login (`wrangler`), cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](../example/) | a Worker repo that uses it, exactly as any other repo would (`mise run example` stamps it) |
| [e2e/](../e2e/) | `mise run prove` |
