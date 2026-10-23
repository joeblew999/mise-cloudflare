#!/usr/bin/env bash
# A Claude Code cloud session starts on a bare Linux machine with only this repo cloned.
# Install mise, then every tool the repo pins, so `mise run …` works straight away. Does nothing on your own machine.
set -euo pipefail
[ "${CLAUDE_CODE_REMOTE:-}" = true ] || exit 0
export PATH="$HOME/.local/bin:$PATH"
command -v mise > /dev/null || curl -fsSL https://mise.run | sh
cd "$CLAUDE_PROJECT_DIR"
mise trust -q mise.toml
mise trust -q example/mise.toml
mise install
(cd example && mise install && mise run cloudflare:setup)
# Keep mise on PATH for the rest of the session's commands.
[ -n "${CLAUDE_ENV_FILE:-}" ] && echo "export PATH=\"$HOME/.local/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
echo "mise $(mise --version | cut -d' ' -f1) and every pinned tool installed"
