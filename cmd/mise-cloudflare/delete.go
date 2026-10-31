package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/joeblew999/mise-cloudflare/internal/cfapi"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// delete is deploy undone: the Worker deleted with wrangler, and with it its own Durable Objects and all their data
// (it says so when it asks), then the KV, D1 and R2 that deploy made for it and its previews' own, each found by the
// name deploy and preview give it (wrangler delete leaves them). A resource the config names is the developer's: it
// stays, and it says so. It never deletes what's in an R2 bucket: one that still holds objects stays, and it says so.
// Every other binding it names as left as it is: it's never silent about one. It asks first, unless yes.
func (r repo) delete(ctx context.Context, yes bool) {
	token, account := r.cloudflareAPI(ctx)
	// Whether the Worker is there is Cloudflare's API's to say, by its status: 404 is not there.
	got, err := cfapi.Call(ctx, token, http.MethodGet, "/accounts/"+account+"/workers/scripts/"+url.PathEscape(r.wc.Name)+"/settings", nil, "")
	switch {
	case err != nil:
		fail(err)
	case got.Status != http.StatusNotFound && !got.OK():
		fail(fmt.Errorf("the Worker %s: %s", r.wc.Name, got.Said()))
	}
	there := got.OK()
	var classes []string
	goneWith := map[string]bool{} // the bindings to its own Durable Objects, which go with it
	own, kept := r.ownResources()
	previews := r.previewNeeds()
	if there {
		var ids map[string]bool
		if classes, ids, err = r.durableObjects(ctx, token, account); err != nil {
			fail(err)
		}
		var s struct {
			Bindings []map[string]any `json:"bindings"`
		}
		json.Unmarshal(got.Result, &s)
		bindings := map[string]map[string]any{}
		for _, b := range s.Bindings {
			name, _ := b["name"].(string)
			bindings[name] = b
			if id, _ := b["namespace_id"].(string); ids[id] {
				goneWith[name] = true
			}
		}
		// What its bindings and its Previews Base use, by id, before wrangler delete takes them with the Worker.
		own, previews = withIDs(own, bindings), withIDs(previews, r.previewsBase(ctx, token, account))
	}
	if !yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fail(errors.New("delete asks first, and there's no terminal to ask on: run it with --yes"))
		}
		fmt.Printf("Delete the Worker %s%s? Type its name: ", r.wc.Name, r.deleting(classes))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != r.wc.Name {
			fail(errors.New("not deleted"))
		}
	}
	var left []string
	if !there {
		fmt.Printf("✓ the Worker %s isn't on Cloudflare\n", r.wc.Name)
	} else {
		if _, err := wrangler.Output(r.w.Cmd(ctx, "delete", "--force")); err != nil {
			fail(err)
		}
		fmt.Printf("✓ the Worker %s is deleted\n", r.wc.Name)
		// wrangler delete deletes the Worker's own Durable Objects with it: Cloudflare's list says whether it did.
		still, _, err := r.durableObjects(ctx, token, account)
		if err != nil {
			fail(err)
		}
		for _, c := range classes {
			if slices.Contains(still, c) {
				fmt.Printf("✗ Durable Object %s: still on Cloudflare\n", c)
				left = append(left, "Durable Object "+c)
			} else {
				fmt.Printf("✓ Durable Object %s: deleted with the Worker, all its data too\n", c)
			}
		}
	}
	for _, p := range append(own, previews...) {
		done, gone := r.deleteResource(ctx, p, token, account)
		mark := "✓"
		if !gone {
			mark, left = "✗", append(left, p.Kind+" "+p.Binding+", "+p.Name)
		}
		fmt.Printf("%s %s %s, %s: %s\n", mark, p.Kind, p.Binding, p.Name, done)
	}
	for _, k := range kept {
		fmt.Printf("· %s is the one %s names: yours, so it stays\n", k, filepath.Base(r.wc.Path))
	}
	if left := r.notDeleted(goneWith); len(left) > 0 {
		fmt.Printf("· not deleted here: %s. What any of them has on Cloudflare of its own (a Hyperdrive config, say; Workers AI has none) is still there\n", strings.Join(left, ", "))
	}
	if len(left) > 0 {
		fail(fmt.Errorf("not all deleted: %s is still on Cloudflare (what Cloudflare said is above)", strings.Join(left, "; ")))
	}
}

// deleting is what delete deletes besides the Worker, for its question: the Worker's own Durable Objects (classes) and
// what deploy and the previews made for it, by name, all with their data.
func (r repo) deleting(classes []string) string {
	var what []string
	if len(classes) > 0 {
		what = append(what, "its Durable Objects ("+strings.Join(classes, ", ")+")")
	}
	own, _ := r.ownResources()
	for _, p := range append(own, r.previewNeeds()...) {
		what = append(what, p.Kind+" "+p.Name)
	}
	if len(what) == 0 {
		return ""
	}
	return ", with " + strings.Join(what, ", ") + " and all their data"
}

// goesWithWorker is the binding kinds that are the Worker's own, in what it uploads: its vars, secrets and static
// assets, deleted with it.
var goesWithWorker = map[string]bool{"plain_text": true, "json": true, "secret_text": true, "assets": true}

// notDeleted is every binding delete leaves as it is, by wrangler's types: all but the kinds it deletes or keeps (and
// says so), and what goes with the Worker (goneWith: the bindings to its own Durable Objects). Delete is never silent
// about one.
func (r repo) notDeleted(goneWith map[string]bool) []string {
	var out []string
	for _, n := range slices.Sorted(maps.Keys(r.wc.Bindings)) {
		typ := r.wc.Bindings[n]
		if _, own := deletes[typ]; own || goesWithWorker[typ] || goneWith[n] {
			continue
		}
		out = append(out, n+" ("+typ+")")
	}
	return out
}

// durableObjects is the Worker's own Durable Object namespaces on Cloudflare (their script is the Worker), by class,
// and their ids.
func (r repo) durableObjects(ctx context.Context, token, account string) ([]string, map[string]bool, error) {
	got, err := cfapi.Call(ctx, token, http.MethodGet, "/accounts/"+account+"/workers/durable_objects/namespaces?per_page=1000", nil, "")
	var list []struct {
		ID     string `json:"id"`
		Class  string `json:"class"`
		Script string `json:"script"`
	}
	if err == nil && !got.OK() {
		err = fmt.Errorf("Cloudflare's Durable Object namespaces: %s", got.Said())
	}
	if err == nil {
		err = json.Unmarshal(got.Result, &list)
	}
	var classes []string
	ids := map[string]bool{}
	for _, n := range list {
		if n.Script == r.wc.Name {
			classes, ids[n.ID] = append(classes, n.Class), true
		}
	}
	return classes, ids, err
}
