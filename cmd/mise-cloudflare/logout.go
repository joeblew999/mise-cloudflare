package main

import (
	"context"
	"fmt"

	"github.com/joeblew999/mise-cloudflare/internal/cf"
)

// logout logs the repo's login out on Cloudflare and deletes it, and cf's login for Workers Builds if there is one.
func (r repo) logout(ctx context.Context) {
	if r.w.LoggedIn(ctx) {
		r.refuseStrayTokens()
		must(r.w.Delete(ctx))
	}
	if cf.Installed() {
		c := cf.CF{Dir: r.dir, Profile: r.w.Profile}
		if who, err := c.Whoami(ctx); err == nil && who.Authenticated {
			must(c.Delete(ctx))
		}
	}
	fmt.Printf("✓ %s is logged out and deleted\n", r.w.Profile)
}
