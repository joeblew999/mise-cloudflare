package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// delete is deploy undone: the Worker deleted with wrangler, then its own KV, D1 and R2 and its previews' own, each
// found by the name deploy and preview give it. It never deletes what's in an R2 bucket: one that still holds objects
// stays, and it says so. It asks first, unless yes.
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
	r.ensureLogin(ctx)
	if _, err := wrangler.Output(r.w.Cmd(ctx, "delete", "--force")); err == nil {
		fmt.Printf("✓ the Worker %s is deleted\n", r.wc.Name)
	} else if strings.Contains(err.Error(), "10007") {
		fmt.Printf("✓ the Worker %s isn't on Cloudflare\n", r.wc.Name)
	} else {
		fail(err)
	}
	for _, p := range append(r.ownResources(), r.previewNeeds()...) {
		fmt.Printf("✓ %s %s, %s: %s\n", p.Kind, p.Binding, p.Name, r.deleteResource(ctx, p))
	}
}
