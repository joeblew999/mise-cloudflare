// Package cf runs Cloudflare's own CLI, cf, on the repo's cf login, for Workers Builds: wrangler's login can't hold
// its scope (workers_builds:write).
package cf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// Scopes is what connect-github needs: the account and Workers Builds.
var Scopes = []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "workers_builds:write"}

// CF is cf on one profile, in one folder, on one account (once known).
type CF struct {
	Dir     string
	Profile string
	Account string
}

// Installed reports whether cf is on the PATH (the cloudflare:09-connect-github task brings it).
func Installed() bool {
	_, err := exec.LookPath("cf")
	return err == nil
}

func (c CF) cmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cf", append(args, "--profile", c.Profile)...)
	cmd.Dir = c.Dir
	cmd.Env = os.Environ()
	if c.Account != "" {
		cmd.Env = append(cmd.Env, "CLOUDFLARE_ACCOUNT_ID="+c.Account)
	}
	return cmd
}

// Error is a cf command that failed, with what it printed.
type Error struct {
	Args   []string
	Output string
}

func (e *Error) Error() string {
	out := e.Output
	// A call cf can't take prints its whole help first, then the error in a box: only the box says what went wrong.
	if i := strings.Index(out, "┌ Error"); i >= 0 {
		out = out[i:]
		if j := strings.Index(out, "└"); j >= 0 {
			out = out[:j+len("└")]
		}
	}
	return fmt.Sprintf("cf %s: %s", strings.Join(e.Args, " "), out)
}

// NotFound reports whether a cf command failed because the thing doesn't exist.
func NotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && (strings.Contains(e.Output, "404") || strings.Contains(strings.ToLower(e.Output), "not found"))
}

// JSON runs a cf command and decodes what it prints into out; an API envelope's result is unwrapped.
func (c CF) JSON(ctx context.Context, out any, args ...string) error {
	cmd := c.cmd(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return &Error{Args: args, Output: strings.TrimSpace(stderr.String() + "\n" + string(b))}
	}
	if out == nil {
		return nil
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(b, &env) == nil && len(env.Result) > 0 {
		b = env.Result
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("cf %s printed %.300s: %w", strings.Join(args, " "), b, err)
	}
	return nil
}

// Who is cf auth whoami.
type Who struct {
	Authenticated bool     `json:"authenticated"`
	Scopes        []string `json:"scopes"`
	Accounts      []struct {
		ID string `json:"id"`
	} `json:"accounts"`
}

// Whoami is the profile's login; not logged in is Authenticated false, not an error.
func (c CF) Whoami(ctx context.Context) (Who, error) {
	var who Who
	cmd := c.cmd(ctx, "auth", "whoami")
	out, _ := cmd.Output() // not logged in, it still prints JSON, whatever it exits with (1 before cf 1.0.0-beta.26, 0 now)
	if err := json.Unmarshal(out, &who); err != nil {
		return who, fmt.Errorf("cf auth whoami printed %.300s: %w", out, err)
	}
	return who, nil
}

// HasScopes reports whether the login holds every scope connect-github needs.
func (w Who) HasScopes() bool {
	for _, s := range Scopes {
		if !slices.Contains(w.Scopes, s) {
			return false
		}
	}
	return true
}

// Login makes or remakes the profile's login with Scopes: a link and a code to approve on any device (cf's device
// flow), opened in this machine's browser too unless there's none.
func (c CF) Login(ctx context.Context, browser bool) error {
	args := append([]string{"auth", "create", c.Profile, "--device", "--scopes"}, Scopes...)
	if !browser {
		args = append(args, "--no-browser")
	}
	cmd := exec.CommandContext(ctx, "cf", args...)
	cmd.Dir, cmd.Env = c.Dir, os.Environ()
	fmt.Printf("Logging %s in to Cloudflare's cf, for Workers Builds: approve it on any device (your phone works).\n", c.Profile)
	if err := wrangler.RunDeviceLogin(cmd, true); err != nil {
		return fmt.Errorf("cf auth create %s: %w", c.Profile, err)
	}
	return nil
}

// Delete logs the profile out on Cloudflare and deletes it.
func (c CF) Delete(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "cf", "auth", "delete", c.Profile)
	cmd.Dir, cmd.Env = c.Dir, os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return &Error{Args: []string{"auth", "delete", c.Profile}, Output: strings.TrimSpace(string(out))}
	}
	return nil
}
