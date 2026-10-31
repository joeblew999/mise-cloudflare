// The extras: bindings of kinds beyond the template's KV, D1 and R2, which the walk adds to a new repo's wrangler
// config as a developer would (mise run example:e2e-extras, E2E_EXTRAS=true), to show on Cloudflare that wrangler
// deploy makes each one and cloudflare:13-delete deletes it. The template itself stays as minimal as it is. A kind is
// one row.

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joeblew999/mise-cloudflare/internal/cfapi"
)

// extra is one binding the walk adds: what the developer writes at the config's top level, and where Cloudflare's
// API lists what wrangler deploy makes for it, <worker>-<suffix>, by which field of each item.
type extra struct {
	add, list, field, suffix string
}

var extras = []extra{
	{add: `"queues": { "producers": [{ "binding": "JOBS" }] }`, list: "/queues", field: "queue_name", suffix: "jobs"},
}

// withExtras reports whether this walk adds the extras (mise run example:e2e-extras).
func withExtras() bool { return os.Getenv("E2E_EXTRAS") == "true" }

// addExtras writes the extras into the repo's wrangler.jsonc, as a developer adds bindings at its top level.
func (r repo) addExtras() {
	path := filepath.Join(r.dir, "wrangler.jsonc")
	b, err := os.ReadFile(path)
	end := bytes.LastIndexByte(b, '}')
	if err != nil || end < 0 {
		r.t.Fatalf("%s: %v", path, err)
	}
	out := bytes.TrimRight(b[:end], " \n")
	for _, e := range extras {
		out = append(out, ",\n  "+e.add...)
	}
	if err := os.WriteFile(path, append(append(out, '\n'), b[end:]...), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.pass("the developer adds %d binding(s) beyond the template's to wrangler.jsonc: the extras", len(extras))
}

// extrasOn names the extras' resources Cloudflare's API lists on the account.
func (r repo) extrasOn(token, account string) []string {
	var on []string
	for _, e := range extras {
		got, err := cfapi.Call(context.Background(), token, http.MethodGet, "/accounts/"+account+e.list, nil, "")
		var items []map[string]any
		if err != nil || !got.OK() || json.Unmarshal(got.Result, &items) != nil {
			r.t.Fatalf("Cloudflare's %s: %v %s", e.list, err, got.Said())
		}
		for _, it := range items {
			if it[e.field] == r.worker+"-"+e.suffix {
				on = append(on, e.list[1:]+" "+r.worker+"-"+e.suffix)
			}
		}
	}
	return on
}
