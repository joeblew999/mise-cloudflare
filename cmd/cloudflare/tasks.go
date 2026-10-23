package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/joeblew999/cloudflare-zero/mise"
)

// version is this build's version: set by the release build (goreleaser's -X main.version), else the module version Go
// recorded (go tool, go install), else "(devel)".
var version = ""

func buildVersion() string {
	if version != "" {
		return strings.TrimPrefix(version, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "(devel)"
}

const toolPath = "github.com/joeblew999/cloudflare-zero/cmd/cloudflare"

// goTool reports whether the repo at dir runs cloudflare-zero with go tool: its go.mod has the tool line.
func goTool(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && regexp.MustCompile(`(?m)^\s*tool\s+(\(\s*)?`+regexp.QuoteMeta(toolPath)+`\s*$|^\s+`+regexp.QuoteMeta(toolPath)+`\s*$`).Match(b)
}

var (
	taskStart = regexp.MustCompile(`(?m)^\["cloudflare:[^"]+"\]\n`)
	toolsLine = regexp.MustCompile(`(?m)^tools = \{ `)
)

// tasksFor is the tasks file for the repo at dir. A repo with go.mod's tool line runs go tool cloudflare, at go.mod's
// version, as the file is embedded. Any other repo needs no Go: its tasks run the cloudflare binary, each pinning this
// release (mise's github backend downloads it from the repository's releases), as a task pins any tool it runs.
func tasksFor(dir string) ([]byte, error) {
	if goTool(dir) {
		return mise.Tasks, nil
	}
	v := buildVersion()
	if v == "(devel)" || strings.Contains(v, "-0.") {
		return nil, errors.New("this is a development build, which no release matches: in a repo without go.mod's tool line, use a released cloudflare (mise use github:joeblew999/cloudflare-zero)")
	}
	pin := `"github:joeblew999/cloudflare-zero" = "` + v + `"`
	b := bytes.ReplaceAll(mise.Tasks, []byte("go tool cloudflare"), []byte("cloudflare"))
	// Each task pins the release: in its tools table if it has one, else in a new one after its name.
	heads := taskStart.FindAllIndex(b, -1)
	var out []byte
	for i, h := range heads {
		if i == 0 {
			out = append(out, b[:h[0]]...)
		}
		end := len(b)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		task := b[h[1]:end]
		if loc := toolsLine.FindIndex(task); loc != nil {
			task = append(slices.Clone(task[:loc[1]]), append([]byte(pin+", "), task[loc[1]:]...)...)
			out = append(append(out, b[h[0]:h[1]]...), task...)
		} else {
			out = append(append(append(out, b[h[0]:h[1]]...), "tools = { "+pin+" }\n"...), task...)
		}
	}
	b = out
	return b, nil
}
