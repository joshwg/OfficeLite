// Non-Windows stand-ins for the OS hooks: owner-file liveness by age,
// no share-mode checks, no owner files written.

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

// sharingViolation is always false: there are no share modes to violate.
func sharingViolation(error) bool { return false }

// isNetwork is always false: owner files are only written on Windows.
func isNetwork(string) bool { return false }

// createOwner always fails with errUnsupported.
func createOwner(string, []byte) (*os.File, error) { return nil, errUnsupported }

// pidAlive reports whether process pid is running (signal 0 succeeds, or
// is refused for a process of another user).
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// allowForeground does nothing: only Windows restricts raising windows.
func allowForeground(int) {}

// displayName always fails with errUnsupported, so UserName falls back to
// the account name.
func displayName() (string, error) { return "", errUnsupported }
