package ui

import (
	"image"
	"slices"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/excel/model"
)

// salesTable enters a small sales table in A1:C7.
func (h *harness) salesTable() {
	rows := [][]string{
		{"Region", "Product", "Sales"},
		{"East", "Pen", "10"},
		{"West", "Pen", "20"},
		{"East", "Ink", "30"},
		{"West", "Pad", "7"},
		{"North", "Ink", "8"},
		{"East", "Pad", "1"},
	}
	for r, row := range rows {
		for c, v := range row {
			if _, err := h.a.book.Enter(0, r, c, v); err != nil {
				h.t.Fatal(err)
			}
		}
	}
	h.a.syncSheets()
	h.frame()
}

// paneOrigin is the field list's top-left in the window.
func (h *harness) paneOrigin() f32.Point { return f32.Pt(float32(1000-paneWidth), float32(gridTop)) }

// chipPt is the middle of chip idx of area, in the window.
func (h *harness) chipPt(area, idx int) f32.Point {
	h.t.Helper()
	for _, c := range h.a.pv.pane.chips {
		if c.area == area && c.idx == idx {
			m := c.r.Min.Add(c.r.Max).Div(2)
			return h.paneOrigin().Add(f32.Pt(float32(m.X), float32(m.Y)))
		}
	}
	h.t.Fatalf("no chip %d in area %d (%v)", idx, area, h.a.pv.pane.chips)
	return f32.Point{}
}

// boxPt is a point near the bottom of an area's box, in the window.
func (h *harness) boxPt(area int) f32.Point {
	b := h.a.pv.pane.boxes[area]
	return h.paneOrigin().Add(f32.Pt(float32(b.Min.X+b.Max.X)/2, float32(b.Max.Y-10)))
}

func TestCreatePivotAndFieldList(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.salesTable()
	a.selectRange(0, model.Range{R1: 2, C1: 1, R2: 2, C2: 1})
	h.frame()

	// Insert > PivotTable… offers the data around the active cell and
	// a new worksheet.
	items := a.menus()[menuInsert].Items
	if items[insPivot].Label != "PivotTable…" {
		t.Fatalf("Insert menu item %q", items[insPivot].Label)
	}
	a.insertChosen(insPivot)
	h.frame()
	d := &a.pv.dlg
	if !d.open || d.kind != pdCreate || d.source.Text() != "Sheet1!$A$1:$C$7" || d.where.Value != "new" {
		t.Fatalf("Create PivotTable: open %v, source %q, where %q", d.open, d.source.Text(), d.where.Value)
	}
	p := h.locate(&d.ok, modalArea(460), 8)
	h.click(p.X, p.Y)
	h.frame()
	if d.open || len(a.wb.Sheets) != 2 || a.active != 0 || a.wb.Sheets[0].Name != "Sheet2" || len(a.wb.Sheets[0].Pivots) != 1 {
		t.Fatalf("created: open %v, %d sheets, active %d", d.open, len(a.wb.Sheets), a.active)
	}
	if !a.showPane() || h.cell("A4").Text != "PivotTable1" {
		t.Fatalf("pane %v, A4 %q", a.showPane(), h.cell("A4").Text)
	}

	// Checking Region puts it in Rows, Sales (numbers) in Values.
	paneArea := image.Rectangle{Min: image.Pt(1000-paneWidth, gridTop), Max: image.Pt(1000, gridTop+300)}
	for _, f := range []int{0, 2} {
		p := h.locate(&a.pv.pane.checks[f], paneArea, 6)
		h.click(p.X, p.Y)
	}
	pt := a.wb.Sheets[0].Pivots[0]
	if !slices.Equal(pt.Rows, []int{0}) || len(pt.Values) != 1 || pt.Values[0].Name != "Sum of Sales" {
		t.Fatalf("checked: rows %v values %+v", pt.Rows, pt.Values)
	}
	h.expect("report", map[string]string{"A3": "Row Labels", "B3": "Sum of Sales", "A4": "East", "B4": "41", "A7": "Grand Total", "B7": "76"})
	if a.wb.Sheets[0].ColWidth[0] == 0 {
		t.Error("columns not fitted")
	}

	// Drag Region from Rows to Columns.
	h.dragMods(h.chipPt(areaRows, 0), h.boxPt(areaColumns), 0)
	pt = a.wb.Sheets[0].Pivots[0]
	if len(pt.Rows) != 0 || !slices.Equal(pt.Cols, []int{0}) || h.cell("B3").Text != "Column Labels" {
		t.Fatalf("dragged: rows %v cols %v, B3 %q", pt.Rows, pt.Cols, h.cell("B3").Text)
	}
	// Undo takes it back in one step.
	h.press("Z", key.ModShortcut)
	if pt := a.wb.Sheets[0].Pivots[0]; !slices.Equal(pt.Rows, []int{0}) || h.cell("A4").Text != "East" {
		t.Fatalf("undone: rows %v", pt.Rows)
	}

	// A click on a chip opens its menu: Move to Column Labels.
	a.selectRange(0, model.Range{R1: 3, R2: 3})
	h.frame()
	cp := h.chipPt(areaRows, 0)
	h.clickMods(cp, pointer.ButtonPrimary, 0)
	if !a.pv.pane.menu.IsOpen() {
		t.Fatal("chip menu not open")
	}
	for i, act := range a.pv.pane.menuActs {
		if act != nil && i == 7 { // Move to Column Labels
			act()
		}
	}
	a.pv.pane.menu.Close()
	h.frame()
	if pt := a.wb.Sheets[0].Pivots[0]; !slices.Equal(pt.Cols, []int{0}) {
		t.Fatalf("Move to Column Labels: cols %v", pt.Cols)
	}

	// Dragging a chip out of the areas removes it.
	h.dragMods(h.chipPt(areaColumns, 0), h.paneOrigin().Add(f32.Pt(100, 60)), 0)
	if pt := a.wb.Sheets[0].Pivots[0]; len(pt.Cols) != 0 {
		t.Fatalf("dragged out: cols %v", pt.Cols)
	}

	// The pane's X closes it; the Field List button shows it again.
	p = h.locate(&a.pv.pane.close, paneArea, 4)
	h.click(p.X, p.Y)
	if a.showPane() {
		t.Fatal("pane still showing")
	}
	h.click(h.menuPt(menuView+1).X, h.menuPt(menuView+1).Y)
	if a.tab != tabPivotAnalyze {
		t.Fatalf("tab %d", a.tab)
	}
	p = h.locate(&a.pv.an.fieldList, ribbonArea, 8)
	h.click(p.X, p.Y)
	if !a.showPane() {
		t.Fatal("Field List didn't show the pane")
	}
	// Leaving the report hides the contextual tabs and the pane.
	h.click(h.cellPt("F12").X, h.cellPt("F12").Y)
	if a.showPane() || len(a.menus()) != menuView+1 || a.tab != tabHome {
		t.Fatalf("outside: pane %v, %d menus, tab %d", a.showPane(), len(a.menus()), a.tab)
	}
}

