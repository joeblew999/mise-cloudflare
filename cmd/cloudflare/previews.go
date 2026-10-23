package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// previewKinds are the bindings a preview gets from the Worker's Previews Base when the previews block leaves them out:
// KV, D1 and R2, which wrangler deploy creates for production and wrangler preview doesn't create at all.
var previewKinds = []struct{ key, kind, what string }{
	{"kv_namespaces", "kv_namespace", "KV"}, {"d1_databases", "d1", "D1"}, {"r2_buckets", "r2_bucket", "R2"},
}

// previewResource is one of the previews' own KV namespaces, D1s or R2 buckets.
type previewResource struct {
	Binding string `json:"binding"`
	Kind    string `json:"kind"` // KV, D1 or R2
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"` // a KV namespace's or a D1's; an R2 bucket goes by its name
	Done    string `json:"done"`         // made, there already, deleted
}

// previews gives the Worker's previews their own KV, D1 and R2. Cloudflare makes previews none (only Durable Objects
// and Containers), and wrangler preview creates none, as wrangler deploy does for production. So for each KV, D1 and R2
// binding its previews block leaves out, it makes <worker>-previews-<binding> once, with the repo's own wrangler, and
// puts it in the Worker's Previews Base on Cloudflare, which every new preview starts from: nothing goes into the repo,
// and production's and staging's stay theirs. Run again, it makes only what's new. With off, it takes them out of the
// Previews Base and deletes them. The Worker must be on Cloudflare already: deployed once, or by Workers Builds.
func (r repo) previews(ctx context.Context, off, jsonOut bool) []previewResource {
	declared, _ := r.wc.Raw["previews"].(map[string]any)
	var out []previewResource
	for _, k := range previewKinds {
		have := map[string]bool{}
		for _, b := range bindingList(declared[k.key]) {
			have[fmt.Sprint(b["binding"])] = true
		}
		for _, b := range bindingList(r.wc.Raw[k.key]) {
			if n := fmt.Sprint(b["binding"]); !have[n] {
				out = append(out, previewResource{Binding: n, Kind: k.what, Name: previewName(r.wc.Name, n)})
			}
		}
	}
	if len(out) == 0 {
		if !jsonOut {
			fmt.Printf("✓ %s has no KV, D1 or R2 binding for its previews to get their own of\n", r.wc.Name)
		}
		return []previewResource{}
	}
	token, account := r.cloudflareAPI(ctx)
	env := map[string]any{}
	for i, p := range out {
		var err error
		if off {
			env[p.Binding] = nil // a JSON merge patch's null takes the key out
			continue
		}
		if out[i].ID, out[i].Done, err = r.previewResource(ctx, p); err != nil {
			fail(err)
		}
		switch p.Kind {
		case "KV":
			env[p.Binding] = map[string]string{"type": "kv_namespace", "namespace_id": out[i].ID}
		case "D1":
			env[p.Binding] = map[string]string{"type": "d1", "database_id": out[i].ID}
		case "R2":
			env[p.Binding] = map[string]string{"type": "r2_bucket", "bucket_name": p.Name}
		}
	}
	// Off, a Worker already deleted has no Previews Base left to take them out of: its resources still go.
	if err := patchPreviewsBase(ctx, token, account, r.wc.Name, env); err != nil && !(off && errors.Is(err, errNotOnCloudflare)) {
		fail(err)
	}
	if off {
		for i, p := range out {
			out[i].Done = r.deletePreviewResource(ctx, p)
		}
	}
	if !jsonOut {
		for _, p := range out {
			fmt.Printf("✓ %s's previews: %s %s is %s (%s)\n", r.wc.Name, p.Kind, p.Binding, p.Name, p.Done)
		}
		if !off {
			fmt.Printf("  in its Previews Base on Cloudflare, which every new preview starts from\n")
		}
	}
	return out
}

// previewName is the name of the previews' own resource for a binding: <worker>-previews-<binding>, lowercase, with
// dashes (R2 allows no other), as wrangler names production's <worker>-<binding>.
func previewName(worker, binding string) string {
	return worker + "-previews-" + strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(binding), "-"), "-")
}

// findPreview finds the previews' own KV namespace, D1 or R2 bucket with the repo's wrangler: whether it's there, and
// its id (a KV namespace's or a D1's; an R2 bucket has none but its name).
func (r repo) findPreview(ctx context.Context, p previewResource) (bool, string, error) {
	switch p.Kind {
	case "KV":
		var list []struct{ ID, Title string }
		if err := r.wranglerJSON(ctx, &list, "kv", "namespace", "list"); err != nil {
			return false, "", err
		}
		for _, n := range list {
			if n.Title == p.Name {
				return true, n.ID, nil
			}
		}
	case "D1":
		var list []struct{ UUID, Name string }
		if err := r.wranglerJSON(ctx, &list, "d1", "list", "--json"); err != nil {
			return false, "", err
		}
		for _, d := range list {
			if d.Name == p.Name {
				return true, d.UUID, nil
			}
		}
	case "R2":
		_, err := wrangler.Output(r.wranglerCmd(ctx, "r2", "bucket", "info", p.Name))
		return err == nil, "", nil
	}
	return false, "", nil
}

