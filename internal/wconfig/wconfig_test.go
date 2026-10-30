package wconfig

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestRead reads example/'s config with its own wrangler, which example's setup task installs.
func TestRead(t *testing.T) {
	dir, _ := filepath.Abs("../../example")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "wrangler")); err != nil {
		t.Skip("example/ has no wrangler installed: run mise run 02-setup")
	}
	c, err := Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "mise-cloudflare-example" || !slices.Equal(c.Secrets, []string{"GREETING"}) || filepath.Base(c.Path) != "wrangler.jsonc" {
		t.Errorf("got %+v", c)
	}
	if want := map[string]string{"DB": "d1", "KV": "kv_namespace", "BUCKET": "r2_bucket"}; !maps.Equal(c.Bindings, want) {
		t.Errorf("bindings %v, want %v", c.Bindings, want)
	}
	if _, ok := c.Raw["d1_databases"]; !ok {
		t.Errorf("raw config has no d1_databases: %v", c.Raw)
	}
}
