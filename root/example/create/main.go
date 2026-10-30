// Command new makes a new repo for this repo's steps to run on: examples/new, made from only what a user
// brings to a repo of theirs, copied from the template, templates (the Worker's code, its package.json and lockfile, and the
// repo's mise.toml), under a Worker name of its own, pinning this checkout's build (local), or with -version a release
// from GitHub, as mise use does. Then it gives it its own git and runs init in it, as a person
// adding mise-cloudflare does: mise run example:e2e is the end-to-end on it. It is the root's (mise run example:create), never part of the tool.
package main

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	from, to       = "templates", "examples/new"
	worker, itsOwn = "mise-cloudflare-template", "mise-cloudflare-new"
)

// brought is what a user's repo has before mise-cloudflare. newRepo then gives it its own git and runs init, as a person
// adding mise-cloudflare does; what setup installs is the end-to-end's (mise run example:e2e).
var brought = []string{"mise.toml", "package.json", "pnpm-lock.yaml", "wrangler.jsonc", ".dev.vars.example", "src"}

func main() {
	version := flag.String("version", "", "pin this release of mise-cloudflare from GitHub (v0.1.8), not this checkout's build")
	flag.Parse()
	if err := newRepo(strings.TrimPrefix(*version, "v")); err != nil {
		fmt.Fprintln(os.Stderr, "new:", err)
		os.Exit(1)
	}
	pin := "local, this checkout's build"
	if *version != "" {
		pin = *version + ", from GitHub"
	}
	fmt.Printf("made %s from %s: %s, the Worker %s, mise-cloudflare at %s, its own git, and what init writes\n", to, from, strings.Join(brought, ", "), itsOwn, pin)
	fmt.Println("next: mise run example:e2e")
}

// newRepo makes it, pinning version (or this checkout's build, local, when it is ""). It never deletes one that's there:
// mise run example:create runs example:delete first.
func newRepo(version string) error {
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s is there: mise run example:delete first (mise run example:create does)", to)
	}
	for _, b := range brought {
		err := filepath.WalkDir(filepath.Join(from, b), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(from, path)
			if rel == "package.json" || rel == "wrangler.jsonc" {
				data = bytes.ReplaceAll(data, []byte(`"`+worker+`"`), []byte(`"`+itsOwn+`"`))
			}
			if rel == "mise.toml" && version != "" {
				pinned := []byte(`"github:joeblew999/mise-cloudflare" = "local"`)
				if !bytes.Contains(data, pinned) {
					return fmt.Errorf("%s/mise.toml doesn't pin mise-cloudflare at local", from)
				}
				data = bytes.ReplaceAll(data, pinned, []byte(`"github:joeblew999/mise-cloudflare" = "`+version+`"`))
			}
			dst := filepath.Join(to, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			return err
		}
	}
	// Then as a person adding mise-cloudflare: its own git, and mise-cloudflare init, at the version its mise.toml pins.
	if out, err := exec.Command("git", "-C", to, "init", "-q").CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v\n%s", err, out)
	}
	abs, err := filepath.Abs(to)
	if err != nil {
		return err
	}
	cmd := exec.Command("mise", "exec", "github:joeblew999/mise-cloudflare@"+cmp.Or(version, "local"), "--", "mise-cloudflare", "init")
	cmd.Dir = to
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(abs), "MISE_TRUSTED_CONFIG_PATHS="+abs, "MISE_YES=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
