//go:build !windows

package gui

import (
	"os/exec"
	"runtime"
)

// OpenWithDefault opens path in the program the OS associates with its
// type, e.g. a PDF viewer.
func OpenWithDefault(path string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, path).Start()
}
