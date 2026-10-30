#!/usr/bin/env bash
# This repo's own Claude Code cloud session hook, for working on mise-cloudflare: mise, then every tool this repo's
# mise.toml pins, so mise run … works at once. Other repos get theirs from templates/session-start.sh, through init.
# Does nothing on your own machine.
set -euo pipefail
[ "${CLAUDE_CODE_REMOTE:-}" = true ] || exit 0
export PATH="$HOME/.local/bin:$PATH"
# A cloud session's GitHub token can see only the session's own repos, and mise sends it with downloads from others.
unset GITHUB_TOKEN GH_TOKEN
command -v mise > /dev/null || curl -fsSL https://mise.run | sh
cd "$CLAUDE_PROJECT_DIR"
mise trust -q mise.toml
mise install
# Keep mise on PATH for the rest of the session's commands.
[ -n "${CLAUDE_ENV_FILE:-}" ] && echo "export PATH=\"$HOME/.local/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
echo "mise $(mise --version | cut -d' ' -f1) and this repo's tools installed"
