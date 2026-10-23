package wrangler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestWranglerScopes: the scopes come from example/'s own wrangler, its login's list, which has KV's and D1's among them.
func TestWranglerScopes(t *testing.T) {
	dir, _ := filepath.Abs("../../example")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "wrangler")); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("example/ has no wrangler installed, and CI must read its scopes: run mise run setup there first")
		}
		t.Skip("example/ has no wrangler installed: run mise run setup there")
	}
	scopes, err := W{Dir: dir}.Scopes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "workers_kv:write", "d1:write"} {
		if !slices.Contains(scopes, s) {
			t.Errorf("wrangler's scopes %q lack %s", scopes, s)
		}
	}
}
