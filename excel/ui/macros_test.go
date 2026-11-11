// macros_test.go checks what the window tells about a workbook's macros:
// a notice on opening, and a warning before a save loses them.

package ui

import (
	"os"
	"testing"

	"gioui.org/io/key"

	"officelite/common/macrotest"
	"officelite/excel/fileio"
)

// macroWorkbook writes a workbook with a (dummy) VBA project to a new
// .xlsx and returns its path.
func macroWorkbook(t *testing.T) string {
	t.Helper()
	path := savedWorkbook(t, "Macro data")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if data, err = macrotest.AddToPackage(data, "xl/_rels/workbook.xml.rels"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// answer clicks button i of h's message box.
func (h *harness) answer(i int) {
	h.t.Helper()
	a := h.a
	if !a.dialog.Visible || a.answer == nil {
		h.t.Fatalf("no question showing (visible %v)", a.dialog.Visible)
	}
	answer := a.answer
	a.dialog.Hide()
	a.answer = nil
	answer(i)
}

// TestMacroWorkbook opens a workbook with macros: a notice says they
// won't run and AutoSave stays off; Save asks first, Cancel keeps the
// file as it is, and Save writes it without them, after which AutoSave
// can go on and saves don't ask again.
func TestMacroWorkbook(t *testing.T) {
	path := macroWorkbook(t)
	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	a.openPath(path)
	h.waitLoad()
	if !a.wb.HasMacros {
		t.Fatal("the workbook's macros weren't found")
	}
	if !a.dialog.Visible || a.dialog.Title != "Macros won't run" {
		t.Fatalf("on opening: dialog %v %q", a.dialog.Visible, a.dialog.Title)
	}
	if a.autoSaveBlocked() == "" {
		t.Error("AutoSave can go on for a workbook with macros")
	}
	h.press(key.NameEscape, 0)
	if a.dialog.Visible {
		t.Fatal("Esc didn't close the notice")
	}

	h.typeText("1")
	h.press(key.NameReturn, 0)
	a.save()
	if !a.dialog.Visible || a.dialog.Title != "Save without macros?" {
		t.Fatalf("Save: dialog %v %q", a.dialog.Visible, a.dialog.Title)
	}
	h.answer(1) // Cancel
	if !a.modified || !a.wb.HasMacros {
		t.Fatalf("Cancel saved: modified %v, macros %v", a.modified, a.wb.HasMacros)
	}
	if data, _ := os.ReadFile(path); !fileHasMacros(t, path, data) {
		t.Fatal("Cancel changed the file")
	}

	a.save()
	h.answer(0) // Save
	if a.modified || a.wb.HasMacros {
		t.Fatalf("after Save: modified %v, macros %v", a.modified, a.wb.HasMacros)
	}
	if data, _ := os.ReadFile(path); fileHasMacros(t, path, data) {
		t.Error("the saved file still has macros")
	}
	if msg := a.autoSaveBlocked(); msg != "" {
		t.Errorf("AutoSave still blocked: %q", msg)
	}
	h.typeText("2")
	h.press(key.NameReturn, 0)
	a.save()
	if a.dialog.Visible || a.modified {
		t.Errorf("second save: dialog %v %q, modified %v", a.dialog.Visible, a.dialog.Title, a.modified)
	}
}

// fileHasMacros reads data, the file at path, as a workbook and reports
// whether it has macros.
func fileHasMacros(t *testing.T, path string, data []byte) bool {
	t.Helper()
	wb, err := fileio.Read(path, data)
	if err != nil {
		t.Fatal(err)
	}
	return wb.HasMacros
}
