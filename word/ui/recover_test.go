package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"officelite/common/autorecover"
	"officelite/common/gui"
	"officelite/word/fileio"
)

// recoverFiles lists the Recovery directory.
func recoverFiles(dir string) []string {
	des, _ := os.ReadDir(dir)
	var out []string
	for _, de := range des {
		out = append(out, de.Name())
	}
	return out
}

// startRecover gives the harness's window an AutoRecover saver writing to
// dir every few milliseconds.
func (h *harness) startRecover(dir string) {
	d := h.d
	d.ar.saver = autorecover.NewSaver(autorecover.Word, d.calls.Post, d.recoverySnapshot)
	d.ar.saver.Dir = dir
	d.ar.saver.Interval = func() time.Duration { return 5 * time.Millisecond }
	d.ar.saver.Start()
	h.t.Cleanup(d.ar.saver.Close)
}

// waitRecover runs the window's posted calls (the timer's ticks) and
// frames until the Recovery directory has n files.
func (h *harness) waitRecover(dir string, n int) {
	h.t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		h.d.calls.Run()
		h.frame()
		h.d.ar.saver.Wait()
		if len(recoverFiles(dir)) == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("recovery files %v, want %d", recoverFiles(dir), n)
}

func (h *harness) sidecar(dir string) autorecover.Entry {
	var e autorecover.Entry
	for _, n := range recoverFiles(dir) {
		if filepath.Ext(n) == ".json" {
			b, _ := os.ReadFile(filepath.Join(dir, n))
			json.Unmarshal(b, &e)
			e.Copy = filepath.Join(dir, n[:len(n)-len(".json")]+".docx")
		}
	}
	return e
}

// TestAutoRecover: a modified document gets a recovery copy when the timer
// fires; Save deletes it, and so does closing with Don't Save.
func TestAutoRecover(t *testing.T) {
	h := newHarness(t)
	d := h.d
	dir := t.TempDir()
	h.startRecover(dir)
	time.Sleep(30 * time.Millisecond)
	h.waitRecover(dir, 0) // unmodified: nothing

	h.typeText("Unsaved words")
	h.waitRecover(dir, 2)
	e := h.sidecar(dir)
	if e.App != autorecover.Word || e.Name != "Test" || e.Path != "" || e.PID != os.Getpid() {
		t.Errorf("sidecar %+v", e)
	}
	doc, err := fileio.Open(e.Copy)
	if err != nil || len(doc.Body) == 0 {
		t.Fatalf("copy: %v", err)
	}

	// Save deletes the copy.
	d.path = filepath.Join(t.TempDir(), "Notes.docx")
	d.save()
	h.waitRecover(dir, 0)
	if d.modified() {
		t.Fatal("not saved")
	}
	// A .doc document's copy is .docx, with the format noted.
	d.path = filepath.Join(filepath.Dir(d.path), "Old.doc")
	h.typeText(" more")
	h.waitRecover(dir, 2)
	if e := h.sidecar(dir); e.Path != d.path || e.Format != ".doc" {
		t.Errorf("sidecar of the .doc document %+v", e)
	}

	// Close, Don't Save: the copy goes.
	d.confirm.show = true
	h.frame()
	d.confirm.dontSave.Click()
	h.frame()
	h.waitRecover(dir, 0)
	if !d.allowClose {
		t.Error("Don't Save didn't close")
	}
}

// TestOpenRecovered: a recovered document shows [Recovered], is modified,
// and Save goes to Save As with the original folder, name and format.
func TestOpenRecovered(t *testing.T) {
	dir := t.TempDir()
	src := newHarness(t)
	src.typeText("Recovered text")
	e := autorecover.Entry{Path: filepath.Join(t.TempDir(), "Letter.doc"), Name: "Letter.doc", App: autorecover.Word,
		Format: ".doc", Copy: filepath.Join(dir, "Letter~1.docx"), Sidecar: filepath.Join(dir, "Letter~1.json")}
	if err := fileio.SaveFile(src.d.doc, e.Copy, fileio.DOCX); err != nil {
		t.Fatal(err)
	}
	autorecover.Release(e)

	h := newHarness(t)
	d := h.d
	h.startRecover(t.TempDir())
	d.openRecovered(e)
	for end := time.Now().Add(5 * time.Second); d.loading.Load() && time.Now().Before(end); {
		d.takeResult()
		time.Sleep(time.Millisecond)
	}
	h.frame()
	if d.errMsg != "" || h.para(0).Text() != "Recovered text" {
		t.Fatalf("loaded %q, error %q", h.para(0).Text(), d.errMsg)
	}
	if d.docName() != "Letter.doc [Recovered]" || !d.modified() || d.path != "" {
		t.Errorf("name %q, modified %v, path %q", d.docName(), d.modified(), d.path)
	}
	d.ar.saver.Wait()
	if o := autorecover.Orphans(dir); len(o) != 0 {
		t.Errorf("adopted copy still listed: %+v", o)
	}
	d.save()
	if !d.back.Visible || d.back.Page != gui.PageSaveAs || saveFormats[d.bs.saveFmt] != fileio.DOC {
		t.Errorf("Save: backstage %v page %v format %v", d.back.Visible, d.back.Page, saveFormats[d.bs.saveFmt])
	}
	if got := d.saveAsName(fileio.DOC); got != e.Path {
		t.Errorf("Save As suggests %q, want %q", got, e.Path)
	}
	d.savedAs(fileio.DOC, e.Path, false, d.bodyEd().Version, nil)
	d.ar.saver.Wait()
	if d.docName() != "Letter.doc" || d.modified() {
		t.Errorf("after Save As: %q, modified %v", d.docName(), d.modified())
	}
	if f := recoverFiles(dir); len(f) != 0 {
		t.Errorf("recovered copy left after Save As: %v", f)
	}
}
