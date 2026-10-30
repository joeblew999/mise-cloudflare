package main

import (
	"context"
	"os"
	"path/filepath"
)

// deploy deploys the Worker (wrangler deploy) on the repo's login, writing its URL where show reads it.
func (r repo) deploy(ctx context.Context, args []string) {
	if os.Getenv("WRANGLER_OUTPUT_FILE_PATH") == "" {
		os.Setenv("WRANGLER_OUTPUT_FILE_PATH", filepath.Join(".wrangler", "output.json"))
	}
	r.wranglerHere(ctx, true, append([]string{"deploy"}, args...))
}
