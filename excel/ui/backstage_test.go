package ui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/excel/fileio"
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

func TestSaveAsWritesInBackgroundAndKeepsLaterEdits(t *testing.T) {
	h := newHarness(t)
	a := h.a
	w := &blockingWriteCloser{started: make(chan struct{}), release: make(chan struct{})}
	a.saving.Store(true)
	a.writeAsFile(w, "saved.xlsx", fileio.XLSX, false)

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
	a.writeAsFile(failingWriteCloser{err: wantErr}, "saved.xlsx", fileio.XLSX, false)

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
