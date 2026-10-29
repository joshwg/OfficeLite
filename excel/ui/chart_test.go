package ui

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/internal/chartsample"
	"officelite/excel/model"
	"officelite/excel/render"
)

// chartPt is a point inside a chart (window coordinates): its middle,
// or near its top-left corner.
func (h *harness) chartPt(c *model.Chart, corner bool) f32.Point {
	h.t.Helper()
	rc, ok := h.a.state().view.ChartRect(c)
	if !ok {
		h.t.Fatalf("chart %s not on screen", c.Name)
	}
	if corner {
		return f32.Pt(float32(rc.Min.X+12), float32(gridTop+rc.Min.Y+12))
	}
	return f32.Pt(float32(rc.Min.X+rc.Max.X)/2, float32(gridTop+(rc.Min.Y+rc.Max.Y)/2))
}

func chartHarness(t *testing.T) *harness {
	h := newHarness(t)
	h.a.setWorkbook(chartsample.Workbook())
	h.frame()
	h.frame()
	return h
}

func TestChartSelectDeleteUndo(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	first := sh.Charts[0]
	n := len(sh.Charts)

	// A click on the chart selects it, not the cell under it.
	before := h.active()
	h.clickMods(h.chartPt(first, false), pointer.ButtonPrimary, 0)
	if v.SelChart != first || h.active() != before {
		t.Fatalf("click: selected %v, active %s (was %s)", v.SelChart != nil, h.active(), before)
	}
	// Typing doesn't start an entry while a chart is selected.
	h.typeText("7")
	if a.ed.active {
		t.Fatal("typing edited a cell under a chart")
	}
	// Delete deletes it; Ctrl+Z brings it back.
	h.press(key.NameDeleteForward, 0)
	if len(sh.Charts) != n-1 || sh.Charts[0] == first || v.SelChart != nil {
		t.Fatalf("delete: %d charts, selection %v", len(sh.Charts), v.SelChart)
	}
	if !a.modified {
		t.Error("deleting a chart isn't a change")
	}
	h.press("Z", key.ModShortcut)
	if len(sh.Charts) != n || sh.Charts[0].Name != first.Name {
		t.Fatalf("undo: %d charts", len(sh.Charts))
	}
	h.press("Y", key.ModShortcut)
	if len(sh.Charts) != n-1 {
		t.Fatalf("redo: %d charts", len(sh.Charts))
	}
	h.press("Z", key.ModShortcut)

	// A click on a cell drops the selection and selects the cell.
	h.clickMods(h.chartPt(sh.Charts[0], false), pointer.ButtonPrimary, 0)
	if v.SelChart == nil {
		t.Fatal("not selected again")
	}
	h.clickMods(h.cellPt("B3"), pointer.ButtonPrimary, 0)
	if v.SelChart != nil || h.active() != "B3" {
		t.Fatalf("cell click: chart %v, active %s", v.SelChart != nil, h.active())
	}
	// Delete there clears the cell, not a chart.
	h.press(key.NameDeleteForward, 0)
	if len(sh.Charts) != n || h.cell("B3").Text != "" {
		t.Errorf("delete on a cell: %d charts, B3 %q", len(sh.Charts), h.cell("B3").Text)
	}
	// Esc drops a selection too.
	h.clickMods(h.chartPt(sh.Charts[2], true), pointer.ButtonPrimary, 0)
	h.press(key.NameEscape, 0)
	if v.SelChart != nil {
		t.Error("Esc kept the chart selected")
	}
}

// Inserting rows above a chart moves it and its references, undoably.
func TestChartFollowsInsertedRows(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	c := sh.Charts[2] // rows 16..31
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 1, C2: model.MaxCols - 1})
	h.frame()
	h.press("=", key.ModShortcut|key.ModShift)
	c = sh.Charts[2]
	if c.Anchor.From.Row != 18 || c.Plots[0].Series[0].Val.Ref != "Data!$B$4:$B$9" {
		t.Fatalf("insert: anchor %+v ref %s", c.Anchor.From, c.Plots[0].Series[0].Val.Ref)
	}
	h.press("Z", key.ModShortcut)
	c = sh.Charts[2]
	if c.Anchor.From.Row != 16 || c.Plots[0].Series[0].Val.Ref != "Data!$B$2:$B$7" {
		t.Fatalf("undo: anchor %+v ref %s", c.Anchor.From, c.Plots[0].Series[0].Val.Ref)
	}
}

