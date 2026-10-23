# Shell helpers for the shared cloudflare tasks. Source with: . "$(dirname "$0")/.helpers.sh"
# They use only what the including repo is told to set (CF_PROFILE, CF_SCOPES), never anything else of its.

# wrangler's own folder on this machine, found the way wrangler finds it: ~/.wrangler if it exists,
# else $XDG_CONFIG_HOME, else ~/Library/Preferences on macOS or ~/.config elsewhere.
wrangler_home() {
  if [ -d "$HOME/.wrangler" ]; then echo "$HOME/.wrangler"
  elif [ -n "${XDG_CONFIG_HOME:-}" ]; then echo "$XDG_CONFIG_HOME/.wrangler"
  elif [ "$(uname)" = Darwin ]; then echo "$HOME/Library/Preferences/.wrangler"
  else echo "$HOME/.config/.wrangler"; fi
}

# sha256 of a file, or "none" if it doesn't exist. The file's contents are never printed.
file_hash() { if [ -f "$1" ]; then shasum -a 256 "$1" | cut -c1-16; else echo none; fi; }

# The wrangler pinned in this repo's package.json, never a global one.
wrangler() {
  [ -f node_modules/wrangler/bin/wrangler.js ] || { echo "wrangler isn't installed here: run cloudflare:setup" >&2; return 1; }
  node node_modules/wrangler/bin/wrangler.js "$@"
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

profile_file() { echo "$(wrangler_home)/config/$CF_PROFILE.toml"; }

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

# Log this repo in as its own profile if it isn't yet (the browser opens once), bind the profile to this folder, and
# log in again if the repo's CF_SCOPES now lists a scope the login doesn't have (the browser opens once more).
ensure_login() {
  refuse_stray_tokens
  if [ ! -f "$(profile_file)" ]; then
    echo "Logging in $CF_PROFILE: your browser opens once. Click Allow and pick the account."
    # shellcheck disable=SC2086 # CF_SCOPES is a list
    wrangler auth create "$CF_PROFILE" ${CF_SCOPES:+--scopes $CF_SCOPES}
  fi
  wrangler auth activate "$CF_PROFILE" "$PWD" > /dev/null
  [ -n "${CF_SCOPES:-}" ] || return 0
  local granted s missing=""
  granted=$(wrangler whoami --json </dev/null 2>/dev/null | jq -r '.tokenPermissions[]?' || true)
  for s in $CF_SCOPES; do grep -qx "$s" <<<"$granted" || missing+=" $s"; done
  if [ -n "$missing" ]; then
    echo "This repo now also needs:$missing. Your browser opens once more: click Allow."
    # shellcheck disable=SC2086
    wrangler auth create "$CF_PROFILE" --scopes $CF_SCOPES
  fi
}

# The URL of the last deploy from this folder, saved by cloudflare:deploy.
deployed_url() { cat .wrangler/cloudflare-url 2>/dev/null || true; }
