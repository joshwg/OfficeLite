package ui

import (
	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/widget"

	"officelite/excel/model"
)

// locate finds a point over btn by moving the pointer across area
// (window coordinates).
func (h *harness) locate(btn *widget.Clickable, area image.Rectangle, step int) f32.Point {
	h.t.Helper()
	for y := area.Min.Y + step/2; y < area.Max.Y; y += step {
		for x := area.Min.X + step/2; x < area.Max.X; x += step {
			p := f32.Pt(float32(x), float32(y))
			h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p})
			h.frame()
			if btn.Hovered() {
				return p
			}
		}
	}
	h.t.Fatalf("button not found in %v", area)
	return f32.Point{}
}

// menuPt is the middle of a menu bar header.
func (h *harness) menuPt(i int) f32.Point {
	r := h.a.menu.HeaderRect(i)
	return f32.Pt(float32(r.Min.X+r.Max.X)/2, float32(36+r.Min.Y+r.Max.Y/2))
}

// ribbonArea is the ribbon, in window coordinates.
var ribbonArea = image.Rect(0, 36+30, 700, 36+30+ribbonHeight-18)

// fillTable enters a small table in A1:C7.
func (h *harness) fillTable() {
	a := h.a
	rows := [][]string{
		{"Name", "Qty", "Kind"},
		{"apple", "10", "fruit"},
		{"bean", "25", "veg"},
		{"cherry", "5", "fruit"},
		{"date", "40", "fruit"},
		{"", "15", "veg"},
		{"endive", "30", "veg"},
	}
	for r, row := range rows {
		for c, v := range row {
			if v != "" {
				if _, err := a.book.Enter(0, r, c, v); err != nil {
					h.t.Fatal(err)
				}
			}
		}
	}
	a.syncSheets()
	h.frame()
}

// hiddenRows lists the 1-based rows of 2..7 that are hidden.
func (h *harness) hiddenRows() []int {
	var out []int
	sh := h.a.wb.Sheets[0]
	for r := 1; r < 7; r++ {
		if sh.RowHidden(r) {
			out = append(out, r+1)
		}
	}
	return out
}

func TestDataTabAndFilterButton(t *testing.T) {
	h := newHarness(t)
	a := h.a
	if a.tab != tabHome || !a.menus()[menuHome].Selected || a.menus()[menuData].Label != "Data" {
		t.Fatalf("start: tab %d", a.tab)
	}
	// Clicking the Data tab shows the Data ribbon; a tab opens no menu.
	h.click(h.menuPt(menuData).X, h.menuPt(menuData).Y)
	if a.tab != tabData || a.menu.IsOpen() || !a.menus()[menuData].Selected || a.menus()[menuHome].Selected {
		t.Fatalf("Data: tab %d, open %v", a.tab, a.menu.IsOpen())
	}
	h.click(h.menuPt(menuHome).X, h.menuPt(menuHome).Y)
	if a.tab != tabHome || a.menu.IsOpen() {
		t.Fatalf("Home: tab %d, open %v", a.tab, a.menu.IsOpen())
	}
	h.click(h.menuPt(menuData).X, h.menuPt(menuData).Y)

	h.fillTable()
	a.selectRange(0, model.Range{R1: 2, C1: 1, R2: 2, C2: 1})
	h.frame()
	p := h.locate(&a.data.filter, ribbonArea, 8)
	h.click(p.X, p.Y)
	sh := a.wb.Sheets[0]
	if sh.AutoFilter == nil || sh.AutoFilter.Range != (model.Range{R2: 6, C2: 2}) || !a.modified {
		t.Fatalf("Filter: %+v", sh.AutoFilter)
	}
	h.press("L", key.ModShortcut|key.ModShift)
	if sh.AutoFilter != nil {
		t.Fatal("Ctrl+Shift+L left the filter on")
	}
	h.press("Z", key.ModShortcut)
	if sh.AutoFilter == nil {
		t.Fatal("undo did not bring the filter back")
	}

	// The header buttons take a click before the cells: the drop-down
	// opens and the selection stays.
	v := &a.state().view
	b, ok := v.FilterButtonRect(1)
	if !ok {
		t.Fatal("no button on B1")
	}
	before := v.Sel
	h.click(float32(b.Min.X+b.Max.X)/2, float32(gridTop+(b.Min.Y+b.Max.Y)/2))
	if !a.filt.open || a.filt.col != 1 || v.Sel.Active != before.Active {
		t.Fatalf("button: open %v col %d, active %v", a.filt.open, a.filt.col, v.Sel.Active)
	}
	if got := len(a.filt.items); got != 6 {
		t.Errorf("%d items", got)
	}
	// Clicking elsewhere closes it without filtering.
	h.click(h.cellPt("F12").X, h.cellPt("F12").Y)
	if a.filt.open || sh.AutoFilter.Active() {
		t.Fatalf("outside click: open %v, active %v", a.filt.open, sh.AutoFilter.Active())
	}

	// Home's tab brings the Home ribbon back.
	h.click(h.menuPt(menuHome).X, h.menuPt(menuHome).Y)
	h.press(key.NameEscape, 0)
	if a.tab != tabHome {
		t.Errorf("Home: tab %d", a.tab)
	}
}