// A chart sheet shows its chart; commands that change cells are off.
func TestChartSheet(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	h.press(key.NamePageDown, key.ModShortcut)
	if a.active != 1 || !a.onChartSheet() {
		t.Fatalf("active sheet %d", a.active)
	}
	h.frame()
	v := &a.state().view
	h.clickMods(h.chartPt(a.wb.Sheets[1].ChartSheet, false), pointer.ButtonPrimary, 0)
	if v.SelChart != a.wb.Sheets[1].ChartSheet {
		t.Error("chart sheet's chart not selected")
	}
	h.typeText("5")
	h.press(key.NameDeleteForward, 0)
	h.press("D", key.ModShortcut)
	if a.ed.active || !a.wb.Sheets[1].IsChartSheet() || a.wb.Sheets[1].UsedRows() != 0 || a.modified {
		t.Errorf("chart sheet changed: editing %v, rows %d, modified %v", a.ed.active, a.wb.Sheets[1].UsedRows(), a.modified)
	}
	for _, m := range a.menus() {
		for _, it := range m.Items {
			if it.Label == "Fill Down" && !it.Disabled {
				t.Error("Fill Down is on for a chart sheet")
			}
			if it.Label == "Insert Sheet" && it.Disabled {
				t.Error("Insert Sheet is off for a chart sheet")
			}
		}
	}
	// Back to the worksheet: everything works again.
	h.press(key.NamePageUp, key.ModShortcut)
	if a.active != 0 || a.onChartSheet() {
		t.Fatalf("back: sheet %d", a.active)
	}
	h.enter("A30", "ok")
	h.expect("worksheet", map[string]string{"A30": "ok"})
}

// chartRibbon is the whole ribbon (the Charts group is past
// ribbonArea), in window coordinates.
var chartRibbon = image.Rect(0, 36+30, 1000, 36+30+ribbonHeight-18)

// dataArea is where Select Data Source, a tall dialog, lies.
var dataArea = image.Rect(220, 40, 780, 700)

// clickAt clicks at p at time t (for double-clicks).
func (h *harness) clickAt(p f32.Point, t time.Duration) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p, Time: t})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p, Time: t})
	h.frame()
}

// chartTable enters a small table: months down A, two regions.
func (h *harness) chartTable() {
	rows := [][]string{{"Month", "North", "South"}, {"Jan", "10", "20"}, {"Feb", "15", "25"}, {"Mar", "12", "22"}}
	for r, row := range rows {
		for c, v := range row {
			if _, err := h.a.book.Enter(0, r, c, v); err != nil {
				h.t.Fatal(err)
			}
		}
	}
	h.a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	h.frame()
}

