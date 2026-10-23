package wconfig

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestRead reads example/'s config with its own wrangler, which go tool cloudflare setup installs there.
func TestRead(t *testing.T) {
	dir, _ := filepath.Abs("../../example")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "wrangler")); err != nil {
		t.Skip("example/ has no wrangler installed: run mise run cloudflare:setup there")
	}
	c, err := Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "cloudflare-zero-example" || !slices.Equal(c.Secrets, []string{"GREETING"}) || filepath.Base(c.Path) != "wrangler.jsonc" {
		t.Errorf("got %+v", c)
	}
	if !slices.Equal(c.Kinds, []string{"d1_databases", "kv_namespaces", "r2_buckets"}) {
		t.Errorf("kinds %q, want D1, KV and R2", c.Kinds)
	}
	if _, ok := c.Raw["d1_databases"]; !ok {
		t.Errorf("raw config has no d1_databases: %v", c.Raw)
	}
}
