package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gioui.org/io/event"

	"officelite/common/doctype"
)

// MaxDrop is how many files one drop opens; Office asks before opening
// many, OfficeLite opens the first ones and says so.
const MaxDrop = 20

// FileDrop takes files dragged onto a window from Explorer, which Gio
// doesn't pass on: on Windows the window accepts dropped files
// (filedrop_windows.go); elsewhere nothing is ever dropped. Feed it every
// window event and take the paths with Dropped during layout.
type FileDrop struct {
	// Window is the window the files are dropped on: it is woken after a
	// drop, and the drop target is set up on its thread. Nil in tests.
	Window DropWindow

	mu    sync.Mutex
	paths []string
	hwnd  uintptr // the window accepting drops (Windows); 0 if none
	msg   string  // the last drop's message, waiting for a load to finish
}

// DropWindow is the part of *app.Window FileDrop uses (named so that
// this file builds without Gio's platform code).
type DropWindow interface {
	Invalidate()
	Run(f func())
}

// Event watches for the native window; call it with every window event.
func (f *FileDrop) Event(e event.Event) { f.listen(e) }

// Add queues dropped paths and wakes the window.
func (f *FileDrop) Add(paths ...string) {
	if len(paths) == 0 {
		return
	}
	f.mu.Lock()
	f.paths = append(f.paths, paths...)
	f.mu.Unlock()
	if f.Window != nil {
		// Not from the window procedure, which Add is called in.
		go f.Window.Invalidate()
	}
}

// Dropped takes the paths dropped since the last call.
func (f *FileDrop) Dropped() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.paths
	f.paths = nil
	return p
}

// Take opens the files dropped since the last call (see OpenDropped)
// when ready says the window can take them; while a dialog is open a drop
// is ignored. It returns the message about files left out once busy (a
// file loading into the window, which clears the status line) is over,
// and "" otherwise.
func (f *FileDrop) Take(ready func() bool, open func(path string), busy func() bool) (errMsg string) {
	if paths := f.Dropped(); len(paths) > 0 && ready() {
		if msg := OpenDropped(paths, open); msg != "" {
			f.msg = msg
		}
	}
	if f.msg == "" || busy() {
		return ""
	}
	errMsg, f.msg = f.msg, ""
	return errMsg
}

// OpenDropped opens dropped files as File ▸ Open does, calling open (the
// window's openFile, which routes each file to the right app) for each
// document or workbook, the first MaxDrop of them. It returns the message
// for the files left out ("" when there are none); it never runs
// anything.
func OpenDropped(paths []string, open func(path string)) (errMsg string) {
	files, bad := sortDrop(paths)
	var msgs []string
	if len(bad) > 0 {
		names := bad
		if len(names) > 3 {
			names = append(names[:3:3], fmt.Sprintf("%d more", len(bad)-3))
		}
		msgs = append(msgs, "OfficeLite can't open "+strings.Join(names, ", "))
	}
	if len(files) > MaxDrop {
		msgs = append(msgs, fmt.Sprintf("Opened the first %d of %d files dropped", MaxDrop, len(files)))
		files = files[:MaxDrop]
	}
	for _, p := range files {
		open(p)
	}
	return strings.Join(msgs, "; ")
}

// sortDrop splits dropped paths into the files OfficeLite opens, without
// repeats, and the names of the rest: folders, missing files and files
// that are neither a document nor a workbook.
func sortDrop(paths []string) (files, bad []string) {
	seen := map[string]bool{}
	for _, p := range paths {
		p = trimLongPrefix(p)
		k := filepath.Clean(p)
		if runtime.GOOS == "windows" {
			k = strings.ToLower(k)
		}
		if p == "" || seen[k] {
			continue
		}
		seen[k] = true
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			bad = append(bad, dropName(p))
			continue
		}
		if kind, err := doctype.DetectFile(p); err != nil || kind == doctype.Unknown {
			bad = append(bad, dropName(p))
			continue
		}
		files = append(files, p)
	}
	return files, bad
}

// dropName is a dropped path's file name, for messages.
func dropName(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// trimLongPrefix drops the \\?\ of a long Windows path (\\?\UNC\server\x
// becomes \\server\x), so the path matches the one File ▸ Open gives for
// the same file; Go adds the prefix itself where it is needed.
func trimLongPrefix(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`) && len(p) > 6 && p[5] == ':':
		return p[len(`\\?\`):]
	}
	return p
}
