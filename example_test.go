// The root's example: tasks, the copy stamped out from the template: mise run example:create makes it, mise run
// example:delete deletes it. Each runs only from its own task (MISE_TASK_NAME).

package e2e

import (
	"bytes"
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	stampFrom, stampTo            = "templates", "examples/new"
	templateWorker, stampedWorker = "mise-cloudflare-template", "mise-cloudflare-new"
)

// brought is what a user's repo has before mise-cloudflare. TestExampleCreate then gives it its own git and runs init,
// as a person adding mise-cloudflare does; what setup installs is the end-to-end's (mise run example:e2e-serve).
var brought = []string{"mise.toml", "package.json", "pnpm-lock.yaml", "wrangler.jsonc", ".dev.vars.example", "src"}

// TestExampleCreate: mise run example:create makes examples/new, a new repo for the steps, from only what a user brings
// to a repo of theirs, copied from the template (the Worker's code, its package.json and lockfile, and the repo's
// mise.toml), under a Worker name of its own, pinning this checkout's build (local), or with --version a release from
// GitHub (MISE_CLOUDFLARE_VERSION), as mise use does. Then it gives it its own git and runs init in it, as a person
// adding mise-cloudflare does: mise run example:e2e-serve is the end-to-end on it. It never deletes one that's there:
// mise run example:create runs example:delete first.
func TestExampleCreate(t *testing.T) {
	onlyFrom(t, "example:create")
	version := strings.TrimPrefix(os.Getenv("MISE_CLOUDFLARE_VERSION"), "v")
	if _, err := os.Stat(stampTo); err == nil {
		t.Fatalf("%s is there: mise run example:delete first (mise run example:create does)", stampTo)
	}
	for _, b := range brought {
		err := filepath.WalkDir(filepath.Join(stampFrom, b), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(stampFrom, path)
			if rel == "package.json" || rel == "wrangler.jsonc" {
				data = bytes.ReplaceAll(data, []byte(`"`+templateWorker+`"`), []byte(`"`+stampedWorker+`"`))
			}
			if rel == "mise.toml" && version != "" {
				pinned := []byte(`"github:joeblew999/mise-cloudflare" = "local"`)
				if !bytes.Contains(data, pinned) {
					return fmt.Errorf("%s/mise.toml doesn't pin mise-cloudflare at local", stampFrom)
				}
				data = bytes.ReplaceAll(data, pinned, []byte(`"github:joeblew999/mise-cloudflare" = "`+version+`"`))
			}
			dst := filepath.Join(stampTo, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Then as a person adding mise-cloudflare: its own git, and mise-cloudflare init, at the version its mise.toml pins.
	if out, err := exec.Command("git", "-C", stampTo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	abs, err := filepath.Abs(stampTo)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs) // mise stops only at a ceiling given as the real path
	}
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("mise", "exec", "github:joeblew999/mise-cloudflare@"+cmp.Or(version, "local"), "--", "mise-cloudflare", "init")
	cmd.Dir = stampTo
	cmd.Env = append(os.Environ(), "MISE_CEILING_PATHS="+filepath.Dir(abs), "MISE_TRUSTED_CONFIG_PATHS="+abs, "MISE_YES=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("mise-cloudflare init: %v", err)
	}
	pin := "local, this checkout's build"
	if version != "" {
		pin = version + ", from GitHub"
	}
	fmt.Printf("made %s from %s: %s, the Worker %s, mise-cloudflare at %s, its own git, and what init writes\n", stampTo, stampFrom, strings.Join(brought, ", "), stampedWorker, pin)
	fmt.Println("next: mise run example:e2e-serve")
}

// TestExampleDelete: mise run example:delete deletes the copy example:create stamped out, examples/new, with its own
// git and what its setup installed: mise run example:create makes it again. Only the folder: the walk deletes its
// Worker on Cloudflare itself (13-delete), and its login stays on this machine.
func TestExampleDelete(t *testing.T) {
	onlyFrom(t, "example:delete")
	if _, err := os.Stat(stampTo); os.IsNotExist(err) {
		fmt.Println(stampTo + " isn't there: nothing to delete")
		return
	}
	if err := os.RemoveAll(stampTo); err != nil {
		t.Fatal(err)
	}
	fmt.Println("deleted " + stampTo + ": mise run example:create makes it again")
}

// onlyFrom skips a test that only its own root task runs (mise sets MISE_TASK_NAME), so go test ./... never does it.
func onlyFrom(t *testing.T, task string) {
	t.Helper()
	if os.Getenv("MISE_TASK_NAME") != task {
		t.Skip("mise run " + task)
	}
}
