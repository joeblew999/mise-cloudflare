module github.com/joeblew999/cloudflare-zero/example

go 1.26.0

toolchain go1.27.3

tool github.com/joeblew999/cloudflare-zero/cmd/cloudflare

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/joeblew999/cloudflare-zero v0.0.0 // indirect
)

// A repo elsewhere drops this line and gets cloudflare-zero from GitHub: go get -tool github.com/joeblew999/cloudflare-zero/cmd/cloudflare@<version>
replace github.com/joeblew999/cloudflare-zero => ../
