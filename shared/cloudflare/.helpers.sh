# Shell helpers for the shared cloudflare tasks. Source with: . "$(dirname "$0")/.helpers.sh"
# They use only what the including repo is told to set (CF_PROFILE, CF_SCOPES), never anything else of its.

# Where the repo's login comes from depends on where it runs:
# - On a developer's machine (the Mac): a wrangler auth profile named $CF_PROFILE, bound to this folder, made in the
#   browser that opens on the machine.
# - In a Claude Code cloud session (CLAUDE_CODE_REMOTE=true): no browser there, and wrangler's profiles can't use the
#   OAuth device flow (wrangler 4.152.0). So the login is wrangler's plain login, made with the device flow (a link to
#   open on any device) and kept in this repo's own git-ignored .wrangler/home, never in the machine's wrangler folder.
# Either way it has exactly the repo's CF_SCOPES.
cloud_session() { [ "${CLAUDE_CODE_REMOTE:-}" = true ]; }
cloud_home() { echo "$PWD/.wrangler/home"; }

# wrangler's own folder on this machine, found the way wrangler finds it: ~/.wrangler if it exists,
# else $XDG_CONFIG_HOME, else ~/Library/Preferences on macOS or ~/.config elsewhere. In a cloud session, the repo's own.
wrangler_home() {
  if cloud_session; then echo "$(cloud_home)/.wrangler"
  elif [ -d "$HOME/.wrangler" ]; then echo "$HOME/.wrangler"
  elif [ -n "${XDG_CONFIG_HOME:-}" ]; then echo "$XDG_CONFIG_HOME/.wrangler"
  elif [ "$(uname)" = Darwin ]; then echo "$HOME/Library/Preferences/.wrangler"
  else echo "$HOME/.config/.wrangler"; fi
}

# sha256 of a file, or "none" if it doesn't exist. The file's contents are never printed.
file_hash() { if [ -f "$1" ]; then shasum -a 256 "$1" | cut -c1-16; else echo none; fi; }

# The wrangler pinned in this repo's package.json, never a global one.
wrangler() {
  [ -f node_modules/wrangler/bin/wrangler.js ] || { echo "wrangler isn't installed here: run cloudflare:setup" >&2; return 1; }
  if cloud_session; then XDG_CONFIG_HOME=$(cloud_home) node node_modules/wrangler/bin/wrangler.js "$@"
  else node node_modules/wrangler/bin/wrangler.js "$@"; fi
}

# This repo's wrangler config as JSON, read by wrangler itself so jsonc and toml both work: the Worker's name, its
# required secrets, and the names of its D1, KV and R2 (deploy names them <worker>-<binding> unless the config does).
worker_config() {
  node --input-type=module -e "const { unstable_readConfig } = await import('wrangler');
    const c = unstable_readConfig({});
    const named = (b) => c.name + '-' + b.toLowerCase();
    console.log(JSON.stringify({ name: c.name, secrets: c.secrets?.required ?? [],
      d1: c.d1_databases.map((d) => d.database_name ?? named(d.binding)),
      kv: c.kv_namespaces.map((k) => named(k.binding)),
      r2: c.r2_buckets.map((r) => r.bucket_name ?? named(r.binding)) }));" | tail -1
}

# This repo's D1, KV and R2 that exist on Cloudflare, one per line: "d1 <name> <id>", "kv <name> <id>", "r2 <name>".
resources() {
  local cfg d1 kv r2 n
  cfg=$(worker_config)
  d1=$(wrangler d1 list --json 2>/dev/null </dev/null)
  kv=$(wrangler kv namespace list 2>/dev/null </dev/null)
  r2=$(wrangler r2 bucket list 2>/dev/null </dev/null | awk '/^name:/ {print $2}')
  for n in $(jq -r '.d1[]' <<<"$cfg"); do jq -r --arg n "$n" '.[] | select(.name == $n) | "d1 \(.name) \(.uuid)"' <<<"$d1"; done
  for n in $(jq -r '.kv[]' <<<"$cfg"); do jq -r --arg n "$n" '.[] | select(.title == $n) | "kv \(.title) \(.id)"' <<<"$kv"; done
  for n in $(jq -r '.r2[]' <<<"$cfg"); do if grep -qx "$n" <<<"$r2"; then echo "r2 $n"; fi; done
}

