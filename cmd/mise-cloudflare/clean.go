package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// clean is the repo's local state undone, as delete undoes deploy's on Cloudflare: .wrangler/ (wrangler's local D1,
// KV and R2, and what each deploy and preview wrote there) and .dev.vars (serve makes it from .dev.vars.example), so
// the next serve starts from nothing. It never touches Cloudflare or the logins. It asks first, unless yes.
func (r repo) clean(yes bool) {
	var here []string
	for _, p := range []string{".wrangler", ".dev.vars"} {
		if _, err := os.Stat(filepath.Join(r.dir, p)); err == nil {
			here = append(here, p)
		}
	}
	if len(here) == 0 {
		fmt.Println("✓ nothing to clean: no .wrangler/ or .dev.vars")
		return
	}
	if !yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fail(errors.New("clean asks first, and there's no terminal to ask on: run it with --yes"))
		}
		fmt.Printf("Delete %s: the local D1, KV and R2, and your local secret values? [y/N] ", strings.Join(here, " and "))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			fail(errors.New("not cleaned"))
		}
	}
	for _, p := range here {
		must(os.RemoveAll(filepath.Join(r.dir, p)))
		fmt.Printf("✓ %s deleted\n", p)
	}
}
