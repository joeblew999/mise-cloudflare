# Examples

Each folder here is another repo: a user's repo that uses mise-cloudflare, kept inside this one so every change is
tried on it. It is set up exactly as any repo is:

- **its own git**: `.git`, which `mise run example:00-git-init` makes; it is never committed here;
- **its own `mise.toml`**, pinning mise-cloudflare at `local`: this checkout's build, which `mise run tool:install` links.
  A real repo pins a release instead;
- **what `mise-cloudflare init` writes**, all inside it: its tasks, its workflow, its Claude Code hook and skill, its
  docs (`docs/mise-cloudflare/`);
- **its own Worker**: `src/`, `wrangler.jsonc`, `package.json`.

None of it is this repo's. mise there reads only the repo's own `mise.toml` (`MISE_CEILING_PATHS`), and none of its
files go in this repo's `.github/` or `.claude/`. `mise run example:04-check` checks the first in each, and `mise run root:check` the second.

Nothing is run in them by hand. The numbered steps at the top, in [example.mise.toml](../example.mise.toml), run each
step in a repo here, in order, as a person would:

| Repo | What it is | Run the steps on it |
|---|---|---|
| [reference/](reference/) | An existing repo, committed: a Worker with a D1 database, a KV namespace, an R2 bucket and a secret | `mise run example:05-status` (the default) |
| `new/` | A new repo, which `mise run example:create` makes from only what a person brings (the reference's Worker code, `package.json`, lockfile and `mise.toml`, the Worker named `mise-cloudflare-new`). The steps do the rest. Never committed; each `mise run example:create` starts it again | `EXAMPLE=new mise run example:05-status` |
