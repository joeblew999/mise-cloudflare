# cloudflare-zero

Cloudflare without the token mess. A repo that uses this gets its own Cloudflare login, made with one browser click. No API tokens to create, nothing machine-wide, and no Cloudflare scripts in the repo itself.

## Use it in your repo

Your repo carries config only. The tasks come from this repo, through mise's git includes.

```toml
# mise.toml
[settings]
experimental = true   # mise needs this for git includes

[tools]
node = "26.11.1"
aube = "2.8.1"
jq = "1.8.2"

[env]
CF_PROFILE = "my-repo"   # the repo's own Cloudflare login (a wrangler auth profile)
CF_SCOPES = "account:read user:read workers:write workers_kv:write workers_scripts:write d1:write"   # what it may do

[task_config]
includes = ["git::https://github.com/joeblew999/cloudflare-zero.git//shared?ref=main"]
```

You also need:

- `wrangler.jsonc` with no `account_id` and no resource IDs: bindings only, plus `"secrets": { "required": [...] }`
- `.dev.vars.example`, with a dummy value for each required secret
- wrangler pinned to an exact version in `package.json`
- `.gitignore` covering `.dev.vars`, `.env*`, `.wrangler/` and `node_modules/`

[example/](example/) is exactly that.

## What a developer does

| Task | Does | Cloudflare account |
|---|---|---|
| `mise run cloudflare:setup` | Installs the pinned wrangler and makes `.dev.vars` from the example | No |
| `mise run cloudflare:dev` | Runs the Worker locally, with local D1, KV and R2 | No |
| `mise run cloudflare:test` | Checks the repo rules, then runs a temporary local Worker | No |
| `mise run cloudflare:deploy` | First time: the browser opens, you click Allow and pick the account. Then it creates D1, KV and R2, asks once for each missing secret, deploys and prints the URL. After that it asks nothing | Yes |
| `mise run cloudflare:test --prod` | The same checks against the live Worker | Yes |
| `mise run cloudflare:status` | The login, its account and scopes, the URL, the resources, and any stray tokens | Yes |
| `mise run cloudflare:teardown` | Deletes the Worker and the D1, KV and R2 it created | Yes |
| `mise run cloudflare:clean` | Removes local state and the repo's login | No |

## Rights: how a repo asks for them, and how people get them

- **The repo says what it may do:** `CF_SCOPES` in `mise.toml`, the OAuth scopes from `wrangler login --scopes-list`.
- **Everyone gets the same:** the first deploy opens Cloudflare's consent page listing exactly those scopes. Whoever clicks Allow, you or any other developer, gets a login that can do only that.
- **When the repo needs more:** add the scope to `CF_SCOPES`. The next deploy sees the login lacks it and opens the browser once more.
- **Whose account:** each person picks the account at the consent page. To deploy to a shared account, they need a role on it that allows those actions. Members are invited in the Cloudflare dashboard.
- **R2:** works with `workers:write`. wrangler has no R2 scope, yet creating, using and emptying buckets all worked, as the proofs show.
- **Basin, tested 23 Oct 2026 with a repo's own login:** turning on Basin Catalog, Basin SQL queries (through wrangler and through Cloudflare's API) and creating a Basin Pipeline stream all work, with no API token. A fake token gets HTTP 403. One thing needs a stored API token: a Basin Pipeline's sink into Basin Catalog. Cloudflare keeps that token and uses it itself, and it rejected the login's token: the pipeline went to `failed` with "The sink token is expired or invalid". `mise run prove-basin` shows this.

## Why it's safe

- The login is a [wrangler auth profile](https://developers.cloudflare.com/workers/wrangler/profiles/) (beta), named after the repo and bound to its folder. The machine's default `wrangler login` and other repos' logins are untouched.
- A `CLOUDFLARE_API_TOKEN` in your shell or in a `.env` would silently override the login, so the tasks refuse to run with one (except in CI).
- The repo never holds account IDs, resource IDs, tokens or secret values. `cloudflare:test` fails if it does.

## Proof

Both tasks below were run on 23 Oct 2026, and both passed.

- `mise run prove`: covers `example/` end to end. A local test with no login. Deploy on the repo's own login, created with one click the first time. A live test of D1, KV, R2 and the secret. A second deploy that asks nothing, keeps the same resources and leaves the repo unchanged. Stray tokens refused, in the shell and in `.env`. Teardown leaves nothing. The machine's default login is untouched throughout. `--clean` also removes the login.
- `mise run prove-other-repo`: makes a brand-new repo in a temporary folder with config only (53 lines including the Worker) and the shared tasks pulled from GitHub. Then local test, deploy on its own login, live test, teardown and clean.
