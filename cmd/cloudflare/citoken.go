package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/cf"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// ciGroups is what CI's token may do: Cloudflare's usual Workers permissions (what Workers Builds' own build token
// has), and cloudflare.toml's [workflow] permissions for the Worker's other bindings.
var ciGroups = []string{"Account Settings Read", "Workers Scripts Write", "Workers KV Storage Write", "Workers R2 Storage Write"}

// ciToken makes the Cloudflare API token GitHub Actions deploys with (cloudflare.toml's [workflow] deploy), on the
// repo's cf login, and stores it in the repository's GitHub secrets with gh: CLOUDFLARE_API_TOKEN and
// CLOUDFLARE_ACCOUNT_ID, and the token's id as the variable CLOUDFLARE_API_TOKEN_ID, so off can delete it. Its value
// goes from cf to gh through a file only this user can read, and is never printed. With off it deletes the token and the
// secrets. The one Cloudflare token in the whole setup: GitHub has no one to click Allow.
func (r repo) ciToken(ctx context.Context, off bool) {
	if ci() {
		fail(errors.New("CI's token is made once, from a machine or a Claude Code session, not in CI"))
	}
	if !cf.Installed() {
		fail(errors.New("cf isn't installed: run mise run cloudflare:ci-token, which brings it"))
	}
	if _, err := exec.LookPath("gh"); err != nil {
		fail(errors.New("gh isn't installed: run mise run cloudflare:ci-token, which brings it"))
	}
	if exec.CommandContext(ctx, "gh", "auth", "status").Run() != nil {
		fail(errors.New("gh isn't logged in: run gh auth login (it gives a code to approve on GitHub), then this again"))
	}
	r.refuseStrayTokens()
	gh, err := githubRepo(ctx, r.dir)
	if err != nil {
		fail(err)
	}
	repoName := gh.Owner.Login + "/" + gh.Name
	c := cf.CF{Dir: r.dir, Profile: r.w.Profile}
	scopes := append(slices.Clone(cf.Scopes), "account_api_tokens:create")
	who, err := c.Whoami(ctx)
	if err != nil {
		fail(err)
	}
	if !who.Authenticated || !holds(who.Scopes, scopes) {
		must(c.LoginScopes(ctx, scopes, !wrangler.Device()))
		if who, err = c.Whoami(ctx); err != nil || !who.Authenticated || !holds(who.Scopes, scopes) {
			fail(fmt.Errorf("the cf login %s doesn't hold %s", c.Profile, strings.Join(scopes, " ")))
		}
	}
	if len(who.Accounts) != 1 {
		fail(fmt.Errorf("the cf login %s can use %d accounts; log it in again and pick one", c.Profile, len(who.Accounts)))
	}
	c.Account = who.Accounts[0].ID

	old, _ := exec.CommandContext(ctx, "gh", "variable", "get", "CLOUDFLARE_API_TOKEN_ID", "--repo", repoName).Output()
	if id := strings.TrimSpace(string(old)); id != "" {
		if err := c.JSON(ctx, nil, "accounts", "tokens", "delete", id, "--force"); err != nil && !cf.NotFound(err) {
			fail(err)
		}
	}
	if off {
		for _, s := range []string{"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID"} {
			exec.CommandContext(ctx, "gh", "secret", "delete", s, "--repo", repoName).Run()
		}
		exec.CommandContext(ctx, "gh", "variable", "delete", "CLOUDFLARE_API_TOKEN_ID", "--repo", repoName).Run()
		fmt.Printf("✓ CI's Cloudflare token is deleted, and %s's secrets with it\n", repoName)
		return
	}

	groups := append(slices.Clone(ciGroups), r.workflow.Permissions...)
	var all []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := c.JSON(ctx, &all, "accounts", "tokens", "permission-groups", "list"); err != nil {
		fail(err)
	}
	var ids []map[string]string
	for _, g := range groups {
		i := slices.IndexFunc(all, func(a struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}) bool {
			return a.Name == g && slices.Contains(a.Scopes, "com.cloudflare.api.account")
		})
		if i < 0 {
			fail(fmt.Errorf("Cloudflare has no account permission group named %q (cloudflare.toml's [workflow] permissions)", g))
		}
		ids = append(ids, map[string]string{"id": all[i].ID})
	}
	tmp, err := os.MkdirTemp("", "cloudflare-ci-token-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(tmp)
	policies, _ := json.Marshal([]map[string]any{{
		"effect":            "allow",
		"resources":         map[string]string{"com.cloudflare.api.account." + c.Account: "*"},
		"permission_groups": ids,
	}})
	policyFile, secretFile := filepath.Join(tmp, "policies.json"), filepath.Join(tmp, "secret")
	must(os.WriteFile(policyFile, policies, 0o600))
	var token struct{ ID, Value string }
	name := r.wc.Name + " GitHub Actions (mise-cloudflare)"
	if err := c.JSON(ctx, &token, "accounts", "tokens", "create", "--name", name, "--policies", "@"+policyFile); err != nil {
		fail(err)
	}
	if token.ID == "" || token.Value == "" {
		fail(errors.New("cf accounts tokens create returned no token"))
	}
	must(os.WriteFile(secretFile, []byte(token.Value), 0o600))
	set := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "gh", append(args, "--repo", repoName)...)
		if args[0] == "secret" && args[2] == "CLOUDFLARE_API_TOKEN" {
			f, err := os.Open(secretFile)
			if err != nil {
				return err
			}
			defer f.Close()
			cmd.Stdin = f
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("gh %s: %w: %s", strings.Join(args[:3], " "), err, out)
		}
		return nil
	}
	for _, args := range [][]string{
		{"secret", "set", "CLOUDFLARE_API_TOKEN"},
		{"secret", "set", "CLOUDFLARE_ACCOUNT_ID", "--body", c.Account},
		{"variable", "set", "CLOUDFLARE_API_TOKEN_ID", "--body", token.ID},
	} {
		if err := set(args...); err != nil {
			c.JSON(ctx, nil, "accounts", "tokens", "delete", token.ID, "--force") // don't leave a token nobody can use
			fail(err)
		}
	}
	fmt.Printf("✓ %s's GitHub secrets hold CI's Cloudflare token %q, which may: %s\n", repoName, name, strings.Join(groups, ", "))
}

func holds(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}