// previewResource finds or makes the previews' own KV namespace, D1 or R2 bucket, and returns its id and what it did.
func (r repo) previewResource(ctx context.Context, p previewResource) (string, string, error) {
	if there, id, err := r.findPreview(ctx, p); err != nil || there {
		return id, "there already", err
	}
	create := map[string][]string{"KV": {"kv", "namespace", "create", p.Name}, "D1": {"d1", "create", p.Name}, "R2": {"r2", "bucket", "create", p.Name}}
	if _, err := wrangler.Output(r.wranglerCmd(ctx, create[p.Kind]...)); err != nil {
		return "", "", fmt.Errorf("making the previews' %s %s: %w", p.Kind, p.Name, err)
	}
	there, id, err := r.findPreview(ctx, p)
	if err == nil && !there {
		err = fmt.Errorf("made the previews' %s %s, then couldn't find it", p.Kind, p.Name)
	}
	return id, "made", err
}

// deletePreviewResource deletes the previews' own KV namespace, D1 or R2 bucket, and says how it went.
func (r repo) deletePreviewResource(ctx context.Context, p previewResource) string {
	there, id, err := r.findPreview(ctx, p)
	if err != nil || !there {
		return "not there"
	}
	args := map[string][]string{
		"KV": {"kv", "namespace", "delete", "--namespace-id", id, "--skip-confirmation"},
		"D1": {"d1", "delete", p.Name, "--skip-confirmation"},
		"R2": {"r2", "bucket", "delete", p.Name},
	}[p.Kind]
	if _, err := wrangler.Output(r.wranglerCmd(ctx, args...)); err != nil {
		if p.Kind == "R2" && strings.Contains(err.Error(), "not empty") {
			return "not deleted: it still holds objects; delete them, then this again"
		}
		return "not deleted: " + err.Error()
	}
	return "deleted"
}

// wranglerCmd is a wrangler command on the repo's login, or in CI on CLOUDFLARE_API_TOKEN.
func (r repo) wranglerCmd(ctx context.Context, args ...string) *exec.Cmd {
	if ci() {
		return r.w.Plain(ctx, args...)
	}
	return r.w.Cmd(ctx, args...)
}

// wranglerJSON runs a wrangler command and decodes its JSON, past anything it prints first (a warning, say).
func (r repo) wranglerJSON(ctx context.Context, v any, args ...string) error {
	out, err := wrangler.Output(r.wranglerCmd(ctx, args...))
	if err != nil {
		return err
	}
	if i := regexp.MustCompile(`(?m)^[\[{]`).FindIndex(out); i != nil {
		out = out[i[0]:]
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("wrangler %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// cloudflareAPI is the token and account a call to Cloudflare's API goes with: CI's secrets in CI, else the repo's own
// login's (its OAuth token, which wrangler renews).
func (r repo) cloudflareAPI(ctx context.Context) (token, account string) {
	if ci() {
		token, account = os.Getenv("CLOUDFLARE_API_TOKEN"), os.Getenv("CLOUDFLARE_ACCOUNT_ID")
		if token == "" || account == "" {
			fail(errors.New("in CI this needs the CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID secrets (cloudflare ci-token stores them)"))
		}
		return token, account
	}
	who := r.login(ctx)
	if len(who.Accounts) != 1 {
		fail(fmt.Errorf("the login %s can use %d accounts; log it in again and pick one", r.w.Profile, len(who.Accounts)))
	}
	token, err := r.w.Token(ctx)
	if err != nil {
		fail(err)
	}
	return token, who.Accounts[0].ID
}

// errNotOnCloudflare is a Worker that isn't deployed.
var errNotOnCloudflare = errors.New("isn't on Cloudflare yet")

// patchPreviewsBase merges env into the bindings of the Worker's Previews Base, as wrangler does for its secrets (a JSON
// merge patch of the Worker, so the base's other bindings and secrets stay).
func patchPreviewsBase(ctx context.Context, token, account, worker string, env map[string]any) error {
	body, _ := json.Marshal(map[string]any{"previews_base_config": map[string]any{"env": env}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch,
		"https://api.cloudflare.com/client/v4/accounts/"+account+"/workers/workers/"+url.PathEscape(worker), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var res struct {
		Success bool
		Errors  []struct {
			Code    int
			Message string
		}
	}
	json.NewDecoder(resp.Body).Decode(&res)
	switch {
	case res.Success:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s %w: deploy it once first (mise run cloudflare:deploy), then this again", worker, errNotOnCloudflare)
	default:
		return fmt.Errorf("Cloudflare refused %s's Previews Base (HTTP %d): %v", worker, resp.StatusCode, res.Errors)
	}
}

func bindingList(v any) []map[string]any {
	items, _ := v.([]any)
	var out []map[string]any
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
