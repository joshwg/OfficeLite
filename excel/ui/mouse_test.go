// mouse_test.go tests context menus, multiple selection, dragging the selection's border and pointing in formulas.

package ui

import (
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/excel/model"
)

// cellPt is the middle of a cell in window coordinates.
func (h *harness) cellPt(ref string) f32.Point {
	h.t.Helper()
	r, c := refPos(ref)
	rc, _, ok := h.a.state().view.CellRect(r, c)
	if !ok {
		h.t.Fatalf("%s not on screen", ref)
	}
	return f32.Pt(float32(rc.Min.X+rc.Max.X)/2, float32(gridTop+(rc.Min.Y+rc.Max.Y)/2))
}

// refPos returns the zero-based row and column of a cell reference such as
// "C3".
func refPos(ref string) (int, int) {
	i := strings.IndexAny(ref, "0123456789")
	row := 0
	for _, d := range ref[i:] {
		row = row*10 + int(d-'0')
	}
	return row - 1, model.ParseColName(ref[:i])
}

// clickMods clicks button b at p (window coordinates) with modifiers held.
func (h *harness) clickMods(p f32.Point, b pointer.Buttons, mods key.Modifiers) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: b, Position: p, Modifiers: mods})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p, Modifiers: mods})
	h.frame()
}

// dragMods drags from one point to another with modifiers held.
func (h *harness) dragMods(from, to f32.Point, mods key.Modifiers) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: from, Modifiers: mods})
	h.frame()
	for _, p := range []f32.Point{from.Add(to).Mul(0.5), to} {
		h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p, Modifiers: mods})
		h.frame()
	}
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: to, Modifiers: mods})
	h.frame()
}

// enter types an entry into a cell.
func (h *harness) enter(ref, text string) {
	r, c := refPos(ref)
	h.a.selectRange(h.a.active, model.Range{R1: r, C1: c, R2: r, C2: c})
	h.frame()
	h.typeText(text)
	h.press(key.NameReturn, 0)
}

// chooseCtx clicks the context menu item labeled label of a menu opened
// at p (window coordinates) by a right-click, with items, in an area
// from origin of size lim, where it may have flipped to fit.
func (h *harness) chooseCtx(p f32.Point, items []ctxItem, label string, origin, lim f32.Point) {
	h.t.Helper()
	height := float32(8)
	for _, it := range items {
		if it.label == "" {
			height += 9
		} else {
			height += 28
		}
	}
	p = p.Sub(origin)
	if p.X+220 > lim.X {
		p.X = max(p.X-220, 0)
	}
	if p.Y+height > lim.Y {
		p.Y = max(p.Y-height, 0)
	}
	p = p.Add(origin)
	y := p.Y + 4
	for _, it := range items {
		if it.label == label {
			h.clickMods(f32.Pt(p.X+40, y+14), pointer.ButtonPrimary, 0)
			return
		}
		if it.label == "" {
			y += 9
		} else {
			y += 28
		}
	}
	h.t.Fatalf("no item %q", label)
}

// gridOrigin and gridSize are the grid's origin and size in the harness's window.
var gridOrigin, gridSize = f32.Pt(0, float32(gridTop)), f32.Pt(1000-14, float32(700-gridTop-30-24))

