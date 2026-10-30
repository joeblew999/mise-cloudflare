package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// serve runs the Worker locally (wrangler dev), with local D1, KV and R2, and the secrets' local values from .dev.vars:
// no login.
func (r repo) serve(ctx context.Context, args []string) {
	devVars(r.dir, r.wc.Secrets)
	r.wranglerHere(ctx, false, append([]string{"dev"}, args...))
}

// devVars makes .dev.vars from .dev.vars.example for wrangler dev when there's none, and names the secrets it has no
// value for, which the Worker would run without.
func devVars(dir string, secrets []string) {
	path := filepath.Join(dir, ".dev.vars")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if b, err := os.ReadFile(filepath.Join(dir, ".dev.vars.example")); err == nil {
			must(os.WriteFile(path, b, 0o600))
			fmt.Println("made .dev.vars from .dev.vars.example (git-ignored): put the local values there")
		}
	}
	b, _ := os.ReadFile(path)
	var empty []string
	for _, s := range secrets {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(s) + `=\S`).Match(b) {
			empty = append(empty, s)
		}
	}
	if len(empty) > 0 {
		fmt.Printf("· no local value for %s: set it in .dev.vars, or the Worker runs without it\n", strings.Join(empty, ", "))
	}
}
