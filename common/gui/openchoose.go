// The Open dialog shared by the apps: the native one with file-type lists
// where there is one, else gioui.org/x/explorer's, which on some platforms
// gives the chosen file's data without its path.

package gui

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"officelite/common/doctype"
)

// OpenHereOnlyEmpty is the message for a file the Open dialog gave
// without a path while the window isn't empty: such a file can only be
// read into an empty window, which it replaces, and has no file to save
// back to.
const OpenHereOnlyEmpty = "Save or close what's in this window first, then open the file in an empty window."

// ChooseOpen is an Open dialog for ChooseOpen.Start.
type ChooseOpen struct {
	// Files and Filters are the native dialog and its file types (the
	// first selected); nil Files goes straight to Choose.
	Files   *FileDialog
	Filters []FileFilter
	// Choose is the fallback dialog (explorer's ChooseFile) offering
	// Exts; Declined is its error for a canceled dialog.
	Choose   func(exts ...string) (io.ReadCloser, error)
	Exts     []string
	Declined error
	Calls    *CallQueue
	// Open gets a chosen file's path (the window's openFile), Read a
	// chosen file without one, which it must close, and Fail a dialog
	// error; all on the UI goroutine. A canceled dialog calls none.
	Open func(path string)
	Read func(rc io.ReadCloser)
	Fail func(err error)
}

// Start shows the dialog on a goroutine; the outcome goes to Open, Read
// or Fail on the UI goroutine.
func (c ChooseOpen) Start() {
	go func() {
		if c.Files != nil {
			path, err := c.Files.Open(c.Filters...)
			switch {
			case err == nil:
				c.Calls.Post(func() { c.Open(path) })
				return
			case errors.Is(err, ErrCancelled):
				return
			}
		}
		rc, err := c.Choose(c.Exts...)
		switch {
		case declined(err, c.Declined):
			return
		case err != nil:
			c.Calls.Post(func() { c.Fail(err) })
			return
		}
		if f, ok := rc.(*os.File); ok && filepath.IsAbs(f.Name()) {
			path := f.Name()
			err := rc.Close()
			c.Calls.Post(func() {
				if err != nil {
					c.Fail(err)
					return
				}
				c.Open(path)
			})
			return
		}
		c.Calls.Post(func() { c.Read(rc) })
	}()
}

// ChosenData is where a file the Open dialog gave without a path (the
// browser's picker) goes, for ChosenData.Start.
type ChosenData struct {
	Own   doctype.Kind // the window's own kind: Document or Spreadsheet
	Calls *CallQueue
	// Pristine reports whether the window may take the file.
	Pristine func() bool
	// Load reads data into this window, Fail shows a failure; both on
	// the UI goroutine.
	Load func(data []byte)
	Fail func(err error)
}

// Start reads rc and closes it on a goroutine, then, on the UI goroutine,
// tells the file's kind from its contents (Own when they don't say): the
// other app's kind, or any file while the window isn't pristine, opens in
// a new browser tab of the right app (OpenDataInTab); otherwise Load gets
// it. Where there are no tabs, the other app's kind fails and the rest go
// to Load, which refuses them when the window isn't pristine.
func (c ChosenData) Start(rc io.ReadCloser) {
	go func() {
		data, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
		c.Calls.Post(func() {
			if err != nil {
				c.Fail(err)
				return
			}
			kind := doctype.Detect("", data)
			if kind != doctype.Document && kind != doctype.Spreadsheet {
				kind = c.Own
			}
			if (kind != c.Own || !c.Pristine()) && OpenDataInTab(data, kind) {
				return
			}
			if kind != c.Own {
				c.Fail(errWrongApp(kind))
				return
			}
			c.Load(data)
		})
	}()
}

// errWrongApp is the failure for a file of kind opened in the other app
// where it can't be handed over.
func errWrongApp(kind doctype.Kind) error {
	if kind == doctype.Document {
		return errors.New("it is a document; open it in WordLite")
	}
	return errors.New("it is a spreadsheet; open it in ExcelLite")
}

// CloseChosen closes a file the Open dialog gave that isn't read after
// all, logging a failure (nothing was written to it).
func CloseChosen(rc io.Closer) {
	if err := rc.Close(); err != nil {
		log.Printf("close the file chosen to open: %v", err)
	}
}
