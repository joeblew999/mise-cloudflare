//go:build !windows

package main

// longPath is p as is: only Windows has short names (paths_windows.go).
func longPath(p string) string { return p }
