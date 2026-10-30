# Examples

Each folder here is a copy stamped out from the template, [templates/](../templates/): another repo, a user's repo that
uses mise-cloudflare, kept inside this one so the end-to-end runs on it as well as on the template. It gets its own git, and
everything else from its own numbered tasks, as any repo does. None of it is committed.

| Repo | What it is | Walk its steps |
|---|---|---|
| `new/` | `mise run example:create` makes it from only what a person brings, copied from the template (the Worker's code, `package.json`, lockfile and `mise.toml`, the Worker named `mise-cloudflare-new`); the steps do the rest. Each `mise run example:create` starts it again | `mise run example:e2e-serve` (`example:e2e-deploy` on Cloudflare) |
