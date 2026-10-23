package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/joeblew999/mise-cloudflare/config"
	"github.com/joeblew999/mise-cloudflare/internal/cf"
	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// buildsReport is go tool cloudflare builds -json: how Workers Builds builds each of the repo's Workers now.
type buildsReport struct {
	Repository string        `json:"repository"`
	Root       string        `json:"root"`
	Build      string        `json:"build"`
	Token      string        `json:"token"`
	Workers    []buildWorker `json:"workers"`
	Removed    bool          `json:"removed,omitempty"`
	// GitHub deletes merged pull requests' branches, and Workers Builds their previews with them.
	DeleteMergedBranches bool `json:"deleteMergedBranches"`
}

// buildWorker is one Worker Workers Builds deploys: production, or a wrangler environment's (staging).
type buildWorker struct {
	Env       string `json:"env,omitempty"` // "" for production
	Worker    string `json:"worker"`
	ScriptTag string `json:"scriptTag"`
	Branch    string `json:"branch"`
	Deploy    string `json:"deploy"`
	Preview   string `json:"preview,omitempty"` // every other branch's preview; production only
}

// builds sets Cloudflare's Workers Builds up for the repo's Workers: a push to the branch deploys production, and every
// other branch gets a preview with its URL on the pull request; with cloudflare.toml's staging, a push to that branch
// deploys staging (the wrangler environment env.staging, its own Worker). Cloudflare does all of it with its own build
// token, so GitHub never holds a Cloudflare credential. It runs on the repo's cf login (Workers Builds' scope, which
// wrangler's login can't have), made the first time with the device flow. With off it removes the setup again.
func (r repo) builds(ctx context.Context, b config.Builds, off, jsonOut bool) {
	if ci() {
		fail(errors.New("Workers Builds is set up once, from a machine or a Claude Code session, not in CI"))
	}
	if !cf.Installed() {
		fail(errors.New("cf isn't installed: run mise run cloudflare:builds, which brings it"))
	}
	r.refuseStrayTokens()
	c := cf.CF{Dir: r.dir, Profile: r.w.Profile}
	who, err := c.Whoami(ctx)
	if err != nil {
		fail(err)
	}
	if !who.Authenticated || !who.HasScopes() {
		must(c.Login(ctx, !wrangler.Device()))
		if who, err = c.Whoami(ctx); err != nil {
			fail(err)
		}
		if !who.Authenticated || !who.HasScopes() {
			fail(fmt.Errorf("the cf login %s doesn't hold %s", c.Profile, strings.Join(cf.Scopes, " ")))
		}
	}
	if len(who.Accounts) != 1 {
		fail(fmt.Errorf("the cf login %s can use %d accounts; log it in again and pick one", c.Profile, len(who.Accounts)))
	}
	c.Account = who.Accounts[0].ID

	// The Workers: production, the Worker CLOUDFLARE_ENV picks (none, usually), and staging's if cloudflare.toml has it.
	env := os.Getenv("CLOUDFLARE_ENV")
	envFlag := func(e string) string {
		if e == "" {
			return ""
		}
		return " --env " + e
	}
	workers := []buildWorker{{Env: env, Worker: r.wc.Name}}
	if b.Staging != "" {
		if !slices.Contains(r.wc.Envs(), "staging") {
			fail(fmt.Errorf("cloudflare.toml's [builds] staging needs the wrangler environment staging in %s (env.staging, with its own bindings)", filepath.Base(r.wc.Path)))
		}
		sc, err := wconfig.ReadEnv(ctx, r.dir, "staging")
		if err != nil {
			fail(err)
		}
		workers = append(workers, buildWorker{Env: "staging", Worker: sc.Name, Branch: b.Staging, Deploy: orDefault(b.Deploy, "npx wrangler deploy") + " --env staging"})
	}
	for i := range workers {
		workers[i].ScriptTag = workerTag(ctx, c, workers[i].Worker, workers[i].Env)
	}
	rep := buildsReport{Workers: workers}

	if off {
		for _, w := range workers {
			if err := c.JSON(ctx, nil, "builds", "workers", "delete", w.ScriptTag, "--force"); err != nil && !cf.NotFound(err) {
				fail(err)
			}
		}
		rep.Removed = true
		if jsonOut {
			emit(rep)
		} else {
			fmt.Println("✓ Workers Builds no longer builds " + workerNames(workers))
		}
		return
	}

	gh, err := githubRepo(ctx, r.dir)
	if err != nil {
		fail(err)
	}
	// The build token Workers Builds deploys with: one Cloudflare made (connecting a repository in the dashboard makes
	// one), cloudflare.toml's by name, else the first. Cloudflare's own covers Workers Scripts, KV, R2 and Workers Routes;
	// a Worker binding anything else (D1, Queues, …) needs that permission added to it in the dashboard.
	tokens, err := buildTokens(ctx, c)
	if err != nil {
		fail(err)
	}
	for _, t := range tokens {
		if b.Token == "" || t.Name == b.Token {
			rep.Token, b.Token = t.UUID, t.Name
			break
		}
	}
	if rep.Token == "" {
		if b.Token != "" {
			fail(fmt.Errorf("the account has no Workers Builds token named %q", b.Token))
		}
		fail(errors.New("the account has no Workers Builds token yet: connect any repository once in Cloudflare's dashboard (a Worker's Settings, Builds), which makes one, then run this again"))
	}
	rep.Repository = gh.Owner.Login + "/" + gh.Name
	rep.Root = orDefault(b.Root, gh.root)
	rep.Build = orDefault(b.Build, installCommand(r.dir))
	prodBranch := gh.DefaultBranch
	if b.Staging != "" {
		prodBranch = "production"
	}
	workers[0].Branch = orDefault(b.Branch, prodBranch)
	workers[0].Deploy = orDefault(b.Deploy, "npx wrangler deploy") + envFlag(env)
	workers[0].Preview = orDefault(b.Preview, "npx wrangler preview") + envFlag(env)
	// Worker Previews (wrangler preview) need a previews block in the config; Cloudflare's build fails without one.
	if _, ok := r.wc.Raw["previews"]; !ok && strings.Contains(workers[0].Preview, "wrangler preview") {
		fail(fmt.Errorf("Worker Previews (wrangler preview) need a previews block in %s: add \"previews\": {} (it can be empty), or set cloudflare.toml's [builds] preview", filepath.Base(r.wc.Path)))
	}
	if len(workers) > 1 && workers[0].Branch == workers[1].Branch {
		fail(fmt.Errorf("production and staging both deploy from %s: give them different branches", workers[0].Branch))
	}
	for _, w := range workers {
		previews := w.Preview != ""
		settings := []string{
			"--git-repository-branch", w.Branch,
			"--git-repository-provider-type", "github",
			"--git-repository-provider-account-id", fmt.Sprint(gh.Owner.ID),
			"--git-repository-provider-account-name", gh.Owner.Login,
			"--git-repository-repo-id", fmt.Sprint(gh.ID),
			"--git-repository-repo-name", gh.Name,
			"--production-settings-build-command", rep.Build,
			"--production-settings-deploy-command", w.Deploy,
			"--production-settings-build-token-uuid", rep.Token,
			"--production-settings-root-directory", rep.Root,
			fmt.Sprintf("--previews-enabled=%v", previews),
			"--previews-base-config-build-command", rep.Build,
			"--previews-base-config-deploy-command", orDefault(w.Preview, "npx wrangler preview"+envFlag(w.Env)),
			"--previews-base-config-build-token-uuid", rep.Token,
			"--previews-base-config-root-directory", rep.Root,
		}
		if c.JSON(ctx, nil, "builds", "workers", "get", w.ScriptTag) == nil {
			err = c.JSON(ctx, nil, append([]string{"builds", "workers", "update", w.ScriptTag}, settings...)...)
		} else {
			err = c.JSON(ctx, nil, append([]string{"builds", "workers", "create", "--script-tag", w.ScriptTag}, settings...)...)
		}
		if err != nil {
			fail(fmt.Errorf("%s: %w\nIf Cloudflare can't see the repository, give Cloudflare's GitHub app access to %s on GitHub (Settings, Applications, Cloudflare Workers and Pages), then run this again", w.Worker, err, rep.Repository))
		}
	}
	// A branch's preview goes when the branch does; GitHub deletes a pull request's branch when it merges, with this on.
	rep.DeleteMergedBranches = deleteMergedBranches(ctx, gh)
	if jsonOut {
		emit(rep)
		return
	}
	fmt.Printf("✓ Workers Builds builds from %s, in %s, installing with %s:\n", rep.Repository, rep.Root, rep.Build)
	for _, w := range workers {
		name := "production"
		if w.Env != "" {
			name = w.Env
		}
		fmt.Printf("  %-10s %s: a push to %s runs %s\n", name, w.Worker, w.Branch, w.Deploy)
	}
	fmt.Printf("  previews   any other branch: %s, its URL on the pull request\n", workers[0].Preview)
	fmt.Printf("  with the account's build token %q; GitHub holds no Cloudflare credential\n", b.Token)
	if rep.DeleteMergedBranches {
		fmt.Printf("  GitHub deletes a pull request's branch when it merges, and Workers Builds its preview with it\n")
	} else {
		fmt.Printf("  · log gh in (gh auth login), or switch on GitHub's \"Automatically delete head branches\" (%s's Settings, General), so merged pull requests' previews go too\n", rep.Repository)
	}
}

