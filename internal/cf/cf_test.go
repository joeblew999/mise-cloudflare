package cf

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestMain is also a stand-in cf: with FAKE_CF_EXIT set, it exits with that code, as mise's shim for cf exits 1 where
// nothing pins cf.
func TestMain(m *testing.M) {
	if code, ok := os.LookupEnv("FAKE_CF_EXIT"); ok {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	os.Exit(m.Run())
}

// TestInstalled: cf on the PATH isn't enough, it has to run.
func TestInstalled(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	name := "cf"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, c := range []struct {
		exit string
		want bool
	}{{"1", false}, {"0", true}} {
		t.Setenv("FAKE_CF_EXIT", c.exit)
		if got := Installed(); got != c.want {
			t.Errorf("a cf that exits %s: Installed() = %v", c.exit, got)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if Installed() {
		t.Error("no cf on the PATH: Installed() = true")
	}
}

// TestStatus: a failed call's HTTP status is read off cf's error box, its line for the call, as cf 1.0.0-beta.26
// printed it for a Worker tag with no Workers Builds; the box's words decide nothing.
func TestStatus(t *testing.T) {
	box := `┌ APIError
│ [12040] No build configuration associated with that script tag was found for this account
│ 404 Not Found · HTTP /accounts/0123456789abcdef0123456789abcdef/builds/workers/0123456789abcdef0123456789abcdef
└`
	err := &Error{Args: []string{"builds", "workers", "get", "x"}, Output: box}
	if Status(err) != 404 || !NotFound(err) {
		t.Errorf("Status %d, NotFound %v", Status(err), NotFound(err))
	}
	other := &Error{Output: strings.Replace(box, "404 Not Found", "403 Forbidden", 1)}
	if Status(other) != 403 || NotFound(other) {
		t.Errorf("a 403: Status %d, NotFound %v", Status(other), NotFound(other))
	}
	if words := (&Error{Output: "the thing was not found"}); Status(words) != 0 || NotFound(words) {
		t.Errorf("words alone: Status %d, NotFound %v", Status(words), NotFound(words))
	}
}
