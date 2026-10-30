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

// statusReport is mise-cloudflare -json: where the repo is, and what to run next.
type statusReport struct {
	Worker      string             `json:"worker"`
	Profile     string             `json:"profile"`
	LoggedIn    bool               `json:"loggedIn"`
	Accounts    []wrangler.Account `json:"accounts"`
	Scopes      []string           `json:"scopes"` // what the login holds
	Needs       []string           `json:"needs"`  // what it must hold: wrangler's own login's
	Deployed    bool               `json:"deployed"`
	Secrets     []string           `json:"secretsToSet"` // the config's secrets.required not yet on Cloudflare
	URL         string             `json:"url,omitempty"`
	GitHub      string             `json:"github"` // "Workers Builds" when it deploys every push, else ""
	StrayTokens []string           `json:"strayTokens"`
	Problems    []string           `json:"problems"` // cloudflare check's: a deploy would fail on them, or leak
	Next        string             `json:"next"`
}

// show is mise-cloudflare with no command or show (mise run cloudflare, cloudflare:05-show): the steps in order, which are done for this repo, and the
// next one. It never logs in.
func (r repo) show(ctx context.Context, jsonOut bool) {
	rep := statusReport{Worker: r.wc.Name, Profile: r.w.Profile, Accounts: []wrangler.Account{}, Scopes: []string{},
		Needs: r.wanted(ctx), StrayTokens: tokenEnvFiles(r.dir), Problems: []string{}}
	if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
		rep.StrayTokens = append([]string{"this shell"}, rep.StrayTokens...)
	}
	// cloudflare check's problems come before deploying: it fails on them, or leaks the login or a secret.
	if problems, err := rules.Check(r.dir, r.wc); err == nil {
		rep.Problems = append(append([]string{}, problems...), r.stale(ctx)...)
	}
	rep.LoggedIn = r.w.LoggedIn(ctx)
	if rep.LoggedIn && len(rep.StrayTokens) == 0 {
		must(r.w.Activate(ctx))
		if who, err := r.w.Whoami(ctx); err == nil {
			rep.Accounts, rep.Scopes = who.Accounts, who.Scopes
		}
		rep.Deployed = r.deployed(ctx)
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
	if rep.Deployed && r.buildsConnected(ctx) {
		rep.GitHub = "Workers Builds"
	}
	// What GitHub deploying needs from the wrangler config first (a previews block with the Worker's bindings), and,
	// once Workers Builds deploys, any KV, D1 or R2 added since the previews' own were made.
	var previewProblems, previewsMissing, previewSecrets []string
	if rep.Deployed && rep.GitHub == "" {
		previewProblems = rules.Previews(r.wc, true)
	}
	if rep.GitHub != "" && len(rep.Accounts) == 1 {
		if token, err := r.w.Token(ctx); err == nil {
			previewsMissing, previewSecrets = r.previewsMissing(ctx, token, rep.Accounts[0].ID)
		}
	}
	switch {
	case len(rep.StrayTokens) > 0:
		rep.Next = "remove the CLOUDFLARE_API_TOKEN in " + strings.Join(rep.StrayTokens, " and ") + ": it would override the repo's login"
	case len(rep.Problems) > 0:
		rep.Next = "fix what mise run cloudflare:04-check lists, first: " + rep.Problems[0]
	case len(rep.Secrets) > 0:
		rep.Next = "mise run cloudflare:07-set-secrets (it asks for " + strings.Join(rep.Secrets, ", ") + ")"
	case !rep.Deployed:
		rep.Next = "mise run cloudflare:08-deploy"
	case len(previewProblems) > 0:
		rep.Next = previewProblems[0] + "; then mise run cloudflare:09-connect-github"
	case rep.GitHub == "":
		rep.Next = "mise run cloudflare:09-connect-github"
	case len(previewSecrets) > 0:
		rep.Next = "mise run cloudflare:10-set-preview-secrets (it asks for " + strings.Join(previewSecrets, ", ") + ": previews don't get the Worker's secrets)"
	case len(previewsMissing) > 0:
		rep.Next = "mise run cloudflare:09-connect-github: previews don't have their own " + strings.Join(previewsMissing, ", ") + " yet"
	default:
		rep.Next = "push a branch and open a pull request: its preview's link comes on the pull request"
	}
	if jsonOut {
		emit(rep)
		return
	}
	mark := map[bool]string{true: "✓", false: "·"}
	fmt.Printf("%s\n", rep.Worker)
	if len(rep.Problems) > 0 {
		fmt.Printf("  ✗ the repo has %d problem(s) to fix first (mise run cloudflare:04-check lists them)\n", len(rep.Problems))
	}
	fmt.Printf("  ✓ 06. Run it locally     mise run cloudflare:06-serve\n")
	fmt.Printf("  %s 08. Deploy it          mise run cloudflare:08-deploy   %s\n", mark[rep.Deployed], cmp.Or(rep.URL, map[bool]string{true: "deployed", false: "not deployed yet"}[rep.Deployed]))
	if len(rep.Secrets) > 0 {
		fmt.Printf("    first, its secrets: %s (mise run cloudflare:07-set-secrets)\n", strings.Join(rep.Secrets, ", "))
	}
	if rep.GitHub != "" {
		fmt.Printf("  ✓ 09. Deploy every push  %s deploys it: a preview for every pull request\n", rep.GitHub)
	} else {
		fmt.Printf("  · 09. Deploy every push  mise run cloudflare:09-connect-github   a preview for every pull request, then staging and production\n")
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

// deployed reports whether the Worker's code is on Cloudflare: its current version has a compatibility date, as every
// deploy's has. wrangler secret put on a Worker that isn't there yet makes a placeholder with none (createDraftWorker
// in wrangler 4.151.3), which isn't the Worker deployed.
func (r repo) deployed(ctx context.Context) bool {
	var d struct {
		Versions []struct {
			ID string `json:"version_id"`
		} `json:"versions"`
	}
	if r.wranglerJSON(ctx, &d, "deployments", "status", "--json") != nil || len(d.Versions) == 0 {
		return false
	}
	var v struct {
		Resources struct {
			Runtime struct {
				Date string `json:"compatibility_date"`
			} `json:"script_runtime"`
		} `json:"resources"`
	}
	return r.wranglerJSON(ctx, &v, "versions", "view", d.Versions[0].ID, "--json") == nil && v.Resources.Runtime.Date != ""
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
