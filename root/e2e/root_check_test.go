package e2e

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
	"github.com/joeblew999/mise-cloudflare/templates"
)

// TestRootCheck checks this repo's own side against what it gives other repos, after the steps have set the reference
// up (mise run root:check runs example:04-check first): the examples keep to themselves, the steps run a repo's tasks
// in the template's order, every task here runs the Go of its name, and the tool reads the reference's config and
// scopes through its own wrangler. That the reference has what this version's init writes is example:04-check's own
// check.
func TestRootCheck(t *testing.T) {
	if os.Getenv("CHECK") != "1" {
		t.Skip("mise run root:check")
	}
	ref, _ := filepath.Abs("../../examples/reference")
	if _, err := os.Stat(filepath.Join(ref, "node_modules", "wrangler")); err != nil {
		t.Fatal("the reference has no wrangler installed: mise run example:02-setup")
	}

	t.Run("the examples and this repo keep to themselves", func(t *testing.T) {
		strays, _ := filepath.Glob("../../.github/workflows/cloudflare-example*.yml")
		for _, p := range append(strays, "../../.claude/hooks/"+templates.HookFile) {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("%s is an example's, in this repo: delete it, and run mise run example:03-init", strings.TrimPrefix(p, "../../"))
			}
		}
		// This repo's own Claude Code hook is its own: nothing in templates/, which is other repos', runs for it.
		if b, _ := os.ReadFile("../../.claude/settings.json"); strings.Contains(string(b), templates.HookFile) || strings.Contains(string(b), "templates/") {
			t.Errorf(".claude/settings.json runs what other repos get: this repo's own hook is .claude/hooks/session-start.sh")
		}
	})

	t.Run("the steps line up with a repo's tasks", func(t *testing.T) {
		steps, err := os.ReadFile("../../example.mise.toml")
		if err != nil {
			t.Fatal(err)
		}
		// One name and one number all the way through: from 04 on, step example:nn-x runs the repo's task
		// cloudflare:nn-x (03-init runs mise-cloudflare init itself, before there are tasks), and example:verify runs
		// cloudflare:verify.
		var stepped []string
		for _, m := range regexp.MustCompile(`(?m)^\["example:((\d\d)-[a-z-]+|verify)"\]\n(?:.*\n)*?run = "([^"]*)"`).FindAllStringSubmatch(string(steps), -1) {
			name, nn, run := m[1], m[2], m[3]
			if nn != "" && nn < "03" {
				continue
			}
			stepped = append(stepped, name)
			if want := "mise run cloudflare:" + name; name != "03-init" && run != want && !strings.HasPrefix(run, want+" ") {
				t.Errorf("step example:%s runs %q, not %s", name, run, want)
			}
		}
		// The template's tasks from 03 on, in its order, are the steps, in theirs.
		var tasks []string
		for _, m := range regexp.MustCompile(`(?m)^\["cloudflare:((\d\d)-[a-z-]+|verify)"\]`).FindAllStringSubmatch(string(templates.Tasks), -1) {
			tasks = append(tasks, m[1])
		}
		if !slices.Equal(stepped, tasks) {
			t.Errorf("the steps %v don't line up with the template's tasks %v", stepped, tasks)
		}
	})

	t.Run("every task here runs the Go of its name", func(t *testing.T) {
		tasks, err := os.ReadFile("../../mise.toml")
		if err != nil {
			t.Fatal(err)
		}
		// noun:verb runs TestNounVerb in root/e2e/noun_verb_test.go, or the program root/noun/verb; the others (tool:build,
		// tool:install, tool:test) run go or mise themselves, with no Go of their own.
		camel := func(s string) string {
			out := ""
			for _, w := range strings.Split(s, "-") {
				out += strings.ToUpper(w[:1]) + w[1:]
			}
			return out
		}
		test, program := regexp.MustCompile(`go test \./root/e2e -run '\^(Test\w+)\$'`), regexp.MustCompile(`go run \./root/(\S+)`)
		for _, m := range regexp.MustCompile(`(?m)^\[tasks\."([a-z]+):([a-z-]+)"\]\n(?:.*\n)*?run = "([^"]*)"`).FindAllStringSubmatch(string(tasks), -1) {
			noun, verb, run := m[1], m[2], m[3]
			if got := test.FindStringSubmatch(run); got != nil {
				want, file := "Test"+camel(noun)+camel(verb), noun+"_"+strings.ReplaceAll(verb, "-", "_")+"_test.go"
				if got[1] != want {
					t.Errorf("%s:%s runs %s, not %s", noun, verb, got[1], want)
				}
				if b, _ := os.ReadFile(file); !strings.Contains(string(b), "func "+want+"(t *testing.T)") {
					t.Errorf("%s:%s: root/e2e/%s has no %s", noun, verb, file, want)
				}
			} else if got := program.FindStringSubmatch(run); got != nil && got[1] != noun+"/"+verb {
				t.Errorf("%s:%s runs root/%s, not root/%s/%s", noun, verb, got[1], noun, verb)
			}
		}
	})

	t.Run("wrangler reads the reference's config", func(t *testing.T) {
		c, err := wconfig.Read(context.Background(), ref)
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
	})

	t.Run("the reference's wrangler gives its login's scopes", func(t *testing.T) {
		scopes, err := wrangler.W{Dir: ref}.Scopes(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"account:read", "user:read", "workers:write", "workers_scripts:write", "workers_kv:write", "d1:write"} {
			if !slices.Contains(scopes, s) {
				t.Errorf("wrangler's scopes %q lack %s", scopes, s)
			}
		}
	})
}