// pivotHarness has the sales table and a report of Sum of Sales by
// Region and Product (rows) at E3 of the same sheet, its first cell
// selected.
func pivotHarness(t *testing.T) *harness {
	h := newHarness(t)
	a := h.a
	h.salesTable()
	pt, err := a.book.NewPivot(0, "Sheet1!$A$1:$C$7", 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	pt.Rows = []int{0, 1}
	pt.Values = []model.PivotValue{{Field: 2, Name: "Sum of Sales"}}
	a.applyPivot(0, -1, pt, nil)
	a.selectRange(0, model.Range{R1: 3, C1: 4, R2: 3, C2: 4})
	h.frame()
	return h
}

func TestPivotDropDownAndSettings(t *testing.T) {
	h := pivotHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	h.expect("report", map[string]string{"E3": "Row Labels", "E4": "East", "E5": "Ink", "F4": "41"})

	// Typing into the report is refused with Excel's message.
	h.enter("F5", "9")
	if !a.dialog.Visible || h.cell("F5").Text != "30" {
		t.Fatalf("typing: dialog %v, F5 %q", a.dialog.Visible, h.cell("F5").Text)
	}
	h.answerDialog(0)
	// Delete on part of it too; on all of it, it goes.
	a.selectRange(0, model.Range{R1: 4, C1: 5, R2: 4, C2: 5})
	h.press(key.NameDeleteForward, 0)
	if !a.dialog.Visible || len(sh.Pivots) != 1 {
		t.Fatalf("delete part: dialog %v", a.dialog.Visible)
	}
	h.answerDialog(0)

	// Row Labels' button opens the drop-down; unchecking East filters it.
	v := &a.state().view
	b, ok := v.PivotButtonRect(2, 4)
	if !ok {
		t.Fatal("no Row Labels button")
	}
	h.click(float32(b.Min.X+b.Max.X)/2, float32(gridTop+(b.Min.Y+b.Max.Y)/2))
	m := &a.pv.menu
	if !m.open || m.field != 0 || len(m.items) != 3 || m.items[0].label != "East" {
		t.Fatalf("drop-down: open %v field %d items %+v", m.open, m.field, m.items)
	}
	area := image.Rect(0, gridTop, 700, 700)
	p := h.locate(&m.rows[0], area, 6)
	h.click(p.X, p.Y)
	p = h.locate(&m.ok, area, 8)
	h.click(p.X, p.Y)
	if m.open || !slices.Equal(sh.Pivots[0].Fields[0].Hidden, []string{"s:east"}) || h.cell("E4").Text != "North" {
		t.Fatalf("filtered: hidden %v, E4 %q", sh.Pivots[0].Fields[0].Hidden, h.cell("E4").Text)
	}
	if bt := sh.Pivots[0].Out.Buttons[0]; !bt.Filtered {
		t.Error("button doesn't show the filter")
	}

	// Value Field Settings: Average, % of Grand Total.
	a.selectRange(0, model.Range{R1: 3, C1: 5, R2: 3, C2: 5})
	h.frame()
	a.valueFieldDialog(0, false)
	h.frame()
	d := &a.pv.dlg
	d.fn.Value = "2"
	d.showAs = int(model.ShowPercentOfTotal)
	p = h.locate(&d.ok, modalArea(460), 8)
	h.click(p.X, p.Y)
	pv := sh.Pivots[0].Values[0]
	if d.open || pv.Func != model.FuncAverage || pv.Name != "Average of Sales" || pv.ShowAs != model.ShowPercentOfTotal {
		t.Fatalf("value settings: %+v", pv)
	}

	// Field Settings: no subtotals for Region, a caption.
	a.fieldSettingsDialog(0)
	h.frame()
	d.name.SetText("Area")
	d.subtotals.Value = "none"
	p = h.locate(&d.ok, modalArea(460), 8)
	h.click(p.X, p.Y)
	if f := sh.Pivots[0].Fields[0]; f.Caption != "Area" || !f.NoSubtotal {
		t.Fatalf("field settings: %+v", f)
	}

	// Design: tabular form.
	a.setReportLayout(2)
	h.frame()
	h.expect("tabular", map[string]string{"E3": "Area", "F3": "Product", "E4": "North", "F4": "Ink"})
	a.setGrandTotals(0)
	h.frame()
	if pt := sh.Pivots[0]; !pt.NoRowGrand || !pt.NoColGrand {
		t.Fatalf("grand totals %v %v", pt.NoRowGrand, pt.NoColGrand)
	}
}

func TestPivotContextMenu(t *testing.T) {
	h := pivotHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	// Right-click a value: its menu has the value field's commands.
	p := h.cellPt("F5")
	h.clickMods(p, pointer.ButtonSecondary, 0)
	if !a.cm.grid.IsOpen() {
		t.Fatal("no menu")
	}
	var labels []string
	pt := sh.Pivots[0]
	items := a.pivotCellMenu(pt)
	for _, it := range items {
		labels = append(labels, it.label)
	}
	for _, want := range []string{"Copy", "Refresh", `Remove "Sum of Sales"`, "Summarize Values By", "Show Values As", "Show Details", "Value Field Settings…", "Hide Field List"} {
		if !slices.Contains(labels, want) {
			t.Errorf("no %q in %q", want, labels)
		}
	}
	// Summarize Values By opens a submenu; Count.
	h.chooseCtx(p, items, "Summarize Values By", gridOrigin, f32.Pt(gridSize.X-paneWidth, gridSize.Y))
	if !a.pv.ctx.IsOpen() {
		t.Fatal("no submenu")
	}
	a.pv.ctxActs[1]()
	a.pv.ctx.Close()
	h.frame()
	if v := sh.Pivots[0].Values[0]; v.Func != model.FuncCount || v.Name != "Count of Sales" || h.cell("F4").Text != "3" {
		t.Fatalf("Count: %+v, F4 %q", v, h.cell("F4").Text)
	}

	// Sort Z to A on a row label sorts the field.
	a.selectRange(0, model.Range{R1: 3, C1: 4, R2: 3, C2: 4})
	h.frame()
	a.sortDescending()
	h.frame()
	if sh.Pivots[0].Fields[0].Sort != model.SortDescending || h.cell("E4").Text != "West" {
		t.Fatalf("sorted: E4 %q", h.cell("E4").Text)
	}

	// A double-click on a value shows its records on a new sheet.
	p = h.cellPt("F4") // West's total
	h.clickMods(p, pointer.ButtonPrimary, 0)
	h.clickMods(p, pointer.ButtonPrimary, 0)
	if len(a.wb.Sheets) != 2 || a.active != 0 || h.cell("A2").Text != "West" || h.cell("A3").Text != "West" || h.cell("A4").Text != "" {
		t.Fatalf("details: %d sheets, active %d, A2 %q", len(a.wb.Sheets), a.active, h.cell("A2").Text)
	}
	h.press("Z", key.ModShortcut)
	if len(a.wb.Sheets) != 1 {
		t.Fatalf("details undone: %d sheets", len(a.wb.Sheets))
	}

	// Refresh All after a source edit.
	a.selectRange(0, model.Range{R1: 3, C1: 4, R2: 3, C2: 4})
	a.changePivot(func(p *model.PivotTable) { setValueFunc(p, 0, model.FuncSum) })
	if _, err := a.book.Enter(0, 1, 2, "100"); err != nil {
		t.Fatal(err)
	}
	a.refreshAllPivots()
	h.frame()
	found := false
	for r := 3; r < 15; r++ {
		found = found || sh.Get(r, 5).Text == "131"
	}
	if !found {
		var col []string
		for r := 2; r < 15; r++ {
			col = append(col, sh.Get(r, 4).Text+"="+sh.Get(r, 5).Text)
		}
		t.Errorf("Refresh All didn't pick up the edit: %q %q", col, a.errMsg)
	}
}
