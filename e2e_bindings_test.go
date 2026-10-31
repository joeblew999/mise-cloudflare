// The extras: bindings of kinds beyond the template's KV, D1 and R2, which the walk adds to a new repo's wrangler
// config as a developer would (mise run new-repo:e2e-extras, E2E_EXTRAS=true), to show on Cloudflare that wrangler
// deploy makes each one and cloudflare:13-delete deletes it, and that cloudflare:06-serve runs one wrangler runs on
// Cloudflare even locally, on the repo's login. The template itself stays as minimal as it is. A kind is one row.

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/joeblew999/mise-cloudflare/internal/cfapi"
)

// extra is one binding the walk adds: what the developer writes at the config's top level ({worker} is the Worker's
// name), and where Cloudflare's API lists what wrangler deploy makes for it, <worker>-<suffix>, by which field of each
// item (no list: it makes nothing); remote when wrangler runs it on Cloudflare even locally (its dev table marks it
// remote), which needs the repo's login.
type extra struct {
	add, list, field, suffix string
	remote                   bool
}

// Of the kinds wrangler deploy makes, these aren't here: Dispatch Namespaces, which this account can't make (403:
// Workers for Platforms, a purchase), and AI Search and Agent Memory namespaces, which wrangler requires the config to
// name: the developer's, which 13-delete leaves (and says so), so a walk would leave them on the account.
var extras = []extra{
	{add: `"queues": { "producers": [{ "binding": "JOBS" }] }`, list: "/queues", field: "queue_name", suffix: "jobs"},
	{add: `"flagship": [{ "binding": "FLAGS" }]`, list: "/flagship/apps", field: "name", suffix: "flags"},
	{add: `"ai": { "binding": "AI" }`, remote: true}, // Workers AI: wrangler runs it on Cloudflare even locally
}

// made is how many of the extras wrangler deploy makes something for.
func made() int {
	n := 0
	for _, e := range extras {
		if e.list != "" {
			n++
		}
	}
	return n
}

// withExtras reports whether this walk adds the extras (mise run new-repo:e2e-extras).
func withExtras() bool { return os.Getenv("E2E_EXTRAS") == "true" }

// remoteExtras reports whether this walk adds an extra wrangler runs on Cloudflare even locally.
func remoteExtras() bool {
	return withExtras() && slices.ContainsFunc(extras, func(e extra) bool { return e.remote })
}

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
		out = append(out, ",\n  "+strings.ReplaceAll(e.add, "{worker}", stampedWorker)...)
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
		if e.list == "" {
			continue
		}
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

// extrasSettle is extrasOn once Cloudflare's lists say what the walk expects, all of the extras (all) or none, looking
// again every few seconds for up to a minute, as a person would: a list can lag what was just made or deleted
// (Flagship's does).
func (r repo) extrasSettle(token, account string, all bool) []string {
	until := time.Now().Add(time.Minute)
	for {
		on := r.extrasOn(token, account)
		if all && len(on) == made() || !all && len(on) == 0 || time.Now().After(until) {
			return on
		}
		time.Sleep(5 * time.Second)
	}
}
