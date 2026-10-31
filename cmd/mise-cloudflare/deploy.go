package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// deploy deploys the Worker (wrangler deploy) on the repo's login, writing its URL where show reads it.
func (r repo) deploy(ctx context.Context, args []string) {
	if os.Getenv("WRANGLER_OUTPUT_FILE_PATH") == "" {
		os.Setenv("WRANGLER_OUTPUT_FILE_PATH", filepath.Join(".wrangler", "output.json"))
	}
	r.wranglerHere(ctx, append([]string{"deploy"}, args...))
	if url := lastDeployURL(r.dir); url != "" && !slices.Contains(args, "--dry-run") {
		waitTillItAnswers(ctx, url)
	}
}

// waitTillItAnswers waits up to a minute for a just-deployed Worker's URL to answer, and says whether it does: right
// after a first deploy, Cloudflare answers "error code: 1042" for a few seconds while the Worker reaches its network.
func waitTillItAnswers(ctx context.Context, url string) {
	client := &http.Client{Timeout: 10 * time.Second}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if resp, err := client.Do(req); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
			resp.Body.Close()
			if resp.StatusCode < 500 && !strings.HasPrefix(string(body), "error code:") {
				fmt.Printf("✓ %s answers\n", url)
				return
			}
		}
	}
	fmt.Printf("· %s doesn't answer yet: a first deploy can take a minute or two to reach Cloudflare's network; open it again shortly\n", url)
}
