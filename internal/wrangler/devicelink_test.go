package wrangler

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDeviceLink: wrangler's and cf's device login prompt gets the link with the code in it.
func TestDeviceLink(t *testing.T) {
	in := "Attempting to login via OAuth Device Authorization Grant...\nTo authorize Wrangler, please visit:\n\n  https://dash.cloudflare.com/oauth2/device/verify\n\nand enter the code:\n\n  WRGAiXv4\n\nYou have 5 minutes to approve this request.\n"
	var out strings.Builder
	DeviceLink(strings.NewReader(in), &out)
	if !strings.HasPrefix(out.String(), in[:40]) || !strings.Contains(out.String(), "https://dash.cloudflare.com/oauth2/device/verify?user_code=WRGAiXv4") {
		t.Errorf("got:\n%s", out.String())
	}
	if strings.Count(out.String(), "user_code=") != 1 {
		t.Error("the link is added once")
	}
}

// TestDeviceLinkOnStderr: cf prints its device login on stderr, and nothing on stdout (cf 1.0.0-beta.26).
func TestDeviceLinkOnStderr(t *testing.T) {
	in := "To authorize cf, please visit:\n\n  https://dash.cloudflare.com/oauth2/device/verify\n\nand enter the code:\n\n  yQD3UaPT\n\n"
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() // before its folder goes: Windows can't delete a file that's open
	stderr := os.Stderr
	os.Stderr = f
	err = RunDeviceLogin(exec.Command("sh", "-c", `printf '%s' "$0" >&2`, in), true)
	os.Stderr = stderr
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f.Name()); !strings.Contains(string(b), "device/verify?user_code=yQD3UaPT") {
		t.Errorf("got:\n%s", b)
	}
}
