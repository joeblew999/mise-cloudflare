package main

import (
	"bufio"
	"context"
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

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// delete is deploy undone: the Worker deleted with wrangler, then the KV, D1 and R2 that deploy made for it and its
// previews' own, each found by the name deploy and preview give it (wrangler delete leaves them). A resource the config
// names is the developer's: it stays, and it says so. It never deletes what's in an R2 bucket: one that still holds
// objects stays, and it says so. Every other binding it names as left as it is: it's never silent about one. It asks
// first, unless yes.
func (r repo) delete(ctx context.Context, yes bool) {
	if !yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fail(errors.New("delete asks first, and there's no terminal to ask on: run it with --yes"))
		}
		fmt.Printf("Delete the Worker %s, its KV, D1 and R2 and its previews' own, with their data? Type its name: ", r.wc.Name)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != r.wc.Name {
			fail(errors.New("not deleted"))
		}
	}
	token, account := r.cloudflareAPI(ctx)
	// Whether the Worker is there is Cloudflare's API's to say, by its status: 404 is not there.
	got, err := callAPI(ctx, token, http.MethodGet, "/accounts/"+account+"/workers/scripts/"+url.PathEscape(r.wc.Name)+"/settings", nil, "")
	switch {
	case err != nil:
		fail(err)
	case got.Status == http.StatusNotFound:
		fmt.Printf("✓ the Worker %s isn't on Cloudflare\n", r.wc.Name)
	case !got.OK():
		fail(fmt.Errorf("the Worker %s: %s", r.wc.Name, got.Said()))
	default:
		if _, err := wrangler.Output(r.w.Cmd(ctx, "delete", "--force")); err != nil {
			fail(err)
		}
		fmt.Printf("✓ the Worker %s is deleted\n", r.wc.Name)
	}
	own, kept := r.ownResources()
	for _, p := range append(own, r.previewNeeds()...) {
		fmt.Printf("✓ %s %s, %s: %s\n", p.Kind, p.Binding, p.Name, r.deleteResource(ctx, p, token, account))
	}
	for _, k := range kept {
		fmt.Printf("· %s is the one %s names: yours, so it stays\n", k, filepath.Base(r.wc.Path))
	}
	if left := r.notDeleted(); len(left) > 0 {
		fmt.Printf("· not deleted here, left as they are: %s (whatever any of them is on Cloudflare, it's still there)\n", strings.Join(left, ", "))
	}
}

// goesWithWorker is the binding kinds that are the Worker's own, in what it uploads: its vars, secrets and static
// assets, deleted with it.
var goesWithWorker = map[string]bool{"plain_text": true, "json": true, "secret_text": true, "assets": true}

// notDeleted is every binding delete leaves as it is, by wrangler's types: all but the KV, D1 and R2 it deletes or
// keeps (and says so), and what goes with the Worker. Delete is never silent about one.
func (r repo) notDeleted() []string {
	var out []string
	for _, n := range slices.Sorted(maps.Keys(r.wc.Bindings)) {
		typ := r.wc.Bindings[n]
		if _, own := wconfig.PreviewsOwn[typ]; own || goesWithWorker[typ] {
			continue
		}
		out = append(out, n+" ("+typ+")")
	}
	return out
}
