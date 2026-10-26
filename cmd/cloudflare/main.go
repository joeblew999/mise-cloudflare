// Command cloudflare gives a repo its own Cloudflare login and runs the repo's own wrangler on it: no API tokens to
// make, nothing machine-wide, the same on a Mac and in a Claude Code cloud session.
//
//	cloudflare                   # where the repo is, step by step, and what to run next
//	cloudflare init              # set the repo up, or update it after upgrading: the mise tasks, the agent skill, the
//	                             # GitHub Actions workflow and the Claude Code hook, none of them for editing
//	cloudflare dev [<args>]      # run the Worker locally (wrangler dev), with local D1, KV and R2; no login
//	cloudflare deploy [<args>]   # deploy it (wrangler deploy) on the repo's login; previews get their KV, D1 and R2
//	cloudflare preview [<args>]  # deploy a preview (wrangler preview), its KV, D1 and R2 its own
//	cloudflare github            # have Cloudflare deploy every push: previews, staging, production (Workers Builds)
//	cloudflare actions           # instead of github: GitHub Actions deploys every push, with a token you make once
//	cloudflare check             # the repo's rules, and whether init's files are current
//	cloudflare wrangler <args>   # any wrangler command, on the repo's login
//	cloudflare login / logout    # the repo's login: other commands make it when they need it
//	cloudflare version           # this build's version
//
// In a Go repo it's go tool cloudflare, at go.mod's version; in any other, the released binary (mise:
// github:joeblew999/mise-cloudflare), which its tasks pin.
//
// The login is a wrangler auth profile named after the Worker, passed to every wrangler
// command with --profile. It asks for wrangler's own login's scopes, and logs in in a browser on this machine, or with a
// link to open on any device (the OAuth device flow) in a Claude Code cloud session or with CLOUDFLARE_LOGIN=device. In
// CI (CI=true) there's no one to log in: a CLOUDFLARE_API_TOKEN secret is CI's login. Deploying and deleting are
// wrangler's own job.
//
// Flags go before the command; for wrangler, everything after it is wrangler's. -json prints one JSON document on
// stdout instead of text. Exit codes: 0 ok, 1 problems, 2 usage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/cf"
	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

func main() {
	// The command first, then its flags: cloudflare github --staging main. dev, deploy, preview and wrangler pass
	// everything after them to wrangler.
	args := os.Args[1:]
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	passes := map[string]bool{"dev": true, "deploy": true, "preview": true, "wrangler": true}
	flags := flag.NewFlagSet("cloudflare "+command, flag.ExitOnError)
	jsonOut := flags.Bool("json", false, "print one JSON document on stdout instead of text")
	off := flags.Bool("off", false, "github or actions: turn it off")
	branch := flags.String("branch", "", "github or actions: the branch whose pushes deploy production (default the repository's default branch, or production with --staging)")
	staging := flags.String("staging", "", "github: a staging branch, whose pushes deploy the wrangler environment env.staging")
	build := flags.String("build", "", "github: the build command (default installing from the lockfile: npm ci, …)")
	flags.Usage = func() { fmt.Fprint(os.Stderr, help) }
	if !passes[command] {
		flags.Parse(args)
		if flags.NArg() > 0 {
			flags.Usage()
			os.Exit(2)
		}
		args = nil
	}
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	switch command {
	case "version":
		fmt.Println(buildVersion())
		return
	case "help":
		fmt.Print(help)
		return
	case "init", "update":
		initRepo(ctx, dir)
		return
	}
	// No person in CI to click Allow, so no login: a CLOUDFLARE_API_TOKEN given to CI as a secret is CI's login. Said
	// first, before anything else is needed.
	if ci() && (command == "login" || passes[command] && command != "dev") && os.Getenv("CLOUDFLARE_API_TOKEN") == "" {
		fail(errors.New("in CI there's no one to log in: give CI a CLOUDFLARE_API_TOKEN (and CLOUDFLARE_ACCOUNT_ID) as secrets, or deploy with Cloudflare's own Workers Builds"))
	}
	r := openRepo(ctx, dir)
	switch command {
	case "":
		r.guide(ctx, *jsonOut)
	case "dev":
		// Local development needs no login: the repo's own wrangler, on local D1, KV and R2, and the secrets' local
		// values from .dev.vars.
		devVars(r.dir, r.wc.Secrets)
		r.run(ctx, false, append([]string{"dev"}, args...))
	case "deploy":
		if os.Getenv("WRANGLER_OUTPUT_FILE_PATH") == "" {
			os.Setenv("WRANGLER_OUTPUT_FILE_PATH", filepath.Join(".wrangler", "output.json")) // the guide reads its URL there
		}
		r.run(ctx, true, append([]string{"deploy"}, args...))
		// Previews get their own KV, D1 and R2, now the Worker is on Cloudflare.
		if _, ok := r.wc.Raw["previews"]; ok {
			r.previews(ctx, false, false)
		}
	case "preview":
		if _, ok := r.wc.Raw["previews"]; ok {
			if !ci() {
				r.login(ctx)
			}
			r.previews(ctx, false, false)
		}
		r.run(ctx, true, append([]string{"preview"}, args...))
	case "wrangler":
		r.run(ctx, true, args)
	case "login":
		if ci() {
			fmt.Println("✓ CI: wrangler uses CLOUDFLARE_API_TOKEN")
			return
		}
		who := r.login(ctx)
		if *jsonOut {
			emit(who)
		} else {
			fmt.Printf("✓ %s: %s\n", r.w.Profile, accountNames(who))
		}
	case "github":
		r.github(ctx, builds{Branch: *branch, Staging: *staging, Build: *build}, *off, *jsonOut)
	case "actions":
		r.actions(ctx, *branch, *off)
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
		fmt.Fprint(os.Stderr, help)
		os.Exit(2)
	}
}

