# Proofs

Everything mise-cloudflare claims is proven on a real Cloudflare account, and again after each change. The proofs are
Go tests in [e2e/](../e2e/), run only with `CLOUDFLARE_E2E=1` (the tasks set it), so `go test ./...` never touches
Cloudflare. Each refuses to start while its Worker or its D1, KV or R2 exists (another machine may be proving it), and
deletes what it made even when it fails.


| Task | Proves | Cloudflare |
|---|---|---|
| `mise run test` | The Go tests: the rules, the scope comparison, the login's guard, the device link, the tasks for repos with and without Go, the skill, example/ current | No |
| `mise run prove` | In example/: a local Worker with no login; deploy on its own login, bound by its real path; the live D1, KV, R2 and secret; a second deploy that asks nothing and writes nothing into the repo; stray tokens refused; everything deleted again; the machine's default login untouched | Yes |
| `mise run prove-other-repo` | A new repo gets mise-cloudflare from GitHub (`go get -tool`, ref `MISE_CLOUDFLARE_REF`), runs `init`, then the same proof on a login of its own | Yes |
| `mise run prove-builds` | Workers Builds, with Cloudflare's own token: staging from one throwaway branch, production from another, both with KV and R2 working; a preview of a pull request's branch, on the previews' own KV and R2 (`cloudflare:previews`); the preview gone once its branch is deleted; then all of it removed. main is never touched | Yes |
| `mise run prove-actions` | GitHub Actions deploying: `ci-token` stores CI's token; a throwaway branch with the stamped workflow deploys a Worker nothing else deploys, with its KV; a pull request gets its `pr-<number>` preview, on the previews' own KV, URL commented; closing it deletes the preview; then all of it removed | Yes |
| `mise run prove-basin` | Basin on a repo's own login: works up to a Basin Pipeline's catalog sink, which needs a stored API token Cloudflare keeps ("The sink token is expired or invalid") | Yes |


## Layout

| Folder | What |
|---|---|
| [cmd/cloudflare/](../cmd/cloudflare/) | the command |
| [config/](../config/) | `cloudflare.toml` |
| [mise/](../mise/) | its tasks, written into a repo by `cloudflare tasks` |
| [skills/](../skills/) | its agent skill, installed by `cloudflare skill` |
| [internal/](../internal/) | the login (`wrangler`), cf (`cf`), the Worker's config as wrangler reads it (`wconfig`), the rules (`rules`) |
| [example/](../example/) | a Worker repo that uses it, exactly as any other repo would (`mise run example` stamps it) |
| [e2e/](../e2e/) | the proofs, and their Workers in [e2e/testdata/](../e2e/testdata/) |
