// Package wrangler runs a repo's own pinned wrangler (node_modules/wrangler) on the repo's own login: a wrangler auth
// profile, passed to every command with --profile, so nothing depends on which login wrangler would pick for the folder.
package wrangler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
)

// W is wrangler in a repo folder, on a profile.
type W struct {
	Dir     string // the repo folder: its node_modules holds the pinned wrangler
	Profile string
}

// Installed reports whether the repo's pinned wrangler is installed (the repo's setup task).
func (w W) Installed() bool {
	_, err := os.Stat(w.bin())
	return err == nil
}

func (w W) bin() string {
	return filepath.Join(w.Dir, "node_modules", "wrangler", "bin", "wrangler.js")
}

// Plain is a wrangler command with no profile: local dev, and the auth commands, which name the profile themselves.
func (w W) Plain(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "node", append([]string{w.bin()}, args...)...)
	cmd.Dir = w.Dir
	cmd.Env = os.Environ()
	return cmd
}

// Cmd is a wrangler command on the repo's profile.
func (w W) Cmd(ctx context.Context, args ...string) *exec.Cmd {
	return w.Plain(ctx, append(args, "--profile", w.Profile)...)
}

// Interactive runs cmd on this terminal.
func Interactive(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// RunDeviceLogin runs a device login (wrangler's or cf's) on this terminal, adding to what it prints the link with the
// code already in it. linkOnStderr is where the tool prints the link: wrangler on stdout, cf on stderr (cf prints all
// but a command's result there).
func RunDeviceLogin(cmd *exec.Cmd, linkOnStderr bool) error {
	cmd.Stdin = os.Stdin
	var out io.ReadCloser
	var err error
	to := os.Stdout
	if linkOnStderr {
		cmd.Stdout, to = os.Stdout, os.Stderr
		out, err = cmd.StderrPipe()
	} else {
		cmd.Stderr = os.Stderr
		out, err = cmd.StdoutPipe()
	}
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	DeviceLink(out, to)
	return cmd.Wait()
}

// Output runs cmd with no input and returns its stdout; on failure the error carries its stderr.
func Output(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stderr = nil, &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("wrangler %s: %w\n%s", cmd.Args[2], err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

// LoggedIn reports whether the profile has a login on this machine, from wrangler's own list of them (wrangler auth
// list).
func (w W) LoggedIn(ctx context.Context) bool {
	out, err := w.Plain(ctx, "auth", "list").CombinedOutput()
	return err == nil && listed(out, w.Profile)
}

// listed reports whether wrangler auth list's table has profile in its first column.
func listed(table []byte, profile string) bool {
	return regexp.MustCompile(`(?m)^│ ` + regexp.QuoteMeta(profile) + ` +│`).Match(table)
}

// Account is a Cloudflare account the login can use.
type Account struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Who is wrangler whoami: the login's email, accounts and OAuth scopes.
type Who struct {
	LoggedIn bool      `json:"loggedIn"`
	Email    string    `json:"email"`
	Accounts []Account `json:"accounts"`
	Scopes   []string  `json:"tokenPermissions"`
}

// Whoami is the profile's login. wrangler whoami takes no --profile (it reports the folder's active login), so this
// runs it on the folder's binding (Activate) and refuses unless wrangler names this profile as the active one.
func (w W) Whoami(ctx context.Context) (Who, error) {
	var who Who
	text, err := Output(w.Plain(ctx, "whoami"))
	if err != nil {
		return who, err
	}
	if !activeProfile(text, w.Profile) {
		return who, fmt.Errorf("wrangler isn't using the login %s in %s, so it would run on another login", w.Profile, w.Dir)
	}
	out, err := Output(w.Plain(ctx, "whoami", "--json"))
	if err != nil {
		return who, err
	}
	return who, json.Unmarshal(out, &who)
}

// activeProfile reports whether wrangler whoami's text names profile as the folder's active login.
func activeProfile(text []byte, profile string) bool {
	for _, line := range bytes.Split(text, []byte("\n")) {
		if string(bytes.TrimSpace(line)) == "Active profile: "+profile {
			return true
		}
	}
	return false
}

// Token is the profile's current OAuth token (wrangler refreshes it when it has expired), for Cloudflare's API.
func (w W) Token(ctx context.Context) (string, error) {
	var t struct{ Type, Token string }
	out, err := Output(w.Cmd(ctx, "auth", "token", "--json"))
	if err == nil {
		err = json.Unmarshal(out, &t)
	}
	if err == nil && t.Token == "" {
		err = errors.New("wrangler auth token: no token")
	}
	return t.Token, err
}

// Scopes are the OAuth scopes wrangler's own login asks for (wrangler login --scopes-list): wrangler keeps them, for
// everything it does, so a repo's login with them can do what wrangler can.
func (w W) Scopes(ctx context.Context) ([]string, error) {
	cmd := w.Plain(ctx, "login", "--scopes-list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wrangler login --scopes-list: %w: %s", err, out)
	}
	var scopes []string
	for _, m := range scopeRow.FindAllSubmatch(out, -1) {
		scopes = append(scopes, string(m[1]))
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("wrangler login --scopes-list listed no scopes:\n%s", out)
	}
	return scopes, nil
}

// scopeRow is one scope in wrangler's table of them: "│ workers_kv:write │ See and change …".
var scopeRow = regexp.MustCompile(`(?m)^│ ([a-z0-9][a-z0-9_:.-]*) +│`)

// Device reports whether to log in with the OAuth device flow (a link to open on any device, a phone included) instead
// of a browser on this machine: in a Claude Code cloud session, which has no browser, or when CLOUDFLARE_LOGIN=device.
func Device() bool {
	return os.Getenv("CLAUDE_CODE_REMOTE") == "true" || os.Getenv("CLOUDFLARE_LOGIN") == "device"
}

// Login makes the profile's login with exactly scopes (in a browser, or with a link for any device), then binds
// the profile to the repo folder.
func (w W) Login(ctx context.Context, scopes []string, device bool) error {
	if device {
		if err := w.deviceLogin(ctx, scopes); err != nil {
			return err
		}
		return w.Activate(ctx)
	}
	fmt.Printf("Logging in %s: your browser opens. Click Allow and pick the account.\n", w.Profile)
	if err := Interactive(w.Plain(ctx, append([]string{"auth", "create", w.Profile, "--scopes"}, scopes...)...)); err != nil {
		return fmt.Errorf("wrangler auth create %s: %w", w.Profile, err)
	}
	return w.Activate(ctx)
}

// deviceLogin runs wrangler login --device in a temporary wrangler folder, then keeps the login file it leaves as the
// profile's: wrangler auth create has no device flow (as of wrangler 4.152.0).
func (w W) deviceLogin(ctx context.Context, scopes []string) error {
	home, _ := os.UserHomeDir()
	if st, err := os.Stat(filepath.Join(home, ".wrangler")); err == nil && st.IsDir() {
		return fmt.Errorf("%s exists, so wrangler would ignore a temporary folder and overwrite the default login in it: move it away", filepath.Join(home, ".wrangler"))
	}
	tmp, err := os.MkdirTemp("", "cloudflare-login-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	fmt.Printf("Logging in %s: open the link below on any device (your phone works), click Allow and pick the account.\n", w.Profile)
	cmd := w.Plain(ctx, append([]string{"login", "--device", "--browser=false", "--scopes"}, scopes...)...)
	cmd.Env = append(cmd.Env, "XDG_CONFIG_HOME="+tmp)
	if err := RunDeviceLogin(cmd, false); err != nil {
		return fmt.Errorf("wrangler login --device: %w", err)
	}
	b, err := os.ReadFile(filepath.Join(tmp, ".wrangler", "config", "default.toml"))
	if err != nil {
		return fmt.Errorf("the device login left no login file: %w", err)
	}
	// wrangler's own folder: ~/.wrangler if it exists (refused above), else XDG_CONFIG_HOME's, else the OS's.
	folder := filepath.Join(home, ".config", ".wrangler")
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		folder = filepath.Join(x, ".wrangler")
	} else if runtime.GOOS == "darwin" {
		folder = filepath.Join(home, "Library", "Preferences", ".wrangler")
	}
	file := filepath.Join(folder, "config", w.Profile+".toml")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	return os.WriteFile(file, b, 0o600)
}

// Activate binds the profile to the repo folder by its real path, the path wrangler looks a folder up by: bound by a
// path through a symlink (macOS's /var is /private/var), it would never match.
func (w W) Activate(ctx context.Context) error {
	dir, err := filepath.EvalSymlinks(w.Dir)
	if err == nil {
		dir, err = filepath.Abs(dir)
	}
	if err != nil {
		return err
	}
	_, err = Output(w.Plain(ctx, "auth", "activate", w.Profile, dir))
	return err
}

// Delete logs the profile out on Cloudflare and deletes it, with its folder bindings.
func (w W) Delete(ctx context.Context) error {
	_, err := Output(w.Plain(ctx, "auth", "delete", w.Profile))
	return err
}

// Missing and Extra compare the scopes a login holds with the scopes the repo wants, ignoring offline_access, which
// wrangler adds to every login.
func Missing(want, have []string) []string { return diff(want, have) }
func Extra(want, have []string) []string   { return diff(have, want) }

func diff(a, b []string) []string {
	var out []string
	for _, s := range a {
		if s != "offline_access" && !slices.Contains(b, s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
