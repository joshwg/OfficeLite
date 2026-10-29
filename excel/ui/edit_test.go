package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/model"
)

// harness drives an App headlessly through an input.Router.
type harness struct {
	t   *testing.T
	a   *App
	r   input.Router
	ops op.Ops
}

func newHarness(t *testing.T) *harness {
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	a := &App{ui: gui.NewTheme(fonts, accent, accentDark), fonts: fonts, recent: recent.Open(""), zoom: 1,
		states: map[int]*sheetState{}, calls: gui.NewCallQueue(nil)}
	a.th = a.ui.Theme
	a.shaper = a.th.Shaper
	a.setWorkbook(emptyWorkbook())
	h := &harness{t: t, a: a}
	h.frame()
	h.frame()
	return h
}

func (h *harness) frame() {
	h.ops.Reset()
	gtx := layout.Context{Ops: &h.ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1000, 700)), Source: h.r.Source()}
	h.a.calls.Run()
	h.a.layout(gtx)
	h.r.Frame(gtx.Ops)
}

// typeText sends text as typed characters, one frame each.
func (h *harness) typeText(s string) {
	for _, c := range s {
		// The window inserts at the focused editor's selection.
		var rng key.Range
		if h.a.ed.active {
			ed := &h.a.ed.cell
			if h.a.ed.inBar {
				ed = &h.a.ed.bar
			}
			rng.Start, rng.End = ed.Selection()
		}
		// As the window does (app.callbacks.EditorInsert): the edit, then
		// the caret after it.
		h.r.Queue(key.EditEvent{Range: rng, Text: string(c)})
		n := min(rng.Start, rng.End) + 1
		h.r.Queue(key.SelectionEvent{Start: n, End: n})
		h.frame()
	}
}

func (h *harness) press(name key.Name, mods key.Modifiers) {
	h.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	h.frame()
	h.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Release})
	h.frame()
}

func (h *harness) cell(ref string) *model.Cell {
	i := strings.IndexAny(ref, "0123456789")
	row := 0
	for _, d := range ref[i:] {
		row = row*10 + int(d-'0')
	}
	return h.a.wb.Sheets[h.a.active].Get(row-1, model.ParseColName(ref[:i]))
}

func (h *harness) expect(what string, want map[string]string) {
	h.t.Helper()
	for ref, w := range want {
		if got := h.cell(ref).Text; got != w {
			h.t.Errorf("%s: %s = %q, want %q", what, ref, got, w)
		}
	}
}

func (h *harness) active() string {
	p := h.a.state().view.Sel.Active
	return model.CellName(p.Row, p.Col)
}

func TestTypeAndUndo(t *testing.T) {
	h := newHarness(t)
	a := h.a

	// Typing starts editing in Enter mode; Enter commits and moves down.
	h.typeText("1,5")
	if !a.ed.active || a.ed.cell.Text() != "1,5" || a.ed.bar.Text() != "1,5" {
		t.Fatalf("editing %v, cell %q, bar %q", a.ed.active, a.ed.cell.Text(), a.ed.bar.Text())
	}
	h.press(key.NameEscape, 0)
	if a.ed.active || h.cell("A1").Kind != model.KindEmpty {
		t.Fatal("Esc did not cancel")
	}
	h.typeText("4")
	h.press(key.NameReturn, 0)
	h.expect("typed", map[string]string{"A1": "4"})
	if h.active() != "A2" || !a.modified {
		t.Errorf("after Enter: active %s, modified %v", h.active(), a.modified)
	}

	// A formula, upper-cased; Tab moves right.
	h.typeText("=a1*2")
	h.press(key.NameTab, 0)
	h.expect("formula", map[string]string{"A2": "8"})
	if f := h.cell("A2").Formula; f != "A1*2" {
		t.Errorf("A2 formula %q", f)
	}
	if h.active() != "B2" {
		t.Errorf("after Tab: active %s", h.active())
	}

	// In Enter mode an arrow key commits; dependents recalculate.
	h.press(key.NameUpArrow, 0) // B1
	h.press(key.NameLeftArrow, 0)
	h.typeText("10")
	h.press(key.NameDownArrow, 0)
	h.expect("recalculated", map[string]string{"A1": "10", "A2": "20"})
	if h.active() != "A2" {
		t.Errorf("after the arrow: active %s", h.active())
	}

	// A bad formula keeps the editor open with a message.
	h.typeText("=1+")
	h.press(key.NameReturn, 0)
	if !a.ed.active || !strings.Contains(a.errMsg, "problem with this formula") {
		t.Errorf("bad formula: editing %v, message %q", a.ed.active, a.errMsg)
	}
	h.press(key.NameEscape, 0)

	// F2 edits the existing content with the caret at the end.
	h.press(key.NameF2, 0)
	if a.ed.cell.Text() != "=A1*2" || a.ed.enter {
		t.Errorf("F2: %q, enter mode %v", a.ed.cell.Text(), a.ed.enter)
	}
	h.typeText("+1")
	h.press(key.NameReturn, 0)
	h.expect("F2", map[string]string{"A2": "21"})

	// Undo and redo.
	h.press("Z", key.ModShortcut)
	h.expect("undo", map[string]string{"A1": "10", "A2": "20"})
	h.press("Z", key.ModShortcut)
	h.expect("undo 2", map[string]string{"A1": "4", "A2": "8"})
	h.press("Y", key.ModShortcut)
	h.expect("redo", map[string]string{"A1": "10", "A2": "20"})

	// Delete clears the selection; Backspace starts an empty entry.
	h.press(key.NameUpArrow, 0) // the selection moved to A2 on undo; go to A1
	h.press(key.NameDeleteForward, 0)
	h.expect("delete", map[string]string{"A1": "", "A2": "0"})
	h.press(key.NameDownArrow, 0)
	h.press(key.NameDeleteBackward, 0)
	if !a.ed.active || a.ed.cell.Text() != "" {
		t.Errorf("Backspace: editing %v %q", a.ed.active, a.ed.cell.Text())
	}
	h.press(key.NameEscape, 0)
}

