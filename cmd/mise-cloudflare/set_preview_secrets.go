package main

import "context"

// setPreviewSecrets sets each secret the Worker requires that its Previews Base doesn't have, which every preview starts
// from: previews don't inherit the Worker's.
func (r repo) setPreviewSecrets(ctx context.Context) {
	r.setSecrets(ctx, true)
}
