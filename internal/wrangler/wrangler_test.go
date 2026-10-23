package wrangler

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScopes(t *testing.T) {
	want := []string{"account:read", "workers:write", "d1:write"}
	have := []string{"account:read", "offline_access", "workers:write", "zone:read"}
	if got := Missing(want, have); !slices.Equal(got, []string{"d1:write"}) {
		t.Errorf("missing %q", got)
	}
	if got := Extra(want, have); !slices.Equal(got, []string{"zone:read"}) {
		t.Errorf("extra %q (offline_access is wrangler's own)", got)
	}
}

func TestHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	if got := Home(); got != filepath.Join(home, "xdg", ".wrangler") {
		t.Errorf("with XDG_CONFIG_HOME: %s", got)
	}
	os.Mkdir(filepath.Join(home, ".wrangler"), 0o700)
	if got := Home(); got != filepath.Join(home, ".wrangler") {
		t.Errorf("~/.wrangler wins: %s", got)
	}
	if got := ProfileFile("p"); got != filepath.Join(home, ".wrangler", "config", "p.toml") {
		t.Errorf("profile file %s", got)
	}
}

func TestDevice(t *testing.T) {
	t.Setenv("CLAUDE_CODE_REMOTE", "")
	t.Setenv("CLOUDFLARE_LOGIN", "")
	if Device() {
		t.Error("a machine logs in in its browser")
	}
	t.Setenv("CLAUDE_CODE_REMOTE", "true")
	if !Device() {
		t.Error("a Claude Code cloud session has no browser")
	}
	t.Setenv("CLAUDE_CODE_REMOTE", "")
	t.Setenv("CLOUDFLARE_LOGIN", "device")
	if !Device() {
		t.Error("CLOUDFLARE_LOGIN=device asks for the device flow")
	}
}

func TestActiveProfile(t *testing.T) {
	text := []byte("\n ⛅️ wrangler 4.151.3\nActive profile: my-repo\n👋 You are logged in with an OAuth Token\n")
	if !activeProfile(text, "my-repo") || activeProfile(text, "my") || activeProfile([]byte("👋 You are logged in"), "my-repo") {
		t.Error("the active profile is the whole name on its own line, and there is none without that line")
	}
}
