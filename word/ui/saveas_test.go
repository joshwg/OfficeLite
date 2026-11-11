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

	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/word/fileio"
	"officelite/word/model"
)

// runCalls runs d's posted calls until done reports true.
func runCalls(t *testing.T, d *window, done func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !done(); {
		if time.Now().After(end) {
			t.Fatal("timed out")
		}
		d.calls.Run()
		time.Sleep(time.Millisecond)
	}
}

// pickedJob is a Save As of d's document as .docx whose native dialog
// picks path, the file being found as st.
func pickedJob(d *window, path string, st lockfile.Status) gui.SaveAs[saveSnap] {
	j := d.saveAsJob(fileio.DOCX, false)
	j.Save = func(string, ...gui.FileFilter) (string, error) { return path, nil }
	j.Check = func(string) lockfile.Status { return st }
	return j
}

// TestSaveAsOntoLockedFile: Save As onto a file someone else has open
// asks first; Cancel writes nothing and keeps the window's (lack of a)
// file; Save Anyway writes it and takes it, with AutoSave blocked as for
// Open Anyway.
func TestSaveAsOntoLockedFile(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, t.TempDir())
	d.ed.InsertText("mine")
	path := filepath.Join(t.TempDir(), "Theirs.docx")
	if err := os.WriteFile(path, []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := lockfile.Status{Owner: "Alice"}

	pickedJob(d, path, st).Start()
	runCalls(t, d, func() bool { return d.lk.dlg.Visible })
	if b, _ := os.ReadFile(path); string(b) != "theirs" {
		t.Fatalf("written before asking: %q", b)
	}
	d.lk.dlg.Hide()
	d.lockAnswer(1) // Cancel
	runCalls(t, d, func() bool { return !d.saving.Load() })
	if b, _ := os.ReadFile(path); string(b) != "theirs" || d.path != "" || d.lk.doc.Path() != "" {
		t.Errorf("after Cancel: file %q, path %q, registered %q", b, d.path, d.lk.doc.Path())
	}

	pickedJob(d, path, st).Start()
	runCalls(t, d, func() bool { return d.lk.dlg.Visible })
	d.lk.dlg.Hide()
	d.lockAnswer(0) // Save Anyway
	runCalls(t, d, func() bool { return !d.saving.Load() })
	if d.path != path || d.lk.doc.Path() != path || !d.as.Elsewhere || d.modified() || d.errMsg != "" {
		t.Errorf("after Save Anyway: path %q, registered %q, elsewhere %v, modified %v, error %q",
			d.path, d.lk.doc.Path(), d.as.Elsewhere, d.modified(), d.errMsg)
	}
	doc, err := fileio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := model.NewEditor(doc).Para(0).Text(); got != "mine" {
		t.Errorf("file holds %q", got)
	}
}

// TestSaveAsFailedWrite: a Save As whose write fails leaves the file the
// dialog chose as it was, with no temporary file beside it, and the
// window keeps its document unsaved.
func TestSaveAsFailedWrite(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, t.TempDir())
	d.ed.InsertText("mine")
	dir := t.TempDir()
	path := filepath.Join(dir, "Old.docx")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := pickedJob(d, path, lockfile.Status{})
	j.Write = func(_ saveSnap, w io.Writer) error {
		if _, err := io.WriteString(w, "half"); err != nil {
			return err
		}
		return errors.New("disk full")
	}
	j.Start()
	runCalls(t, d, func() bool { return !d.saving.Load() })
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Errorf("target now %q", b)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("folder holds %v", ents)
	}
	if d.path != "" || !d.modified() || d.errMsg != "Save failed: disk full" {
		t.Errorf("path %q, modified %v, error %q", d.path, d.modified(), d.errMsg)
	}
}
