// Command mise-cloudflare gives a repo its own Cloudflare login and runs the repo's own wrangler on it: no API tokens
// to make, nothing machine-wide, the same on a Mac and in a Claude Code cloud session. The login is a wrangler auth
// profile named after the Worker. `mise-cloudflare show-help` lists the commands.
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

const help = `This repo's Cloudflare Worker, with mise-cloudflare. Each command is a task, numbered in the order a repo
meets them: mise run cloudflare:<nn>-<command> runs mise-cloudflare <command>.

The steps, in order (mise run cloudflare says which is next):
  mise run cloudflare:03-init                  mise-cloudflare's files written into the repo (again, after upgrading)
  mise run cloudflare:04-check                 the repo's rules: no tokens or account IDs in it, secrets git-ignored, files current
  mise run cloudflare:05-show                  what's done, and what to run next (mise run cloudflare, too)
  mise run cloudflare:06-serve                 the Worker locally, with local D1, KV and R2: no account needed
  mise run cloudflare:07-set-secrets           the secrets the Worker requires that Cloudflare doesn't have yet
  mise run cloudflare:08-deploy                the Worker on Cloudflare, on the repo's login: the first time, you click Allow
  mise run cloudflare:09-connect-github        Cloudflare deploys every push: a preview for every pull request, then
                                               production (--staging main for staging; --off to stop)
  mise run cloudflare:10-set-preview-secrets   the secrets its previews require, which they don't inherit
  mise run cloudflare:11-preview               a preview of what's here now, at its own URL (--name my-test)
  mise run cloudflare:12-login                 the repo's Cloudflare login (the others make it when they need it)
  mise run cloudflare:13-delete                deploy undone: the Worker, its KV, D1 and R2, its previews' own (--yes: don't ask)
  mise run cloudflare:14-logout                the repo's logins logged out and deleted

And:
  mise run cloudflare:verify         what the repo's GitHub workflow runs: its setup and test tasks, then check
  mise run cloudflare:run-wrangler   any other wrangler command, on the repo's login: d1 list
  mise run cloudflare:show-version   this build's version
  mise run cloudflare:show-help      this
`

func main() {
	// The command first, then its flags. serve, deploy, preview and run-wrangler pass everything after them to wrangler.
	args := os.Args[1:]
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	passes := map[string]bool{"serve": true, "deploy": true, "preview": true, "run-wrangler": true}
	flags := flag.NewFlagSet("mise-cloudflare "+command, flag.ExitOnError)
	jsonOut := flags.Bool("json", false, "print one JSON document on stdout instead of text")
	off := flags.Bool("off", false, "connect-github: turn it off")
	branch := flags.String("branch", "", "connect-github: the branch whose pushes deploy production (default the repository's default branch, or production with --staging)")
	staging := flags.String("staging", "", "connect-github: a staging branch, whose pushes deploy the wrangler environment env.staging")
	build := flags.String("build", "", "connect-github: the build command (default installing from the lockfile: npm ci, …)")
	yes := flags.Bool("yes", false, "delete: don't ask first")
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
	case "show-version":
		fmt.Println(buildVersion())
		return
	case "show-help":
		fmt.Print(help)
		return
	case "init", "update": // update: what the tasks of repos set up before init was its one name still run
		initRepo(ctx, dir)
		return
	}
	// CI only checks the repo; Cloudflare's Workers Builds deploys. No one is there to click Allow.
	if inCI() && command != "" && command != "show" && command != "check" && command != "verify" && command != "serve" {
		fail(errors.New("in CI there's no one to log in: CI runs mise run cloudflare:verify, and Cloudflare's Workers Builds deploys (mise run cloudflare:09-connect-github)"))
	}
	r := openRepo(ctx, dir)
	switch command {
	case "", "show":
		r.show(ctx, *jsonOut)
	case "check":
		r.check(ctx, *jsonOut)
	case "verify":
		r.verify(ctx)
	case "serve":
		r.serve(ctx, args)
	case "set-secrets":
		r.setSecrets(ctx, false)
	case "set-preview-secrets":
		r.setPreviewSecrets(ctx)
	case "deploy":
		r.deploy(ctx, args)
	case "connect-github":
		r.connectGithub(ctx, builds{Branch: *branch, Staging: *staging, Build: *build}, *off, *jsonOut)
	case "preview":
		r.preview(ctx, args)
	case "run-wrangler":
		r.runWrangler(ctx, args)
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

// inCI reports whether this runs in CI (CI=true, as GitHub Actions and every other CI set it).
func inCI() bool {
	return os.Getenv("CI") != "" && os.Getenv("CI") != "false" && os.Getenv("CI") != "0"
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
