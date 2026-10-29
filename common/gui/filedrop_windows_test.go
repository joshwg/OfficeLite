// Windows tests for FileDrop: HDROP parsing and subclassing a real window.

package gui

import (
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	globalAlloc    = kernel32.NewProc("GlobalAlloc")
	globalLock     = kernel32.NewProc("GlobalLock")
	globalUnlock   = kernel32.NewProc("GlobalUnlock")
	moveMemory     = kernel32.NewProc("RtlMoveMemory")
	createWindowEx = user32.NewProc("CreateWindowExW")
	destroyWindow  = user32.NewProc("DestroyWindow")
	sendMessage    = user32.NewProc("SendMessageW")
)

// dropHandle builds the HDROP Explorer sends: a DROPFILES header and the
// paths as a double-NUL-terminated UTF-16 list, in global memory.
func dropHandle(t *testing.T, paths []string) uintptr {
	const header = 20 // DROPFILES: pFiles, pt, fNC, fWide
	var list []uint16
	for _, p := range paths {
		list = append(list, utf16.Encode([]rune(p))...)
		list = append(list, 0)
	}
	list = append(list, 0)
	const gmemMoveable, gmemZeroInit = 0x0002, 0x0040
	h, _, err := globalAlloc.Call(gmemMoveable|gmemZeroInit, uintptr(header+2*len(list)))
	if h == 0 {
		t.Fatal("GlobalAlloc:", err)
	}
	p, _, err := globalLock.Call(h)
	if p == 0 {
		t.Fatal("GlobalLock:", err)
	}
	b := make([]byte, header+2*len(list))
	b[0] = header // pFiles
	b[16] = 1     // fWide
	for i, c := range list {
		b[header+2*i], b[header+2*i+1] = byte(c), byte(c>>8)
	}
	moveMemory.Call(p, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	globalUnlock.Call(h)
	return h
}

// TestDropFiles parses an HDROP of Unicode, long and many paths.
func TestDropFiles(t *testing.T) {
	long := `\\?\C:\` + strings.Repeat(`dossier très long\`, 20) + "Budget.xlsx"
	paths := []string{`C:\Users\josh\Documents\Données ✓.xlsx`, long, `\\server\share\日本語.docx`}
	for i := range 30 {
		paths = append(paths, `C:\many\file`+string(rune('a'+i%26))+".csv")
	}
	h := dropHandle(t, paths)
	defer dragFinish.Call(h)
	if got := dropFiles(h); !slices.Equal(got, paths) {
		t.Errorf("got %d paths %q, want %q", len(got), got, paths)
	}
}

// TestFileDropWindow subclasses a real window: a WM_DROPFILES sent to it
// queues the paths, and destroying it gives the window back its own
// procedure and forgets it.
func TestFileDropWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := syscall.UTF16PtrFromString("STATIC")
	hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 100, 100, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal("CreateWindowEx:", err)
	}
	var f FileDrop
	f.attach(hwnd)
	f.attach(hwnd) // a second view event changes nothing
	if p, _, _ := getWindowLongPtrW.Call(hwnd, gwlpWndProc); p != drops.proc || f.hwnd != hwnd {
		t.Fatalf("not subclassed: proc %#x, hwnd %#x", p, f.hwnd)
	}
	const wsExAcceptFiles = 0x10
	if ex, _, _ := getWindowLongPtrW.Call(hwnd, ^uintptr(19)); ex&wsExAcceptFiles == 0 { // GWL_EXSTYLE, -20
		t.Errorf("no WS_EX_ACCEPTFILES: %#x", ex)
	}
	paths := []string{`C:\a\Book1.xlsx`, `D:\b\Résumé.docx`}
	sendMessage.Call(hwnd, wmDropFiles, dropHandle(t, paths), 0) // the window frees it
	if got := f.Dropped(); !slices.Equal(got, paths) {
		t.Errorf("dropped %q, want %q", got, paths)
	}
	// Other messages reach the window's own procedure.
	text, _ := syscall.UTF16PtrFromString("hello")
	const wmSetText, wmGetTextLength = 0x000C, 0x000E
	sendMessage.Call(hwnd, wmSetText, 0, uintptr(unsafe.Pointer(text)))
	if n, _, _ := sendMessage.Call(hwnd, wmGetTextLength, 0, 0); n != 5 {
		t.Errorf("text length %d through the subclass, want 5", n)
	}

	destroyWindow.Call(hwnd)
	drops.mu.Lock()
	_, kept := drops.wins[hwnd]
	drops.mu.Unlock()
	if kept || f.hwnd != 0 {
		t.Errorf("after destroy: still listed %v, hwnd %#x", kept, f.hwnd)
	}
}
