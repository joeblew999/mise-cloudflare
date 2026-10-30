package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// self is mise-cloudflare as a repo's mise pins it (.mise/config.toml, which init writes).
const self = "github:joeblew999/mise-cloudflare"

// update moves the repo's own pins to their newest releases, each with its own tool (mise reads no parent folder's
// config, so nothing outside the repo moves): its mise tools (mise upgrade --bump), its packages (aube, which writes
// whichever lockfile the repo has) and its Go modules (go get -u, go mod tidy) when it has them. mise-cloudflare at
// local, a checkout's build, stays there; moved to a newer release, it says to write init's files again (03-init),
// which it doesn't do itself: in the template, a release's init would write its own older files over the template's.
// mise and aube both skip a release younger than their minimum age. It ends with what moved; git diff shows it, the
// steps prove it.
func (r repo) update(ctx context.Context) {
	if r.exists("package.json") && r.mise(ctx, "which", "aube") == "" {
		fail(errors.New("the packages update with aube, which writes whichever lockfile the repo has, and the repo's mise doesn't pin it: mise use aube"))
	}
	before := r.pins(ctx)
	upgrade := []string{"upgrade", "--bump", "--local"}
	if selfPin(before) == "local" {
		upgrade = append(upgrade, "--exclude", self)
	}
	r.runOwnMise(ctx, upgrade...)
	if r.exists("package.json") {
		r.runOwnMise(ctx, "exec", "--", "aube", "update", "--latest")
	}
	if r.exists("go.mod") {
		r.runOwnMise(ctx, "exec", "--", "go", "get", "-u", "./...")
		r.runOwnMise(ctx, "exec", "--", "go", "mod", "tidy")
	}
	after := r.pins(ctx)
	if b, a := selfPin(before), selfPin(after); a != b {
		fmt.Printf("mise-cloudflare moved from %s to %s: mise run cloudflare:03-init writes its files again (04-check fails until it has)\n", b, a)
	}
	lines, n := moved(before, after)
	if len(lines) == 0 {
		fail(errors.New("no pins read: mise ls --local --json, package.json and go.mod named none"))
	}
	if n == 0 {
		fmt.Println("✓ nothing moved: each pin is its newest release (mise and aube skip one younger than their minimum age)")
	} else {
		fmt.Printf("✓ %d moved (git diff shows them; run the steps to prove them):\n", n)
	}
	for _, l := range lines {
		fmt.Println("  " + l)
	}
}

// pins is every version the repo pins of its own, by file and name: its mise tools by the file mise read each from
// ("mise.toml node"; "go.mod go" for go.mod's toolchain line), its packages ("package.json wrangler") and its Go modules
// ("go.mod golang.org/x/term"), each as that file's tool reports it.
func (r repo) pins(ctx context.Context) map[string]string {
	p := map[string]string{}
	var tools map[string][]struct {
		Version string
		Source  struct{ Path string }
	}
	if json.Unmarshal([]byte(r.mise(ctx, "ls", "--local", "--json")), &tools) == nil {
		for name, vs := range tools {
			for _, v := range vs {
				if v.Source.Path != "" { // a file names it, installed yet or not
					p[filepath.Base(v.Source.Path)+" "+name] = v.Version
				}
			}
		}
	}
	var pkg struct{ Dependencies, DevDependencies map[string]string }
	if b, err := os.ReadFile(filepath.Join(r.dir, "package.json")); err == nil && json.Unmarshal(b, &pkg) == nil {
		for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
			for name, v := range deps {
				p["package.json "+name] = v
			}
		}
	}
	var mod struct {
		Require []struct{ Path, Version string }
	}
	if r.exists("go.mod") && json.Unmarshal([]byte(r.mise(ctx, "exec", "--", "go", "mod", "edit", "-json")), &mod) == nil {
		for _, m := range mod.Require {
			p["go.mod "+m.Path] = m.Version
		}
	}
	return p
}

// selfPin is the version of mise-cloudflare the repo's mise pins, from whichever file it does: "" if none.
func selfPin(p map[string]string) string {
	for k, v := range p {
		if strings.HasSuffix(k, " "+self) {
			return v
		}
	}
	return ""
}

// moved is every pin, one line each in the order of their file and name, and how many moved: one that stayed is
// "aube 2.8.4 (mise.toml)", one that moved "node 26.12.1 → 26.13.0 (mise.toml)"; one only before or after has
// "none" for the other.
func moved(before, after map[string]string) ([]string, int) {
	all := maps.Clone(before)
	maps.Copy(all, after)
	var lines []string
	n := 0
	for _, k := range slices.Sorted(maps.Keys(all)) {
		file, name, _ := strings.Cut(k, " ")
		b, a := before[k], after[k]
		if a == b {
			lines = append(lines, fmt.Sprintf("%s %s (%s)", name, a, file))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s → %s (%s)", name, cmp.Or(b, "none"), cmp.Or(a, "none"), file))
		n++
	}
	return lines, n
}

// exists reports whether the repo has the file f.
func (r repo) exists(f string) bool {
	_, err := os.Stat(filepath.Join(r.dir, f))
	return err == nil
}

// ownMise is the environment that keeps mise to the repo's own config, as in a repo on its own. mise stops at a ceiling
// equal to a parent of its working folder exactly as the OS reports it (mise's src/config/miserc.rs): on macOS and Linux
// the real path, links resolved (/tmp is /private/tmp on macOS); on Windows the path as given, links not resolved, in
// long or short names. So the ceiling is each of those forms, and mise stops at whichever it sees.
func (r repo) ownMise() []string {
	forms := []string{r.dir, longPath(r.dir)}
	if real, err := filepath.EvalSymlinks(r.dir); err == nil {
		forms = append(forms, real, longPath(real))
	}
	var ceilings []string
	for _, f := range forms {
		if d := filepath.Dir(f); !slices.Contains(ceilings, d) {
			ceilings = append(ceilings, d)
		}
	}
	return append(os.Environ(), "MISE_CEILING_PATHS="+strings.Join(ceilings, string(filepath.ListSeparator)))
}

// mise is what mise prints for args on the repo's own config, "" when it fails.
func (r repo) mise(ctx context.Context, args ...string) string {
	cmd := exec.CommandContext(ctx, "mise", args...)
	cmd.Dir, cmd.Env = r.dir, r.ownMise()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runOwnMise runs mise with args on the repo's own config, as the person would, and stops at a failure.
func (r repo) runOwnMise(ctx context.Context, args ...string) {
	fmt.Println("$ mise " + strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "mise", args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = r.dir, r.ownMise(), os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fail(fmt.Errorf("mise %s: %w", strings.Join(args, " "), err))
	}
}
