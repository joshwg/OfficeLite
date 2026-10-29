// FileDrop on Windows: a subclassed window procedure that takes
// WM_DROPFILES.

package gui

import (
	"sync"
	"syscall"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
)

// Files dropped from Explorer arrive as WM_DROPFILES once a window calls
// DragAcceptFiles. Gio's window procedure ignores the message, so the
// window is subclassed: one procedure (callbacks are a limited resource)
// takes WM_DROPFILES for every window and hands everything else to Gio's.

var (
	shell32                     = syscall.NewLazyDLL("shell32.dll")
	dragAcceptFiles             = shell32.NewProc("DragAcceptFiles")
	dragQueryFileW              = shell32.NewProc("DragQueryFileW")
	dragFinish                  = shell32.NewProc("DragFinish")
	user32                      = syscall.NewLazyDLL("user32.dll")
	getWindowLongPtrW           = user32.NewProc("GetWindowLongPtrW")
	setWindowLongPtrW           = user32.NewProc("SetWindowLongPtrW")
	callWindowProcW             = user32.NewProc("CallWindowProcW")
	defWindowProcW              = user32.NewProc("DefWindowProcW")
	changeWindowMessageFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")
)

const (
	wmNCDestroy      = 0x0082
	wmCopyGlobalData = 0x0049
	wmCopyData       = 0x004A
	wmDropFiles      = 0x0233
	gwlpWndProc      = ^uintptr(3) // GWLP_WNDPROC, -4
	msgfltAllow      = 1
)

// drops is the subclass procedure and the windows it serves.
var drops struct {
	once sync.Once
	proc uintptr
	mu   sync.Mutex
	wins map[uintptr]dropWin // by HWND
}

type dropWin struct {
	f    *FileDrop
	orig uintptr // Gio's window procedure
}

// listen attaches the window of a Win32ViewEvent so it takes dropped
// files.
func (f *FileDrop) listen(e event.Event) {
	v, ok := e.(app.Win32ViewEvent)
	if !ok || v.HWND == 0 {
		return // a closing window lets go at WM_NCDESTROY
	}
	// DragAcceptFiles changes the window's style, which sends it a
	// message: from this goroutine that would wait for the window's
	// thread, which waits for this event to be handled. Gio runs f on the
	// window's thread meanwhile.
	if f.Window != nil {
		f.Window.Run(func() { f.attach(v.HWND) })
	} else {
		f.attach(v.HWND)
	}
}

// attach makes hwnd take dropped files; it runs on the window's thread.
func (f *FileDrop) attach(hwnd uintptr) {
	drops.once.Do(func() {
		drops.proc = syscall.NewCallback(dropProc)
		drops.wins = map[uintptr]dropWin{}
	})
	drops.mu.Lock()
	if _, ok := drops.wins[hwnd]; ok {
		drops.mu.Unlock()
		return
	}
	orig, _, _ := getWindowLongPtrW.Call(hwnd, gwlpWndProc)
	if orig == 0 {
		drops.mu.Unlock()
		return
	}
	drops.wins[hwnd] = dropWin{f: f, orig: orig}
	drops.mu.Unlock()
	setWindowLongPtrW.Call(hwnd, gwlpWndProc, drops.proc)
	// An elevated OfficeLite would otherwise not hear from Explorer,
	// which runs at a lower integrity level (Windows 7+).
	for _, m := range []uintptr{wmDropFiles, wmCopyData, wmCopyGlobalData} {
		changeWindowMessageFilterEx.Call(hwnd, m, msgfltAllow, 0)
	}
	dragAcceptFiles.Call(hwnd, 1)
	f.mu.Lock()
	f.hwnd = hwnd
	f.mu.Unlock()
}

// dropProc is the subclass procedure of every window taking drops.
func dropProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	drops.mu.Lock()
	w, ok := drops.wins[hwnd]
	if ok && msg == wmNCDestroy {
		delete(drops.wins, hwnd)
	}
	drops.mu.Unlock()
	if !ok {
		r, _, _ := defWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	}
	switch msg {
	case wmDropFiles:
		paths := dropFiles(wParam)
		dragFinish.Call(wParam)
		w.f.Add(paths...)
		return 0
	case wmNCDestroy:
		// The last message: give Gio its procedure back.
		setWindowLongPtrW.Call(hwnd, gwlpWndProc, w.orig)
		w.f.mu.Lock()
		w.f.hwnd = 0
		w.f.mu.Unlock()
	}
	r, _, _ := callWindowProcW.Call(w.orig, hwnd, msg, wParam, lParam)
	return r
}

// dropFiles lists the paths in an HDROP.
func dropFiles(hdrop uintptr) []string {
	n, _, _ := dragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	paths := make([]string, 0, n)
	for i := uintptr(0); i < n; i++ {
		// The length without the NUL; long (\\?\) paths can pass MAX_PATH.
		l, _, _ := dragQueryFileW.Call(hdrop, i, 0, 0)
		if l == 0 {
			continue
		}
		buf := make([]uint16, l+1)
		if r, _, _ := dragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
			continue
		}
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	return paths
}
