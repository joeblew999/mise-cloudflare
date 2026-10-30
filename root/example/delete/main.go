// Command delete deletes the copy example:create stamped out, examples/new, with its own git and what its setup
// installed: mise run example:create makes it again. Only the folder: the walk deletes its Worker on Cloudflare itself
// (13-delete), and its login stays on this machine. It is the root's (mise run example:delete), never part of the tool.
package main

import (
	"fmt"
	"os"
)

const dir = "examples/new"

func main() {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Println(dir + " isn't there: nothing to delete")
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "delete:", err)
		os.Exit(1)
	}
	fmt.Println("deleted " + dir + ": mise run example:create makes it again")
}