// help is cloudflare help: every command, in the order they're used.
const help = `This repo's Cloudflare Worker, with mise-cloudflare.

The steps, in order (mise run cloudflare says which is next):
  mise run cloudflare:dev      1. run it locally, with local D1, KV and R2: no account needed
  mise run cloudflare:deploy   2. deploy it yourself: the first time, you click Allow
  mise run cloudflare:github   3. have Cloudflare deploy every push: a preview for every pull request,
                                  then production (--staging main for staging; --off to stop)

Now and then, as mise run cloudflare <command>:
  actions          instead of 3, GitHub Actions deploys every push (--off to stop)
  preview          a preview of what's here now, at its own URL (--name my-test)
  wrangler <...>   any other wrangler command on the repo's login: wrangler secret put NAME
  check            check the repo: no tokens or account IDs in it, secrets git-ignored, files current
  login, logout    the repo's Cloudflare login (the others make it when they need it)
  update           after upgrading mise-cloudflare: write its files into the repo again
  version          this build's version
`

// run runs wrangler with args on this terminal: on the repo's login when onLogin (made first if needed), or in CI on
// CLOUDFLARE_API_TOKEN; wrangler's exit code is the command's.
func (r repo) run(ctx context.Context, onLogin bool, args []string) {
	cmd := r.w.Plain(ctx, args...)
	if onLogin && !ci() {
		r.login(ctx)
		cmd = r.w.Cmd(ctx, args...)
	}
	if err := wrangler.Interactive(cmd); err != nil {
		os.Exit(exitCode(err))
	}
}

// repo is a repo with its wrangler installed: its config as wrangler reads it, and wrangler on its login.
type repo struct {
	dir        string
	viaActions bool // GitHub Actions deploys it: its workflow has the deploy jobs (cloudflare actions)
	wc         wconfig.Config
	w          wrangler.W
}

func openRepo(ctx context.Context, dir string) repo {
	w := wrangler.W{Dir: dir}
	if !w.Installed() {
		fail(errors.New("wrangler isn't installed here: install the repo's dependencies (its package.json pins wrangler)"))
	}
	wc, err := wconfig.Read(ctx, dir)
	if err != nil {
		fail(err)
	}
	w.Profile = wc.Name // the repo's login is named after its Worker
	deploys, _ := workflowMode(ctx, dir)
	return repo{dir: dir, viaActions: deploys, wc: wc, w: w}
}

// ci reports whether this runs in CI, where CI=true (GitHub Actions and every other CI set it).
func ci() bool { return os.Getenv("CI") != "" && os.Getenv("CI") != "false" && os.Getenv("CI") != "0" }

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

// version is this build's version: set by the release build (goreleaser's -X main.version), else the module version Go
// recorded (go tool, go install), else "(devel)".
var version = ""

func buildVersion() string {
	if version != "" {
		return strings.TrimPrefix(version, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "(devel)"
}