// openFilter clicks column col's button.
func (h *harness) openFilter(col int) {
	h.t.Helper()
	b, ok := h.a.state().view.FilterButtonRect(col)
	if !ok {
		h.t.Fatalf("no button on column %d", col)
	}
	h.click(float32(b.Min.X+b.Max.X)/2, float32(gridTop+(b.Min.Y+b.Max.Y)/2))
	if !h.a.filt.open {
		h.t.Fatal("drop-down did not open")
	}
}

// popupArea is the open drop-down's area, in window coordinates.
func (h *harness) popupArea() image.Rectangle {
	at := h.a.filt.at
	return image.Rect(at.X, gridTop+at.Y, at.X+filterMenuWidth, gridTop+at.Y+480)
}

func TestFilterDropDown(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.fillTable()
	sh := a.wb.Sheets[0]
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 0, C2: 0})
	a.toggleFilter()
	h.frame()

	// Uncheck "fruit" in the Kind column's list and press OK.
	h.openFilter(2)
	var items []string
	for _, it := range a.filt.items {
		items = append(items, it.Text)
	}
	if !slices.Equal(items, []string{"fruit", "veg"}) {
		t.Fatalf("items %q", items)
	}
	p := h.locate(&a.filt.rows[0], h.popupArea(), 6)
	h.click(p.X, p.Y)
	if a.filt.items[0].Checked {
		t.Fatal("fruit still checked")
	}
	p = h.locate(&a.filt.ok, h.popupArea(), 6)
	h.click(p.X, p.Y)
	if a.filt.open || !slices.Equal(h.hiddenRows(), []int{2, 4, 5}) {
		t.Fatalf("OK: open %v, hidden %v", a.filt.open, h.hiddenRows())
	}
	if a.notice != "3 of 6 records found" {
		t.Errorf("notice %q", a.notice)
	}
	// Filtered rows take no space: row 6 follows row 3 on screen.
	v := &a.state().view
	r3, _, _ := v.CellRect(2, 0)
	r6, _, ok := v.CellRect(5, 0)
	if !ok || r6.Min.Y != r3.Max.Y {
		t.Errorf("row 3 %v, row 6 %v", r3, r6)
	}

	// Number Filters > Greater Than… on Qty, through the Custom
	// AutoFilter dialog.
	h.openFilter(1)
	p = h.locate(&a.filt.sub, h.popupArea(), 6)
	h.click(p.X, p.Y)
	if !a.filt.subMenu.IsOpen() {
		t.Fatal("submenu not open")
	}
	fm := &a.filt
	at := f32.Pt(float32(fm.at.X+filterMenuWidth-4), float32(gridTop+fm.at.Y+fm.subY))
	h.chooseCtx(at, a.subItems(1, fm.kind), "Greater Than…", gridOrigin, gridSize)
	if a.ins.kind != dlgCustomFilter || a.ins.cf.op[0] != opGreater {
		t.Fatalf("dialog %v, op %d", a.ins.kind, a.ins.cf.op[0])
	}
	a.ins.cf.val[0].SetText("20")
	h.press(key.NameReturn, 0)
	if a.ins.kind != dlgNone || !slices.Equal(h.hiddenRows(), []int{2, 4, 5, 6}) {
		t.Fatalf("custom: dialog %v, hidden %v", a.ins.kind, h.hiddenRows())
	}
	fc, _ := sh.AutoFilter.Column(1)
	if fc.Kind != model.FilterCustom || fc.Conds[0] != (model.FilterCond{Op: model.OpGreater, Val: "20"}) {
		t.Errorf("Qty criteria %+v", fc)
	}
	// Filtered columns show the funnel and the rows' numbers turn blue;
	// the list of another column holds only what is shown.
	h.openFilter(0)
	if n := len(a.filt.items); n != 2 {
		t.Errorf("Name items %d", n)
	}
	// Sort Z to A from the drop-down.
	p = h.locate(&a.filt.sortDesc, h.popupArea(), 6)
	h.click(p.X, p.Y)
	if got := h.cell("A2").Text; got != "endive" {
		t.Errorf("after Sort Z to A: A2 = %q", got)
	}
	if !slices.Equal(h.hiddenRows(), []int{3, 4, 6, 7}) {
		t.Errorf("after sort: hidden %v", h.hiddenRows())
	}

	// Undo takes back the sort, then the Qty criteria; Clear shows all.
	h.press("Z", key.ModShortcut)
	if got := h.cell("A2").Text; got != "apple" || !slices.Equal(h.hiddenRows(), []int{2, 4, 5, 6}) {
		t.Errorf("undo sort: A2 %q, hidden %v", got, h.hiddenRows())
	}
	h.press("Z", key.ModShortcut)
	if !slices.Equal(h.hiddenRows(), []int{2, 4, 5}) {
		t.Errorf("undo criteria: hidden %v", h.hiddenRows())
	}
	a.tab = tabData
	h.frame()
	p = h.locate(&a.data.clear, ribbonArea, 6)
	h.click(p.X, p.Y)
	if len(h.hiddenRows()) != 0 || sh.AutoFilter == nil || sh.AutoFilter.Active() {
		t.Errorf("Clear: hidden %v", h.hiddenRows())
	}

	// Search narrows the list; OK keeps just what it found.
	h.openFilter(0)
	a.filt.search.SetText("ea")
	h.frame()
	if n := len(a.filt.shownItems()); n != 1 {
		t.Fatalf("search shows %d", n)
	}
	h.press(key.NameReturn, 0)
	if !slices.Equal(h.hiddenRows(), []int{2, 4, 5, 6, 7}) {
		t.Errorf("search: hidden %v", h.hiddenRows())
	}
	if !strings.HasPrefix(a.notice, "1 of 6") {
		t.Errorf("notice %q", a.notice)
	}
}

