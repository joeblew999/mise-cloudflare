// The tests the root's older tasks still run (template:e2e, example:e2e, example:deploy, root:deploy-every-push),
// until TestWalk replaces them.

package e2e

import (
	"os"
	"testing"
)

// TestTemplateE2e: mise run template:e2e, the stage serve on the template.
func TestTemplateE2e(t *testing.T) {
	if os.Getenv("E2E") != "1" {
		t.Skip("mise run template:e2e")
	}
	r := repoAt(t, os.Getenv("REPO_PATH"))
	r.miseSetUp()
	walkSteps(t, r, serve)
}

// TestExampleE2e: mise run example:e2e, the stage serve on a stamped copy.
func TestExampleE2e(t *testing.T) {
	if os.Getenv("E2E") != "1" {
		t.Skip("mise run example:e2e")
	}
	r := repoAt(t, os.Getenv("REPO_PATH"))
	r.miseSetUp()
	walkSteps(t, r, serve)
}

// TestExampleDeploy: mise run example:deploy, the stage deploy on the repo REPO_PATH names.
func TestExampleDeploy(t *testing.T) {
	if os.Getenv("CLOUDFLARE_E2E") != "1" {
		t.Skip("needs a Cloudflare account: mise run example:deploy")
	}
	r := repoAt(t, os.Getenv("REPO_PATH"))
	r.miseSetUp()
	walkSteps(t, r, deploy)
}

// TestRootDeployEveryPush: mise run root:deploy-every-push, the stage connect-github on the template, in a checkout of
// this repo at HEAD.
func TestRootDeployEveryPush(t *testing.T) {
	if os.Getenv("CLOUDFLARE_E2E") != "1" {
		t.Skip("needs a Cloudflare account: mise run root:deploy-every-push")
	}
	r := repoAt(t, "templates").checkoutOfHead()
	r.miseSetUp()
	walkSteps(t, r, connectGithub)
}