// Insert > Column Chart, the Data ribbon's Combo, Alt+F1 and F11 make
// charts from the data around the active cell, each one undo step.
func TestInsertChart(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.chartTable()
	sh := a.wb.Sheets[0]
	a.menuChosen(gui.MenuEvent{Menu: menuInsert, Item: insColumnChart})
	h.frame()
	if len(sh.Charts) != 1 {
		t.Fatalf("%d charts", len(sh.Charts))
	}
	c := sh.Charts[0]
	if c.Plots[0].Type != model.ChartBar || len(c.Plots[0].Series) != 2 || c.Plots[0].Series[1].Val.Ref != "Sheet1!$C$2:$C$4" {
		t.Fatalf("chart %+v", c.Plots)
	}
	// Right of the data, one column clear, Excel's default size.
	if c.Anchor.From.Col != 4 || c.Anchor.From.Row != 0 {
		t.Errorf("placed at %+v", c.Anchor.From)
	}
	x0, y0, x1, y1 := render.ChartBox(c, a.state().view.Geo)
	if math.Abs(x1-x0-newChartW) > 1 || math.Abs(y1-y0-newChartH) > 1 {
		t.Errorf("size %vx%v", x1-x0, y1-y0)
	}
	if a.state().view.SelChart != c || a.tab != tabChartDesign {
		t.Errorf("not selected, tab %d", a.tab)
	}
	h.press("Z", key.ModShortcut)
	if len(sh.Charts) != 0 {
		t.Fatal("undo left the chart")
	}
	// The Data ribbon's Combo button.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	h.dataTab()
	p := h.locate(&a.ch.charts.combo, chartRibbon, 10)
	h.click(p.X, p.Y)
	if len(sh.Charts) != 1 || len(sh.Charts[0].Plots) != 2 || sh.Charts[0].Plots[1].Type != model.ChartLine {
		t.Fatalf("combo: %d charts", len(sh.Charts))
	}
	// Alt+F1: the default chart beside the data.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	h.frame()
	h.press(key.NameF1, key.ModAlt)
	if len(sh.Charts) != 2 || sh.Charts[1].Plots[0].Type != model.ChartBar || sh.Charts[1].Name != "Chart 2" {
		t.Fatalf("Alt+F1: %d charts", len(sh.Charts))
	}
	// F11: a chart sheet before the active sheet.
	a.selectRange(0, model.Range{R1: 1, C1: 1, R2: 1, C2: 1})
	h.frame()
	h.press(key.NameF11, 0)
	if len(a.wb.Sheets) != 2 || !a.wb.Sheets[0].IsChartSheet() || a.active != 0 || a.wb.Sheets[0].Name != "Chart1" {
		t.Fatalf("F11: %d sheets, active %d", len(a.wb.Sheets), a.active)
	}
	h.frame()
	// The chart tabs show on a chart sheet, and its ribbon works.
	if a.tab != tabChartDesign {
		t.Errorf("tab %d on the chart sheet", a.tab)
	}
	h.press("Z", key.ModShortcut)
	if len(a.wb.Sheets) != 1 || a.wb.Sheets[0].IsChartSheet() {
		t.Errorf("undo F11: %d sheets", len(a.wb.Sheets))
	}
	// An empty sheet has nothing to chart: Excel's message, no chart.
	a.selectRange(0, model.Range{R1: 30, C1: 10, R2: 30, C2: 10})
	h.frame()
	h.press(key.NameF1, key.ModAlt)
	if len(sh.Charts) != 2 || !a.dialog.Visible {
		t.Errorf("chart from nothing: %d charts, message %v", len(sh.Charts), a.dialog.Visible)
	}
}