// TestContextMenus checks the cell, row header and sheet tab context menus:
// what a right-click selects and what their items do.
func TestContextMenus(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.enter("B2", "1")
	h.enter("C3", "2")
	h.expect("entered", map[string]string{"B2": "1", "C3": "2"})
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	h.frame()

	// Right-clicking a cell outside the selection selects it and opens
	// the cell menu.
	p := h.cellPt("C3")
	h.clickMods(p, pointer.ButtonSecondary, 0)
	if h.active() != "C3" || !a.cm.grid.IsOpen() {
		t.Fatalf("right-click: active %s, menu open %v", h.active(), a.cm.grid.IsOpen())
	}
	h.chooseCtx(p, a.cellMenu(), "Clear Contents", gridOrigin, gridSize)
	if a.cm.grid.IsOpen() || h.cell("C3").Text != "" || h.cell("B2").Text != "1" {
		t.Fatalf("Clear Contents: menu open %v, C3 %q, B2 %q", a.cm.grid.IsOpen(), h.cell("C3").Text, h.cell("B2").Text)
	}

	// Inside a selection the selection stays.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 2, C2: 2})
	h.frame()
	h.clickMods(h.cellPt("C3"), pointer.ButtonSecondary, 0)
	if r := a.state().view.Sel.Range; r != (model.Range{R1: 1, C1: 1, R2: 2, C2: 2}) {
		t.Errorf("right-click inside the selection changed it to %+v", r)
	}
	h.press(key.NameEscape, 0)
	if a.cm.grid.IsOpen() {
		t.Error("Esc didn't close the menu")
	}

	// A row header: its menu inserts rows.
	v := &a.state().view
	hw, _ := v.HeaderSize()
	y0, y1, _ := v.RowSpan(1)
	hp := f32.Pt(float32(hw/2), float32(gridTop+(y0+y1)/2))
	h.clickMods(hp, pointer.ButtonSecondary, 0)
	if !wholeRows(v.Sel.Range) || v.Sel.Range.R1 != 1 {
		t.Fatalf("row header right-click selected %+v", v.Sel.Range)
	}
	h.chooseCtx(hp, a.headerMenu(false), "Insert", gridOrigin, gridSize)
	h.expect("insert row", map[string]string{"B2": "", "B3": "1"})

	// A sheet tab: Rename edits the tab's name.
	tabY := float32(700 - 24 - 15)
	tp := f32.Pt(30, tabY)
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: tp})
	h.frame()
	h.clickMods(tp, pointer.ButtonSecondary, 0)
	if !a.cm.tabs.IsOpen() {
		t.Fatal("tab menu didn't open")
	}
	h.chooseCtx(tp, a.tabMenu(0), "Rename", f32.Point{}, f32.Pt(1000, 700))
	if !a.rename.active || a.rename.sheet != 0 {
		t.Errorf("Rename: active %v sheet %d", a.rename.active, a.rename.sheet)
	}
}

// TestMultipleSelection checks Ctrl+click and Ctrl+drag add ranges, commands
// act on all of them as one undo step, copying is refused and an arrow key
// collapses the selection.
func TestMultipleSelection(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.enter("A1", "1")
	h.enter("C3", "3")
	h.enter("E5", "5")
	v := &a.state().view
	h.clickMods(h.cellPt("A1"), pointer.ButtonPrimary, 0)
	h.clickMods(h.cellPt("C3"), pointer.ButtonPrimary, key.ModShortcut)
	if len(v.Sel.Extra) != 1 || h.active() != "C3" {
		t.Fatalf("Ctrl+click: %+v", v.Sel)
	}
	// Ctrl+drag adds a range.
	h.dragMods(h.cellPt("E5"), h.cellPt("F6"), key.ModShortcut)
	if len(v.Sel.Extra) != 2 || v.Sel.Range != (model.Range{R1: 4, C1: 4, R2: 5, C2: 5}) {
		t.Fatalf("Ctrl+drag: %+v", v.Sel)
	}

	// Bold and Delete act on every range, as one undo step each.
	h.press("B", key.ModShortcut)
	for _, ref := range []string{"A1", "C3", "E5"} {
		if !a.wb.Styles[h.cell(ref).Style].Font.Bold {
			t.Errorf("%s not bold", ref)
		}
	}
	if a.wb.Styles[h.cell("B2").Style].Font.Bold {
		t.Error("B2 (not selected) bold")
	}
	h.press(key.NameDeleteForward, 0)
	h.expect("delete", map[string]string{"A1": "", "C3": "", "E5": ""})
	h.press("Z", key.ModShortcut)
	h.expect("undo delete", map[string]string{"A1": "1", "C3": "3", "E5": "5"})
	h.press("Z", key.ModShortcut)
	if a.wb.Styles[h.cell("A1").Style].Font.Bold || a.wb.Styles[h.cell("E5").Style].Font.Bold {
		t.Error("undo bold left cells bold")
	}

	// Copying several ranges is refused.
	h.clickMods(h.cellPt("A1"), pointer.ButtonPrimary, 0)
	h.clickMods(h.cellPt("C3"), pointer.ButtonPrimary, key.ModShortcut)
	h.press("C", key.ModShortcut)
	if a.clip != nil || !strings.Contains(a.errMsg, "multiple selections") {
		t.Errorf("copy of a multiple selection: clip %v, message %q", a.clip != nil, a.errMsg)
	}
	// An arrow key collapses it.
	h.press(key.NameDownArrow, 0)
	if v.Sel.Multiple() || h.active() != "C4" {
		t.Errorf("after arrow: %+v", v.Sel)
	}
}