# The file holding this repo's login.
profile_file() {
  if cloud_session; then echo "$(wrangler_home)/config/default.toml"; else echo "$(wrangler_home)/config/$CF_PROFILE.toml"; fi
}

# Make this repo's login with its CF_SCOPES. On a machine, the browser opens. In a cloud session, wrangler prints a link
# and a code: open it on any device and click Allow.
repo_login() {
  if cloud_session; then
    echo "Logging in $CF_PROFILE from a cloud session: open the link below on any device, click Allow and pick the account."
    # shellcheck disable=SC2086 # CF_SCOPES is a list
    wrangler login --device --browser=false ${CF_SCOPES:+--scopes $CF_SCOPES}
  else
    echo "Logging in $CF_PROFILE: your browser opens. Click Allow and pick the account."
    # shellcheck disable=SC2086
    wrangler auth create "$CF_PROFILE" ${CF_SCOPES:+--scopes $CF_SCOPES}
  fi
}

# A CLOUDFLARE_API_TOKEN beats every profile, so outside CI one in the shell or in a .env is refused.
refuse_stray_tokens() {
  [ -n "${CI:-}" ] && return 0
  if [ -n "${CLOUDFLARE_API_TOKEN:-}" ]; then
    echo "refusing: CLOUDFLARE_API_TOKEN is set in this shell, and it would override this repo's own login ($CF_PROFILE). Unset it and run again." >&2
    exit 1
  fi
  local envs; envs=$(grep -ls '^CLOUDFLARE_API_TOKEN=.' .env .env.* 2>/dev/null | tr '\n' ' ' || true)
  if [ -n "$envs" ]; then
    echo "refusing: ${envs% } sets CLOUDFLARE_API_TOKEN, and wrangler would use it instead of this repo's own login ($CF_PROFILE). Remove it and run again." >&2
    exit 1
  fi
}

# Log this repo in if it isn't yet (once), bind a machine's profile to this folder, and log in again if the repo's
# CF_SCOPES now lists a scope the login doesn't have.
ensure_login() {
  refuse_stray_tokens
  [ -f "$(profile_file)" ] || repo_login
  cloud_session || wrangler auth activate "$CF_PROFILE" "$PWD" > /dev/null
  [ -n "${CF_SCOPES:-}" ] || return 0
  local granted s missing=""
  granted=$(wrangler whoami --json </dev/null 2>/dev/null | jq -r '.tokenPermissions[]?' || true)
  for s in $CF_SCOPES; do grep -qx "$s" <<<"$granted" || missing+=" $s"; done
  if [ -n "$missing" ]; then
    echo "This repo now also needs:$missing."
    repo_login
  fi
}

# The URL of the last deploy from this folder, saved by cloudflare:deploy.
deployed_url() { cat .wrangler/cloudflare-url 2>/dev/null || true; }

# Empty an R2 bucket so it can be deleted. wrangler can't list objects; Cloudflare's API can, with this login's own token.
empty_r2_bucket() {
  local acct token
  acct=$(wrangler whoami --json </dev/null | jq -r 'if (.accounts | length) == 1 then .accounts[0].id else empty end')
  [ -n "$acct" ] || { echo "can't tell which account holds R2 $1: empty it in the dashboard, then run again" >&2; return 1; }
  token=$(wrangler auth token --json </dev/null | jq -r .token)
  curl -fsS -m 30 -H "Authorization: Bearer $token" "https://api.cloudflare.com/client/v4/accounts/$acct/r2/buckets/$1/objects" \
    | jq -r '.result[].key' | while read -r key; do wrangler r2 object delete "$1/$key" --remote </dev/null > /dev/null; done
}