// Dragging a chart moves it, a handle sizes it (Shift keeps a corner's
// proportions, Alt snaps to the cells), an arrow nudges it; each is one
// undo step.
func TestChartMoveResize(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	c := sh.Charts[0]
	from := c.Anchor
	mid := h.chartPt(c, false)
	h.clickMods(mid, pointer.ButtonPrimary, 0)
	// A drag: the outline follows, the chart moves on release.
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: mid})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: mid.Add(f32.Pt(70, 45))})
	h.frame()
	if d := a.ch.drag; d == nil || !d.moved || sh.Charts[0].Anchor != from {
		t.Fatal("no outline while dragging, or moved too soon")
	}
	if h.r.Cursor() != pointer.CursorAllScroll {
		t.Errorf("cursor %v while moving", h.r.Cursor())
	}
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: mid.Add(f32.Pt(70, 45))})
	h.frame()
	g := v.Geo
	x0, y0, x1, y1 := render.ChartBox(sh.Charts[0], g)
	ox0, oy0, ox1, oy1 := render.ChartBox(c, g)
	if math.Abs(x0-ox0-70) > 1 || math.Abs(y0-oy0-45) > 1 || math.Abs((x1-x0)-(ox1-ox0)) > 1 || math.Abs((y1-y0)-(oy1-oy0)) > 1 {
		t.Fatalf("moved to %v,%v-%v,%v from %v,%v-%v,%v", x0, y0, x1, y1, ox0, oy0, ox1, oy1)
	}
	if v.SelChart != sh.Charts[0] {
		t.Error("the moved chart isn't selected")
	}
	h.press("Z", key.ModShortcut)
	if sh.Charts[0].Anchor != from {
		t.Fatalf("undo move: %+v", sh.Charts[0].Anchor)
	}
	// Alt: the top-left corner snaps to a cell's corner.
	h.clickMods(h.chartPt(sh.Charts[0], false), pointer.ButtonPrimary, 0)
	h.dragMods(h.chartPt(sh.Charts[0], false), h.chartPt(sh.Charts[0], false).Add(f32.Pt(37, 23)), key.ModAlt)
	if a := sh.Charts[0].Anchor; a.From.DX != 0 || a.From.DY != 0 || a == from {
		t.Errorf("Alt drag: %+v", a.From)
	}
	h.press("Z", key.ModShortcut)
	// The bottom-right handle sizes it; the cursor says so.
	h.clickMods(h.chartPt(sh.Charts[0], false), pointer.ButtonPrimary, 0)
	rc, _ := v.ChartRect(sh.Charts[0])
	hp := render.ChartHandles(rc)[4]
	hpt := f32.Pt(float32(hp.X), float32(gridTop+hp.Y))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: hpt})
	h.frame()
	if h.r.Cursor() != pointer.CursorNorthWestSouthEastResize {
		t.Errorf("cursor %v over the corner handle", h.r.Cursor())
	}
	h.drag(hpt, hpt.Add(f32.Pt(-100, 40)))
	x0, y0, x1, y1 = render.ChartBox(sh.Charts[0], g)
	if math.Abs(x0-ox0) > 1 || math.Abs(y0-oy0) > 1 || math.Abs((x1-x0)-(ox1-ox0-100)) > 1 || math.Abs((y1-y0)-(oy1-oy0+40)) > 1 {
		t.Errorf("sized to %v,%v-%v,%v", x0, y0, x1, y1)
	}
	h.press("Z", key.ModShortcut)
	// Shift keeps the proportions.
	h.clickMods(h.chartPt(sh.Charts[0], false), pointer.ButtonPrimary, 0)
	h.dragMods(hpt, hpt.Add(f32.Pt(60, 5)), key.ModShift)
	x0, y0, x1, y1 = render.ChartBox(sh.Charts[0], g)
	if r0, r1 := (ox1-ox0)/(oy1-oy0), (x1-x0)/(y1-y0); math.Abs(r0-r1) > 0.02 || x1-x0 < ox1-ox0+50 {
		t.Errorf("Shift: ratio %v, was %v (width %v)", r1, r0, x1-x0)
	}
	h.press("Z", key.ModShortcut)
	if sh.Charts[0].Anchor != from {
		t.Fatalf("undo size: %+v", sh.Charts[0].Anchor)
	}
	// Arrows nudge a pixel.
	h.clickMods(h.chartPt(sh.Charts[0], false), pointer.ButtonPrimary, 0)
	h.press(key.NameRightArrow, 0)
	h.press(key.NameDownArrow, 0)
	x0, y0, _, _ = render.ChartBox(sh.Charts[0], g)
	if math.Abs(x0-ox0-1) > 0.01 || math.Abs(y0-oy0-1) > 0.01 || v.SelChart == nil {
		t.Errorf("nudged to %v,%v from %v,%v", x0, y0, ox0, oy0)
	}
	h.press("Z", key.ModShortcut)
	h.press("Z", key.ModShortcut)
	if sh.Charts[0].Anchor != from {
		t.Errorf("undo nudges: %+v", sh.Charts[0].Anchor)
	}
}

