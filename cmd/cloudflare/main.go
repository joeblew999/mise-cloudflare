// Command cloudflare gives a repo its own Cloudflare login and runs the repo's own wrangler on it: no API tokens to
// make, nothing machine-wide, the same on a Mac and in a Claude Code cloud session.
//
//	cloudflare init              # set the repo up, or update it after upgrading: cloudflare.toml (if it has none), the
//	                             # mise tasks, the agent skill, the GitHub Actions workflow and the Claude Code hook
//	cloudflare dev [<args>]      # wrangler dev: the Worker locally, with local D1, KV and R2; no login
//	cloudflare login             # the repo's login: once, then again only when what it needs changes
//	cloudflare wrangler <args>   # the repo's wrangler on that login (deploy, delete, d1, secret, …)
//	cloudflare status            # the repo's login: its account, the scopes it holds and needs; stray tokens
//	cloudflare check             # the repo's rules, and whether init's files are current
//	cloudflare builds            # Workers Builds: a push to the branch deploys, every other branch gets a preview
//	cloudflare previews          # the previews' own KV, D1 and R2 (builds and the deploy workflow run it)
//	cloudflare ci-token          # GitHub Actions' token, for cloudflare.toml's [workflow] deploy
//	cloudflare logout            # log the repo's logins out on Cloudflare and delete them
//	cloudflare version           # this build's version
//
// In a Go repo it's go tool cloudflare, at go.mod's version; in any other, the released binary (mise:
// github:joeblew999/mise-cloudflare), which its tasks pin.
//
// The login is a wrangler auth profile (cloudflare.toml's profile, default the Worker's name), passed to every wrangler
// command with --profile. It asks for wrangler's own login's scopes, and logs in in a browser on this machine, or with a
// link to open on any device (the OAuth device flow) in a Claude Code cloud session or with CLOUDFLARE_LOGIN=device. In
// CI (CI=true) there's no one to log in: a CLOUDFLARE_API_TOKEN secret is CI's login. Deploying and deleting are
// wrangler's own job.
//
// Flags go before the command; for wrangler, everything after it is wrangler's. -json prints one JSON document on
// stdout instead of text. Exit codes: 0 ok, 1 problems, 2 usage.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/config"
	"github.com/joeblew999/mise-cloudflare/internal/cf"
	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
	"github.com/joeblew999/mise-cloudflare/mise"
	"github.com/joeblew999/mise-cloudflare/skills"
)

