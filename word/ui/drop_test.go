package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"officelite/common/doctype"
)

type launched struct {
	kind doctype.Kind
	path string
}

// TestDropFiles: files dragged onto the window open as File ▸ Open opens
// them: the first document into the untouched new document, the others in
// new windows, a workbook in ExcelLite; a picture is refused (not
// inserted); a drop while a dialog is open does nothing.
func TestDropFiles(t *testing.T) {
	h := newHarness(t)
	d := h.d
	var opened []launched
	d.cfg.Launch = func(k doctype.Kind, p string) { opened = append(opened, launched{k, p}) }
	dir := t.TempDir()
	for name, data := range map[string]string{
		"Brief ✓.txt": "Hello\r\n", "Second.txt": "Two\r\n", "Book.xlsx": "not a zip", "photo.png": "\x89PNG\r\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	brief, second := filepath.Join(dir, "Brief ✓.txt"), filepath.Join(dir, "Second.txt")
	book := filepath.Join(dir, "Book.xlsx")

	d.drop.Add(brief, filepath.Join(dir, "photo.png"), book, second)
	h.frame()
	select {
	case r := <-d.results:
		if r.path != brief || r.err != nil {
			t.Fatalf("loaded %q: %v", r.path, r.err)
		}
		d.results <- r
		d.takeResult()
		h.frame() // the message waits for the load, which clears the status line
	case <-time.After(10 * time.Second):
		t.Fatal("the first document didn't load into this window")
	}
	if d.path != brief {
		t.Errorf("window shows %q, want %q", d.path, brief)
	}
	want := []launched{{doctype.Spreadsheet, book}, {doctype.Document, second}}
	if len(opened) != 2 || opened[0] != want[0] || opened[1] != want[1] {
		t.Errorf("launched %v, want %v", opened, want)
	}
	if !strings.Contains(d.errMsg, "can't open photo.png") {
		t.Errorf("message %q", d.errMsg)
	}

	// While a dialog is open the drop is dropped.
	opened = nil
	d.lk.dlg.Show("File In Use", "A question", "OK")
	d.drop.Add(second)
	h.frame()
	d.lk.dlg.Hide()
	h.frame()
	if len(opened) != 0 || d.drop.Dropped() != nil {
		t.Errorf("drop under a dialog launched %v", opened)
	}

	// Onto the File page of a document with a file: a new window.
	d.back.Show(0)
	d.drop.Add(book)
	h.frame()
	if len(opened) != 1 || opened[0] != (launched{doctype.Spreadsheet, book}) || d.back.Visible {
		t.Errorf("drop on the File page: launched %v, backstage %v", opened, d.back.Visible)
	}
}