func workerNames(ws []buildWorker) string {
	var names []string
	for _, w := range ws {
		names = append(names, w.Worker)
	}
	return strings.Join(names, " and ")
}

// workerTag is a deployed Worker's tag, which Workers Builds attaches to, as Cloudflare has it (cf workers scripts
// search: its id). The search matches names partly, so only the exact name counts.
func workerTag(ctx context.Context, c cf.CF, name, env string) string {
	var found []struct {
		ID   string `json:"id"`
		Name string `json:"script_name"`
	}
	if err := c.JSON(ctx, &found, "workers", "scripts", "search", "--name", name); err != nil {
		fail(err)
	}
	for _, f := range found {
		if f.Name == name && f.ID != "" {
			return f.ID
		}
	}
	deploy := "mise run cloudflare:deploy"
	if env != "" {
		deploy += " -- --env " + env
	}
	fail(fmt.Errorf("Workers Builds attaches to a deployed Worker, and %s isn't deployed: deploy it once first (%s), then run this again", name, deploy))
	return ""
}

// installCommand installs the Worker's dependencies on Workers Builds, by the lockfile committed in dir: Workers Builds
// checks the repository out, so a lockfile only on this machine doesn't count. npm when there's none, or one for a
// package manager Workers Builds doesn't have (aube): package.json pins wrangler exactly either way.
func installCommand(dir string) string {
	tracked := func(f string) bool {
		return exec.Command("git", "-C", dir, "ls-files", "--error-unmatch", f).Run() == nil
	}
	for _, l := range []struct{ file, cmd string }{
		{"pnpm-lock.yaml", "pnpm install --frozen-lockfile"},
		{"bun.lock", "bun install --frozen-lockfile"},
		{"bun.lockb", "bun install --frozen-lockfile"},
		{"yarn.lock", "yarn install --frozen-lockfile"},
		{"package-lock.json", "npm ci"},
	} {
		if tracked(l.file) {
			return l.cmd
		}
	}
	return "npm install"
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// ghRepo is the repo's GitHub repository, as GitHub's API has it, and the repo folder's path in it.
type ghRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"owner"`
	root string
}

var githubURL = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(\.git)?$`)

