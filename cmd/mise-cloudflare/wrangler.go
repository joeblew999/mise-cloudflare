package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// wrangler runs any wrangler command on the repo's login.
func (r repo) wrangler(ctx context.Context, args []string) {
	r.wranglerHere(ctx, true, args)
}

// wranglerHere runs wrangler on this terminal, on the repo's login when onLogin (made first if needed); its exit code
// is the command's.
func (r repo) wranglerHere(ctx context.Context, onLogin bool, args []string) {
	cmd := r.w.Plain(ctx, args...)
	if onLogin {
		r.ensureLogin(ctx)
		cmd = r.w.Cmd(ctx, args...)
	}
	if err := wrangler.Interactive(cmd); err != nil {
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