func main() {
	cfg, err := config.Find(".")
	if err != nil {
		fail(err)
	}
	profile := flag.String("profile", cfg.Profile, "the repo's login, a wrangler auth profile (default the Worker's name)")
	jsonOut := flag.Bool("json", false, "print one JSON document on stdout instead of text")
	off := flag.Bool("off", false, "builds: remove the Workers Builds setup; ci-token: delete CI's token and its secrets; previews: delete the previews' own KV, D1 and R2")
	showVersion := flag.Bool("version", false, "print this build's version")
	branch := flag.String("branch", "", "builds: the branch whose pushes deploy production (default cloudflare.toml's, else the repository's default, or production with staging)")
	staging := flag.String("staging", "", "builds: the branch whose pushes deploy staging, the wrangler environment env.staging (default cloudflare.toml's)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cloudflare [-json] [-profile name] init|login|status|check|logout|version, cloudflare [-off] [-branch b] [-staging b] builds|previews|ci-token, or cloudflare dev|wrangler <args>")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if *showVersion {
		args = []string{"version"}
	}
	if len(args) == 0 || (args[0] != "wrangler" && args[0] != "dev" && len(args) != 1) {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	switch args[0] {
	case "version":
		fmt.Println(buildVersion())
		return
	case "init":
		initRepo(ctx, dir, cfg, *profile)
		return
	}
	// No person in CI to click Allow, so no login: a CLOUDFLARE_API_TOKEN given to CI as a secret is CI's login. Said
	// first, before anything else is needed.
	if ci() && (args[0] == "login" || args[0] == "wrangler") && os.Getenv("CLOUDFLARE_API_TOKEN") == "" {
		fail(errors.New("in CI there's no one to log in: give CI a CLOUDFLARE_API_TOKEN (and CLOUDFLARE_ACCOUNT_ID) as secrets, or deploy with Cloudflare's own Workers Builds"))
	}
	r := openRepo(ctx, dir, cfg, *profile)
	if args[0] == "dev" {
		// Local development needs no login: the repo's own wrangler, on local D1, KV and R2.
		if err := wrangler.Interactive(r.w.Plain(ctx, append([]string{"dev"}, args[1:]...)...)); err != nil {
			os.Exit(exitCode(err))
		}
		return
	}
	if ci() && (args[0] == "login" || args[0] == "wrangler") {
		if args[0] == "login" {
			fmt.Println("✓ CI: wrangler uses CLOUDFLARE_API_TOKEN")
			return
		}
		if err := wrangler.Interactive(r.w.Plain(ctx, args[1:]...)); err != nil {
			os.Exit(exitCode(err))
		}
		return
	}
	switch args[0] {
	case "login":
		who := r.login(ctx)
		if *jsonOut {
			emit(who)
		} else {
			fmt.Printf("✓ %s: %s, scopes %s\n", r.w.Profile, accountNames(who), strings.Join(who.Scopes, " "))
		}
	case "wrangler":
		r.login(ctx)
		if err := wrangler.Interactive(r.w.Cmd(ctx, args[1:]...)); err != nil {
			os.Exit(exitCode(err))
		}
	case "ci-token":
		r.ciToken(ctx, *off)
	case "previews":
		res := r.previews(ctx, *off, *jsonOut)
		if *jsonOut {
			emit(struct {
				Worker   string            `json:"worker"`
				Previews []previewResource `json:"previews"`
			}{r.wc.Name, res})
		}
	case "builds":
		b := cfg.Builds
		if *branch != "" {
			b.Branch = *branch
		}
		if *staging != "" {
			b.Staging = *staging
		}
		r.builds(ctx, b, *off, *jsonOut)
	case "status":
		r.status(ctx, *jsonOut)
	case "check":
		r.check(ctx, *jsonOut)
	case "logout":
		if r.w.LoggedIn(ctx) {
			r.refuseStrayTokens()
			must(r.w.Delete(ctx))
		}
		// The cf login, for Workers Builds, if this machine has one.
		if cf.Installed() {
			c := cf.CF{Dir: r.dir, Profile: r.w.Profile}
			if who, err := c.Whoami(ctx); err == nil && who.Authenticated {
				must(c.Delete(ctx))
			}
		}
		fmt.Printf("✓ %s is logged out and deleted\n", r.w.Profile)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// repo is a repo with its wrangler installed: its config as wrangler reads it, and wrangler on its login.
type repo struct {
	dir      string
	scopes   []string
	workflow config.Workflow
	wc       wconfig.Config
	w        wrangler.W
}

func openRepo(ctx context.Context, dir string, cfg config.Config, profile string) repo {
	w := wrangler.W{Dir: dir}
	if !w.Installed() {
		fail(errors.New("wrangler isn't installed here: install the repo's dependencies (its package.json pins wrangler)"))
	}
	wc, err := wconfig.Read(ctx, dir)
	if err != nil {
		fail(err)
	}
	w.Profile = cmp.Or(profile, wc.Name)
	return repo{dir: dir, scopes: cfg.Scopes, workflow: cfg.Workflow, wc: wc, w: w}
}

// wanted is the scopes the repo's login holds: wrangler's own login's (wrangler keeps them, for everything it does),
// unless cloudflare.toml's scopes says exactly which instead.
func (r repo) wanted(ctx context.Context) []string {
	if len(r.scopes) > 0 {
		return r.scopes
	}
	scopes, err := r.w.Scopes(ctx)
	if err != nil {
		fail(err)
	}
	return scopes
}

// login logs the repo in if it isn't yet, and again whenever the login's scopes differ from the repo's (a scope added
// or removed), then checks they match. It refuses a stray token first: one would override every login.
func (r repo) login(ctx context.Context) wrangler.Who {
	r.refuseStrayTokens()
	want := r.wanted(ctx)
	device := wrangler.Device()
	if !r.w.LoggedIn(ctx) {
		must(r.w.Login(ctx, want, device))
	} else {
		must(r.w.Activate(ctx))
	}
	who, err := r.w.Whoami(ctx)
	if err != nil {
		fail(err)
	}
	add, drop := wrangler.Missing(want, who.Scopes), wrangler.Extra(want, who.Scopes)
	if len(add) == 0 && len(drop) == 0 {
		return who
	}
	msg := "This repo's scopes changed."
	if len(add) > 0 {
		msg += " Needs: " + strings.Join(add, " ") + "."
	}
	if len(drop) > 0 {
		msg += " No longer needs: " + strings.Join(drop, " ") + "."
	}
	fmt.Fprintln(os.Stderr, msg)
	must(r.w.Login(ctx, want, device))
	if who, err = r.w.Whoami(ctx); err != nil {
		fail(err)
	}
	if len(wrangler.Missing(want, who.Scopes))+len(wrangler.Extra(want, who.Scopes)) > 0 {
		fail(fmt.Errorf("the new login's scopes (%s) still don't match the repo's (%s)", strings.Join(who.Scopes, " "), strings.Join(want, " ")))
	}
	return who
}

// ci reports whether this runs in CI, where CI=true (GitHub Actions and every other CI set it).
func ci() bool { return os.Getenv("CI") != "" && os.Getenv("CI") != "false" && os.Getenv("CI") != "0" }

// refuseStrayTokens: a CLOUDFLARE_API_TOKEN beats every login, so outside CI one in the shell or in a .env is refused.
func (r repo) refuseStrayTokens() {
	if ci() {
		return
	}
	if os.Getenv("CLOUDFLARE_API_TOKEN") != "" {
		fail(fmt.Errorf("refusing: CLOUDFLARE_API_TOKEN is set in this shell, and it would override this repo's own login (%s). Unset it and run again", r.w.Profile))
	}
	if files := tokenEnvFiles(r.dir); len(files) > 0 {
		fail(fmt.Errorf("refusing: %s sets CLOUDFLARE_API_TOKEN, and wrangler would use it instead of this repo's own login (%s). Remove it and run again", strings.Join(files, " "), r.w.Profile))
	}
}

var tokenLine = regexp.MustCompile(`(?m)^CLOUDFLARE_API_TOKEN=.`)

func tokenEnvFiles(dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, ".env*"))
	out := []string{}
	for _, m := range matches {
		if b, err := os.ReadFile(m); err == nil && tokenLine.Match(b) {
			out = append(out, filepath.Base(m))
		}
	}
	return out
}

func (r repo) check(ctx context.Context, jsonOut bool) {
	problems, err := rules.Check(r.dir, r.wc, r.workflow.Deploy) // the deploy workflow runs Worker Previews
	if err != nil {
		fail(err)
	}
	problems = append(problems, r.stale(ctx)...)
	if jsonOut {
		emit(struct {
			OK       bool     `json:"ok"`
			Problems []string `json:"problems"`
		}{len(problems) == 0, append([]string{}, problems...)})
	} else {
		if len(problems) == 0 {
			fmt.Println("rules: pass")
		}
		for _, p := range problems {
			fmt.Println("FAIL " + p)
		}
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
}

// statusReport is go tool cloudflare status -json. It never logs in.
type statusReport struct {
	Profile      string             `json:"profile"`
	LoggedIn     bool               `json:"loggedIn"`
	Accounts     []wrangler.Account `json:"accounts"`
	Scopes       []string           `json:"scopes"` // what the login holds
	Needs        []string           `json:"needs"`  // what it must hold: wrangler's own login's, or cloudflare.toml's
	TokenInShell bool               `json:"tokenInShell"`
	TokenInEnv   []string           `json:"tokenInEnvFiles"`
}

func (r repo) status(ctx context.Context, jsonOut bool) {
	rep := statusReport{
		Profile: r.w.Profile, LoggedIn: r.w.LoggedIn(ctx), Accounts: []wrangler.Account{}, Scopes: []string{}, Needs: r.wanted(ctx),
		TokenInShell: os.Getenv("CLOUDFLARE_API_TOKEN") != "", TokenInEnv: tokenEnvFiles(r.dir),
	}
	if rep.LoggedIn && !rep.TokenInShell && len(rep.TokenInEnv) == 0 {
		must(r.w.Activate(ctx))
		who, err := r.w.Whoami(ctx)
		if err != nil {
			fail(err)
		}
		rep.Accounts, rep.Scopes = who.Accounts, who.Scopes
	}
	if jsonOut {
		emit(rep)
		return
	}
	yes := map[bool]string{true: "logged in", false: "not logged in"}
	fmt.Printf("this repo's login    %s: %s\n", rep.Profile, yes[rep.LoggedIn])
	for _, a := range rep.Accounts {
		fmt.Printf("account              %s https://dash.cloudflare.com/%s\n", a.Name, a.ID)
	}
	if len(rep.Scopes) > 0 {
		fmt.Printf("scopes               %s\n", strings.Join(rep.Scopes, " "))
	}
	source := "wrangler's own login's"
	if len(r.scopes) > 0 {
		source = "cloudflare.toml's"
	}
	fmt.Printf("needs                %s (%s)\n", strings.Join(rep.Needs, " "), source)
	fmt.Printf("CLOUDFLARE_API_TOKEN %s\n", map[bool]string{true: "SET in this shell (would override every login)", false: "not set"}[rep.TokenInShell])
	fmt.Printf(".env with a token    %s\n", cmp.Or(strings.Join(rep.TokenInEnv, " "), "none"))
	fmt.Println("(wrangler auth list shows this machine's other logins, and the folders each is bound to)")
}

func accountNames(who wrangler.Who) string {
	var names []string
	for _, a := range who.Accounts {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

func exitCode(err error) int {
	var e *exec.ExitError
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return 1
}

func emit(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fail(err)
	}
}

func must(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "cloudflare:", err)
	os.Exit(1)
}

// tasksFile is where init writes the mise tasks: mise finds a TOML file in .mise/tasks/ by itself.
var tasksFile = filepath.Join(".mise", "tasks", "mise-cloudflare.toml")

// initRepo sets the repo up, or updates it after an upgrade: cloudflare.toml if it has none (yours to edit after), then
// the files mise-cloudflare owns, written for this version: the mise tasks, the agent skill, the GitHub Actions
// workflow and the Claude Code hook.
func initRepo(ctx context.Context, dir string, cfg config.Config, profile string) {
	switch {
	case cfg.Path != "":
		fmt.Println("✓ " + cfg.Path + " is there: yours to edit")
	case profile != "":
		writeFile(config.File, []byte(config.TemplateFor(profile)))
	default:
		writeFile(config.File, []byte(config.Template))
	}
	b, err := tasksFor(dir)
	if err != nil {
		fail(err)
	}
	writeFile(tasksFile, b)
	// The tasks' old name, before 0.1.3: one mise-cloudflare wrote goes.
	old := filepath.Join(".mise", "tasks", "cloudflare.toml")
	if b, err := os.ReadFile(old); err == nil && bytes.HasPrefix(b, []byte("# mise-cloudflare's mise tasks")) {
		must(os.Remove(old))
		fmt.Println("removed", old, "(now "+tasksFile+")")
	}
	writeSkill(filepath.Join(".claude", "skills"))
	if path, w, err := workflowFor(ctx, dir, cfg.Workflow); err == nil {
		writeFile(path, w)
	}
	claude(ctx, dir)
}

// writeSkill installs the agent skill into dir (a repo's .claude/skills).
func writeSkill(dir string) {
	err := fs.WalkDir(skills.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skills.FS.ReadFile(path)
		if err == nil {
			writeFile(filepath.Join(dir, path), b)
		}
		return err
	})
	if err != nil {
		fail(err)
	}
}

func writeFile(path string, b []byte) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		fmt.Printf("✓ %s is up to date\n", path)
		return
	}
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, b, 0o644))
	fmt.Println("wrote", path)
}

