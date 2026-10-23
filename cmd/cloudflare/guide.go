package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/cf"
	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// guideReport is cloudflare -json: where the repo is, and what to run next.
type guideReport struct {
	Worker      string             `json:"worker"`
	Profile     string             `json:"profile"`
	LoggedIn    bool               `json:"loggedIn"`
	Accounts    []wrangler.Account `json:"accounts"`
	Scopes      []string           `json:"scopes"` // what the login holds
	Needs       []string           `json:"needs"`  // what it must hold: wrangler's own login's
	Deployed    bool               `json:"deployed"`
	Secrets     []string           `json:"secretsToSet"` // the config's secrets.required not yet on Cloudflare
	URL         string             `json:"url,omitempty"`
	GitHub      string             `json:"github"` // who deploys every push: "Workers Builds", "GitHub Actions", or ""
	StrayTokens []string           `json:"strayTokens"`
	Next        string             `json:"next"`
}

// guide is cloudflare with no command (mise run cloudflare): the steps in order, which are done for this repo, and the
// next one. It never logs in.
func (r repo) guide(ctx context.Context, jsonOut bool) {
	rep := guideReport{Worker: r.wc.Name, Profile: r.w.Profile, Accounts: []wrangler.Account{}, Scopes: []string{},
		Needs: r.wanted(ctx), StrayTokens: tokenEnvFiles(r.dir)}
	if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
		rep.StrayTokens = append([]string{"this shell"}, rep.StrayTokens...)
	}
	rep.LoggedIn = r.w.LoggedIn(ctx)
	if rep.LoggedIn && len(rep.StrayTokens) == 0 {
		must(r.w.Activate(ctx))
		if who, err := r.w.Whoami(ctx); err == nil {
			rep.Accounts, rep.Scopes = who.Accounts, who.Scopes
		}
		_, err := wrangler.Output(r.w.Cmd(ctx, "deployments", "list", "--json"))
		rep.Deployed = err == nil
		if rep.Deployed {
			rep.URL = lastDeployURL(r.dir)
		}
	}
	// The secrets the Worker requires that Cloudflare doesn't have yet: a deploy refuses until they're set. wrangler
	// secret put sets one, and makes the Worker if it isn't there yet.
	if len(r.wc.Secrets) > 0 && len(rep.StrayTokens) == 0 {
		var have []struct{ Name string }
		if rep.LoggedIn {
			r.wranglerJSON(ctx, &have, "secret", "list")
		}
		for _, s := range r.wc.Secrets {
			if !slices.ContainsFunc(have, func(h struct{ Name string }) bool { return h.Name == s }) {
				rep.Secrets = append(rep.Secrets, s)
			}
		}
	}
	switch {
	case r.viaActions:
		rep.GitHub = "GitHub Actions"
	case rep.Deployed && r.buildsConnected(ctx):
		rep.GitHub = "Workers Builds"
	}
	// What GitHub deploying needs from the wrangler config first (a previews block with the Worker's bindings), and,
	// once Workers Builds deploys, any KV, D1 or R2 added since the previews' own were made.
	var previewProblems, previewsMissing []string
	if rep.Deployed && rep.GitHub == "" {
		previewProblems = rules.Previews(r.wc, true)
	}
	if rep.GitHub == "Workers Builds" && len(rep.Accounts) == 1 {
		if token, err := r.w.Token(ctx); err == nil {
			previewsMissing = r.previewsMissing(ctx, token, rep.Accounts[0].ID)
		}
	}
	switch {
	case len(rep.StrayTokens) > 0:
		rep.Next = "remove the CLOUDFLARE_API_TOKEN in " + strings.Join(rep.StrayTokens, " and ") + ": it would override the repo's login"
	case len(rep.Secrets) > 0:
		rep.Next = "mise run cloudflare wrangler secret put " + rep.Secrets[0] + " (it asks for the value)"
	case !rep.Deployed:
		rep.Next = "mise run cloudflare:deploy"
	case len(previewProblems) > 0:
		rep.Next = previewProblems[0] + "; then mise run cloudflare:github"
	case rep.GitHub == "":
		rep.Next = "mise run cloudflare:github"
	case len(previewsMissing) > 0:
		rep.Next = "mise run cloudflare:github: previews don't have their own " + strings.Join(previewsMissing, ", ") + " yet"
	default:
		rep.Next = "push a branch and open a pull request: its preview's link comes on the pull request"
	}
	if jsonOut {
		emit(rep)
		return
	}
	mark := map[bool]string{true: "✓", false: "·"}
	fmt.Printf("%s\n", rep.Worker)
	fmt.Printf("  ✓ 1. Run it locally     mise run cloudflare:dev\n")
	fmt.Printf("  %s 2. Deploy it          mise run cloudflare:deploy   %s\n", mark[rep.Deployed], cmp.Or(rep.URL, map[bool]string{true: "deployed", false: "not deployed yet"}[rep.Deployed]))
	if len(rep.Secrets) > 0 {
		fmt.Printf("    first, its secrets: %s (mise run cloudflare wrangler secret put <name>)\n", strings.Join(rep.Secrets, ", "))
	}
	if rep.GitHub != "" {
		fmt.Printf("  ✓ 3. Deploy every push  %s deploys it: a preview for every pull request\n", rep.GitHub)
	} else {
		fmt.Printf("  · 3. Deploy every push  mise run cloudflare:github   a preview for every pull request, then staging and production\n")
	}
	switch {
	case !rep.LoggedIn:
		fmt.Printf("  login: none yet; deploying makes it (you click Allow)\n")
	case len(rep.Accounts) > 0:
		more := ""
		if len(wrangler.Missing(rep.Needs, rep.Scopes)) > 0 {
			more = "; it needs more permissions, so the next deploy asks you to Allow"
		}
		fmt.Printf("  login: %s, on %s%s\n", rep.Profile, accountNames(wrangler.Who{Accounts: rep.Accounts}), more)
	}
	fmt.Printf("next: %s\n", rep.Next)
}

// lastDeployURL is the workers.dev URL of the last deploy wrangler wrote to .wrangler/output.json (cloudflare deploy
// has it write there), or "".
func lastDeployURL(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, ".wrangler", "output.json"))
	urls := regexp.MustCompile(`https://[a-z0-9.-]+\.workers\.dev`).FindAll(b, -1)
	if len(urls) == 0 {
		return ""
	}
	return string(urls[len(urls)-1])
}

// buildsConnected reports whether Workers Builds builds the Worker, if this machine has cf logged in for the repo.
func (r repo) buildsConnected(ctx context.Context) bool {
	if !cf.Installed() {
		return false
	}
	c := cf.CF{Dir: r.dir, Profile: r.w.Profile}
	who, err := c.Whoami(ctx)
	if err != nil || !who.Authenticated || len(who.Accounts) != 1 {
		return false
	}
	c.Account = who.Accounts[0].ID
	tag := findWorker(ctx, c, r.wc.Name)
	return tag != "" && c.JSON(ctx, nil, "builds", "workers", "get", tag) == nil
}
