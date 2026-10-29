package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/common/autorecover"
	"officelite/common/gui"
	"officelite/excel/fileio"
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
	a := h.a
	a.ar.saver = autorecover.NewSaver(autorecover.Excel, a.calls.Post, a.recoverySnapshot)
	a.ar.saver.Dir = dir
	a.ar.saver.Interval = func() time.Duration { return 5 * time.Millisecond }
	a.ar.saver.Start()
	h.t.Cleanup(a.ar.saver.Close)
}

// waitRecover runs frames (which run the timer's ticks) until the
// Recovery directory has n files.
func (h *harness) waitRecover(dir string, n int) {
	h.t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		h.frame()
		h.a.ar.saver.Wait()
		if len(recoverFiles(dir)) == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("recovery files %v, want %d", recoverFiles(dir), n)
}

// TestAutoRecover: a modified workbook gets a recovery copy when the timer
// fires; Save deletes it, and so does closing with Don't Save.
func TestAutoRecover(t *testing.T) {
	h := newHarness(t)
	a := h.a
	dir := t.TempDir()
	h.startRecover(dir)
	time.Sleep(30 * time.Millisecond)
	h.waitRecover(dir, 0) // unmodified: nothing

	h.typeText("42")
	h.press(key.NameReturn, 0)
	h.waitRecover(dir, 2)
	var copyPath string
	var e autorecover.Entry
	for _, n := range recoverFiles(dir) {
		switch filepath.Ext(n) {
		case ".xlsx":
			copyPath = filepath.Join(dir, n)
		case ".json":
			b, _ := os.ReadFile(filepath.Join(dir, n))
			json.Unmarshal(b, &e)
		}
	}
	if e.App != autorecover.Excel || e.Name != a.wb.Name || e.Path != "" || e.PID != os.Getpid() {
		t.Errorf("sidecar %+v", e)
	}
	wb, err := fileio.Open(copyPath)
	if err != nil || wb.Sheets[0].Get(0, 0).Num != 42 {
		t.Fatalf("copy: %v", err)
	}

	// Save (to a file of its own) deletes the copy.
	path := filepath.Join(t.TempDir(), "Budget.xls")
	a.path = path
	a.save()
	h.waitRecover(dir, 0)
	if a.modified {
		t.Fatal("not saved")
	}
	// The copy of a .xls workbook is still .xlsx, with the format noted.
	h.typeText("7")
	h.press(key.NameReturn, 0)
	h.waitRecover(dir, 2)
	for _, n := range recoverFiles(dir) {
		if strings.HasSuffix(n, ".json") {
			b, _ := os.ReadFile(filepath.Join(dir, n))
			json.Unmarshal(b, &e)
		} else if !strings.HasSuffix(n, ".xlsx") {
			t.Errorf("copy %s", n)
		}
	}
	if e.Path != path || e.Format != ".xls" {
		t.Errorf("sidecar of the .xls workbook %+v", e)
	}

	// Close, Don't Save: the copy goes.
	a.askSaveBeforeClose()
	a.dialog.Hide()
	a.answer(1)
	h.waitRecover(dir, 0)
}

// TestOpenRecovered: a recovered workbook shows [Recovered], is modified,
// and Save goes to Save As with the original folder, name and format.
func TestOpenRecovered(t *testing.T) {
	dir := t.TempDir()
	src := newHarness(t)
	src.typeText("Recovered text")
	src.press(key.NameReturn, 0)
	e := autorecover.Entry{Path: filepath.Join(t.TempDir(), "Budget.xls"), Name: "Budget.xls", App: autorecover.Excel,
		Format: ".xls", Copy: filepath.Join(dir, "Budget~1.xlsx"), Sidecar: filepath.Join(dir, "Budget~1.json")}
	if err := fileio.SaveFile(src.a.wb, e.Copy, fileio.XLSX, 0); err != nil {
		t.Fatal(err)
	}
	autorecover.Release(e)

	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	h.startRecover(t.TempDir())
	a.openRecovered(e)
	for end := time.Now().Add(5 * time.Second); a.loading.Load() && time.Now().Before(end); {
		a.takeResult()
		h.frame()
	}
	h.frame()
	if a.errMsg != "" || h.cell("A1").Text != "Recovered text" {
		t.Fatalf("loaded %q, error %q", h.cell("A1").Text, a.errMsg)
	}
	if a.docName() != "Budget.xls [Recovered]" || !a.modified || a.path != "" {
		t.Errorf("name %q, modified %v, path %q", a.docName(), a.modified, a.path)
	}
	// The window took the copy over: no longer an orphan.
	a.ar.saver.Wait()
	if o := autorecover.Orphans(dir); len(o) != 0 {
		t.Errorf("adopted copy still listed: %+v", o)
	}
	a.save()
	if !a.back.Visible || a.back.Page != gui.PageSaveAs || saveFormats[a.bs.saveFmt] != fileio.XLS {
		t.Errorf("Save: backstage %v page %v format %v", a.back.Visible, a.back.Page, saveFormats[a.bs.saveFmt])
	}
	if got, want := a.saveAsName(fileio.XLS), e.Path; got != want {
		t.Errorf("Save As suggests %q, want %q", got, want)
	}
	// Saved as: no longer [Recovered], and the copy is gone.
	saved := filepath.Join(filepath.Dir(e.Path), "Budget.xls")
	a.saved(a.wb, fileio.XLS, saved, false, nil)
	a.ar.saver.Wait()
	if a.docName() != "Budget.xls" || a.modified {
		t.Errorf("after Save As: %q, modified %v", a.docName(), a.modified)
	}
	if f := recoverFiles(dir); len(f) != 0 {
		t.Errorf("recovered copy left after Save As: %v", f)
	}
}
