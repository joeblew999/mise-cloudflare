package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
)

// serve runs the Worker locally (wrangler dev), with local D1, KV and R2, and the secrets' local values from .dev.vars:
// no login. wrangler dev itself names the required secrets it has no value for.
func (r repo) serve(ctx context.Context, args []string) {
	devVars(r.dir)
	r.wranglerHere(ctx, false, append([]string{"dev"}, args...))
}

// devVars makes .dev.vars from .dev.vars.example for wrangler dev when there's none, git-ignored first.
func devVars(dir string) {
	path := filepath.Join(dir, ".dev.vars")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return
	}
	b, err := os.ReadFile(filepath.Join(dir, ".dev.vars.example"))
	if err != nil {
		return
	}
	must(rules.EnsureIgnored(dir, ".dev.vars"))
	must(os.WriteFile(path, b, 0o600))
	fmt.Println("made .dev.vars from .dev.vars.example (git-ignored): put the local values there")
}
