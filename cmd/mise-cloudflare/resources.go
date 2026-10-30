package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// resource is one of a Worker's own KV namespaces, D1s or R2 buckets, or one of its previews' own.
type resource struct {
	Binding string `json:"binding"`
	Type    string `json:"type"` // wrangler's: kv_namespace, d1 or r2_bucket
	Kind    string `json:"kind"` // KV, D1 or R2
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"` // a KV namespace's or a D1's; an R2 bucket goes by its name
	Done    string `json:"done"`         // made, there already, deleted
}

// previewResources gives previews their own KV, D1 and R2 (<worker>-previews-<binding>), which Cloudflare doesn't, and puts them
// in the Worker's Previews Base. Nothing goes into the repo. --off deletes them. The Worker must be deployed first.
func (r repo) previewResources(ctx context.Context, off, jsonOut bool) []resource {
	out := r.previewNeeds()
	if len(out) == 0 {
		if !jsonOut {
			fmt.Printf("✓ %s has no KV, D1 or R2 binding for its previews to get their own of\n", r.wc.Name)
		}
		return []resource{}
	}
	token, account := r.cloudflareAPI(ctx)
	env := map[string]any{}
	for i, p := range out {
		var err error
		if off {
			env[p.Binding] = nil // a JSON merge patch's null takes the key out
			continue
		}
		if out[i].ID, out[i].Done, err = r.makeResource(ctx, p); err != nil {
			fail(err)
		}
		// In a preview's bindings, as wrangler sends them: R2 by its bucket's name, KV and D1 by id.
		id := out[i].ID
		if id == "" {
			id = p.Name
		}
		env[p.Binding] = map[string]string{"type": p.Type, wconfig.Created[p.Type].BaseKey: id}
	}
	// Off, a Worker already deleted has no Previews Base left to take them out of: its resources still go.
	if err := patchPreviewsBase(ctx, token, account, r.wc.Name, env); err != nil && !(off && errors.Is(err, errNotOnCloudflare)) {
		fail(err)
	}
	if off {
		for i, p := range out {
			out[i].Done = r.deleteResource(ctx, p)
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

// previewNeeds is the previews' own KV, D1 and R2 the Worker needs: one for each of its bindings of the kinds wrangler
// deploy creates (wconfig.Created) that its previews block leaves out, by wrangler's own types for them.
func (r repo) previewNeeds() []resource {
	var out []resource
	for _, n := range slices.Sorted(maps.Keys(r.wc.Bindings)) {
		typ := r.wc.Bindings[n]
		c, created := wconfig.Created[typ]
		if _, declared := r.wc.PreviewBindings[n]; created && !declared {
			out = append(out, resource{Binding: n, Type: typ, Kind: c.What, Name: previewName(r.wc.Name, n)})
		}
	}
	return out
}

// previewsMissing is what the Previews Base lacks: KV, D1 or R2 added since, and the Worker's required secrets. Nil if
// it can't tell.
func (r repo) previewsMissing(ctx context.Context, token, account string) (resources, secrets []string) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.cloudflare.com/client/v4/accounts/"+account+"/workers/workers/"+url.PathEscape(r.wc.Name), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	var res struct {
		Success bool
		Result  struct {
			Base struct {
				Env map[string]any `json:"env"`
			} `json:"previews_base_config"`
		}
	}
	if json.NewDecoder(resp.Body).Decode(&res) != nil || !res.Success {
		return nil, nil
	}
	for _, p := range r.previewNeeds() {
		if _, ok := res.Result.Base.Env[p.Binding]; !ok {
			resources = append(resources, p.Kind+" "+p.Binding)
		}
	}
	for _, s := range r.wc.Secrets {
		if _, ok := res.Result.Base.Env[s]; !ok {
			secrets = append(secrets, s)
		}
	}
	return resources, secrets
}

// previewName is the name of the previews' own resource for a binding: <worker>-previews-<binding>, lowercase, with
// dashes (R2 allows no other), as wrangler names production's <worker>-<binding>.
func previewName(worker, binding string) string { return worker + "-previews-" + slug(binding) }

// slug is s lowercase, with dashes for anything but letters and digits: what a resource's or a file's name allows.
func slug(s string) string {
	return strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// findResource finds a KV namespace, D1 or R2 bucket by its name with the repo's wrangler: whether it's there, and
// its id (a KV namespace's or a D1's; an R2 bucket has none but its name).
func (r repo) findResource(ctx context.Context, p resource) (bool, string, error) {
	switch p.Type {
	case "kv_namespace":
		var list []struct{ ID, Title string }
		if err := r.wranglerJSON(ctx, &list, "kv", "namespace", "list"); err != nil {
			return false, "", err
		}
		for _, n := range list {
			if n.Title == p.Name {
				return true, n.ID, nil
			}
		}
	case "d1":
		var list []struct{ UUID, Name string }
		if err := r.wranglerJSON(ctx, &list, "d1", "list", "--json"); err != nil {
			return false, "", err
		}
		for _, d := range list {
			if d.Name == p.Name {
				return true, d.UUID, nil
			}
		}
	case "r2_bucket":
		_, err := wrangler.Output(r.w.Cmd(ctx, "r2", "bucket", "info", p.Name))
		return err == nil, "", nil
	}
	return false, "", nil
}

// makeResource finds or makes the previews' own KV namespace, D1 or R2 bucket, and returns its id and what it did.
func (r repo) makeResource(ctx context.Context, p resource) (string, string, error) {
	if there, id, err := r.findResource(ctx, p); err != nil || there {
		return id, "there already", err
	}
	create := map[string][]string{"kv_namespace": {"kv", "namespace", "create", p.Name}, "d1": {"d1", "create", p.Name}, "r2_bucket": {"r2", "bucket", "create", p.Name}}
	if _, err := wrangler.Output(r.w.Cmd(ctx, create[p.Type]...)); err != nil {
		return "", "", fmt.Errorf("making the previews' %s %s: %w", p.Kind, p.Name, err)
	}
	there, id, err := r.findResource(ctx, p)
	if err == nil && !there {
		err = fmt.Errorf("made the previews' %s %s, then couldn't find it", p.Kind, p.Name)
	}
	return id, "made", err
}

// deleteResource deletes a KV namespace, D1 or R2 bucket, and says how it went.
func (r repo) deleteResource(ctx context.Context, p resource) string {
	there, id, err := r.findResource(ctx, p)
	if err != nil || !there {
		return "not there"
	}
	args := map[string][]string{
		"kv_namespace": {"kv", "namespace", "delete", "--namespace-id", id, "--skip-confirmation"},
		"d1":           {"d1", "delete", p.Name, "--skip-confirmation"},
		"r2_bucket":    {"r2", "bucket", "delete", p.Name},
	}[p.Type]
	if _, err := wrangler.Output(r.w.Cmd(ctx, args...)); err != nil {
		if p.Type == "r2_bucket" && strings.Contains(err.Error(), "not empty") {
			return "not deleted: it still holds objects; delete them, then this again"
		}
		return "not deleted: " + err.Error()
	}
	return "deleted"
}

// cloudflareAPI is the repo's login's OAuth token (wrangler renews it) and its account, for Cloudflare's API.
func (r repo) cloudflareAPI(ctx context.Context) (token, account string) {
	who := r.ensureLogin(ctx)
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
		return fmt.Errorf("%s %w: deploy it once first (mise run cloudflare:08-deploy), then this again", worker, errNotOnCloudflare)
	default:
		return fmt.Errorf("Cloudflare refused %s's Previews Base (HTTP %d): %v", worker, resp.StatusCode, res.Errors)
	}
}

// ownResources is the Worker's own KV, D1 and R2: one for each of its bindings of the kinds wrangler deploy creates,
// by the name its config gives it (a D1's database_name, an R2 bucket's bucket_name), else the one deploy gives it,
// <worker>-<binding>.
func (r repo) ownResources() []resource {
	var out []resource
	for _, n := range slices.Sorted(maps.Keys(r.wc.Bindings)) {
		typ := r.wc.Bindings[n]
		c, created := wconfig.Created[typ]
		if !created {
			continue
		}
		name := r.wc.Name + "-" + slug(n)
		if typ != "kv_namespace" { // a KV namespace's binding names it by its id, never by a name
			items, _ := r.wc.Raw[c.Key].([]any)
			for _, it := range items {
				if b, _ := it.(map[string]any); b["binding"] == n {
					if v, _ := b[c.Names].(string); v != "" {
						name = v
					}
				}
			}
		}
		out = append(out, resource{Binding: n, Type: typ, Kind: c.What, Name: name})
	}
	return out
}
