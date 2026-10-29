// outline_test.go tests grouping, outline buttons, Show / Hide Detail, Clear Outline and Subtotal through the UI.

package ui

import (
	"slices"
	"testing"

	"gioui.org/io/key"

	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// outlineButton finds an outline button on screen: level button n, or
// with n 0 the +/- button of the group whose summary is at summary.
func (h *harness) outlineButton(rows bool, n, summary int) render.OutlineButton {
	h.t.Helper()
	h.frame() // laid out after the last change
	for _, b := range h.a.state().view.OutlineButtons() {
		if b.Rows == rows && b.Level == n && (n > 0 || b.Group.Summary == summary) {
			return b
		}
	}
	h.t.Fatalf("no outline button rows %v level %d summary %d in %+v", rows, n, summary, h.a.state().view.OutlineButtons())
	return render.OutlineButton{}
}

// clickOutline clicks an outline button.
func (h *harness) clickOutline(rows bool, n, summary int) {
	h.t.Helper()
	b := h.outlineButton(rows, n, summary)
	h.click(float32(b.Rect.Min.X+b.Rect.Max.X)/2, float32(gridTop+(b.Rect.Min.Y+b.Rect.Max.Y)/2))
}

// TestOutlineGroupUI checks grouping rows and columns, the gutters, the +/-
// and level buttons, Show / Hide Detail, Ungroup and Clear Outline.
func TestOutlineGroupUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	h.fillTable()
	v := &a.state().view
	hw0, hh0 := v.HeaderSize()

	// Whole rows 2:4: Group groups them at once.
	a.selectRange(0, model.Range{R1: 1, R2: 3, C2: model.MaxCols - 1})
	h.dataTab()
	p := h.locate(&a.data.group, ribbonArea, 6)
	h.click(p.X, p.Y)
	if got := []int{sh.Level(true, 0), sh.Level(true, 1), sh.Level(true, 3), sh.Level(true, 4)}; !slices.Equal(got, []int{0, 1, 1, 0}) {
		t.Fatalf("levels %v", got)
	}
	// The row gutter opens left of the row headers; the cells move right.
	gw, gh := v.Gutters()
	hw, hh := v.HeaderSize()
	if gw <= 0 || gh != 0 || hw != hw0+gw || hh != hh0 {
		t.Fatalf("gutters %d,%d headers %d,%d (were %d,%d)", gw, gh, hw, hh, hw0, hh0)
	}
	if hit := v.HitTest(float32(gw/2), float32(hh+30)); hit.Area != render.AreaOutline {
		t.Errorf("gutter hit %+v", hit)
	}
	if hit := v.HitTest(float32(gw+2), float32(hh+30)); hit.Area != render.AreaRowHeader || hit.Row != 1 {
		t.Errorf("row header hit %+v", hit)
	}
	c := h.cellPt("B3")
	h.click(c.X, c.Y)
	if h.active() != "B3" {
		t.Fatalf("click on B3 selected %s", h.active())
	}

	// - collapses the group: rows 2-4 hide, the active cell moves to the
	// summary row 5.
	h.clickOutline(true, 0, 4)
	if got := h.hiddenRows(); !slices.Equal(got, []int{2, 3, 4}) || !sh.IsCollapsed(true, 4) || h.active() != "B5" {
		t.Fatalf("collapsed: hidden %v, active %s", got, h.active())
	}
	// + expands it.
	h.clickOutline(true, 0, 4)
	if got := h.hiddenRows(); len(got) != 0 || sh.IsCollapsed(true, 4) {
		t.Fatalf("expanded: hidden %v", got)
	}
	// Level buttons.
	h.clickOutline(true, 1, 0)
	if got := h.hiddenRows(); !slices.Equal(got, []int{2, 3, 4}) {
		t.Fatalf("level 1: hidden %v", got)
	}
	h.press("Z", key.ModShortcut)
	if got := h.hiddenRows(); len(got) != 0 {
		t.Fatalf("undo level 1: hidden %v", got)
	}

	// Hide Detail / Show Detail work on the selection.
	a.selectRange(0, model.Range{R1: 2, C1: 1, R2: 2, C2: 1})
	h.frame()
	p = h.locate(&a.data.hideDetail, ribbonArea, 4)
	h.click(p.X, p.Y)
	if got := h.hiddenRows(); !slices.Equal(got, []int{2, 3, 4}) {
		t.Fatalf("Hide Detail: hidden %v", got)
	}
	p = h.locate(&a.data.showDetail, ribbonArea, 4)
	h.click(p.X, p.Y)
	if got := h.hiddenRows(); len(got) != 0 {
		t.Fatalf("Show Detail: hidden %v", got)
	}

	// A cell range asks Rows or Columns; Shift+Alt+Right is Group.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 2, C2: 2})
	h.frame()
	h.press(key.NameRightArrow, key.ModShift|key.ModAlt)
	if a.ins.kind != dlgGroup || a.ins.grp.ungroup || a.ins.grp.axis.Value != "rows" {
		t.Fatalf("Group dialog %v", a.ins.kind)
	}
	a.ins.grp.axis.Value = "cols"
	h.frame()
	ok := h.locate(&a.ins.ok, modalArea(240), 8)
	h.click(ok.X, ok.Y)
	if a.ins.kind != dlgNone || sh.Level(false, 1) != 1 || sh.Level(false, 2) != 1 || sh.Level(false, 3) != 0 {
		t.Fatalf("columns: dialog %v, levels %d %d %d", a.ins.kind, sh.Level(false, 1), sh.Level(false, 2), sh.Level(false, 3))
	}
	// The column gutter opens above the headers; - over column D hides
	// B:C.
	h.frame()
	if _, gh := v.Gutters(); gh <= 0 {
		t.Fatal("no column gutter")
	}
	h.clickOutline(false, 0, 3)
	if !sh.IndexHidden(false, 1) || !sh.IndexHidden(false, 2) || !sh.IsCollapsed(false, 3) {
		t.Fatal("columns not collapsed")
	}
	h.clickOutline(false, 0, 3)
	if sh.IndexHidden(false, 1) {
		t.Fatal("columns not expanded")
	}

	// Shift+Alt+Left on whole rows ungroups them.
	a.selectRange(0, model.Range{R1: 1, R2: 3, C2: model.MaxCols - 1})
	h.frame()
	h.press(key.NameLeftArrow, key.ModShift|key.ModAlt)
	if sh.MaxLevel(true) != 0 {
		t.Fatalf("rows still grouped: %v", sh.Outline.RowLevel)
	}
	// Ungroup ▾ > Clear Outline.
	btn := a.data.ungroupDD.Button()
	p = h.locate(btn, ribbonArea, 4)
	h.click(p.X, p.Y)
	if !a.data.ungroupDD.IsOpen() {
		t.Fatal("Ungroup menu not open")
	}
	a.data.ungroupDD.Close() // chosen as the list would (see TestValidationUI)
	a.ungroupMenu(1)
	h.frame()
	if sh.MaxLevel(false) != 0 {
		t.Fatal("Clear Outline left column levels")
	}
	if gw, gh := v.Gutters(); gw != 0 || gh != 0 {
		t.Errorf("gutters stay: %d %d", gw, gh)
	}
}

