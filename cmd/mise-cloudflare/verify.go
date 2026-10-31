package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
	"github.com/joeblew999/mise-cloudflare/internal/wrangler"
)

// verify is what the repo's GitHub workflow runs once it has checked out and installed mise: the repo's own setup and test
// tasks, when it has them, then check. Run here, it does what GitHub does. It needs nothing installed first: on a fresh
// checkout, as on GitHub, its setup is what installs the wrangler check needs, or with no setup task its lockfile's
// install.
func verify(ctx context.Context, dir string) {
	// A repo with no setup task (one mise-cloudflare didn't make) installs its dependencies its own way, by its
	// lockfile, when a fresh checkout has no wrangler yet.
	if !runTask(ctx, dir, "setup") && !(wrangler.W{Dir: dir}).Installed() {
		_, install := rules.Lockfile(dir)
		fmt.Println("$ " + install)
		f := strings.Fields(install)
		run := exec.CommandContext(ctx, f[0], f[1:]...)
		run.Dir, run.Stdin, run.Stdout, run.Stderr = dir, os.Stdin, os.Stdout, os.Stderr
		if err := run.Run(); err != nil {
			fail(fmt.Errorf("%s: %w", install, err))
		}
	}
	runTask(ctx, dir, "test")
	openRepo(ctx, dir).check(ctx, false)
}

// runTask runs the repo's own task when it has one, and says whether it had.
func runTask(ctx context.Context, dir, task string) bool {
	has := exec.CommandContext(ctx, "mise", "tasks", "info", task)
	has.Dir = dir
	if has.Run() != nil {
		return false
	}
	fmt.Println("$ mise run " + task)
	run := exec.CommandContext(ctx, "mise", "run", task)
	run.Dir, run.Stdin, run.Stdout, run.Stderr = dir, os.Stdin, os.Stdout, os.Stderr
	if err := run.Run(); err != nil {
		fail(fmt.Errorf("mise run %s: %w", task, err))
	}
	return true
}
