package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/mise"
)

const hookFile = "mise-cloudflare-session-start.sh"

// gitTop is the repository dir is in, and dir's path in it ("." at its root), both by their real paths.
func gitTop(ctx context.Context, dir string) (top, rel string, err error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", errors.New("not in a git repository")
	}
	top, _ = filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	real, _ := filepath.EvalSymlinks(dir)
	rel, err = filepath.Rel(top, real)
	return top, filepath.ToSlash(rel), err
}

// claude sets the repository dir is in up for Claude Code cloud sessions: the hook script in .claude/hooks, and a
// SessionStart entry in .claude/settings.json that runs it for dir's folder. Settings already there are kept: the
// entry is added to them, or, if one already runs the hook, left as it is.
func claude(ctx context.Context, dir string) {
	top, rel, err := gitTop(ctx, dir)
	if err != nil {
		fail(err)
	}
	script := filepath.Join(top, ".claude", "hooks", hookFile)
	writeFile(script, mise.SessionStart)
	must(os.Chmod(script, 0o755))

	path := filepath.Join(top, ".claude", "settings.json")
	settings := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &settings); err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
	}
	if b, _ := json.Marshal(settings); strings.Contains(string(b), hookFile) {
		fmt.Printf("✓ %s runs the hook already\n", path)
		return
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	entry := map[string]any{
		"matcher": "startup",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/` + hookFile + " " + rel,
			"timeout": 600,
		}},
	}
	start, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(slices.Clone(start), entry)
	settings["hooks"] = hooks
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fail(err)
	}
	writeFile(path, append(b, '\n'))
}
