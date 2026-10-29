package ui

import (
	"testing"

	"gioui.org/io/key"

	"officelite/excel/model"
)

func (h *harness) style(ref string) model.Style { return h.a.wb.Styles[h.cell(ref).Style] }

func TestFormatShortcuts(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.typeText("1234.5")
	h.press(key.NameReturn, 0)
	h.press(key.NameUpArrow, 0)

	h.press("B", key.ModShortcut)
	h.press("I", key.ModShortcut)
	h.press("U", key.ModShortcut)
	h.press("5", key.ModShortcut)
	if f := h.style("A1").Font; !f.Bold || !f.Italic || !f.Underline || !f.Strike {
		t.Errorf("font after Ctrl+B/I/U/5: %+v", f)
	}
	if st := a.activeStyle(); !st.Font.Bold {
		t.Error("the ribbon doesn't see the active cell bold")
	}
	h.press("4", key.ModShortcut|key.ModShift)
	h.expect("Ctrl+Shift+$", map[string]string{"A1": "$1,234.50 "})
	h.press("5", key.ModShortcut|key.ModShift)
	h.expect("Ctrl+Shift+%", map[string]string{"A1": "123450%"})
	h.press("`", key.ModShortcut|key.ModShift)
	h.expect("Ctrl+Shift+~", map[string]string{"A1": "1234.5"})
	h.press("Z", key.ModShortcut)
	h.expect("undo", map[string]string{"A1": "123450%"})
	h.press("B", key.ModShortcut) // toggles off
	if h.style("A1").Font.Bold {
		t.Error("Ctrl+B did not toggle bold off")
	}

	// Ctrl+1 opens Format Cells; Esc closes it unchanged, Enter applies.
	h.press("1", key.ModShortcut)
	if !a.fmtDlg.open {
		t.Fatal("Ctrl+1: no dialog")
	}
	if !h.r.Source().Focused(&a.fmtDlg) {
		t.Error("Format Cells doesn't have the keyboard")
	}
	h.press(key.NameEscape, 0)
	if a.fmtDlg.open {
		t.Fatal("Esc did not close Format Cells")
	}
	h.press("1", key.ModShortcut)
	a.fmtDlg.cur.Wrap = true
	h.press(key.NameReturn, 0)
	if a.fmtDlg.open || !h.style("A1").Wrap {
		t.Errorf("Format Cells OK: open %v, wrap %v", a.fmtDlg.open, h.style("A1").Wrap)
	}
	if f := h.style("A1").Font; !f.Italic {
		t.Error("Format Cells changed what it didn't touch")
	}
}

func TestFormatPainter(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.press("B", key.ModShortcut) // A1 bold
	a.startPainter(false)
	v := &a.state().view
	x0, x1, _ := v.ColSpan(2)
	y0, y1, _ := v.RowSpan(2)
	h.click(float32(x0+x1)/2, float32(gridTop+(y0+y1)/2)) // C3
	h.frame()
	if !h.style("C3").Font.Bold {
		t.Errorf("painter: C3 not bold (active %s)", h.active())
	}
	if a.home.painterClip != nil {
		t.Error("painter still on after one use")
	}
}

func TestCellsAndEditingMenus(t *testing.T) {
	h := newHarness(t)
	a := h.a
	for _, s := range []string{"1", "2", "3"} {
		h.typeText(s)
		h.press(key.NameReturn, 0)
	}
	// AutoSum under a column of numbers sums them.
	a.autoSum(0)
	if !a.ed.active || a.ed.cell.Text() != "=SUM(A1:A3)" {
		t.Fatalf("AutoSum: editing %v %q", a.ed.active, a.ed.cell.Text())
	}
	h.frame() // the editor takes the focus
	h.press(key.NameReturn, 0)
	h.expect("AutoSum", map[string]string{"A4": "6"})

	// Format > Row Height asks for points.
	a.formatMenu(a.gtx, fmtRowHeight)
	h.frame()
	if !a.askDlg.open {
		t.Fatal("no Row Height prompt")
	}
	a.askDlg.ed.SetText("30")
	h.press(key.NameReturn, 0)
	if got := a.wb.Sheets[0].RowHeight[4]; got != 40 || a.askDlg.open {
		t.Errorf("row 5 height %v px, prompt open %v", got, a.askDlg.open)
	}

	// Hide Sheet keeps one sheet visible; Unhide brings it back.
	a.formatMenu(a.gtx, fmtHideSheet)
	if a.wb.Sheets[0].Hidden || a.errMsg == "" {
		t.Error("the only sheet was hidden")
	}
	a.insertSheet()
	h.frame()
	a.formatMenu(a.gtx, fmtHideSheet)
	if !a.wb.Sheets[0].Hidden || a.active != 1 {
		t.Fatalf("hide: hidden %v active %d", a.wb.Sheets[0].Hidden, a.active)
	}
	a.formatMenu(a.gtx, fmtUnhideSheet)
	h.frame()
	h.press(key.NameReturn, 0)
	if a.wb.Sheets[0].Hidden || a.active != 0 {
		t.Errorf("unhide: hidden %v active %d", a.wb.Sheets[0].Hidden, a.active)
	}
}

// Hide keeps a row's height and a column's width; Unhide on the whole
// sheet finds them far past the used area and gives them back.
func TestHideUnhideSizes(t *testing.T) {
	h := newHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	h.typeText("x")
	h.press(key.NameReturn, 0)
	a.book.ResizeRows(0, []int{199}, 40)
	a.book.ResizeCols(0, []int{60}, 25)
	a.selectRange(0, model.Range{R1: 199, R2: 199, C2: model.MaxCols - 1})
	a.formatMenu(a.gtx, fmtHideRows)
	a.selectRange(0, model.Range{C1: 60, C2: 60, R2: model.MaxRows - 1})
	a.formatMenu(a.gtx, fmtHideCols)
	if !sh.RowHidden(199) || !sh.IndexHidden(false, 60) {
		t.Fatalf("not hidden: %v %v", sh.RowHeight, sh.ColWidth)
	}
	a.selectRange(0, model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}) // Select All
	a.formatMenu(a.gtx, fmtUnhideRows)
	a.formatMenu(a.gtx, fmtUnhideCols)
	if sh.RowHeight[199] != 40 || sh.ColWidth[60] != 25 {
		t.Errorf("unhidden: row 200 %v, column BI %v", sh.RowHeight[199], sh.ColWidth[60])
	}
	a.undo()
	a.undo()
	if !sh.RowHidden(199) || !sh.IndexHidden(false, 60) {
		t.Errorf("undo unhide: %v %v", sh.RowHeight[199], sh.ColWidth[60])
	}
}
