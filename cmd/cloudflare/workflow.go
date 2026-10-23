package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/mise-cloudflare/config"
	"github.com/joeblew999/mise-cloudflare/mise"
)

// workflowFor is the GitHub Actions workflow for the Worker in dir, and where it goes: the repository's
// .github/workflows/cloudflare.yml, or cloudflare-<folder>.yml for a Worker in a subfolder (a monorepo has one each).
// With wf.Deploy, it deploys too: production from wf.Branch (default main), and a preview per pull request.
func workflowFor(ctx context.Context, dir string, wf config.Workflow) (path string, b []byte, err error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", nil, err
	}
	top, _ := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	real, _ := filepath.EvalSymlinks(dir)
	rel, err := filepath.Rel(top, real)
	if err != nil {
		return "", nil, err
	}
	rel = filepath.ToSlash(rel)
	name := "cloudflare"
	if rel != "." {
		name += "-" + strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(rel), "-"), "-")
	}
	tmpl := string(mise.Workflow)
	if wf.Deploy {
		tmpl += string(mise.WorkflowDeploy)
	}
	branch := wf.Branch
	if branch == "" {
		branch = "main"
	}
	b = []byte(strings.NewReplacer("VERSION", buildVersion(), "NAME", name, "DIR", rel, "BRANCH", branch).Replace(tmpl))
	return filepath.Join(top, ".github", "workflows", name+".yml"), b, nil
}
