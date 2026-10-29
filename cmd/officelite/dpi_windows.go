// Per-monitor DPI awareness for the whole process on Windows.

package main

import "syscall"

// Gio makes its window threads per-monitor DPI aware, but a process
// without a manifest stays DPI unaware. On a scaled display (e.g. 125%)
// Windows then reports WM_POINTERDOWN/UP positions in a different scale
// from WM_POINTERUPDATE, so clicks land several cells away from the cursor.
// Declaring the whole process per-monitor aware (v2) keeps them consistent.
func init() {
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")
	if proc.Find() == nil {
		proc.Call(perMonitorAwareV2)
	}
}
