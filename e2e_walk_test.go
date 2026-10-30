// Package e2e is this repo's end-to-end test: a walk of a repo's numbered tasks, through the repo's own mise file. It
// sits at the top of this repo, beside the root mise.toml whose tasks run it, in a file for each thing it does:
//
//   - e2e_walk_test.go, this file: TestWalk, the test the root's tasks run (which repo, REPO_PATH, and how far,
//     E2E_STAGE), the stages, and walking the steps.
//   - e2e_steps_test.go: the steps, the repo's numbered tasks in their order, each with its check.
//   - e2e_mise_test.go: the repo, and the calls into its own mise file, made in it as a person there makes them.
//   - e2e_cloudflare_test.go: what the walk looks at besides, as a person does: the Worker's URL, the account's D1, KV
//     and R2, Cloudflare's builds.
//   - e2e_older_test.go: the tests the root's older tasks still run, until TestWalk replaces them.
//
// mise_test.go checks that the mise files and the Go line up, both ways.
package e2e

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestWalk walks the repo REPO_PATH names (a folder of this repo: templates, or a stamped copy, examples/new) as far as
// the stage E2E_STAGE names (serve, deploy or connect-github). The root's tasks set both: mise run template:e2e-serve
// is REPO_PATH=templates E2E_STAGE=serve. Without E2E_STAGE it skips, so go test ./... never touches Cloudflare.
func TestWalk(t *testing.T) {
	name := os.Getenv("E2E_STAGE")
	if name == "" {
		t.Skip("E2E_STAGE is how far to walk, and the root's task sets it: mise run template:e2e-serve")
	}
	upTo := slices.Index(stageNames, name)
	if upTo < 0 {
		t.Fatalf("E2E_STAGE=%s: the stages are %s", name, strings.Join(stageNames, ", "))
	}
	r := repoAt(t, os.Getenv("REPO_PATH"))
	if stage(upTo) == connectGithub {
		r = r.checkoutOfHead()
	}
	r.miseSetUp()
	walkSteps(t, r, stage(upTo))
}

// stage is how far a walk goes. Each runs the steps the stages before it run, and the steps that need it.
type stage int

const (
	serve         stage = iota // as far as 06-serve: the Worker locally, needing nothing, CI included
	deploy                     // as far as 08-deploy: deployed from here, needing a Cloudflare account (and an Allow, the first time)
	connectGithub              // as far as 09-connect-github: Cloudflare deploying every push, needing the repo on GitHub too
)

// stageNames are the stages as E2E_STAGE names them: after the step each goes as far as.
var stageNames = []string{"serve", "deploy", "connect-github"}

func (s stage) String() string { return stageNames[s] }

// walk is the steps run on a repo in their order, as far as upTo, with what they leave for the steps after them.
type walk struct {
	repo
	upTo       stage
	before     map[string][32]byte // the repo's files before the steps that deploy, which must change none of them
	url        string              // the Worker's, once 08-deploy has deployed it
	prod, prev string              // the throwaway branches connect-github's steps push
}

// walkSteps runs the repo's numbered tasks in their order, as far as upTo, each with its check (mise_test.go checks
// that steps are the template's numbered tasks, in their order). A walk that deploys starts by
// deleting whatever a stopped one left on Cloudflare, and deletes what it made even when it fails.
func walkSteps(t *testing.T, r repo, upTo stage) {
	w := &walk{repo: r, upTo: upTo}
	if upTo >= deploy {
		w.miseDeleteEverythingOnCloudflare()
		t.Cleanup(func() {
			if t.Failed() {
				w.heading("Cleaning up after the failure")
				w.miseDeleteEverythingOnCloudflare()
			}
		})
		w.before = w.snapshot()
	}
	if upTo >= connectGithub {
		stamp := time.Now().Unix()
		w.prod, w.prev = fmt.Sprintf("e2e-prod-%d", stamp), fmt.Sprintf("e2e-preview-%d", stamp)
	}
	for _, s := range steps {
		switch {
		case s.stage > upTo:
			w.pass("cloudflare:%s: for the stage %s, not this walk's (%s)", s.task, s.stage, upTo)
		case s.skip != "":
			w.pass("cloudflare:%s: %s", s.task, s.skip)
		default:
			w.heading("cloudflare:%s", s.task)
			s.check(w)
		}
	}
	fmt.Println("\nALL PASSED")
}

// snapshot is the repo's files by their content, all but what isn't the repo's own: its git, installed packages, and
// the local state 06-serve and 15-clean make and undo.
func (w *walk) snapshot() map[string][32]byte {
	w.t.Helper()
	out := map[string][32]byte{}
	err := filepath.WalkDir(w.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch d.Name() {
		case ".git", "node_modules", ".wrangler", ".dev.vars":
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(w.dir, p)
		out[rel] = sha256.Sum256(b)
		return nil
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return out
}

// diff names the files added, removed or changed between two snapshots.
func diff(a, b map[string][32]byte) []string {
	var changed []string
	for p, h := range b {
		if a[p] != h {
			changed = append(changed, p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			changed = append(changed, p)
		}
	}
	slices.Sort(changed)
	return changed
}