// TestSubtotalUI checks Subtotal adds per-group sums with an outline, level
// buttons show the totals only, and Remove All is undoable.
func TestSubtotalUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	h.fillTable()
	// Sorted by Kind: fruit, fruit, fruit, veg...
	a.selectRange(0, model.Range{R1: 1, C1: 2, R2: 1, C2: 2})
	h.dataTab()
	a.runSort(edit.SortSpec{Range: model.Range{R2: 6, C2: 2}, Keys: []edit.SortKey{{Col: 2}}, Header: true})
	h.frame()
	p := h.locate(&a.data.subtotal, ribbonArea, 6)
	h.click(p.X, p.Y)
	d := &a.ins.sub
	if a.ins.kind != dlgSubtotal || d.region != (model.Range{R2: 6, C2: 2}) || !d.replace.Value || !d.below.Value || !d.cols[2].Value || d.cols[1].Value {
		t.Fatalf("dialog %v region %v", a.ins.kind, d.region)
	}
	// At each change in Kind, Sum of Qty.
	d.by = 2
	d.cols[1].Value, d.cols[2].Value = true, false
	h.frame()
	ok := h.locate(&a.ins.ok, modalArea(340), 8)
	h.click(ok.X, ok.Y)
	if a.ins.kind != dlgNone {
		t.Fatalf("still open: %q", a.ins.err)
	}
	h.expect("subtotals", map[string]string{"C5": "fruit Total", "B5": "55", "C9": "veg Total", "B9": "70", "C10": "Grand Total", "B10": "125"})
	if sh.MaxLevel(true) != 2 {
		t.Fatalf("levels %v", sh.Outline.RowLevel)
	}
	// Level 2 shows the totals only.
	h.clickOutline(true, 2, 0)
	if got := h.hiddenRows(); !slices.Equal(got, []int{2, 3, 4, 6, 7}) {
		t.Fatalf("level 2 hides %v", got)
	}
	h.clickOutline(true, 3, 0)

	// Remove All, then undo it.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	a.subtotalDialog()
	h.frame()
	if d.region != (model.Range{R2: 9, C2: 2}) {
		t.Fatalf("region %v", d.region)
	}
	a.ins.sub.removeAll.Click()
	h.frame()
	if a.ins.kind != dlgNone || h.cell("C5").Text != "veg" || h.cell("C9").Text != "" || sh.MaxLevel(true) != 0 {
		t.Fatalf("remove all: C5 %q C9 %q levels %d", h.cell("C5").Text, h.cell("C9").Text, sh.MaxLevel(true))
	}
	h.frame() // the grid takes the focus back
	h.press("Z", key.ModShortcut)
	if h.cell("C10").Text != "Grand Total" {
		t.Fatalf("undo: C10 %q", h.cell("C10").Text)
	}
}