// stale names the files init wrote into the repo that this version would write differently; init (cloudflare:update)
// writes them again.
func (r repo) stale(ctx context.Context) []string {
	var out []string
	differs := func(path string, want []byte) bool {
		b, err := os.ReadFile(path)
		return err == nil && !bytes.Equal(b, want)
	}
	if want, err := tasksFor(r.dir); err == nil && differs(filepath.Join(r.dir, tasksFile), want) {
		out = append(out, tasksFile+" is out of date for mise-cloudflare "+buildVersion()+": run mise run cloudflare:update")
	}
	if want, err := skills.FS.ReadFile("cloudflare/SKILL.md"); err == nil && differs(filepath.Join(r.dir, ".claude", "skills", "cloudflare", "SKILL.md"), want) {
		out = append(out, ".claude/skills/cloudflare/SKILL.md is out of date: run mise run cloudflare:update")
	}
	if top, _, err := gitTop(ctx, r.dir); err == nil && differs(filepath.Join(top, ".claude", "hooks", hookFile), mise.SessionStart) {
		out = append(out, ".claude/hooks/"+hookFile+" is out of date: run mise run cloudflare:update")
	}
	if path, want, err := workflowFor(ctx, r.dir, r.workflow); err == nil && differs(path, want) {
		rel, _ := filepath.Rel(r.dir, path)
		out = append(out, rel+" is out of date: run mise run cloudflare:update")
	}
	return out
}
