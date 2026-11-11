// macros_test.go checks what the window tells about a document's macros:
// a notice on opening, and a warning before a save loses them.

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"

	"officelite/common/macrotest"
	"officelite/word/fileio"
)

// macroDoc writes a document with a (dummy) VBA project to dir/name and
// returns its path.
func macroDoc(t *testing.T, dir, name string) string {
	t.Helper()
	path := saveTemplate(t, dir, fileio.DOCX)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if data, err = macrotest.AddToPackage(data, "word/_rels/document.xml.rels"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, name)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

// fileHasMacros reports whether the document at path has macros.
func fileHasMacros(t *testing.T, path string) bool {
	t.Helper()
	doc, err := fileio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc.HasMacros
}

// TestMacroDocument opens a document with macros: a notice says they
// won't run and AutoSave stays off; Save asks first, Cancel (Esc) keeps
// the file as it is, and Save writes it without them, after which
// AutoSave can go on and saves don't ask again.
func TestMacroDocument(t *testing.T) {
	templateEnv(t)
	path := macroDoc(t, t.TempDir(), "Report.docx")
	h := newHarness(t)
	d := h.d
	d.openPath(path)
	waitLoaded(t, d)
	if !d.doc.HasMacros {
		t.Fatal("the document's macros weren't found")
	}
	if !d.lossy.Visible || d.lossy.Title != "Macros won't run" {
		t.Fatalf("on opening: dialog %v %q", d.lossy.Visible, d.lossy.Title)
	}
	if d.autoSaveBlocked() == "" {
		t.Error("AutoSave can go on for a document with macros")
	}
	h.press(key.NameEscape, 0)
	if d.lossy.Visible {
		t.Fatal("Esc didn't close the notice")
	}

	h.frame()
	h.typeText("x")
	if !d.modified() {
		t.Fatal("typing didn't change the document")
	}
	d.save()
	if !d.lossy.Visible || d.lossy.Title != "Save without macros?" {
		t.Fatalf("Save: dialog %v %q", d.lossy.Visible, d.lossy.Title)
	}
	h.press(key.NameEscape, 0) // Cancel
	if !d.modified() || !d.doc.HasMacros || !fileHasMacros(t, path) {
		t.Fatalf("Cancel saved: modified %v, macros %v", d.modified(), d.doc.HasMacros)
	}

	d.save()
	save := d.lossyGo
	if save == nil {
		t.Fatal("no save waiting for the answer")
	}
	d.lossy.Hide()
	d.lossyGo = nil
	save() // Save
	if d.modified() || d.doc.HasMacros || fileHasMacros(t, path) {
		t.Fatalf("after Save: modified %v, macros %v", d.modified(), d.doc.HasMacros)
	}
	if msg := d.autoSaveBlocked(); msg != "" {
		t.Errorf("AutoSave still blocked: %q", msg)
	}
	h.typeText("y")
	d.save()
	if d.lossy.Visible || d.modified() {
		t.Errorf("second save: dialog %v %q, modified %v", d.lossy.Visible, d.lossy.Title, d.modified())
	}
}

// TestMacroTemplate makes a document from a template with macros: the
// notice says the template's macros won't run, and saving the new
// document doesn't warn (the template keeps them).
func TestMacroTemplate(t *testing.T) {
	templateEnv(t)
	path := macroDoc(t, t.TempDir(), "Letter.dotm")
	h := newHarness(t)
	d := h.d
	d.newFromTemplate(path)
	waitLoaded(t, d)
	if !d.lossy.Visible || d.lossy.Title != "Macros won't run" {
		t.Fatalf("on opening: dialog %v %q", d.lossy.Visible, d.lossy.Title)
	}
	if d.doc.HasMacros {
		t.Error("the new document has the template's macros")
	}
	d.checkMacros(false, func() {})
	if d.lossy.Visible && d.lossy.Title == "Save without macros?" {
		t.Error("saving the new document warns about macros")
	}
}
