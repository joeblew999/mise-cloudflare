package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joeblew999/mise-cloudflare/internal/rules"
)

func (r repo) check(ctx context.Context, jsonOut bool) {
	problems, err := rules.Check(r.dir, r.wc)
	if err != nil {
		fail(err)
	}
	problems = append(problems, r.stale(ctx)...)
	if jsonOut {
		emit(struct {
			OK       bool     `json:"ok"`
			Problems []string `json:"problems"`
		}{len(problems) == 0, append([]string{}, problems...)})
	} else {
		if len(problems) == 0 {
			fmt.Println("rules: pass")
		}
		for _, p := range problems {
			fmt.Println("FAIL " + p)
		}
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
}
