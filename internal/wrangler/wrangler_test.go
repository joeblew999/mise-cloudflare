package wrangler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestHasProfiles: the template's wrangler has auth profiles, and its version is its package.json's.
func TestHasProfiles(t *testing.T) {
	dir, _ := filepath.Abs(filepath.Join("..", "..", "templates"))
	b, err := os.ReadFile(filepath.Join(dir, "node_modules", "wrangler", "package.json"))
	if err != nil {
		t.Skip("the template's wrangler isn't installed: mise run template:setup")
	}
	var p struct{ Version string }
	json.Unmarshal(b, &p)
	w := W{Dir: dir, Profile: "mise-cloudflare-template"}
	if !w.HasProfiles(context.Background()) || w.Version() != p.Version {
		t.Errorf("profiles %v, version %q, want %q", w.HasProfiles(context.Background()), w.Version(), p.Version)
	}
	if v := (W{Dir: t.TempDir()}).Version(); v != "not installed" {
		t.Errorf("no wrangler: version %q", v)
	}
}

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

// TestListed: a profile is in wrangler auth list's table when it's the whole first column of a row.
func TestListed(t *testing.T) {
	table := []byte("│ Profile                 │ Bound Directories │\n│ my-repo                 │ /src/my-repo      │\n│ default                 │ -                 │\n")
	if !listed(table, "my-repo") || listed(table, "my") || listed(nil, "my-repo") {
		t.Error("a profile is listed when it's a row's whole first column")
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
