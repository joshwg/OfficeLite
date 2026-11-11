// Save As and export plumbing shared by the apps: asking for a file on a
// goroutine, checking that it isn't open elsewhere, writing a snapshot of
// the document in the background (through a temporary file renamed over
// the target, where the dialog gives a path) and handing the result back
// to the UI goroutine, with the window's saving flag set throughout.

package gui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"officelite/common/atomicfile"
	"officelite/common/lockfile"
)

// SaveAs is a Save As or an export of a document whose snapshot is an S
// (a clone of the document, with whatever the app needs to finish).
type SaveAs[S any] struct {
	Name string // the file name the dialog suggests, perhaps with a folder
	// Locate, when set, replaces Name with where to suggest saving; it
	// runs off the UI goroutine, so it may touch the disk.
	Locate func(name string) string
	// Save is the native Save As dialog (FileDialog.Save) with Filters,
	// which returns the chosen path without creating the file; nil, or
	// its ErrNoFileDialog, falls back to Create.
	Save    func(name string, filters ...FileFilter) (string, error)
	Filters []FileFilter
	// Create is the fallback Save dialog (explorer's CreateFile): it
	// creates the chosen file. Declined is its error for a canceled
	// dialog.
	Create   func(name string) (io.WriteCloser, error)
	Declined error
	Calls    *CallQueue
	// Busy is the window's saving flag: set from Start until the result
	// is handled on the UI goroutine, so the window isn't taken for a
	// pristine one (and its document isn't replaced) in between.
	Busy *atomic.Bool
	// Snapshot clones the document on the UI goroutine once the file is
	// chosen; Write writes the clone, in the background.
	Snapshot func() S
	Write    func(s S, w io.Writer) error
	// Ask, for a Save As (nil for an export, which takes over nothing),
	// asks on the UI goroutine whether to save over a file that Check
	// (nil: lockfile.Check) found open elsewhere, and calls answer with
	// the reply. Declined, nothing more is written and Cancel runs.
	Ask   func(st lockfile.Status, path string, answer func(ok bool))
	Check func(path string) lockfile.Status
	// Done gets the outcome on the UI goroutine, Busy already cleared.
	// s is the zero S when the dialog failed before the snapshot was
	// taken.
	Done func(s S, r SaveAsResult)
	// Cancel runs on the UI goroutine, Busy cleared, when the user
	// cancels the dialog or declines to save over a file open elsewhere.
	Cancel func()
}

// SaveAsResult is how a Save As or export ended.
type SaveAsResult struct {
	// Path is the file written: its full path when the dialog gave one,
	// else the suggested name.
	Path string
	Err  error
	// Shared: the file was open elsewhere and the user chose to save over
	// it anyway; like a file opened with Open Anyway, it is someone
	// else's to save too (no AutoSave).
	Shared bool
}

// Start sets Busy and asks for the file on a goroutine; the rest follows
// on its own (a lock check, the write, then Done or Cancel). The caller
// has checked that Busy isn't set.
func (j SaveAs[S]) Start() {
	j.Busy.Store(true)
	go func() {
		name := j.Name
		if j.Locate != nil {
			name = j.Locate(name)
		}
		if j.Save != nil {
			path, err := j.Save(name, j.Filters...)
			switch {
			case err == nil:
				j.chosen(path) // nothing written yet: checked first
				return
			case errors.Is(err, ErrCancelled):
				j.Calls.Post(j.cancel)
				return
			case !errors.Is(err, ErrNoFileDialog) || j.Create == nil:
				j.Calls.Post(func() { j.fail(name, err) })
				return
			}
		}
		wc, err := j.Create(name)
		switch {
		case declined(err, j.Declined):
			j.Calls.Post(j.cancel)
			return
		case err != nil:
			j.Calls.Post(func() { j.fail(name, err) })
			return
		}
		if f, ok := wc.(*os.File); ok && filepath.IsAbs(f.Name()) {
			// A real file: written whole through a temporary file instead,
			// so a failed write doesn't leave it half written.
			if err := wc.Close(); err != nil {
				j.Calls.Post(func() { j.fail(f.Name(), err) })
				return
			}
			j.chosen(f.Name())
			return
		}
		j.Calls.Post(func() { j.WriteTo(wc, name) })
	}()
}

// chosen runs off the UI goroutine once the dialog gave path: for a Save
// As it checks whether the file is open elsewhere, then has the UI
// goroutine ask (when it is) and write.
func (j SaveAs[S]) chosen(path string) {
	var st lockfile.Status
	if j.Ask != nil {
		check := j.Check
		if check == nil {
			check = lockfile.Check
		}
		st = check(path)
	}
	j.Calls.Post(func() {
		if !st.Locked() {
			j.writePath(path, false)
			return
		}
		j.Ask(st, path, func(ok bool) {
			if !ok {
				j.cancel()
				return
			}
			j.writePath(path, true)
		})
	})
}

// writePath runs on the UI goroutine with Busy set: it takes the snapshot
// and writes it to path in the background through a temporary file
// renamed over it (common/atomicfile), so a failed write leaves path as
// it was, then posts the outcome to Done.
func (j SaveAs[S]) writePath(path string, shared bool) {
	s := j.Snapshot()
	go func() {
		err := atomicfile.Write(path, 0o644, func(w io.Writer) error { return j.Write(s, w) })
		j.Calls.Post(func() { j.finish(s, SaveAsResult{Path: path, Err: err, Shared: shared && err == nil}) })
	}()
}

// WriteTo runs on the UI goroutine with Busy set: it takes the snapshot
// and writes it to wc (a file the dialog created as name, which has no
// path to write through) in the background, so a large document doesn't
// block input, then closes wc and posts the outcome to Done.
func (j SaveAs[S]) WriteTo(wc io.WriteCloser, name string) {
	s := j.Snapshot()
	go func() {
		err := j.Write(s, wc)
		if cerr := wc.Close(); err == nil {
			err = cerr
		}
		path := name
		if file, ok := wc.(*os.File); ok && err == nil {
			path = file.Name()
		}
		j.Calls.Post(func() { j.finish(s, SaveAsResult{Path: path, Err: err}) })
	}()
}

// cancel clears Busy and runs Cancel, on the UI goroutine.
func (j SaveAs[S]) cancel() {
	j.Busy.Store(false)
	if j.Cancel != nil {
		j.Cancel()
	}
}

// fail ends the job with err before anything was written, on the UI
// goroutine.
func (j SaveAs[S]) fail(path string, err error) {
	var none S
	j.finish(none, SaveAsResult{Path: path, Err: err})
}

// finish clears Busy and hands the outcome to Done, on the UI goroutine.
func (j SaveAs[S]) finish(s S, r SaveAsResult) {
	j.Busy.Store(false)
	j.Done(s, r)
}

// FormatFilter is the Save As dialog's file type for a format the app
// labels label (e.g. "Word Document (*.docx)") with extension ext.
func FormatFilter(label, ext string) FileFilter {
	return FileFilter{Name: strings.TrimSuffix(label, " (*"+ext+")"), Exts: []string{ext}}
}
