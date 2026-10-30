package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/templates"
)

// theirs are the template's files a repo may have of its own already: init writes them only when it has none.
// .mise/config.toml is mise-cloudflare's pin: written once, then the repo's (cloudflare:update moves it).
var theirs = map[string]bool{".gitignore": true, ".claude/settings.json": true, ".mise/config.toml": true}

// pinLine is mise-cloudflare's pin in .mise/config.toml.
var pinLine = regexp.MustCompile(regexp.QuoteMeta(`"`+self+`" = "`) + `[^"]*"`)

// pinned is the template's .mise/config.toml pinning this build: a release pins itself, a build of a checkout local
// (mise run tool:install there links it), so a repo runs exactly what set it up.
func pinned(b []byte) []byte {
	pin := "local"
	if version != "" {
		pin = strings.TrimPrefix(version, "v")
	}
	if !pinLine.Match(b) {
		fail(fmt.Errorf("the template's .mise/config.toml doesn't pin %s", self))
	}
	return pinLine.ReplaceAll(b, []byte(`"`+self+`" = "`+pin+`"`))
}

// initRepo writes the template's tool half into the repo, each file at its path there.
func initRepo(dir string) {
	must(fs.WalkDir(templates.Tool, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		to := filepath.Join(dir, filepath.FromSlash(p))
		if _, err := os.Stat(to); err == nil && theirs[p] {
			fmt.Printf("✓ %s is the repo's own: left as it is\n", p)
			return nil
		}
		b, err := fs.ReadFile(templates.Tool, p)
		if err == nil {
			if p == ".mise/config.toml" {
				b = pinned(b)
			}
			writeFile(to, b)
		}
		if strings.HasSuffix(p, ".sh") {
			must(os.Chmod(to, 0o755))
		}
		return err
	}))
}

// stale names the files init wrote that this version would write differently, and a .claude/settings.json of the
// repo's own that doesn't run the hook.
func (r repo) stale(ctx context.Context) []string {
	var out []string
	fs.WalkDir(templates.Tool, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || theirs[p] {
			return err
		}
		want, _ := fs.ReadFile(templates.Tool, p)
		// Line endings aside: a Windows checkout of the repo may have \r\n where init wrote \n.
		crlf := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
		if b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(p))); err == nil && !bytes.Equal(crlf(b), crlf(want)) {
			out = append(out, p+" is out of date: run mise run cloudflare:03-init")
		}
		return nil
	})
	if b, err := os.ReadFile(filepath.Join(r.dir, ".claude", "settings.json")); err == nil && !bytes.Contains(b, []byte(templates.HookFile)) {
		out = append(out, ".claude/settings.json doesn't run .claude/hooks/"+templates.HookFile+": add its SessionStart entry (docs/mise-cloudflare/README.md)")
	}
	return out
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
