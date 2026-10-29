// insert_test.go tests the Insert menu, Fill, the fill handle, Find / Go To / Sort, notes, links and Insert Cells.

package ui

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"

	"officelite/excel/model"
	"officelite/excel/render"
)

// TestInsertMenuAndFill checks the Insert menu exists and Ctrl+D fills down,
// undoably.
func TestInsertMenuAndFill(t *testing.T) {
	h := newHarness(t)
	a := h.a
	if m := a.menus()[menuInsert]; m.Label != "Insert" || len(m.Items) == 0 {
		t.Fatalf("Insert menu = %+v", m)
	}
	h.typeText("5")
	h.press(key.NameReturn, 0)
	a.selectRange(0, model.Range{R2: 3})
	h.frame()
	h.press("D", key.ModShortcut) // Fill Down
	h.expect("fill down", map[string]string{"A2": "5", "A4": "5"})
	h.press("Z", key.ModShortcut)
	h.expect("undo", map[string]string{"A2": "", "A4": ""})
}

// TestFillHandleDrag checks dragging the fill handle extends a series and the
// selection.
func TestFillHandleDrag(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("1")
	h.press(key.NameReturn, 0)
	h.typeText("2")
	h.press(key.NameReturn, 0)
	a.selectRange(0, model.Range{R2: 1})
	h.frame()
	v := &a.state().view
	hr, ok := a.fillHandleRect(a.gtx, v)
	if !ok {
		t.Fatal("no fill handle")
	}
	a5, _, _ := v.CellRect(4, 0)
	from := f32.Pt(float32(hr.Min.X+hr.Dx()/2), float32(gridTop+hr.Min.Y+hr.Dy()/2))
	to := f32.Pt(float32(a5.Min.X+5), float32(gridTop+a5.Min.Y+5))
	h.drag(from, to)
	h.expect("series", map[string]string{"A3": "3", "A4": "4", "A5": "5", "A6": ""})
	if s := v.Sel.Range; s != (model.Range{R2: 4}) {
		t.Errorf("selection %+v", s)
	}
}

// TestFindGoToSort checks Ctrl+F finds a cell, Esc closes it, Ctrl+G goes to a
// reference and Sort Ascending sorts without a guessed header row.
func TestFindGoToSort(t *testing.T) {
	h := newHarness(t)
	a := h.a
	for _, s := range []string{"Name", "pear", "apple", "fig"} {
		h.typeText(s)
		h.press(key.NameReturn, 0)
	}
	a.selectRange(0, model.Range{})
	h.frame()
	h.press("F", key.ModShortcut)
	if a.ins.kind != dlgFind {
		t.Fatalf("Ctrl+F: dialog %v", a.ins.kind)
	}
	a.ins.find.what.SetText("app")
	h.frame()
	h.press(key.NameReturn, 0) // Find Next
	if p := a.state().view.Sel.Active; p != (render.Pos{Row: 2}) {
		t.Errorf("found at %+v", p)
	}
	h.press(key.NameEscape, 0)
	if a.ins.kind != dlgNone {
		t.Errorf("Esc left dialog %v open", a.ins.kind)
	}

	h.press("G", key.ModShortcut)
	a.ins.gt.ref.SetText("c7")
	h.frame()
	h.press(key.NameReturn, 0)
	if r := a.state().view.Sel.Range; r != (model.Range{R1: 6, C1: 2, R2: 6, C2: 2}) || a.ins.kind != dlgNone {
		t.Errorf("Go To: %+v, dialog %v", r, a.ins.kind)
	}

	a.selectRange(0, model.Range{R1: 1, R2: 1})
	a.sortAscending()
	h.frame()
	// All text, one font: no header row, as Excel guesses.
	h.expect("sorted", map[string]string{"A1": "apple", "A2": "fig", "A3": "Name", "A4": "pear"})
}

// TestNotesAndLinksUI checks Ctrl+K adds a followable internal link and
// Shift+F2 adds an undoable note.
func TestNotesAndLinksUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("site")
	h.press(key.NameReturn, 0)
	a.selectRange(0, model.Range{})
	h.frame()
	h.press("K", key.ModShortcut)
	if a.ins.kind != dlgHyperlink {
		t.Fatalf("Ctrl+K: dialog %v", a.ins.kind)
	}
	a.ins.link.address.SetText("#B5")
	h.frame()
	h.press(key.NameReturn, 0)
	if l, ok := a.wb.Sheets[0].LinkAt(0, 0); !ok || l.Location != "B5" {
		t.Fatalf("link = %+v %v (err %q)", l, ok, a.ins.err)
	}
	v := &a.state().view
	if !followLinkTest(a, v) {
		t.Error("link not followed")
	}
	if r := v.Sel.Range; r != (model.Range{R1: 4, C1: 1, R2: 4, C2: 1}) {
		t.Errorf("after following: %+v", r)
	}

	a.selectRange(0, model.Range{R1: 2, C1: 2, R2: 2, C2: 2})
	h.press(key.NameF2, key.ModShift)
	if a.ins.kind != dlgNote {
		t.Fatalf("Shift+F2: dialog %v", a.ins.kind)
	}
	a.ins.note.ed.SetText("remember")
	h.frame()
	a.ins.ok.Click()
	h.frame()
	h.frame()
	if n, ok := a.wb.Sheets[0].NoteAt(2, 2); !ok || n.Text != "remember" {
		t.Errorf("note = %+v %v", n, ok)
	}
	h.press("Z", key.ModShortcut)
	if _, ok := a.wb.Sheets[0].NoteAt(2, 2); ok {
		t.Error("undo kept the note")
	}
}

// followLinkTest follows the link in A1 of v, reporting whether there was one.
func followLinkTest(a *App, v *render.View) bool {
	return a.followLinkAt(v, render.Hit{Area: render.AreaCell, Row: 0, Col: 0})
}

// TestInsertCellsDialog checks Insert Cells shifts down by default and Delete
// Cells shifts back up.
func TestInsertCellsDialog(t *testing.T) {
	h := newHarness(t)
	a := h.a
	for _, s := range []string{"1", "2", "3"} {
		h.typeText(s)
		h.press(key.NameReturn, 0)
	}
	a.selectRange(0, model.Range{R1: 1, R2: 1})
	a.insertCellsDialog()
	h.frame()
	if a.ins.kind != dlgInsertCells || a.ins.shift.Value != "down" {
		t.Fatalf("dialog %v shift %q", a.ins.kind, a.ins.shift.Value)
	}
	h.press(key.NameReturn, 0)
	h.expect("inserted", map[string]string{"A1": "1", "A2": "", "A3": "2", "A4": "3"})
	a.deleteCellsDialog()
	h.frame()
	h.press(key.NameReturn, 0)
	h.expect("deleted", map[string]string{"A2": "2", "A3": "3"})
}