func TestAltEnterAndStructure(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("ab")
	h.press(key.NameReturn, key.ModAlt)
	h.typeText("cd")
	h.press(key.NameReturn, 0)
	h.expect("line break", map[string]string{"A1": "ab\ncd"})

	h.typeText("=A1")
	h.press(key.NameReturn, 0) // into A2
	// Insert a row at row 2 (Ctrl+Shift+=): the formula moves down.
	h.press(key.NameUpArrow, 0)
	h.press("+", key.ModShortcut|key.ModShift)
	if f := h.cell("A3").Formula; f != "A1" {
		t.Errorf("after inserting row 2: A3 formula %q", f)
	}
	h.press("-", key.ModShortcut)
	if f := h.cell("A2").Formula; f != "A1" {
		t.Errorf("after deleting row 2: A2 formula %q", f)
	}
	h.press("Z", key.ModShortcut)
	h.press("Z", key.ModShortcut)
	if f := h.cell("A2").Formula; f != "A1" {
		t.Errorf("after undoing both: A2 formula %q", f)
	}

	// Insert and rename a sheet; formulas follow the rename.
	a.insertSheet()
	h.frame()
	if len(a.wb.Sheets) != 2 || a.active != 0 || a.wb.Sheets[0].Name != "Sheet2" {
		t.Fatalf("sheets %d, active %d (%s)", len(a.wb.Sheets), a.active, a.wb.Sheets[0].Name)
	}
	a.startRename(0)
	h.frame()
	a.rename.ed.SetText("Input")
	h.press(key.NameReturn, 0)
	if a.rename.active || a.wb.Sheets[0].Name != "Input" {
		t.Errorf("rename: active %v, name %q", a.rename.active, a.wb.Sheets[0].Name)
	}
}

func TestPaste(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("2")
	h.press(key.NameReturn, 0)
	h.typeText("=A1*3")
	h.press(key.NameReturn, 0)
	// Copy A2, paste into B2:C2: the reference shifts.
	h.press(key.NameUpArrow, 0)
	h.frame()
	a.copy(a.gtx, false)
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 2})
	a.paste(a.clipText)
	if f := h.cell("C2").Formula; f != "C1*3" {
		t.Errorf("C2 formula %q", f)
	}
	// Text from another program is parsed as typed.
	a.selectRange(0, model.Range{R1: 4, C1: 0, R2: 4, C2: 0})
	a.paste("5%\t1/2/2026\r\n")
	h.expect("pasted text", map[string]string{"A5": "5%", "B5": "1/2/2026"})
	if h.cell("A5").Num != 0.05 {
		t.Errorf("A5 = %v", h.cell("A5").Num)
	}
}

func (h *harness) click(x, y float32) {
	pos := f32.Pt(x, y)
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	h.frame()
}

func TestFormulaBarAndDoubleClick(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("=SEQUENCE(3)")
	h.press(key.NameReturn, 0)
	// A spilled cell shows its formula in the bar.
	if got := a.ed.bar.Text(); got != "=SEQUENCE(3)" {
		t.Errorf("bar on A2 = %q", got)
	}
	// Clicking the formula bar edits there.
	h.click(200, float32(36+30+ribbonHeight+15))
	if !a.ed.active || !a.ed.inBar {
		t.Fatalf("bar click: editing %v, in bar %v", a.ed.active, a.ed.inBar)
	}
	a.ed.bar.SetText("x")
	h.press(key.NameReturn, 0)
	h.expect("bar entry", map[string]string{"A1": "#SPILL!", "A2": "x"})

	// Double-clicking a cell edits it.
	v := &a.state().view
	var x, y float32
	for y = 0; y < 300; y++ {
		if hit := v.HitTest(60, y); hit.Row == 1 && hit.Col == 0 {
			break
		}
	}
	x, y = 60, y+3+float32(gridTop)
	h.click(x, y)
	h.click(x, y)
	if !a.ed.active || a.ed.inBar || a.ed.cell.Text() != "x" {
		t.Fatalf("double-click: editing %v, text %q", a.ed.active, a.ed.cell.Text())
	}
	h.press(key.NameEscape, 0)
}

