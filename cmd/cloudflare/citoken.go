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
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/joeblew999/mise-cloudflare/config"
)

// ciTokenHelp is how a person makes CI's token: Cloudflare gives no login the right to make API tokens (cf asks for
// account_api_tokens:create; Cloudflare doesn't grant it), so it's made once in the dashboard, from a link that has the
// permissions the Worker's bindings need picked already. An account's token, not a person's: it outlives anyone leaving.
func ciTokenHelp(worker string, kinds []string) string {
	type perm struct {
		Key  string `json:"key"`
		Type string `json:"type"`
	}
	perms := []perm{{"workers_scripts", "edit"}}
	var byHand []string
	for _, k := range kinds {
		for _, p := range config.TokenPermissions[k] {
			key, typ, _ := strings.Cut(p, ":")
			if !slices.Contains(perms, perm{key, typ}) {
				perms = append(perms, perm{key, typ})
			}
		}
		byHand = append(byHand, config.TokenByHand[k]...)
	}
	b, _ := json.Marshal(perms)
	link := "https://dash.cloudflare.com/?to=/:account/api-tokens&permissionGroupKeys=" + url.QueryEscape(string(b)) +
		"&name=" + url.QueryEscape(worker+" GitHub Actions")
	help := "Make it once, in Cloudflare's dashboard: open this link, pick your account, Continue to summary, Create Token.\n" +
		"It has what the Worker needs picked already: Workers Scripts to deploy, and its bindings' own.\n  " + link
	if len(byHand) > 0 {
		help += "\nAdd " + strings.Join(byHand, ", ") + " there too: the link can't pick it."
	}
	return help
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
			fail(errors.New("no CLOUDFLARE_CI_TOKEN and no terminal to type it in.\n" + ciTokenHelp(r.wc.Name, r.wc.Kinds)))
		}
		defer tty.Close()
		fmt.Fprintln(os.Stderr, ciTokenHelp(r.wc.Name, r.wc.Kinds))
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
		fail(fmt.Errorf("Cloudflare refused that token for the account's Workers (HTTP %d).\n%s", resp.StatusCode, ciTokenHelp(r.wc.Name, r.wc.Kinds)))
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
