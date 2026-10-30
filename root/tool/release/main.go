// Command release releases mise-cloudflare from the commit checked out (mise run tool:release; release.yml runs the
// same on GitHub): the commit's v* tag, or the next patch version, tagged and pushed, when it has none; goreleaser's
// binaries for every machine on that tag's GitHub release (.goreleaser.yaml); a new repo pinning the release, tried as
// a person tries it (mise run example:create --version, then mise run example:e2e); and the version fetched by proxy.golang.org, so go get and pkg.go.dev have it at
// once. With -dry-run it tags and publishes nothing: goreleaser builds a snapshot into dist/. It is this repo's, never
// part of the tool.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "tag nothing and publish nothing: goreleaser builds a snapshot into dist/")
	flag.Parse()
	tag, isNew, err := version()
	if err != nil {
		fail(err)
	}
	if *dryRun {
		fmt.Printf("· would release %s%s\n", tag, map[bool]string{true: " (a new tag on this commit)", false: " (this commit's tag)"}[isNew])
		run(nil, "goreleaser", "release", "--snapshot", "--clean")
		fmt.Println("· dry run: nothing tagged, nothing published")
		return
	}
	if isNew {
		identity := []string{}
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			identity = []string{"-c", "user.name=github-actions", "-c", "user.email=github-actions@users.noreply.github.com"}
		}
		run(nil, "git", append(identity, "tag", "-a", tag, "-m", tag)...)
		run(nil, "git", "push", "-q", "origin", tag)
		fmt.Println("tagged", tag)
	}
	run([]string{"GORELEASER_CURRENT_TAG=" + tag}, "goreleaser", "release", "--clean")
	run(nil, "mise", "run", "example:create", "--version", tag)
	run(nil, "mise", "run", "example:e2e")
	run([]string{"GOPROXY=https://proxy.golang.org", "GOFLAGS=-mod=mod"}, "go", "list", "-m", module()+"@"+tag)
}

// version is the tag to release: the commit's own v* tag, or the next patch version after the newest v* tag (v0.0.1
// for the first), which is new.
func version() (tag string, isNew bool, err error) {
	if out, err := exec.Command("git", "describe", "--tags", "--exact-match", "--match", "v*").Output(); err == nil {
		return strings.TrimSpace(string(out)), false, nil
	}
	out, err := exec.Command("git", "tag", "-l", "v*").Output()
	if err != nil {
		return "", false, fmt.Errorf("git tag -l: %w", err)
	}
	newest := [3]int{}
	for _, t := range strings.Fields(string(out)) {
		if v, ok := parse(t); ok && less(newest, v) {
			newest = v
		}
	}
	return fmt.Sprintf("v%d.%d.%d", newest[0], newest[1], newest[2]+1), true, nil
}

// parse reads vMAJOR.MINOR.PATCH; anything else isn't a release tag.
func parse(tag string) ([3]int, bool) {
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if !strings.HasPrefix(tag, "v") || len(parts) != 3 {
		return [3]int{}, false
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// module is this repo's Go module, from go.mod: what proxy.golang.org fetches.
func module() string {
	out, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		fail(fmt.Errorf("go list -m: %w", err))
	}
	return strings.TrimSpace(string(out))
}

// run runs a command with env added to this one's, its output shown, and stops at its failure.
func run(env []string, name string, args ...string) {
	fmt.Println("$", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fail(fmt.Errorf("%s: %w", name, err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release:", err)
	os.Exit(1)
}