// Any change ends cut or copy mode, as in Excel; a copy survives its own
// paste, a cut doesn't.
func TestClipEndsOnChange(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("1")
	h.press(key.NameReturn, 0)
	cell := func(ref string) model.Range {
		r, c := refPos(ref)
		return model.Range{R1: r, C1: c, R2: r, C2: c}
	}
	for _, tc := range []struct {
		name string
		edit func()
	}{
		{"insert row", func() { a.selectRange(0, cell("A5")); a.structural(true, false) }},
		{"insert sheet", func() { a.insertSheet(); a.active = 0 }},
		{"undo", a.undo},
		{"delete", func() { a.selectRange(0, cell("C3")); a.clearSelection() }},
		{"fill", func() { a.selectRange(0, cell("C4")); a.fillDown() }},
	} {
		a.selectRange(0, cell("A1"))
		a.copy(a.gtx, true)
		tc.edit()
		if a.clip != nil {
			t.Errorf("%s: still in cut mode", tc.name)
		}
	}
	a.selectRange(0, cell("A1"))
	a.copy(a.gtx, false)
	a.selectRange(0, cell("B1"))
	a.paste(a.clipText)
	if a.clip == nil {
		t.Error("a copy ended with its paste")
	}
	a.selectRange(0, cell("B2"))
	a.pasteSpecial(1) // values
	if a.clip == nil || h.cell("B2").Text != "1" {
		t.Errorf("paste values: clip %v, B2 %q", a.clip != nil, h.cell("B2").Text)
	}
	a.copy(a.gtx, true)
	a.selectRange(0, cell("D1"))
	a.paste(a.clipText)
	if a.clip != nil || h.cell("D1").Text != "1" {
		t.Errorf("cut paste: clip %v, D1 %q", a.clip != nil, h.cell("D1").Text)
	}
}

// The window's shortcuts wait while a modal dialog is open; a tab being
// renamed takes its name first.
func TestShortcutsWaitForDialogs(t *testing.T) {
	h := newHarness(t)
	a := h.a
	for _, open := range []func(){
		func() { a.showGoTo() },
		func() { a.openFormatCells(tabNumber) },
		func() { a.dialog.Show(appName, "Message", "OK") },
		func() { a.createPivotDialog() },
	} {
		open()
		h.frame()
		h.press(key.NameF11, key.ModShift)
		h.press("-", key.ModShortcut|key.ModAlt)
		if len(a.wb.Sheets) != 1 || a.zoom != 1 {
			t.Fatalf("shortcuts behind a dialog: %d sheets, zoom %v", len(a.wb.Sheets), a.zoom)
		}
		h.press(key.NameEscape, 0)
		if a.modalOpen() {
			t.Fatal("Esc left the dialog open")
		}
	}
	a.startRename(0)
	h.frame()
	h.frame()
	a.rename.ed.SetText("Data")
	h.press(key.NameF11, key.ModShift)
	if a.rename.active || len(a.wb.Sheets) != 2 || a.wb.Sheets[1].Name != "Data" {
		t.Errorf("rename then Shift+F11: renaming %v, sheets %d, %q", a.rename.active, len(a.wb.Sheets), a.wb.Sheets[len(a.wb.Sheets)-1].Name)
	}
}

// Undoing Delete Sheet goes back to the sheet put back; redoing it to
// its neighbour.
func TestUndoDeleteSheetSelects(t *testing.T) {
	h := newHarness(t)
	a := h.a
	a.newSheet()
	a.newSheet() // Sheet1 Sheet2 Sheet3, the last active
	a.active = 1
	a.deleteSheet()
	if len(a.wb.Sheets) != 2 || a.active != 1 {
		t.Fatalf("after delete: %d sheets, active %d", len(a.wb.Sheets), a.active)
	}
	a.active = 0
	a.undo()
	if a.active != 1 || a.wb.Sheets[1].Name != "Sheet2" {
		t.Errorf("undo: active %d (%s)", a.active, a.wb.Sheets[a.active].Name)
	}
	a.redo()
	if a.active != 1 || a.wb.Sheets[1].Name != "Sheet3" {
		t.Errorf("redo: active %d (%s)", a.active, a.wb.Sheets[a.active].Name)
	}
}

// The mouse wheel doesn't scroll the grid behind a modal dialog.
func TestWheelBehindDialog(t *testing.T) {
	h := newHarness(t)
	a := h.a
	wheel := func() {
		h.r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(300, 600), Scroll: f32.Pt(0, 200)})
		h.frame()
		h.frame()
	}
	wheel()
	if a.state().view.TopRow == 0 {
		t.Fatal("the wheel didn't scroll the grid")
	}
	a.state().view.TopRow = 0
	for _, open := range []func(){
		func() { a.openFormatCells(tabNumber) },
		func() { a.showGoTo() },
		func() { a.dialog.Show(appName, "Message", "OK") },
	} {
		open()
		h.frame()
		wheel()
		if top := a.state().view.TopRow; top != 0 {
			t.Errorf("scrolled behind a dialog to row %d", top)
		}
		h.press(key.NameEscape, 0)
	}
}
