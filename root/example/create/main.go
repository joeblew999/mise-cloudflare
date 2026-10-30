// Command new makes a new repo for this repo's steps to run on: examples/new, made from only what a user
// brings to a repo of theirs, copied from examples/reference (the Worker's code, its package.json and lockfile, and the
// repo's mise.toml), under a Worker name of its own, pinning this checkout's build (local), or with -version a release
// from GitHub, as mise use does. The steps do the rest, as on any repo: EXAMPLE=new mise run example:05-show, and so
// on. It is the root's (mise run example:create), never part of the tool.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	from, to       = "examples/reference", "examples/new"
	worker, itsOwn = "mise-cloudflare-example", "mise-cloudflare-new"
)

// brought is what a user's repo has before mise-cloudflare. Everything else in a repo of examples/ is the steps' work:
// its git, what init writes (the .gitignore's lines among it), and what setup installs.
var brought = []string{"mise.toml", "package.json", "aube-lock.yaml", "wrangler.jsonc", ".dev.vars.example", "src"}

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
	fmt.Printf("made %s from %s: %s, the Worker %s, mise-cloudflare at %s\n", to, from, strings.Join(brought, ", "), itsOwn, pin)
	fmt.Println("next: EXAMPLE=new mise run example:05-show (each step runs the ones it needs first)")
}

// newRepo makes it, pinning version (or this checkout's build, local, when it is "").
func newRepo(version string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
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
	return nil
}