// githubRepo is the GitHub repository of the git repo dir is in (its origin remote), from GitHub's API, with
// GITHUB_TOKEN when there is one (a private repository needs it).
func githubRepo(ctx context.Context, dir string) (ghRepo, error) {
	var g ghRepo
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return g, errors.New("this repo has no origin remote: Workers Builds builds from a GitHub repository")
	}
	m := githubURL.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return g, fmt.Errorf("origin isn't a GitHub repository (%s): Workers Builds here builds from GitHub", strings.TrimSpace(string(out)))
	}
	top, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return g, err
	}
	realTop, _ := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	realDir, _ := filepath.EvalSymlinks(dir)
	rel, err := filepath.Rel(realTop, realDir)
	if err != nil {
		return g, err
	}
	g.root = "/" // as Cloudflare writes it, the repository's root
	if rel != "." {
		g.root = "/" + filepath.ToSlash(rel)
	}
	for _, token := range []string{cmpOrEnv("GITHUB_TOKEN", "GH_TOKEN"), ""} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+m[1]+"/"+m[2], nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return g, err
		}
		err = json.NewDecoder(resp.Body).Decode(&g)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && err == nil && g.ID != 0 {
			return g, nil
		}
	}
	return g, fmt.Errorf("GitHub doesn't show the repository %s/%s: for a private one, set GITHUB_TOKEN", m[1], m[2])
}

func cmpOrEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

type buildToken struct {
	UUID    string `json:"build_token_uuid"`
	Name    string `json:"build_token_name"`
	TokenID string `json:"cloudflare_token_id"`
}

func buildTokens(ctx context.Context, c cf.CF) ([]buildToken, error) {
	var tokens []buildToken
	err := c.JSON(ctx, &tokens, "builds", "tokens", "list")
	return tokens, err
}

// deleteMergedBranches switches on the repository's "Automatically delete head branches" setting, with GITHUB_TOKEN or
// the gh CLI's login (the cloudflare:builds task brings gh), and reports whether it's on. Without a GitHub login that
// may change it, it's left to the person.
func deleteMergedBranches(ctx context.Context, g ghRepo) bool {
	token := cmpOrEnv("GITHUB_TOKEN", "GH_TOKEN")
	if token == "" {
		if out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output(); err == nil {
			token = strings.TrimSpace(string(out))
		}
	}
	if token == "" {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, "https://api.github.com/repos/"+g.Owner.Login+"/"+g.Name,
		strings.NewReader(`{"delete_branch_on_merge":true}`))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var r struct {
		On bool `json:"delete_branch_on_merge"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&r) == nil && r.On
}
