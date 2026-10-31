package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// runWrangler runs any wrangler command on the repo's login.
func (r repo) runWrangler(ctx context.Context, args []string) {
	r.wranglerHere(ctx, args)
}

// wranglerHere runs wrangler on this terminal, on the repo's login (made first if needed); its exit code is the
// command's.
func (r repo) wranglerHere(ctx context.Context, args []string) {
	r.ensureLogin(ctx)
	if err := wrangler.Interactive(r.w.Cmd(ctx, args...)); err != nil {
		os.Exit(exitCode(err))
	}
}

// build runs the package's own build script, when it has one (rules.Build), before wrangler runs, deploys or previews
// the Worker: what wrangler reads then is what the build wrote (Cloudflare's Vite plugin's config, say). Its exit code
// is the build's when it fails.
func (r repo) build(ctx context.Context) {
	run := rules.Build(r.dir)
	if run == "" {
		return
	}
	fmt.Printf("$ %s   (package.json's build script: wrangler runs what it writes)\n", run)
	f := strings.Fields(run)
	cmd := exec.CommandContext(ctx, f[0], f[1:]...)
	cmd.Dir = r.dir
	if err := wrangler.Interactive(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "mise-cloudflare: %s failed: wrangler isn't run on a build that failed\n", run)
		os.Exit(exitCode(err))
	}
}

// wranglerJSON runs a wrangler command and decodes its JSON, past anything it prints first (a warning, say).
func (r repo) wranglerJSON(ctx context.Context, v any, args ...string) error {
	out, err := wrangler.Output(r.w.Cmd(ctx, args...))
	if err != nil {
		return err
	}
	if i := regexp.MustCompile(`(?m)^[\[{]`).FindIndex(out); i != nil {
		out = out[i[0]:]
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("wrangler %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
