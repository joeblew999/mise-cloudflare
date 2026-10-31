package main

import "context"

// preview deploys a preview of what's here now (wrangler preview), on the repo's login: the previews' own KV, D1, R2,
// Queues and Flagship apps first, which wrangler preview doesn't make, and the package's own build, when it has one.
func (r repo) preview(ctx context.Context, args []string) {
	r.ensureLogin(ctx)
	if r.wc.Previews {
		r.previewResources(ctx, false, false)
	}
	r.build(ctx)
	r.wranglerHere(ctx, append([]string{"preview"}, args...))
}
