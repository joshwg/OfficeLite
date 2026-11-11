// Tests for Save As through the shared gui.SaveAs: onto a file open
// elsewhere, and a write that fails.

package ui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/excel/fileio"
)

// runCalls runs h's frames (and so its posted calls) until done reports
// true.
func (h *harness) runCalls(done func() bool) {
	h.t.Helper()
	for end := time.Now().Add(5 * time.Second); !done(); {
		if time.Now().After(end) {
			h.t.Fatal("timed out")
		}
		h.a.calls.Run()
		time.Sleep(time.Millisecond)
	}
}

// pickedJob is a Save As of a's workbook as .xlsx whose native dialog
// picks path, the file being found as st.
func pickedJob(a *App, path string, st lockfile.Status) gui.SaveAs[saveSnap] {
	j := a.saveAsJob(fileio.XLSX, false)
	j.Save = func(string, ...gui.FileFilter) (string, error) { return path, nil }
	j.Check = func(string) lockfile.Status { return st }
	return j
}

// TestSaveAsOntoLockedFile: Save As onto a file someone else has open
// asks first; Cancel writes nothing and keeps the window's (lack of a)
// file; Save Anyway writes it and takes it, with AutoSave blocked as for
// Open Anyway.
func TestSaveAsOntoLockedFile(t *testing.T) {
	h := newHarness(t)
	a := h.a
	a.lk.doc = lockfile.NewDoc(func() {})
	t.Cleanup(a.lk.doc.Close)
	h.typeText("mine")
	h.press(key.NameReturn, 0)
	path := filepath.Join(t.TempDir(), "Theirs.xlsx")
	if err := os.WriteFile(path, []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := lockfile.Status{InUse: true}

	pickedJob(a, path, st).Start()
	h.runCalls(func() bool { return a.dialog.Visible })
	if b, _ := os.ReadFile(path); string(b) != "theirs" {
		t.Fatalf("written before asking: %q", b)
	}
	h.press(key.NameEscape, 0) // Cancel
	h.runCalls(func() bool { return !a.saving.Load() })
	if b, _ := os.ReadFile(path); string(b) != "theirs" || a.path != "" || a.lk.doc.Path() != "" {
		t.Errorf("after Cancel: file %q, path %q, registered %q", b, a.path, a.lk.doc.Path())
	}

	pickedJob(a, path, st).Start()
	h.runCalls(func() bool { return a.dialog.Visible })
	a.dialog.Hide()
	answer := a.answer
	a.answer = nil
	answer(0) // Save Anyway
	h.runCalls(func() bool { return !a.saving.Load() })
	if a.path != path || a.lk.doc.Path() != path || !a.as.Elsewhere || a.modified || a.errMsg != "" {
		t.Errorf("after Save Anyway: path %q, registered %q, elsewhere %v, modified %v, error %q",
			a.path, a.lk.doc.Path(), a.as.Elsewhere, a.modified, a.errMsg)
	}
	if got := fileA1(t, path); got != "mine" {
		t.Errorf("file A1 %q", got)
	}
}

// TestSaveAsFailedWrite: a Save As whose write fails leaves the file the
// dialog chose as it was, with no temporary file beside it, and the
// window keeps its workbook unsaved.
func TestSaveAsFailedWrite(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("mine")
	h.press(key.NameReturn, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, "Old.xlsx")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := pickedJob(a, path, lockfile.Status{})
	j.Write = func(_ saveSnap, w io.Writer) error {
		if _, err := io.WriteString(w, "half"); err != nil {
			return err
		}
		return errors.New("disk full")
	}
	j.Start()
	h.runCalls(func() bool { return !a.saving.Load() })
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Errorf("target now %q", b)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("folder holds %v", ents)
	}
	if a.path != "" || !a.modified || a.errMsg != "Save failed: disk full" {
		t.Errorf("path %q, modified %v, error %q", a.path, a.modified, a.errMsg)
	}
}
