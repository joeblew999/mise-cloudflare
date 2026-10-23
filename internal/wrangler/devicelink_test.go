package wrangler

import (
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
