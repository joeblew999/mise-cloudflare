// Command mise-cloudflare gives a repo its own Cloudflare login and runs the repo's own wrangler on it: no API tokens
// to make, nothing machine-wide, the same on a Mac and in a Claude Code cloud session. The login is a wrangler auth
// profile named after the Worker. `mise-cloudflare help` lists the commands.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/wconfig"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

const help = `This repo's Cloudflare Worker, with mise-cloudflare. Each command is a task:
mise run cloudflare:<command> runs mise-cloudflare <command>.

The steps, in order (mise run cloudflare:status says which is next):
  mise run cloudflare:dev      1. run it locally, with local D1, KV and R2: no account needed
  mise run cloudflare:deploy   2. deploy it yourself: the first time, you click Allow
  mise run cloudflare:github   3. have Cloudflare deploy every push: a preview for every pull request,
                                  then production (--staging main for staging; --off to stop)

Now and then:
  mise run cloudflare:status     what's done, and what to run next (mise run cloudflare, too)
  mise run cloudflare:check      the repo's rules: no tokens or account IDs in it, secrets git-ignored, files current
  mise run cloudflare:secret     the secrets the Worker requires that Cloudflare doesn't have yet (--previews: its previews')
  mise run cloudflare:preview    a preview of what's here now, at its own URL (--name my-test)
  mise run cloudflare:wrangler   any other wrangler command on the repo's login: wrangler d1 list
  mise run cloudflare:login      the repo's Cloudflare login (the others make it when they need it)
  mise run cloudflare:delete     deploy undone: the Worker, its KV, D1 and R2, its previews' own (--yes: don't ask)
  mise run cloudflare:logout     the repo's logins logged out and deleted
  mise run cloudflare:init       after upgrading mise-cloudflare: its files written into the repo again
  mise run cloudflare:version    this build's version
  mise run cloudflare:help       this
`

func main() {
	// The command first, then its flags. dev, deploy, preview and wrangler pass everything after them to wrangler.
	args := os.Args[1:]
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	passes := map[string]bool{"dev": true, "deploy": true, "preview": true, "wrangler": true}
	flags := flag.NewFlagSet("mise-cloudflare "+command, flag.ExitOnError)
	jsonOut := flags.Bool("json", false, "print one JSON document on stdout instead of text")
	off := flags.Bool("off", false, "github: turn it off")
	branch := flags.String("branch", "", "github: the branch whose pushes deploy production (default the repository's default branch, or production with --staging)")
	staging := flags.String("staging", "", "github: a staging branch, whose pushes deploy the wrangler environment env.staging")
	build := flags.String("build", "", "github: the build command (default installing from the lockfile: npm ci, …)")
	yes := flags.Bool("yes", false, "delete: don't ask first")
	previews := flags.Bool("previews", false, "secret: the previews' secrets, in the Worker's Previews Base")
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
	must(err)
	switch command {
	case "version":
		fmt.Println(buildVersion())
		return
	case "help":
		fmt.Print(help)
		return
	case "init", "update": // update: what the tasks of repos set up before init was its one name still run
		initRepo(ctx, dir)
		return
	}
	// CI only checks the repo; Cloudflare's Workers Builds deploys. No one is there to click Allow.
	if ci() && command != "" && command != "status" && command != "check" && command != "dev" {
		fail(errors.New("in CI there's no one to log in: CI runs mise run cloudflare:check, and Cloudflare's Workers Builds deploys (mise run cloudflare:github)"))
	}
	r := openRepo(ctx, dir)
	switch command {
	case "", "status":
		r.status(ctx, *jsonOut)
	case "check":
		r.check(ctx, *jsonOut)
	case "dev":
		r.dev(ctx, args)
	case "secret":
		r.secret(ctx, *previews)
	case "deploy":
		r.deploy(ctx, args)
	case "github":
		r.github(ctx, builds{Branch: *branch, Staging: *staging, Build: *build}, *off, *jsonOut)
	case "preview":
		r.preview(ctx, args)
	case "wrangler":
		r.wrangler(ctx, args)
	case "login":
		r.login(ctx, *jsonOut)
	case "delete":
		r.delete(ctx, *yes)
	case "logout":
		r.logout(ctx)
	default:
		fmt.Fprint(os.Stderr, help)
		os.Exit(2)
	}
}

// repo is a Worker repo with its wrangler installed: its config as wrangler reads it, and wrangler on its login.
type repo struct {
	dir string
	wc  wconfig.Config
	w   wrangler.W
}

func openRepo(ctx context.Context, dir string) repo {
	w := wrangler.W{Dir: dir}
	if !w.Installed() {
		fail(errors.New("wrangler isn't installed here: install the repo's dependencies (its package.json pins wrangler)"))
	}
	wc, err := wconfig.Read(ctx, dir)
	must(err)
	w.Profile = wc.Name // the login is named after the Worker
	return repo{dir: dir, wc: wc, w: w}
}

// ci reports whether this runs in CI (CI=true, as GitHub Actions and every other CI set it).
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
	must(enc.Encode(v))
}

func must(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mise-cloudflare:", err)
	os.Exit(1)
}

// version is set by the release build (goreleaser's -X main.version); otherwise Go's module version, or "(devel)".
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
