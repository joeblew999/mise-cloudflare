// Command cloudflare gives a repo its own Cloudflare login, with exactly the scopes it says, and runs the repo's own
// wrangler on it. No API tokens to make, nothing machine-wide, and the same on a Mac and in a Claude Code cloud session.
//
//	go tool cloudflare login            # the repo's login: once, then again only when cloudflare.toml's scopes change
//	go tool cloudflare wrangler <args>  # the repo's pinned wrangler on that login (deploy, delete, d1, secret, …)
//	go tool cloudflare status           # the repo's login, its account and scopes; this machine's other logins; stray tokens
//	go tool cloudflare check            # the repo's rules: no account or resource IDs, no tokens, secrets listed, wrangler pinned
//	go tool cloudflare logout           # log the repo's login out on Cloudflare and delete it
//	go tool cloudflare init             # cloudflare.toml, the mise tasks and the agent skill, into a repo
//	go tool cloudflare tasks            # write its mise tasks: .mise/tasks/cloudflare.toml
//	go tool cloudflare skill            # install its agent skill: .claude/skills/cloudflare
//
// The login is a wrangler auth profile (cloudflare.toml's profile, default the Worker's name), passed to every wrangler
// command with --profile, so nothing depends on which login wrangler would pick for the folder. It logs in in a browser
// on this machine, or with a link to open on any device, a phone included (the OAuth device flow), in a Claude Code cloud
// session or with CLOUDFLARE_LOGIN=device. Anything else, deploying and deleting included, is wrangler's own job.
//
// Flags go before the command; for wrangler, everything after it is wrangler's. -json prints one JSON document on
// stdout instead of text. Exit codes: 0 ok, 1 problems, 2 usage.
package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
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

	"github.com/joeblew999/cloudflare-zero/config"
	"github.com/joeblew999/cloudflare-zero/internal/rules"
	"github.com/joeblew999/cloudflare-zero/internal/wconfig"
	"github.com/joeblew999/cloudflare-zero/internal/wrangler"
	"github.com/joeblew999/cloudflare-zero/mise"
	"github.com/joeblew999/cloudflare-zero/skills"
)

func main() {
	cfg, err := config.Find(".")
	if err != nil {
		fail(err)
	}
	profile := flag.String("profile", cfg.Profile, "the repo's login, a wrangler auth profile (default the Worker's name)")
	jsonOut := flag.Bool("json", false, "print one JSON document on stdout instead of text")
	out := flag.String("o", filepath.Join(".mise", "tasks", "cloudflare.toml"), "tasks: the file to write")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cloudflare [-json] [-profile name] login|status|check|logout|init|tasks|skill, or cloudflare wrangler <args>")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || (args[0] != "wrangler" && len(args) != 1) {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	switch args[0] {
	case "tasks":
		writeFile(*out, mise.Tasks)
		return
	case "skill":
		writeSkill(filepath.Join(".claude", "skills"))
		return
	case "init":
		switch {
		case cfg.Path != "":
			fmt.Println("✓ " + cfg.Path + " is there: yours to edit")
		case *profile != "":
			writeFile(config.File, []byte(config.TemplateFor(*profile)))
		default:
			writeFile(config.File, []byte(config.Template))
		}
		writeFile(*out, mise.Tasks)
		writeSkill(filepath.Join(".claude", "skills"))
		return
	}
	r := openRepo(ctx, dir, cfg, *profile)
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
	case "status":
		r.status(ctx, *jsonOut)
	case "check":
		r.check(*jsonOut)
	case "logout":
		if r.w.LoggedIn() {
			r.refuseStrayTokens()
			must(r.w.Delete(ctx))
		}
		fmt.Printf("✓ %s is logged out and deleted\n", r.w.Profile)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// repo is a repo with its wrangler installed: its config as wrangler reads it, and wrangler on its login.
type repo struct {
	dir    string
	scopes []string
	wc     wconfig.Config
	w      wrangler.W
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
	return repo{dir: dir, scopes: cfg.Scopes, wc: wc, w: w}
}

