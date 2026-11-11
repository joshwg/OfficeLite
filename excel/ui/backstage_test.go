package ui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/common/gui"
	"officelite/excel/fileio"
	"officelite/excel/model"
)

type blockingWriteCloser struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriteCloser) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return len(p), nil
}

func (w *blockingWriteCloser) Close() error { return nil }

type failingWriteCloser struct{ err error }

func (w failingWriteCloser) Write([]byte) (int, error) { return 0, w.err }
func (w failingWriteCloser) Close() error              { return nil }

// saved finishes a Save As or export of wb (the window's workbook, as it
// is now) to path in format f, as the background write would.
func (a *App) saved(wb *model.Workbook, f fileio.Format, path string, export bool, err error) {
	a.savedSnapshot(saveSnap{owner: wb, wb: wb, revision: a.revision, sheet: a.active, f: f, export: export}, gui.SaveAsResult{Path: path, Err: err})
}

func TestSaveAsWritesInBackgroundAndKeepsLaterEdits(t *testing.T) {
	h := newHarness(t)
	a := h.a
	w := &blockingWriteCloser{started: make(chan struct{}), release: make(chan struct{})}
	a.saving.Store(true)
	a.saveAsJob(fileio.XLSX, false).WriteTo(w, "saved.xlsx")

	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background workbook write did not start")
	}

	h.typeText("later")
	h.press(key.NameReturn, 0)
	if !a.modified {
		t.Fatal("editing was blocked while Save As was writing")
	}

	close(w.release)
	deadline := time.Now().Add(5 * time.Second)
	for a.saving.Load() && time.Now().Before(deadline) {
		h.frame()
		time.Sleep(time.Millisecond)
	}
	if a.saving.Load() {
		t.Fatal("Save As did not finish")
	}
	if !a.modified {
		t.Fatal("Save As marked edits made during the write as saved")
	}
	if got := a.wb.Name; got != "saved.xlsx" {
		t.Errorf("workbook name = %q, want %q", got, "saved.xlsx")
	}
}

func TestSaveAsWriteFailureReenablesUIAndReportsError(t *testing.T) {
	h := newHarness(t)
	a := h.a
	wantErr := errors.New("disk full")
	a.saving.Store(true)
	a.saveAsJob(fileio.XLSX, false).WriteTo(failingWriteCloser{err: wantErr}, "saved.xlsx")

	deadline := time.Now().Add(5 * time.Second)
	for a.saving.Load() && time.Now().Before(deadline) {
		h.frame()
		time.Sleep(time.Millisecond)
	}
	if a.saving.Load() {
		t.Fatal("Save As did not finish")
	}
	if !strings.Contains(a.errMsg, wantErr.Error()) {
		t.Errorf("save error = %q, want it to contain %q", a.errMsg, wantErr)
	}
}

// TestExportAbandonsClose: after "Save changes?" → Save sends an untitled
// workbook to the Save As page, an export leaves the backstage without
// saving; a later Save As must not close the window.
func TestExportAbandonsClose(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("x")
	h.press(key.NameReturn, 0)
	a.closing = true
	a.save() // untitled: the Save As page
	a.saved(a.wb, fileio.CSV, "out.csv", true, nil)
	if a.closing {
		t.Error("closing still set after an export")
	}
	a.closing = true
	a.handleBackstage(gui.BackstageEvent{Action: gui.ActNew, Template: len(templates)}) // out of range: ignored
	if a.closing {
		t.Error("closing still set after New")
	}
}

// TestNotPristineWhileSaving checks that an untouched new workbook isn't
// replaced by an opened file while its Save As or export runs, whose
// result would otherwise go to the file that replaced it.
func TestNotPristineWhileSaving(t *testing.T) {
	h := newHarness(t)
	a := h.a
	a.path, a.modified = "", false
	if !a.pristine() {
		t.Fatal("a new workbook isn't pristine")
	}
	a.saving.Store(true)
	defer a.saving.Store(false)
	if a.pristine() {
		t.Error("pristine while a Save As runs")
	}
}
