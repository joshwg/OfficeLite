// Tests for loading into a window: input while a file loads, and the Open
// dialog's fallback when the platform gives no file path.

package ui

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/excel/fileio"
)

// waitLoad takes h's load result once it arrives, failing after a few
// seconds.
func (h *harness) waitLoad() {
	h.t.Helper()
	for end := time.Now().Add(5 * time.Second); h.a.loading.Load(); {
		if time.Now().After(end) {
			h.t.Fatal("the load didn't finish")
		}
		h.a.takeResult()
		h.frame()
		time.Sleep(time.Millisecond)
	}
	h.frame()
}

// savedWorkbook writes a workbook whose A1 is text to a new .xlsx and
// returns its path.
func savedWorkbook(t *testing.T, text string) string {
	t.Helper()
	src := newHarness(t)
	src.typeText(text)
	src.press(key.NameReturn, 0)
	path := filepath.Join(t.TempDir(), "Budget.xlsx")
	if err := fileio.SaveFile(src.a.wb, path, fileio.XLSX, 0); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestEditsDuringLoadIgnored types into cells, presses editing keys and
// picks editing commands while a file loads into the window: the
// placeholder workbook doesn't change, and the loaded one arrives intact
// and unmodified.
func TestEditsDuringLoadIgnored(t *testing.T) {
	path := savedWorkbook(t, "From the file")
	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	a.openPath(path)
	if !a.opening() {
		t.Fatal("not opening")
	}
	placeholder := a.wb
	h.typeText("typed while loading")
	h.press(key.NameReturn, 0)
	h.press(key.NameDeleteForward, 0)
	h.press(key.NameF11, key.ModShift) // insert a sheet
	a.menuChosen(gui.MenuEvent{Menu: menuHome, Item: homeInsertRows})
	a.menuChosen(gui.MenuEvent{Menu: menuHome, Item: homeInsertSheet})
	a.save()
	if a.wb != placeholder || a.modified || a.ed.active || a.revision != 0 || len(a.wb.Sheets) != 1 {
		t.Errorf("the placeholder was edited: modified %v, editing %v, revision %d, %d sheets",
			a.modified, a.ed.active, a.revision, len(a.wb.Sheets))
	}
	if a.back.Visible {
		t.Error("Save went to the Save As page while loading")
	}
	h.waitLoad()
	if a.path != path || a.modified || h.cell("A1").Text != "From the file" {
		t.Errorf("loaded: path %q, modified %v, A1 %q", a.path, a.modified, h.cell("A1").Text)
	}
	h.typeText("x")
	h.press(key.NameReturn, 0)
	if !a.modified {
		t.Error("typing after the load is still ignored")
	}
}

// closeTracker is a reader that records being closed.
type closeTracker struct {
	io.Reader
	closed bool
}

// Close records the close.
func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

// TestOpenReader: a file the Open dialog gave without a path is read into
// a pristine window, which then has no path and no registration of the
// file it had; a window with a workbook in it doesn't read it, closes it
// and says why; a document isn't read as a workbook (on the desktop, with
// no tabs to hand it to, it is refused).
func TestOpenReader(t *testing.T) {
	data, err := os.ReadFile(savedWorkbook(t, "From the dialog"))
	if err != nil {
		t.Fatal(err)
	}

	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	h.typeText("mine")
	h.press(key.NameReturn, 0)
	rc := &closeTracker{Reader: bytes.NewReader(data)}
	a.openReader(rc)
	runUntil(t, a.calls, func() bool { return a.errMsg != "" })
	if !rc.closed || a.loading.Load() || a.errMsg != gui.OpenHereOnlyEmpty || h.cell("A1").Text != "mine" {
		t.Errorf("modified window: closed %v, loading %v, error %q", rc.closed, a.loading.Load(), a.errMsg)
	}

	h = newHarness(t)
	a = h.a
	a.results = make(chan loadResult, 1)
	a.lk.doc = lockfile.NewDoc(func() {})
	t.Cleanup(a.lk.doc.Close)
	a.lk.doc.Set(filepath.Join(t.TempDir(), "Stale.xlsx"), false)
	a.openReader(&closeTracker{Reader: bytes.NewReader(data)})
	runUntil(t, a.calls, a.loading.Load)
	h.waitLoad()
	if a.errMsg != "" || a.path != "" || a.lk.doc.Path() != "" || h.cell("A1").Text != "From the dialog" {
		t.Errorf("pristine window: error %q, path %q, registered %q, A1 %q", a.errMsg, a.path, a.lk.doc.Path(), h.cell("A1").Text)
	}

	h = newHarness(t)
	a = h.a
	a.openReader(&closeTracker{Reader: bytes.NewReader(zipWith(t, "word/document.xml"))})
	runUntil(t, a.calls, func() bool { return a.errMsg != "" })
	if a.loading.Load() || !strings.Contains(a.errMsg, "WordLite") {
		t.Errorf("document: loading %v, error %q", a.loading.Load(), a.errMsg)
	}
}

// zipWith returns a zip holding one empty part named name, enough for
// doctype.Detect to tell its kind.
func zipWith(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create(name); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// runUntil runs calls' queued work until done reports true, failing the
// test after a while.
func runUntil(t *testing.T, calls *gui.CallQueue, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for calls.Run(); !done(); calls.Run() {
		if time.Now().After(deadline) {
			t.Fatal("the posted work never came")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSecondLoadRefused starts a load while one is under way: the second
// is refused at once instead of blocking its loader on the results
// channel, and the first arrives.
func TestSecondLoadRefused(t *testing.T) {
	path := savedWorkbook(t, "First")
	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	a.openPath(path)
	a.openPath(path)
	if a.errMsg != busyLoading {
		t.Errorf("second load: error %q", a.errMsg)
	}
	h.waitLoad()
	if a.path != path || h.cell("A1").Text != "First" {
		t.Errorf("path %q, A1 %q", a.path, h.cell("A1").Text)
	}
}