// login logs the repo in if it isn't yet, and again whenever the login's scopes differ from the repo's (a scope added
// or removed), then checks they match. It refuses a stray token first: one would override every login.
func (r repo) login(ctx context.Context) wrangler.Who {
	r.refuseStrayTokens()
	device := wrangler.Device()
	if !r.w.LoggedIn() {
		must(r.w.Login(ctx, r.scopes, device))
	} else {
		must(r.w.Activate(ctx))
	}
	who, err := r.w.Whoami(ctx)
	if err != nil {
		fail(err)
	}
	add, drop := wrangler.Missing(r.scopes, who.Scopes), wrangler.Extra(r.scopes, who.Scopes)
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
	must(r.w.Login(ctx, r.scopes, device))
	if who, err = r.w.Whoami(ctx); err != nil {
		fail(err)
	}
	if len(wrangler.Missing(r.scopes, who.Scopes))+len(wrangler.Extra(r.scopes, who.Scopes)) > 0 {
		fail(fmt.Errorf("the new login's scopes (%s) still don't match the repo's (%s)", strings.Join(who.Scopes, " "), strings.Join(r.scopes, " ")))
	}
	return who
}

// refuseStrayTokens: a CLOUDFLARE_API_TOKEN beats every login, so outside CI one in the shell or in a .env is refused.
func (r repo) refuseStrayTokens() {
	if os.Getenv("CI") != "" {
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

func (r repo) check(jsonOut bool) {
	problems, err := rules.Check(r.dir, r.wc)
	if err != nil {
		fail(err)
	}
	if jsonOut {
		emit(struct {
			OK       bool     `json:"ok"`
			Problems []string `json:"problems"`
		}{len(problems) == 0, append([]string{}, problems...)})
	} else if len(problems) == 0 {
		fmt.Println("rules: pass")
	} else {
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
	Profile        string             `json:"profile"`
	LoggedIn       bool               `json:"loggedIn"`
	Accounts       []wrangler.Account `json:"accounts"`
	Scopes         []string           `json:"scopes"`
	TokenInShell   bool               `json:"tokenInShell"`
	TokenInEnv     []string           `json:"tokenInEnvFiles"`
	WranglerFolder string             `json:"wranglerFolder"`
	DefaultLogin   string             `json:"defaultLogin"` // a checksum of the machine's own wrangler login, never its contents
	OtherProfiles  []string           `json:"otherProfiles"`
	FolderBindings map[string]string  `json:"folderBindings"`
}

func (r repo) status(ctx context.Context, jsonOut bool) {
	rep := statusReport{
		Profile: r.w.Profile, LoggedIn: r.w.LoggedIn(), Accounts: []wrangler.Account{}, Scopes: []string{},
		TokenInShell: os.Getenv("CLOUDFLARE_API_TOKEN") != "", TokenInEnv: tokenEnvFiles(r.dir),
		WranglerFolder: wrangler.Home(), DefaultLogin: fileHash(wrangler.DefaultLogin()), OtherProfiles: []string{},
		FolderBindings: map[string]string{},
	}
	if entries, err := os.ReadDir(filepath.Join(rep.WranglerFolder, "config")); err == nil {
		for _, e := range entries {
			if name, ok := strings.CutSuffix(e.Name(), ".toml"); ok && name != "default" && name != r.w.Profile {
				rep.OtherProfiles = append(rep.OtherProfiles, name)
			}
		}
	}
	if b, err := os.ReadFile(wrangler.Bindings()); err == nil {
		json.Unmarshal(b, &rep.FolderBindings)
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
	home, _ := os.UserHomeDir()
	short := func(s string) string { return strings.ReplaceAll(s, home, "~") }
	yes := map[bool]string{true: "logged in", false: "not logged in"}
	fmt.Printf("this repo's login    %s: %s\n", rep.Profile, yes[rep.LoggedIn])
	for _, a := range rep.Accounts {
		fmt.Printf("account              %s https://dash.cloudflare.com/%s\n", a.Name, a.ID)
	}
	if len(rep.Scopes) > 0 {
		fmt.Printf("scopes               %s\n", strings.Join(rep.Scopes, " "))
	}
	fmt.Printf("CLOUDFLARE_API_TOKEN %s\n", map[bool]string{true: "SET in this shell (would override every login)", false: "not set"}[rep.TokenInShell])
	fmt.Printf(".env with a token    %s\n", cmp.Or(strings.Join(rep.TokenInEnv, " "), "none"))
	fmt.Printf("wrangler folder      %s\n", short(rep.WranglerFolder))
	fmt.Printf("default login        config/default.toml %s\n", rep.DefaultLogin)
	fmt.Printf("other logins         %s\n", cmp.Or(strings.Join(rep.OtherProfiles, " "), "none"))
	b, _ := json.Marshal(rep.FolderBindings)
	fmt.Printf("folder bindings      %s\n", short(string(b)))
}

func accountNames(who wrangler.Who) string {
	var names []string
	for _, a := range who.Accounts {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// fileHash is a short sha256 of a file, or "none"; the file's contents are never printed.
func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "none"
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))[:16]
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
