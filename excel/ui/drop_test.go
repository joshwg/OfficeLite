package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/app"

	"officelite/common/doctype"
)

type launched struct {
	kind doctype.Kind
	path string
}

// dropFiles writes files with the given contents for dropping.
func dropFiles(t *testing.T, files map[string]string) string {
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestDropFiles: files dragged onto the window open as File ▸ Open opens
// them: the first workbook into the untouched new workbook, the others in
// new windows, a document in WordLite; a program is refused; a drop while
// a dialog is open does nothing.
func TestDropFiles(t *testing.T) {
	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	a.win = new(app.Window) // never shown: the loader's Invalidate does nothing
	var opened []launched
	a.launch = func(k doctype.Kind, p string) { opened = append(opened, launched{k, p}) }
	dir := dropFiles(t, map[string]string{
		"Données.csv": "a,b\n1,2\n", "Second.csv": "3\n", "Letter.docx": "not a zip", "setup.exe": "MZ",
	})
	csv, second := filepath.Join(dir, "Données.csv"), filepath.Join(dir, "Second.csv")
	doc := filepath.Join(dir, "Letter.docx")

	a.drop.Add(csv, filepath.Join(dir, "setup.exe"), doc, second)
	h.frame()
	select {
	case r := <-a.results:
		if r.path != csv || r.err != nil {
			t.Fatalf("loaded %q: %v", r.path, r.err)
		}
		a.results <- r
		a.takeResult()
		h.frame() // the message waits for the load, which clears the status line
	case <-time.After(10 * time.Second):
		t.Fatal("the first workbook didn't load into this window")
	}
	if a.path != csv {
		t.Errorf("window shows %q, want %q", a.path, csv)
	}
	want := []launched{{doctype.Document, doc}, {doctype.Spreadsheet, second}}
	if len(opened) != 2 || opened[0] != want[0] || opened[1] != want[1] {
		t.Errorf("launched %v, want %v", opened, want)
	}
	if !strings.Contains(a.errMsg, "can't open setup.exe") {
		t.Errorf("message %q", a.errMsg)
	}

	// While a dialog is open the drop is dropped.
	opened = nil
	a.dialog.Show("Microsoft Excel", "A question", "OK")
	a.drop.Add(second)
	h.frame()
	a.dialog.Hide()
	h.frame()
	if len(opened) != 0 || a.drop.Dropped() != nil {
		t.Errorf("drop under a dialog launched %v", opened)
	}

	// Onto the File page of a workbook with a file: a new window.
	a.back.Show(0)
	a.drop.Add(second)
	h.frame()
	if len(opened) != 1 || opened[0] != (launched{doctype.Spreadsheet, second}) || a.back.Visible {
		t.Errorf("drop on the File page: launched %v, backstage %v", opened, a.back.Visible)
	}
}
