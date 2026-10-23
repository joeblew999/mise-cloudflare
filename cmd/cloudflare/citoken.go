package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// ciTokenHelp is how a person makes CI's token: Cloudflare gives no login the right to make API tokens (cf asks for
// account_api_tokens:create; Cloudflare doesn't grant it), so it's made once in the dashboard, from a link with every
// Workers permission Cloudflare's token links can pick already picked. An account's token, not a person's: it outlives
// anyone leaving.
func ciTokenHelp(worker string) string {
	b, _ := json.Marshal(ciTokenPermissions)
	link := "https://dash.cloudflare.com/?to=/:account/api-tokens&permissionGroupKeys=" + url.QueryEscape(string(b)) +
		"&name=" + url.QueryEscape(worker+" GitHub Actions")
	return "Make it once, in Cloudflare's dashboard: open this link, pick your account, Continue to summary, Create Token.\n" +
		"It picks Workers Scripts, KV, R2, D1, Queues, Workers Routes and Zone Read; a Worker using more (Secrets Store,\n" +
		"Pipelines, …) needs that permission added there too.\n  " + link
}

// ciTokenPermissions are the permissions CI's token link picks, by their keys in Cloudflare's token links
// (fundamentals/api/how-to/account-owned-token-template): every Workers one those links have a key for.
var ciTokenPermissions = []struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}{
	{"workers_scripts", "edit"}, {"workers_kv_storage", "edit"}, {"workers_r2", "edit"}, {"d1", "edit"},
	{"queues", "edit"}, {"workers_routes", "edit"}, {"zone", "read"},
}

// ciToken stores the Cloudflare API token GitHub Actions deploys with (cloudflare.toml's [workflow] deploy) as the
// repository's GitHub secrets, with gh: CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID. The token comes from
// CLOUDFLARE_CI_TOKEN, else it's typed in, hidden; it's checked against Cloudflare first, goes to gh on stdin, and is
// never printed or written into the repo. The one Cloudflare token in the whole setup: GitHub has no one to click Allow.
// With off it deletes the secrets (the token itself is revoked in the dashboard).
func (r repo) ciToken(ctx context.Context, off bool) {
	if ci() {
		fail(errors.New("CI's token is stored once, from a machine or a Claude Code session, not in CI"))
	}
	if _, err := exec.LookPath("gh"); err != nil {
		fail(errors.New("gh isn't installed: run mise run cloudflare:ci-token, which brings it"))
	}
	if exec.CommandContext(ctx, "gh", "auth", "status").Run() != nil {
		fail(errors.New("gh isn't logged in: run gh auth login (it gives a code to approve on GitHub), then this again"))
	}
	gh, err := githubRepo(ctx, r.dir)
	if err != nil {
		fail(err)
	}
	repoName := gh.Owner.Login + "/" + gh.Name
	if off {
		for _, s := range []string{"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID"} {
			exec.CommandContext(ctx, "gh", "secret", "delete", s, "--repo", repoName).Run()
		}
		fmt.Printf("✓ %s's Cloudflare secrets are deleted; revoke the token itself at https://dash.cloudflare.com/profile/api-tokens\n", repoName)
		return
	}
	who := r.login(ctx) // the account the repo's own login deploys to
	if len(who.Accounts) != 1 {
		fail(fmt.Errorf("the login %s can use %d accounts; log it in again and pick one", r.w.Profile, len(who.Accounts)))
	}
	account := who.Accounts[0].ID
	token := os.Getenv("CLOUDFLARE_CI_TOKEN")
	if token == "" {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			fail(errors.New("no CLOUDFLARE_CI_TOKEN and no terminal to type it in.\n" + ciTokenHelp(r.wc.Name)))
		}
		defer tty.Close()
		fmt.Fprintln(os.Stderr, ciTokenHelp(r.wc.Name))
		fmt.Fprint(os.Stderr, "Paste CI's token (hidden): ")
		b, err := term.ReadPassword(int(tty.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil || len(b) == 0 {
			fail(errors.New("no token"))
		}
		token = strings.TrimSpace(string(b))
	}
	// It must be able to see the account's Workers, as deploying needs.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.cloudflare.com/client/v4/accounts/"+account+"/workers/scripts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		fail(err)
	}
	var check struct{ Success bool }
	json.NewDecoder(resp.Body).Decode(&check)
	resp.Body.Close()
	if !check.Success {
		fail(fmt.Errorf("Cloudflare refused that token for the account's Workers (HTTP %d).\n%s", resp.StatusCode, ciTokenHelp(r.wc.Name)))
	}
	for _, s := range []struct{ name, value string }{{"CLOUDFLARE_API_TOKEN", token}, {"CLOUDFLARE_ACCOUNT_ID", account}} {
		cmd := exec.CommandContext(ctx, "gh", "secret", "set", s.name, "--repo", repoName)
		cmd.Stdin = strings.NewReader(s.value)
		if out, err := cmd.CombinedOutput(); err != nil {
			fail(fmt.Errorf("gh secret set %s: %w: %s", s.name, err, out))
		}
	}
	fmt.Printf("✓ %s's GitHub secrets hold CI's Cloudflare token and account; GitHub Actions deploys with them\n", repoName)
}
