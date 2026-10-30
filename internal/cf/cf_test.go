package cf

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
