package e2e

import "testing"

// TestExampleClean puts the repo EXAMPLE names back to the start (mise run example:clean), whatever a stopped
// example:deploy or root:deploy-every-push left: nothing of its Worker on Cloudflare (cleanCloudflare: 09-connect-github
// --off, the e2e-* branches, 13-delete), and none of its local state (15-clean). It ends as a first run starts.
func TestExampleClean(t *testing.T) {
	needAccount(t)
	r := example(t)
	r.mustRootStep(nil, "12-login")
	r.cleanCloudflare()
	r.mustRootStep(nil, "15-clean")
}
