package config

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestScopes: the login holds the base, what each kind of binding needs, and cloudflare.toml's extras, each once.
func TestScopes(t *testing.T) {
	got := Scopes([]string{"d1", "kv_namespace", "r2_bucket", "routes"}, []string{"pipelines:write", "d1:write"})
	want := []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "d1:write", "workers_kv:write",
		"workers_routes:write", "zone:read", "pipelines:write"}
	if !slices.Equal(got, want) {
		t.Errorf("Scopes = %q, want %q", got, want)
	}
	if got := Scopes(nil, nil); !slices.Equal(got, BaseScopes) {
		t.Errorf("no bindings: %q, want the base %q", got, BaseScopes)
	}
	if got := Unknown([]string{"kv_namespace", "send_email", "a_kind_wrangler_adds"}); !slices.Equal(got, []string{"send_email", "a_kind_wrangler_adds"}) {
		t.Errorf("Unknown = %q", got)
	}
}

// wranglerCLI is the wrangler example/ installs (mise run setup there), the one the repo pins.
func wranglerCLI(t *testing.T) string {
	b, err := os.ReadFile("../example/node_modules/wrangler/wrangler-dist/cli.js")
	if err != nil && os.Getenv("CI") != "" {
		t.Fatal("example/ has no wrangler installed, and CI must check against it: run mise run setup there first")
	} else if err != nil {
		t.Skip("example/ has no wrangler installed: run mise run setup there")
	}
	return string(b)
}

// TestBindingTypes: every type of binding the installed wrangler converts a config's bindings to (its
// convertConfigToBindings) is in BindingScopes or NoScopeKnown, and every scope BindingScopes names is one wrangler's
// login can ask for (its DefaultScopes). A new wrangler with a new kind fails here, so the table can't fall behind it.
func TestBindingTypes(t *testing.T) {
	cli := wranglerCLI(t)
	fn := regexp.MustCompile(`(?s)\nfunction convertConfigToBindings\(.*?\n}\n`).FindString(cli)
	var types []string
	for _, m := range regexp.MustCompile(`type: "([a-z0-9_]+)"`).FindAllStringSubmatch(fn, -1) {
		if !slices.Contains(types, m[1]) {
			types = append(types, m[1])
		}
	}
	if len(types) < 25 {
		t.Fatalf("found %d binding types in wrangler's convertConfigToBindings: has wrangler changed how it converts bindings?", len(types))
	}
	for _, typ := range types {
		if _, ok := BindingScopes[typ]; !ok && !slices.Contains(NoScopeKnown, typ) {
			t.Errorf("wrangler has the binding type %q, which BindingScopes doesn't cover: add it, with the scope whose description in wrangler's login names its product, or none", typ)
		}
	}
	block := regexp.MustCompile(`(?s)DefaultScopes = \{(.*?)\n\s*\}`).FindStringSubmatch(cli)
	if block == nil {
		t.Fatal("found no DefaultScopes in wrangler: has wrangler changed its login's scopes?")
	}
	var scopes []string
	for _, m := range regexp.MustCompile(`"([a-z0-9_:.-]+)":`).FindAllStringSubmatch(block[1], -1) {
		scopes = append(scopes, m[1])
	}
	for typ, b := range BindingScopes {
		for _, s := range b.Scopes {
			if !slices.Contains(scopes, s) {
				t.Errorf("BindingScopes gives %s the scope %q, which wrangler's login can't ask for (it has %s)", typ, s, strings.Join(scopes, " "))
			}
		}
	}
	for _, s := range BaseScopes {
		if !slices.Contains(scopes, s) {
			t.Errorf("BaseScopes has %q, which wrangler's login can't ask for", s)
		}
	}
}

// referenceTable is docs/reference.md's table of BindingScopes.
func referenceTable() string {
	var b strings.Builder
	b.WriteString("| Binding type (wrangler's) | Scope beyond the four | Proven on a real account |\n|---|---|---|\n")
	types := make([]string, 0, len(BindingScopes))
	for typ := range BindingScopes {
		types = append(types, typ)
	}
	slices.Sort(types)
	for _, typ := range types {
		s, p := "none", "Not yet"
		if sc := BindingScopes[typ].Scopes; len(sc) > 0 {
			s = "`" + strings.Join(sc, "`, `") + "`"
		}
		if pr := BindingScopes[typ].Proven; pr != "" {
			p = "Yes (`" + strings.Join(strings.Split(pr, ", "), "`, `") + "`)"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", typ, s, p)
	}
	for _, typ := range NoScopeKnown {
		fmt.Fprintf(&b, "| `%s` | unknown: `cloudflare:status` and `check` say so | Not yet |\n", typ)
	}
	return b.String()
}

// TestReferenceTable: docs/reference.md has BindingScopes' table as it is, so the docs can't say other than the code.
func TestReferenceTable(t *testing.T) {
	doc, _ := os.ReadFile("../docs/reference.md")
	if want := referenceTable(); !strings.Contains(string(doc), want) {
		t.Errorf("docs/reference.md doesn't have BindingScopes' table; put this there:\n%s", want)
	}
}
