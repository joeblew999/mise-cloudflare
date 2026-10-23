# Shell helpers for the shared cloudflare tasks. Source with: . "$(dirname "$0")/.helpers.sh"
# They use only what the including repo is told to set (CF_PROFILE), never anything else of its.

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