// The Chart Design tab's buttons change the selected chart, each one
// undo step; Select Data's Cancel changes nothing, its OK the series.
func TestChartDesignTab(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	h.clickMods(h.chartPt(sh.Charts[0], true), pointer.ButtonPrimary, 0)
	tabs := 0
	for _, m := range a.menus() {
		if m.Label == "Chart Design" || m.Label == "Format" && m.Tab {
			tabs++
		}
	}
	if tabs != 2 {
		t.Fatalf("%d chart tabs", tabs)
	}
	a.tab = tabChartDesign
	h.frame()
	// Switch Row/Column: 3 series of 6 months become 6 of 3.
	p := h.locate(&a.ch.design.switchRC, chartRibbon, 10)
	h.click(p.X, p.Y)
	if n := len(sh.Charts[0].AllSeries()); n != 6 {
		t.Fatalf("switched: %d series", n)
	}
	if sh.Charts[0].Raw != nil {
		t.Error("an edited chart keeps its part as read")
	}
	h.press("Z", key.ModShortcut)
	if n := len(sh.Charts[0].AllSeries()); n != 3 {
		t.Fatalf("undo switch: %d series", n)
	}
	// Add Chart Element: legend at the top, labels, no title.
	h.clickMods(h.chartPt(sh.Charts[0], true), pointer.ButtonPrimary, 0)
	a.addElement(23)
	a.addElement(13)
	a.addElement(7)
	c := sh.Charts[0]
	if c.Legend == nil || c.Legend.Pos != "t" || c.Title != nil || !c.Plots[0].Labels.Any() {
		t.Errorf("elements: legend %+v title %+v", c.Legend, c.Title)
	}
	items := elementItems(c)
	if !strings.HasPrefix(items[23].Label, "✓") || strings.HasPrefix(items[22].Label, "✓") {
		t.Errorf("legend items %q %q", items[22].Label, items[23].Label)
	}
	h.press("Z", key.ModShortcut)
	h.press("Z", key.ModShortcut)
	h.press("Z", key.ModShortcut)
	if c := sh.Charts[0]; c.Legend.Pos != "b" || c.Title == nil {
		t.Errorf("undo elements: %+v", c.Legend)
	}
	// Change Colors, Chart Styles.
	h.clickMods(h.chartPt(sh.Charts[0], true), pointer.ButtonPrimary, 0)
	a.paletteChosen(6) // Monochromatic Palette 1
	if f := sh.Charts[0].Plots[0].Series[0].Shape.Fill; f.Kind != model.FillSolid {
		t.Errorf("palette: %+v", f)
	}
	// Select Data: Cancel changes nothing.
	p = h.locate(&a.ch.design.data, chartRibbon, 10)
	h.click(p.X, p.Y)
	d := &a.ch.dlg
	if d.kind != cdData || d.rangeEd.Text() != "=Data!$A$1:$D$7" {
		t.Fatalf("Select Data: %d %q", d.kind, d.rangeEd.Text())
	}
	d.rangeEd.SetText("=Data!$A$1:$B$7")
	h.frame()
	q := h.locate(&d.cancel, dataArea, 10)
	h.click(q.X, q.Y)
	if d.kind != cdNone || len(sh.Charts[0].AllSeries()) != 3 {
		t.Fatalf("cancel: %d series", len(sh.Charts[0].AllSeries()))
	}
	// OK with a narrower range: one series.
	h.click(p.X, p.Y)
	d.rangeEd.SetText("=Data!$A$1:$B$7")
	h.frame()
	h.frame()
	if len(a.workSeries()) != 1 {
		t.Fatalf("the range typed isn't applied: %d series", len(a.workSeries()))
	}
	q = h.locate(&d.ok, dataArea, 10)
	h.click(q.X, q.Y)
	if c := sh.Charts[0]; len(c.AllSeries()) != 1 || c.Plots[0].Series[0].Val.Ref != "Data!$B$2:$B$7" {
		t.Fatalf("OK: %d series", len(c.AllSeries()))
	}
	// Edit Series: add one by its references.
	h.click(p.X, p.Y)
	a.openEditSeries(-1)
	h.frame()
	d.serName.SetText("Extra")
	d.serVals.SetText("=Data!$D$2:$D$7")
	q = h.locate(&d.ok, modalArea(440), 8)
	h.click(q.X, q.Y)
	if d.kind != cdData || len(a.workSeries()) != 2 {
		t.Fatalf("Edit Series: dialog %d, %d series", d.kind, len(a.workSeries()))
	}
	q = h.locate(&d.ok, dataArea, 10)
	h.click(q.X, q.Y)
	if c := sh.Charts[0]; len(c.AllSeries()) != 2 || c.Plots[0].Series[1].Name.Literal != "Extra" {
		t.Fatalf("added series: %+v", c.Plots[0].Series)
	}
	// Change Chart Type: a line.
	p = h.locate(&a.ch.design.kind, chartRibbon, 10)
	h.click(p.X, p.Y)
	if d.kind != cdType || d.pick != edit.ColumnClustered {
		t.Fatalf("Change Chart Type: %d %v", d.kind, d.pick)
	}
	d.pick = edit.LineMarkers
	q = h.locate(&d.ok, modalArea(660), 8)
	h.click(q.X, q.Y)
	if c := sh.Charts[0]; c.Plots[0].Type != model.ChartLine || !c.Plots[0].Marker {
		t.Errorf("type: %+v", c.Plots[0])
	}
	// Move Chart to a new sheet, then back.
	p = h.locate(&a.ch.design.move, chartRibbon, 10)
	h.click(p.X, p.Y)
	d.sheetName.SetText("Moved")
	q = h.locate(&d.ok, modalArea(460), 8)
	h.click(q.X, q.Y)
	if a.wb.Sheets[1].Name != "Moved" || !a.wb.Sheets[1].IsChartSheet() || a.active != 1 || len(a.wb.Sheets[0].Charts) != len(chartsample.Workbook().Sheets[0].Charts)-1 {
		t.Fatalf("moved: active %d, sheet %q", a.active, a.wb.Sheets[1].Name)
	}
	h.frame()
	h.press("Z", key.ModShortcut)
	if len(a.wb.Sheets) != 2 || a.wb.Sheets[1].IsChartSheet() && a.wb.Sheets[1].Name == "Moved" {
		t.Errorf("undo move: %d sheets", len(a.wb.Sheets))
	}
}

