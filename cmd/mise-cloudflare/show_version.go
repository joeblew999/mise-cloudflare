package main

import "fmt"

// showVersion prints this build's version (mise run cloudflare:show-version).
func showVersion() { fmt.Println(buildVersion()) }
