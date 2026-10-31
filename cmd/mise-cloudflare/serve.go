package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
	"golang.org/x/term"
)

// serve runs the Worker locally (wrangler dev), with local D1, KV and R2, and the secrets' local values from .dev.vars,
// always on the repo's own login (--profile), never another login on the machine. Local bindings need no login; a binding
// wrangler runs on Cloudflare even here (its table marks it remote: Workers AI, an AI Search namespace, …) does. In a
// terminal wrangler logs the repo in itself, in the browser; with no terminal to ask in (an agent, a script) it stops, and
// serve logs the repo in as deploy does, then runs wrangler dev again. wrangler dev itself names the required secrets it
// has no value for.
func (r repo) serve(ctx context.Context, args []string) {
	devVars(r.dir)
	args = append([]string{"dev"}, args...)
	err := wrangler.Interactive(r.w.Cmd(ctx, args...))
	if stopped(err) && !inCI() && !askable() && !r.w.LoggedIn(ctx) {
		fmt.Println("· wrangler dev stopped, and this repo has no Cloudflare login, which a binding its table above marks remote needs (wrangler asks for it only in a terminal): logging in, then running it again")
		r.ensureLogin(ctx)
		err = wrangler.Interactive(r.w.Cmd(ctx, args...))
	}
	if stopped(err) {
		if r.w.LoggedIn(ctx) {
			fmt.Println("· if wrangler's table above marks a binding remote, it runs on Cloudflare even here: its resource must be there first (mise run cloudflare:08-deploy makes it), then this again")
		} else {
			fmt.Println("· if wrangler's table above marks a binding remote, it runs on Cloudflare even here and needs the repo's login: mise run cloudflare:12-login, then this again (never a CLOUDFLARE_API_TOKEN here)")
		}
	}
	if err != nil {
		os.Exit(exitCode(err))
	}
}

// stopped reports whether wrangler dev stopped on its own, failing: not when it was stopped (Ctrl-C).
func stopped(err error) bool {
	code := exitCode(err)
	return err != nil && code != -1 && code != 130
}

// askable reports whether wrangler can ask for a login itself: on a terminal, its stdin and stdout both (wrangler's own
// test, outside CI).
func askable() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
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
