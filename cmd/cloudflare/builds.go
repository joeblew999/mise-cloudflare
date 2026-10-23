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
	"strings"
	"time"

	"github.com/joeblew999/cloudflare-zero/config"
	"github.com/joeblew999/cloudflare-zero/internal/cf"
	"github.com/joeblew999/cloudflare-zero/internal/wrangler"
)

// buildsReport is go tool cloudflare builds -json: how Workers Builds builds the Worker now.
type buildsReport struct {
	Worker     string `json:"worker"`
	ScriptTag  string `json:"scriptTag"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Root       string `json:"root"`
	Build      string `json:"build"`
	Deploy     string `json:"deploy"`
	Preview    string `json:"preview"`
	Token      string `json:"token"`
	Removed    bool   `json:"removed,omitempty"`
}

// builds sets Cloudflare's Workers Builds up for the repo's Worker: a push to the branch deploys it, every other branch
// gets a preview with its URL on the pull request, and Cloudflare does both with its own build token, so GitHub never
// holds a Cloudflare credential. It runs on the repo's cf login (Workers Builds' scope, which wrangler's login can't
// have), made the first time with the device flow. With off it removes the setup again.
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
	tag := r.workerTag(ctx, c)
	rep := buildsReport{Worker: r.wc.Name, ScriptTag: tag}

	if off {
		if err := c.JSON(ctx, nil, "builds", "workers", "delete", tag); err != nil && !cf.NotFound(err) {
			fail(err)
		}
		rep.Removed = true
		if jsonOut {
			emit(rep)
		} else {
			fmt.Printf("✓ Workers Builds no longer builds %s\n", r.wc.Name)
		}
		return
	}

	gh, err := githubRepo(ctx, r.dir)
	if err != nil {
		fail(err)
	}
	var tokens []struct {
		UUID string `json:"build_token_uuid"`
		Name string `json:"build_token_name"`
	}
	if err := c.JSON(ctx, &tokens, "builds", "tokens", "list"); err != nil {
		fail(err)
	}
	for _, t := range tokens {
		if b.Token == "" || t.Name == b.Token {
			rep.Token = t.UUID
			b.Token = t.Name
			break
		}
	}
	if rep.Token == "" {
		fail(errors.New("the account has no Workers Builds token to deploy with" + map[bool]string{true: " named " + b.Token, false: ""}[b.Token != ""] +
			": connecting any repository once in Cloudflare's dashboard (a Worker's Settings, Builds) makes one, and every repo can use it"))
	}
	env := ""
	if e := os.Getenv("CLOUDFLARE_ENV"); e != "" {
		env = " --env " + e
	}
	rep.Repository = gh.Owner.Login + "/" + gh.Name
	rep.Branch = orDefault(b.Branch, gh.DefaultBranch)
	rep.Root = orDefault(b.Root, gh.root)
	rep.Build = orDefault(b.Build, installCommand(r.dir))
	rep.Deploy = orDefault(b.Deploy, "npx wrangler deploy"+env)
	rep.Preview = orDefault(b.Preview, "npx wrangler versions upload"+env)
	settings := []string{
		"--git-repository-branch", rep.Branch,
		"--git-repository-provider-type", "github",
		"--git-repository-provider-account-id", fmt.Sprint(gh.Owner.ID),
		"--git-repository-provider-account-name", gh.Owner.Login,
		"--git-repository-repo-id", fmt.Sprint(gh.ID),
		"--git-repository-repo-name", gh.Name,
		"--production-settings-build-command", rep.Build,
		"--production-settings-deploy-command", rep.Deploy,
		"--production-settings-build-token-uuid", rep.Token,
		"--production-settings-root-directory", rep.Root,
		"--previews-enabled",
		"--previews-base-config-build-command", rep.Build,
		"--previews-base-config-deploy-command", rep.Preview,
		"--previews-base-config-build-token-uuid", rep.Token,
		"--previews-base-config-root-directory", rep.Root,
	}
	if c.JSON(ctx, nil, "builds", "workers", "get", tag) == nil {
		err = c.JSON(ctx, nil, append([]string{"builds", "workers", "update", tag}, settings...)...)
	} else {
		err = c.JSON(ctx, nil, append([]string{"builds", "workers", "create", "--script-tag", tag}, settings...)...)
	}
	if err != nil {
		fail(fmt.Errorf("%w\nIf Cloudflare can't see the repository, give Cloudflare's GitHub app access to %s on GitHub (Settings, Applications, Cloudflare Workers and Pages), then run this again", err, rep.Repository))
	}
	if jsonOut {
		emit(rep)
		return
	}
	fmt.Printf("✓ Workers Builds builds %s from %s, in %s:\n", r.wc.Name, rep.Repository, rep.Root)
	fmt.Printf("  a push to %s:  %s, then %s\n", rep.Branch, rep.Build, rep.Deploy)
	fmt.Printf("  any other branch: %s, then %s (a preview, its URL on the pull request)\n", rep.Build, rep.Preview)
	fmt.Printf("  with the account's build token %q; GitHub holds no Cloudflare credential\n", b.Token)
}

// workerTag is the deployed Worker's tag, which Workers Builds attaches to, as Cloudflare has it (cf workers scripts
// search: its id). The search matches names partly, so only the exact name counts.
func (r repo) workerTag(ctx context.Context, c cf.CF) string {
	var found []struct {
		ID   string `json:"id"`
		Name string `json:"script_name"`
	}
	if err := c.JSON(ctx, &found, "workers", "scripts", "search", "--name", r.wc.Name); err != nil {
		fail(err)
	}
	for _, f := range found {
		if f.Name == r.wc.Name && f.ID != "" {
			return f.ID
		}
	}
	fail(fmt.Errorf("Workers Builds attaches to a deployed Worker, and %s isn't deployed: deploy it once first (mise run cloudflare:deploy), then run this again", r.wc.Name))
	return ""
}

// installCommand installs the Worker's dependencies on Workers Builds, by its lockfile; npm when there's none, or one
// for a package manager Workers Builds doesn't have (aube): package.json pins wrangler exactly either way.
func installCommand(dir string) string {
	for _, l := range []struct{ file, cmd string }{
		{"pnpm-lock.yaml", "pnpm install --frozen-lockfile"},
		{"bun.lock", "bun install --frozen-lockfile"},
		{"bun.lockb", "bun install --frozen-lockfile"},
		{"yarn.lock", "yarn install --frozen-lockfile"},
		{"package-lock.json", "npm ci"},
	} {
		if _, err := os.Stat(filepath.Join(dir, l.file)); err == nil {
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
