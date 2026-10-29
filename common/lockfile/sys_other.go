//go:build !windows

package lockfile

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// staleAfter is how old an owner file must be to count as left over where
// the OS can't tell whether Office has it open.
const staleAfter = 24 * time.Hour

var errUnsupported = errors.New("owner files are written on Windows only")

// ownerLive guesses from the owner file's age: there are no mandatory
// locks to ask.
func ownerLive(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && now().Sub(fi.ModTime()) < staleAfter
}

// inUse can't tell without Windows' share modes.
func inUse(string) bool { return false }

func sharingViolation(error) bool { return false }

// isNetwork: owner files are only written on Windows.
func isNetwork(string) bool { return false }

func createOwner(string, []byte) (*os.File, error) { return nil, errUnsupported }

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func allowForeground(int) {}

func displayName() (string, error) { return "", errUnsupported }