// Right-clicking a chart selects it and opens its menu, whose items run
// the chart commands; the Format tab's colours colour the selected part.
func TestChartMenuAndFormat(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	n := len(sh.Charts)
	p := h.chartPt(sh.Charts[0], true)
	h.clickMods(p, pointer.ButtonSecondary, 0)
	if v.SelChart != sh.Charts[0] || !a.cm.grid.IsOpen() {
		t.Fatal("right-click: no chart menu")
	}
	items := a.chartMenu()
	h.chooseCtx(p, items, "Change Chart Type…", gridOrigin, gridSize)
	if a.ch.dlg.kind != cdType {
		t.Fatalf("menu item: dialog %d", a.ch.dlg.kind)
	}
	h.press(key.NameEscape, 0)
	if a.ch.dlg.kind != cdNone {
		t.Fatal("Esc kept the dialog")
	}
	// Format Chart Area: a fill.
	h.clickMods(p, pointer.ButtonSecondary, 0)
	h.chooseCtx(p, a.chartMenu(), "Format Chart Area…", gridOrigin, gridSize)
	d := &a.ch.dlg
	if d.kind != cdFormat {
		t.Fatalf("format: %d", d.kind)
	}
	d.fill, d.fillSet = gui.ColorChoice{Color: colors.RGB(0xFFF2CC)}, true
	q := h.locate(&d.ok, modalArea(400), 8)
	h.click(q.X, q.Y)
	if f := sh.Charts[0].ChartArea.Fill; f.Kind != model.FillSolid || f.Color != colors.RGB(0xFFF2CC) {
		t.Errorf("chart area %+v", f)
	}
	// Copy and paste: a second chart, its references the same.
	h.clickMods(p, pointer.ButtonPrimary, 0)
	h.press("C", key.ModShortcut)
	if a.ch.clip == nil {
		t.Fatal("not copied")
	}
	a.paste(a.ch.clipText)
	if len(sh.Charts) != n+1 || sh.Charts[n].Plots[0].Series[0].Val.Ref != sh.Charts[0].Plots[0].Series[0].Val.Ref || sh.Charts[n].Name == sh.Charts[0].Name {
		t.Fatalf("paste: %d charts", len(sh.Charts))
	}
	// The menu's Delete; undo brings it back.
	p = h.chartPt(sh.Charts[n], true)
	h.clickMods(p, pointer.ButtonSecondary, 0)
	h.chooseCtx(p, a.chartMenu(), "Delete", gridOrigin, gridSize)
	if len(sh.Charts) != n {
		t.Fatalf("delete: %d charts", len(sh.Charts))
	}
	// Cut: gone, and pasted back.
	h.clickMods(h.chartPt(sh.Charts[2], true), pointer.ButtonPrimary, 0)
	h.press("X", key.ModShortcut)
	if len(sh.Charts) != n-1 {
		t.Fatalf("cut: %d charts", len(sh.Charts))
	}
	a.paste(a.ch.clipText)
	if len(sh.Charts) != n {
		t.Fatalf("paste after cut: %d charts", len(sh.Charts))
	}
	// The Format tab: a series picked in Chart Elements, coloured.
	c := sh.Charts[0]
	h.clickMods(h.chartPt(c, true), pointer.ButtonPrimary, 0)
	a.tab = tabChartFormat
	h.frame()
	v.SelElem = render.ChartElem{Kind: render.ElemSeries, Plot: 0, Series: 1}
	a.colorElem(v.SelElem, true, gui.ColorChoice{Color: colors.RGB(0xC00000)}, false)
	if f := sh.Charts[0].Plots[0].Series[1].Shape.Fill; f.Color != colors.RGB(0xC00000) || v.SelElem.Kind != render.ElemSeries {
		t.Errorf("series fill %+v, selection %+v", f, v.SelElem)
	}
	// Delete with the legend selected deletes the legend, not the chart.
	v.SelElem = render.ChartElem{Kind: render.ElemLegend}
	h.press(key.NameDeleteForward, 0)
	if len(sh.Charts) != n || sh.Charts[0].Legend != nil {
		t.Errorf("delete legend: %d charts, legend %+v", len(sh.Charts), sh.Charts[0].Legend)
	}
}

