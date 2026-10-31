package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// serve runs the Worker locally (wrangler dev), with local D1, KV and R2, and the secrets' local values from .dev.vars.
// It needs no login, but a binding wrangler runs on Cloudflare even here (its table marks it remote: an AI Search
// namespace, …) does: it runs on the repo's own login when the repo has one. wrangler dev itself names the required
// secrets it has no value for.
func (r repo) serve(ctx context.Context, args []string) {
	devVars(r.dir)
	args = append([]string{"dev"}, args...)
	loggedIn := r.w.LoggedIn(ctx)
	cmd := r.w.Plain(ctx, args...)
	if loggedIn {
		cmd = r.w.Cmd(ctx, args...)
	}
	if err := wrangler.Interactive(cmd); err != nil {
		// A binding wrangler runs on Cloudflare even here needs the repo's login (wrangler's own words, with none, are to
		// set a token, which this repo refuses) and its resource there. Not when it was stopped (Ctrl-C).
		if code := exitCode(err); code != -1 && code != 130 {
			if loggedIn {
				fmt.Println("· if wrangler's table above marks a binding remote, it runs on Cloudflare even here: its resource must be there first (mise run cloudflare:08-deploy makes it), then this again")
			} else {
				fmt.Println("· if wrangler's table above marks a binding remote, it runs on Cloudflare even here and needs the repo's login: mise run cloudflare:12-login, then this again (never a CLOUDFLARE_API_TOKEN here)")
			}
		}
		os.Exit(exitCode(err))
	}
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
