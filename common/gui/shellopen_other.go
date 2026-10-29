// Opening files and links with the default program on Linux and macOS
// (xdg-open / open).

//go:build !windows

package gui

import (
	"os"
	"os/exec"
	"runtime"
)

// OpenWithDefault opens path in the program the OS associates with its
// type, e.g. a PDF viewer.
func OpenWithDefault(path string) error {
	return shellOpen(path)
}

// OpenURL opens a link with its default program (xdg-open or open): web,
// FTP and e-mail links, and documents and folders — never programs or
// scripts (see linkAllowed).
func OpenURL(url string) error {
	if !linkAllowed(url) {
		return errUnsafeLink
	}
	return shellOpen(url)
}

// shellOpen starts xdg-open (open on macOS) on target without waiting for
// it to finish.
func shellOpen(target string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it, or it stays a zombie
	return nil
}

// isExecutableFile reports whether path is a file with an execute bit,
// which xdg-open or open might run.
func isExecutableFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}
