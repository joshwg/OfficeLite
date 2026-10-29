package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
	fileAttributeHidden                 = 0x2
	driveRemote                         = 4
	stillActive                         = 259
	processQueryLimited                 = 0x1000
	nameDisplay                         = 3 // EXTENDED_NAME_FORMAT NameDisplay
)

var (
	getDriveType              = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")
	allowSetForeground        = syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow")
	getUserNameEx             = syscall.NewLazyDLL("secur32.dll").NewProc("GetUserNameExW")
	errNoDisplayName          = errors.New("no display name")
	shareAll           uint32 = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
)

func sharingViolation(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation)
}

func createFile(path string, access, share, disposition, attrs uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(p, access, share, nil, disposition, attrs, 0)
}

// ownerLive reports whether an owner file is in use: Office keeps it open
// while the document is, so it can't be opened exclusively. One that can
// is left over from a crash. A file that can't be opened at all (no
// permission) counts as live.
func ownerLive(path string) bool {
	h, err := createFile(path, syscall.GENERIC_READ, 0, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL)
	if err != nil {
		return !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) && !errors.Is(err, syscall.ERROR_PATH_NOT_FOUND)
	}
	syscall.CloseHandle(h)
	return false
}

// inUse reports whether another program holds the file open against
// writing, as Word and Excel do with an open document: opening it for
// writing while sharing everything fails with a sharing violation. (Go's
// os.OpenFile shares reading and writing but not deleting, which would
// also trip on harmless readers.)
func inUse(path string) bool {
	h, err := createFile(path, syscall.GENERIC_WRITE, shareAll, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL)
	if err != nil {
		return sharingViolation(err)
	}
	syscall.CloseHandle(h)
	return false
}

// isNetwork reports whether path is on a network share: a UNC path
// (\\server\share\...) or a drive letter mapped to one.
func isNetwork(path string) bool {
	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if hasPrefixFold(p, `\\?\UNC\`) {
		return true
	}
	if strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) {
		p = p[4:]
	} else if strings.HasPrefix(p, `\\`) {
		return true
	}
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	root, err := syscall.UTF16PtrFromString(p[:2] + `\`)
	if err != nil {
		return false
	}
	r, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
	return r == driveRemote
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// createOwner creates the owner file hidden and keeps it open, sharing
// only reading: Office reads the name but sees the file as live.
func createOwner(path string, data []byte) (*os.File, error) {
	h, err := createFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ,
		syscall.CREATE_ALWAYS, fileAttributeHidden)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return f, nil
}

// pidAlive reports whether process pid is running.
func pidAlive(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED) // running, under another account
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if syscall.GetExitCodeProcess(h, &code) != nil {
		return true
	}
	return code == stillActive
}

// allowForeground lets process pid bring its window to the front: Windows
// only lets the foreground process (the one the user just started) do
// that, and it may pass the right on.
func allowForeground(pid int) {
	allowSetForeground.Call(uintptr(pid))
}

// displayName is the Windows user's full name (GetUserNameEx NameDisplay),
// which a domain or Microsoft account has.
func displayName() (string, error) {
	buf := make([]uint16, 256)
	n := uint32(len(buf))
	r, _, err := getUserNameEx.Call(nameDisplay, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return "", err
	}
	if s := syscall.UTF16ToString(buf[:n]); s != "" {
		return s, nil
	}
	return "", errNoDisplayName
}