func TestFilterTop10AndAverage(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.fillTable()
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	a.toggleFilter()
	h.frame()
	a.top10Dialog(1)
	h.frame()
	a.ins.top.n.SetText("2")
	h.press(key.NameReturn, 0)
	if !slices.Equal(h.hiddenRows(), []int{2, 3, 4, 6}) {
		t.Errorf("top 2: hidden %v", h.hiddenRows())
	}
	h.openFilter(1)
	for _, it := range a.subItems(1, a.filt.kind) {
		if it.label == "Below Average" {
			a.closeFilterMenu()
			it.run()
		}
	}
	h.frame()
	if !slices.Equal(h.hiddenRows(), []int{3, 5, 7}) {
		t.Errorf("below average: hidden %v", h.hiddenRows())
	}

	// Alt+Down on a header cell opens its drop-down; Esc closes it.
	a.selectRange(0, model.Range{R1: 0, C1: 2, R2: 0, C2: 2})
	h.frame()
	h.press(key.NameDownArrow, key.ModAlt)
	if !a.filt.open || a.filt.col != 2 {
		t.Fatalf("Alt+Down: open %v col %d", a.filt.open, a.filt.col)
	}
	h.press(key.NameEscape, 0)
	if a.filt.open {
		t.Error("Esc left the drop-down open")
	}
}

// Ctrl+D, Delete and Ctrl+C under an AutoFilter act on the rows it shows
// only, as in Excel.
func TestFilteredRowsCommands(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.fillTable()
	r := model.Range{R2: 6, C2: 2}
	a.book.SetAutoFilter(0, &r)
	a.book.FilterColumn(0, 1, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "25"}}})
	a.afterFilter(false)
	h.frame()
	if got := h.hiddenRows(); !slices.Equal(got, []int{2, 4, 6}) {
		t.Fatalf("hidden %v", got)
	}
	a.selectRange(0, model.Range{R1: 2, C1: 1, R2: 6, C2: 1}) // B3:B7
	h.press("D", key.ModShortcut)
	h.expect("Ctrl+D", map[string]string{"B4": "5", "B5": "25", "B6": "15", "B7": "25"})
	a.selectRange(0, model.Range{R1: 1, C1: 0, R2: 6, C2: 0}) // A2:A7
	h.press(key.NameDeleteForward, 0)
	h.expect("Delete", map[string]string{"A2": "apple", "A3": "", "A4": "cherry", "A5": "", "A7": ""})
	a.undo()
	a.selectRange(0, model.Range{R1: 1, C1: 0, R2: 6, C2: 0})
	a.copy(a.gtx, false)
	if want := "bean\r\ndate\r\nendive\r\n"; a.clipText != want {
		t.Errorf("copied %q, want %q", a.clipText, want)
	}
	a.selectRange(0, model.Range{R1: 9, C1: 4, R2: 9, C2: 4}) // E10
	a.paste(a.clipText)
	h.expect("paste", map[string]string{"E10": "bean", "E11": "date", "E12": "endive", "E13": ""})
}