// TestDragBorderMove checks the move cursor on the selection's border,
// dragging it moves cells (adjusting references), and Ctrl+drag copies,
// asking before replacing data.
func TestDragBorderMove(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.enter("A1", "1")
	h.enter("A2", "2")
	h.enter("B1", "=A1*10")
	h.enter("D5", "old")
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 1, C2: 0})
	h.frame()
	v := &a.state().view
	box, ok := v.RangeRect(v.Sel.Range)
	if !ok {
		t.Fatal("selection not on screen")
	}
	edge := f32.Pt(float32(box.Min.X+12), float32(gridTop+box.Max.Y))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: edge})
	h.frame()
	if c := h.r.Cursor(); c != pointer.CursorAllScroll {
		t.Errorf("cursor on the border = %v", c)
	}
	inside := f32.Pt(float32(box.Min.X+20), float32(gridTop+box.Min.Y+box.Dy()/4))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: inside})
	h.frame()
	if c := h.r.Cursor(); c == pointer.CursorAllScroll {
		t.Error("move cursor inside the selection")
	}

	// Drag the bottom border (grabbing A2) to C4: A1:A2 moves to C3:C4.
	h.dragMods(edge, h.cellPt("C4"), 0)
	h.expect("move", map[string]string{"A1": "", "A2": "", "C3": "1", "C4": "2", "B1": "10"})
	if f := h.cell("B1").Formula; f != "C3*10" {
		t.Errorf("B1 = %q after the move", f)
	}
	if r := v.Sel.Range; r != (model.Range{R1: 2, C1: 2, R2: 3, C2: 2}) {
		t.Errorf("selection after move %+v", r)
	}
	h.press("Z", key.ModShortcut)
	h.expect("undo move", map[string]string{"A1": "1", "A2": "2", "C3": ""})
	if f := h.cell("B1").Formula; f != "A1*10" {
		t.Errorf("B1 = %q after undo", f)
	}

	// Ctrl+drag copies; onto data it asks first.
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 1, C2: 0})
	h.frame()
	box, _ = v.RangeRect(v.Sel.Range)
	edge = f32.Pt(float32(box.Min.X+12), float32(gridTop+box.Max.Y))
	h.dragMods(edge, h.cellPt("D5"), key.ModShortcut)
	if !a.dialog.Visible {
		t.Fatal("no question before replacing D5")
	}
	answer := a.answer
	a.dialog.Hide()
	a.answer = nil
	answer(0)
	h.frame()
	h.expect("copy", map[string]string{"A1": "1", "A2": "2", "D4": "1", "D5": "2"})
}

// TestPointingInFormula checks clicking and dragging while typing a formula
// puts references into it, each colored, and a click after a number ends the
// entry instead.
func TestPointingInFormula(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.enter("A1", "4")
	h.enter("B2", "5")
	a.selectRange(0, model.Range{R1: 4, C1: 4, R2: 4, C2: 4}) // E5
	h.frame()
	h.typeText("=")
	h.clickMods(h.cellPt("B2"), pointer.ButtonPrimary, 0)
	if !a.ed.active || a.ed.cell.Text() != "=B2" {
		t.Fatalf("after pointing: editing %v, %q", a.ed.active, a.ed.cell.Text())
	}
	// Another click replaces the reference just pointed at.
	h.clickMods(h.cellPt("A1"), pointer.ButtonPrimary, 0)
	if a.ed.cell.Text() != "=A1" {
		t.Fatalf("second click: %q", a.ed.cell.Text())
	}
	h.typeText("+")
	h.dragMods(h.cellPt("A1"), h.cellPt("B2"), 0)
	if got := a.ed.cell.Text(); got != "=A1+A1:B2" {
		t.Fatalf("drag: %q", got)
	}
	spans, cols := a.editRefs()
	if len(spans) != 2 || cols[0] == cols[1] {
		t.Errorf("references %+v colors %v", spans, cols)
	}
	h.press(key.NameReturn, 0)
	if f := h.cell("E5").Formula; f != "A1+A1:B2" {
		t.Errorf("E5 = %q", f)
	}
	if a.ed.active {
		t.Fatal("still editing after Enter")
	}

	// After a number a click ends the entry, as usual.
	a.selectRange(0, model.Range{R1: 6, C1: 4, R2: 6, C2: 4}) // E7
	h.frame()
	h.typeText("=5")
	h.clickMods(h.cellPt("A1"), pointer.ButtonPrimary, 0)
	if a.ed.active || h.cell("E7").Formula != "5" || h.active() != "A1" {
		t.Errorf("click after =5: editing %v, E7 %q, active %s", a.ed.active, h.cell("E7").Formula, h.active())
	}
}
