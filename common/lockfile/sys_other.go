// Non-Windows stand-ins for the OS hooks: owner-file liveness by age,
// no share-mode checks, no owner files acquired (createOwner works but
// isNetwork is always false).

//go:build !windows

package lockfile

import (
	"errors"
	"officelite/common/limits"
	"os"
	"syscall"
)

var errUnsupported = errors.New("owner files are written on Windows only")

// ownerLive guesses from the owner file's age: there are no mandatory
// locks to ask.
func ownerLive(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && now().Sub(fi.ModTime()) < limits.OwnerFileStaleAfter
}

// inUse can't tell without Windows' share modes.
func inUse(string) bool { return false }

// sharingViolation is always false: there are no share modes to violate.
func sharingViolation(error) bool { return false }

// isNetwork is always false: owner files are only written on Windows.
func isNetwork(string) bool { return false }

// createOwner creates the owner file exclusively (O_EXCL: an existing
// one fails with os.IsExist and is left alone) and keeps it open. Nothing
// calls it outside tests here: isNetwork is false, so no owner file is
// acquired.
func createOwner(path string, data []byte) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return f, nil
}

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
