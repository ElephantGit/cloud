// checkformat makes formatting a failing gate instead of merely printing a diff.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Only this module's packages: the nested v2-server module has its own gate.
	dirs, e := exec.Command("go", "list", "-f", "{{.Dir}}", "./...").Output()
	if e != nil {
		fmt.Fprintln(os.Stderr, "go list:", e)
		os.Exit(1)
	}
	// One directory per line; a path may contain spaces.
	args := append([]string{"tool", "gofumpt", "-l", "-extra"}, strings.Split(strings.TrimSpace(string(dirs)), "\n")...)
	output, e := exec.Command("go", args...).CombinedOutput()
	if e != nil {
		fmt.Fprintln(os.Stderr, string(output), e)
		os.Exit(1)
	}
	if len(output) != 0 {
		fmt.Fprintln(os.Stderr, "Run task format:\n"+string(output))
		os.Exit(1)
	}
}
