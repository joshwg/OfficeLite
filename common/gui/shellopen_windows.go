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
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const swShowNormal = 1
	r, _, _ := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	if r <= 32 { // ShellExecute returns an error code up to 32
		if r == 31 { // SE_ERR_NOASSOC
			return fmt.Errorf("no program is set to open %s files", strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")))
		}
		return fmt.Errorf("can't open the file (error %d)", r)
	}
	return nil
}
