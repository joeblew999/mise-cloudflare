package main

import "context"

// preview deploys a preview of what's here now (wrangler preview), on the repo's login: the previews' own KV, D1 and R2
// first, which wrangler preview doesn't make.
func (r repo) preview(ctx context.Context, args []string) {
	if _, ok := r.wc.Raw["previews"]; ok {
		r.ensureLogin(ctx)
		r.previewResources(ctx, false, false)
	}
	r.wranglerHere(ctx, true, append([]string{"preview"}, args...))
}
