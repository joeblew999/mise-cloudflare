module github.com/joeblew999/mise-cloudflare/e2e/testdata/actions

go 1.26.0

toolchain go1.27.3

tool github.com/joeblew999/mise-cloudflare/cmd/cloudflare

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/joeblew999/mise-cloudflare v0.0.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.47.0 // indirect
)

replace github.com/joeblew999/mise-cloudflare => ../../../
