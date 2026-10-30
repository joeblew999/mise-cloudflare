package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// verify is what the repo's GitHub workflow runs once it has checked out and installed mise: the repo's own setup and test
// tasks, when it has them, then check. Run here, it does what GitHub does.
func (r repo) verify(ctx context.Context) {
	for _, task := range []string{"setup", "test"} {
		has := exec.CommandContext(ctx, "mise", "tasks", "info", task)
		has.Dir = r.dir
		if has.Run() != nil {
			continue
		}
		fmt.Println("$ mise run " + task)
		run := exec.CommandContext(ctx, "mise", "run", task)
		run.Dir, run.Stdin, run.Stdout, run.Stderr = r.dir, os.Stdin, os.Stdout, os.Stderr
		if err := run.Run(); err != nil {
			fail(fmt.Errorf("mise run %s: %w", task, err))
		}
	}
	r.check(ctx, false)
}
