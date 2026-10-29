package gui

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var shellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

// OpenWithDefault opens path in the program the OS associates with its
// type (ShellExecute "open"), e.g. a PDF viewer.
func OpenWithDefault(path string) error {
	r, err := shellOpen(path)
	if err != nil {
		return err
	}
	if r <= 32 { // ShellExecute returns an error code up to 32
		if r == 31 { // SE_ERR_NOASSOC
			return fmt.Errorf("no program is set to open %s files", strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")))
		}
		return fmt.Errorf("can't open the file (error %d)", r)
	}
	return nil
}

// OpenURL opens a link with its default program (ShellExecute "open"):
// web, FTP and e-mail links, and documents and folders — never programs
// or scripts (see linkAllowed).
func OpenURL(url string) error {
	if !linkAllowed(url) {
		return errUnsafeLink
	}
	r, err := shellOpen(url)
	if err != nil {
		return err
	}
	if r <= 32 {
		if r == 31 { // SE_ERR_NOASSOC
			return fmt.Errorf("no program is set to open %s", url)
		}
		return fmt.Errorf("can't open %s (error %d)", url, r)
	}
	return nil
}

// shellOpen runs ShellExecute's "open" verb on target and returns its
// result: a value above 32 on success.
func shellOpen(target string) (uintptr, error) {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return 0, err
	}
	file, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return 0, err
	}
	const swShowNormal = 1
	r, _, _ := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	return r, nil
}

// isExecutableFile reports whether path is a program by its file
// attributes; on Windows the extension check in linkAllowed decides.
func isExecutableFile(string) bool { return false }