// A click on a part of a chart selects it; a double-click on the title
// (or typing with it selected) edits it in place.
func TestChartTitleEdit(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	c := sh.Charts[0]
	h.clickMods(h.chartPt(c, false), pointer.ButtonPrimary, 0)
	tr, _, ok := v.ChartElementRect(c, render.ChartElem{Kind: render.ElemTitle})
	if !ok {
		t.Fatal("no title drawn")
	}
	tp := f32.Pt(float32(tr.Min.X+tr.Max.X)/2, float32(gridTop+(tr.Min.Y+tr.Max.Y)/2))
	h.clickAt(tp, time.Second)
	if v.SelElem.Kind != render.ElemTitle {
		t.Fatalf("selected %+v", v.SelElem)
	}
	h.clickAt(tp, time.Second+100*time.Millisecond)
	if !a.ch.title.active {
		t.Fatal("double-click didn't edit the title")
	}
	h.frame()
	if !gtx0Focused(h) {
		t.Error("the title editor hasn't the keyboard")
	}
	a.ch.title.ed.SetText("Regions")
	h.press(key.NameReturn, 0)
	if a.ch.title.active || sh.Charts[0].Title == nil || sh.Charts[0].Title.Text != "Regions" {
		t.Fatalf("title %+v", sh.Charts[0].Title)
	}
	h.press("Z", key.ModShortcut)
	if sh.Charts[0].Title.Text != "Clustered Column" {
		t.Errorf("undo title: %q", sh.Charts[0].Title.Text)
	}
	// Typing with the title selected replaces it; Esc leaves it.
	h.clickAt(tp, 3*time.Second)
	h.typeText("Q")
	if !a.ch.title.active || a.ch.title.ed.Text() != "Q" {
		t.Fatalf("typing: active %v %q", a.ch.title.active, a.ch.title.ed.Text())
	}
	h.press(key.NameEscape, 0)
	if a.ch.title.active || sh.Charts[0].Title.Text != "Clustered Column" {
		t.Errorf("Esc: %q", sh.Charts[0].Title.Text)
	}
	// A click on a series selects it.
	sr, pts, ok := v.ChartElementRect(sh.Charts[0], render.ChartElem{Kind: render.ElemSeries, Plot: 0, Series: 1})
	if !ok || len(pts) == 0 {
		t.Fatal("no series drawn")
	}
	_ = sr
	h.clickAt(f32.Pt(float32(pts[2].X), float32(gridTop+pts[2].Y)), 5*time.Second)
	if e := v.SelElem; e.Kind != render.ElemSeries || e.Series != 1 {
		t.Errorf("series click selected %+v", e)
	}
}

// Deleting a chart sheet (its tab's menu) is undoable.
func TestDeleteChartSheet(t *testing.T) {
	h := chartHarness(t)
	a := h.a
	a.active = 1
	h.frame()
	a.deleteSheet()
	if len(a.wb.Sheets) != 1 || a.onChartSheet() {
		t.Fatalf("%d sheets", len(a.wb.Sheets))
	}
	h.press("Z", key.ModShortcut)
	if len(a.wb.Sheets) != 2 || !a.wb.Sheets[1].IsChartSheet() {
		t.Errorf("undo: %d sheets", len(a.wb.Sheets))
	}
}

// gtx0Focused reports whether the title editor has the keyboard.
func gtx0Focused(h *harness) bool {
	return h.r.Source().Focused(&h.a.ch.title.ed)
}
