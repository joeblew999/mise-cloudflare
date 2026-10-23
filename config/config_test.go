package config

import (
	"slices"
	"testing"
)

// TestScopes: the login holds the base, what each kind of binding needs, and cloudflare.toml's extras, each once.
func TestScopes(t *testing.T) {
	got := Scopes([]string{"d1_databases", "kv_namespaces", "r2_buckets", "routes"}, []string{"pipelines:write", "d1:write"})
	want := []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "d1:write", "workers_kv:write",
		"workers_routes:write", "zone:read", "pipelines:write"}
	if !slices.Equal(got, want) {
		t.Errorf("Scopes = %q, want %q", got, want)
	}
	if got := Scopes(nil, nil); !slices.Equal(got, BaseScopes) {
		t.Errorf("no bindings: %q, want the base %q", got, BaseScopes)
	}
}
