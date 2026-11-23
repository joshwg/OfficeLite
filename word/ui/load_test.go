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
	"officelite/word/fileio"
)

// TestEditsDuringLoadIgnored types, presses editing keys and picks
// editing commands while a file loads into the window: the placeholder
// document doesn't change, and the loaded document arrives intact and
// unmodified.
func TestEditsDuringLoadIgnored(t *testing.T) {
	templateEnv(t)
	path := saveTemplate(t, t.TempDir(), fileio.DOCX)
	h := newHarness(t)
	d := h.d
	d.openPath(path)
	if !d.opening() {
		t.Fatal("not opening")
	}
	placeholder, version := d.doc, d.ed.Version
	h.typeText("typed while loading")
	h.press(key.NameReturn, 0)
	h.press("V", key.ModShortcut)
	h.press("B", key.ModShortcut)
	d.menuChosen(gui.MenuEvent{Menu: menuHome, Item: homeBold})
	d.menuChosen(gui.MenuEvent{Menu: menuHome, Item: homeNormal})
	d.save()
	if d.doc != placeholder || d.ed.Version != version {
		t.Errorf("the placeholder was edited: version %d → %d", version, d.ed.Version)
	}
	if d.back.Visible {
		t.Error("Save went to the Save As page while loading")
	}
	waitLoaded(t, d)
	if d.path != path || d.modified() {
		t.Errorf("loaded: path %q, modified %v", d.path, d.modified())
	}
	if got := d.ed.Para(0).Text(); got != "Company Letterhead" {
		t.Errorf("first paragraph %q", got)
	}
	h.frame() // the page view takes the focus again
	h.typeText("x")
	if !d.modified() {
		t.Error("typing after the load is still ignored")
	}
}

// TestNewBlankStillTypable checks a new document's font load doesn't
// count as opening: it can be typed into at once.
func TestNewBlankStillTypable(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, filepath.Join(t.TempDir(), "none"))
	d.newBlank()
	if d.opening() {
		t.Error("a new document's font load blocks editing")
	}
	waitLoaded(t, d)
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
// file it had; a window with a document in it doesn't read it, closes it
// and says why; a spreadsheet isn't read as a document (on the desktop,
// with no tabs to hand it to, it is refused).
func TestOpenReader(t *testing.T) {
	templateEnv(t)
	data, err := os.ReadFile(saveTemplate(t, t.TempDir(), fileio.DOCX))
	if err != nil {
		t.Fatal(err)
	}

	d, _ := templateWindow(t, t.TempDir())
	d.ed.InsertText("mine")
	before := d.ed.Version
	rc := &closeTracker{Reader: bytes.NewReader(data)}
	d.openReader(rc)
	runUntil(t, d.calls, func() bool { return d.errMsg != "" })
	if !rc.closed || d.loading.Load() || d.errMsg != gui.OpenHereOnlyEmpty || d.ed.Version != before {
		t.Errorf("modified window: closed %v, loading %v, error %q", rc.closed, d.loading.Load(), d.errMsg)
	}

	d, _ = templateWindow(t, t.TempDir())
	stale := filepath.Join(t.TempDir(), "Stale.docx")
	d.lk.doc.Set(stale, false)
	d.openReader(&closeTracker{Reader: bytes.NewReader(data)})
	runUntil(t, d.calls, d.loading.Load)
	waitLoaded(t, d)
	if d.errMsg != "" || d.path != "" || d.lk.doc.Path() != "" {
		t.Errorf("pristine window: error %q, path %q, registered %q", d.errMsg, d.path, d.lk.doc.Path())
	}
	if got := d.ed.Para(0).Text(); got != "Company Letterhead" {
		t.Errorf("first paragraph %q", got)
	}
	if f, ok := d.saveFormat(); ok {
		t.Errorf("Save writes %v without a file", f)
	}

	d, _ = templateWindow(t, t.TempDir())
	d.openReader(&closeTracker{Reader: bytes.NewReader(zipWith(t, "xl/workbook.xml"))})
	runUntil(t, d.calls, func() bool { return d.errMsg != "" })
	if d.loading.Load() || !strings.Contains(d.errMsg, "ExcelLite") {
		t.Errorf("spreadsheet: loading %v, error %q", d.loading.Load(), d.errMsg)
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
	templateEnv(t)
	path := saveTemplate(t, t.TempDir(), fileio.DOCX)
	d, _ := templateWindow(t, t.TempDir())
	d.openPath(path)
	d.openPath(path)
	if d.errMsg != busyLoading {
		t.Errorf("second load: error %q", d.errMsg)
	}
	d.openReader(&closeTracker{Reader: bytes.NewReader(nil)})
	waitLoaded(t, d)
	if d.path != path {
		t.Errorf("path %q", d.path)
	}
}
