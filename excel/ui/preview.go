package ui

import (
	"image"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// PreviewOptions set up the window state Preview draws.
type PreviewOptions struct {
	Page string // a File page ("home", "info", "save as", ...); "" for the sheet
	Menu int    // >= 0 opens that menu's drop-down
	Edit string // text being typed into the active cell
	Ask  bool   // show the "save changes?" prompt
	// Select selects ranges ("B2:C4,E6": a multiple selection).
	Select string
	// Context opens a right-click menu: "cell" (at the active cell),
	// "row", "col" (their headers) or "tab".
	Context string
	// Drag shows the selection being dragged by its border to the cell
	// named, e.g. "E6".
	Drag string
	// Ribbon shows the "home" or "data" ribbon.
	Ribbon string
	// Filter opens the AutoFilter drop-down of the header cell named,
	// e.g. "B1"; FilterSub also opens its Text / Number / Date Filters
	// submenu. Dialog opens "custom" (Custom AutoFilter) or "top10" for
	// that column.
	Filter    string
	FilterSub bool
	Dialog    string
	// Data opens a Data Tools window state for the selection: "ttc"
	// (Text to Columns), "ttc-fixed", "dups" (Remove Duplicates),
	// "validation", "validation-input", "validation-error" (the Data
	// Validation dialog's tabs), "list" (the active cell's validation
	// list), "circles" (Circle Invalid Data) or "alert" (the error alert
	// for the Edit text); or an Outline state: "group" (Group, which asks
	// Rows / Columns for a cell range), "subtotal" (the Subtotal dialog),
	// "hide" / "show" (Hide / Show Detail), "rowsN" / "colsN" (the outline's
	// level button N).
	Data string
	// Sheet is the sheet shown (an index).
	Sheet int
	// Pivot opens a PivotTable state for the active cell's report:
	// "create" (Create PivotTable), "value" (Value Field Settings of the
	// first value field), "field" (Field Settings of the first row
	// field), "menu" (the drop-down of its first button) or "chip" (the
	// menu of the first chip of the field list's Rows).
	Pivot string
	// Chart selects the sheet's chart N (from 1; a chart sheet's chart
	// for any N); 0 selects none.
	Chart int
	// ChartUI sets up a chart state for the selected chart: "insert",
	// "type", "combo", "data", "series", "move", "format" (the dialogs),
	// "ctx" (its menu), "drag" or "resize" (a drag's outline), "title"
	// (the title edited in place), "series1" (its first series
	// selected); or "alt" / "f11" (Alt+F1, F11 on the selection).
	ChartUI string
	// Undo makes a few changes and opens the Home ribbon's "undo" list,
	// or takes four of them back and opens the "redo" list.
	Undo string
	// AutoSave turns the title bar's AutoSave switch "on", or on and just
	// "saved".
	AutoSave string
}

// Preview lays out one frame of a spreadsheet window without a native
// window, for the excelshot tool.
func Preview(gtx layout.Context, fonts *gui.FontLibrary, list *recent.List, wb *model.Workbook, path string, o PreviewOptions) {
	page, menu := o.Page, o.Menu
	a := &App{
		ui:     gui.NewTheme(fonts, accent, accentDark),
		fonts:  fonts,
		recent: list,
		zoom:   1,
		states: map[int]*sheetState{},
	}
	a.th = a.ui.Theme
	a.shaper = a.th.Shaper
	a.setWorkbook(wb)
	if o.Sheet > 0 && o.Sheet < len(wb.Sheets) {
		a.active = o.Sheet
	}
	a.path = path
	if path != "" {
		a.fileInfo, _ = os.Stat(path)
	}
	for p := gui.PageHome; p <= gui.PageClose; p++ {
		if strings.EqualFold(p.String(), page) {
			a.back.Show(p)
		}
	}
	if menu >= 0 {
		a.menu.OpenMenu(menu, len(a.menus()))
	}
	if o.AutoSave != "" {
		a.startAutoSave()
		a.setAutoSave(true)
		if o.AutoSave == "saved" {
			a.as.savedAt = time.Now()
		}
	}
	if o.Ask {
		a.modified = true
		a.askSaveBeforeClose()
	}
	if o.Select != "" {
		var rs []model.Range
		for _, part := range strings.Split(o.Select, ",") {
			if _, r, err := a.book.ResolveRef(strings.TrimSpace(part), a.active); err == nil {
				rs = append(rs, r)
			}
		}
		if len(rs) > 0 {
			a.selectRange(a.active, rs[len(rs)-1])
			a.state().view.Sel.Extra = rs[:len(rs)-1]
		}
	}
	if o.Chart > 0 {
		v := &a.state().view
		switch sh := a.wb.Sheets[a.active]; {
		case sh.IsChartSheet():
			v.SelChart = sh.ChartSheet
		case o.Chart <= len(sh.Charts):
			v.SelChart = sh.Charts[o.Chart-1]
		}
	}
	if o.Edit != "" {
		a.beginEdit(o.Edit, true, false)
		a.ed.cell.SetText(o.Edit) // shown even without the focus
	}
	switch strings.ToLower(o.Ribbon) {
	case "data":
		a.tab = tabData
	case "analyze":
		a.tab = tabPivotAnalyze
	case "design":
		a.tab = tabPivotDesign
	case "chartdesign":
		a.tab = tabChartDesign
	case "chartformat":
		a.tab = tabChartFormat
	}
	if o.Undo != "" {
		a.previewUndo(o.Undo == "redo")
	}
	if o.ChartUI != "" {
		a.layout(gtx)
		gtx.Ops.Reset()
		a.previewChart(o.ChartUI)
		a.layout(gtx)
		gtx.Ops.Reset()
		a.layout(gtx)
		return
	}
	if o.Pivot != "" {
		a.layout(gtx)
		gtx.Ops.Reset()
		a.previewPivot(o.Pivot)
		a.layout(gtx)
		gtx.Ops.Reset()
		a.layout(gtx)
		return
	}
	if o.Data != "" {
		a.layout(gtx)
		gtx.Ops.Reset()
		a.previewData(o.Data)
		a.layout(gtx)
		gtx.Ops.Reset()
		a.layout(gtx)
		return
	}
	if o.Filter != "" {
		a.layout(gtx)
		gtx.Ops.Reset()
		if _, r, err := a.book.ResolveRef(o.Filter, a.active); err == nil {
			switch o.Dialog {
			case "custom":
				a.customFilterDialog(r.C1, -1, 0, true)
			case "top10":
				a.top10Dialog(r.C1)
			default:
				a.openFilterMenu(r.C1)
			}
		}
		if o.FilterSub && a.filt.open {
			a.layout(gtx) // places the submenu's row
			gtx.Ops.Reset()
			fm := &a.filt
			openCtx(&fm.subMenu, &fm.subActs, image.Pt(fm.at.X+gtx.Dp(filterMenuWidth)-gtx.Dp(4), fm.at.Y+fm.subY), a.subItems(fm.col, fm.kind))
		}
		a.layout(gtx)
		return
	}
	if o.Context == "" && o.Drag == "" {
		a.layout(gtx)
		return
	}
	// Lay out once to place the grid, then again with the menu or drag.
	a.layout(gtx)
	gtx.Ops.Reset()
	st := a.state()
	v := &st.view
	p := v.Sel.Active
	rc, _, _ := v.CellRect(p.Row, p.Col)
	at := rc.Min.Add(rc.Max).Div(2)
	hw, hh := v.HeaderSize()
	gw, gh := v.Gutters()
	switch o.Context {
	case "cell":
		a.gridContextMenu(st, f32.Pt(float32(at.X), float32(at.Y)))
	case "row":
		a.gridContextMenu(st, f32.Pt(float32(gw+hw)/2, float32(at.Y)))
	case "col":
		a.gridContextMenu(st, f32.Pt(float32(at.X), float32(gh+hh)/2))
	case "tab":
		y := gtx.Constraints.Max.Y - gtx.Dp(24) - gtx.Dp(15)
		openCtx(&a.cm.tabs, &a.cm.tabActs, image.Pt(gtx.Dp(40), y), a.tabMenu(a.active))
	}
	if o.Drag != "" {
		if _, r, err := a.book.ResolveRef(o.Drag, a.active); err == nil {
			src := v.Sel.Range
			a.grid.move = &moveDrag{src: src, grab: render.Pos{Row: src.R1, Col: src.C1}, dRow: r.R1 - src.R1, dCol: r.C1 - src.C1}
		}
	}
	a.layout(gtx)
}

// previewData sets up the Data Tools state Preview's Data option names.
func (a *App) previewData(what string) {
	switch what {
	case "ttc", "ttc-fixed":
		a.textToColumnsDialog()
		if what == "ttc-fixed" {
			a.ins.ttc.kind.Value = "fixed"
		}
	case "dups":
		a.removeDuplicatesDialog()
	case "validation", "validation-input", "validation-error":
		a.validationDialog()
		a.ins.dv.tab = map[string]int{"validation-input": 1, "validation-error": 2}[what]
	case "list":
		a.openValidList()
	case "circles":
		a.circleInvalid()
	case "alert":
		a.commitEdit(1, 0)
	case "group":
		a.groupCmd()
	case "subtotal":
		a.subtotalDialog()
	case "hide":
		a.hideDetail()
	case "show":
		a.showDetail()
	default:
		for _, axis := range []string{"rows", "cols"} {
			if n, err := strconv.Atoi(strings.TrimPrefix(what, axis)); err == nil && strings.HasPrefix(what, axis) {
				a.book.ShowLevels(a.active, axis == "rows", n)
				a.afterOutline()
			}
		}
	}
}

// previewPivot sets up the PivotTable state Preview's Pivot option names.
func (a *App) previewPivot(what string) {
	pt, _ := a.curPivot()
	switch what {
	case "create":
		a.createPivotDialog()
	case "value":
		a.valueFieldDialog(0, false)
	case "field":
		if pt != nil && len(pt.Rows) > 0 {
			a.fieldSettingsDialog(pt.Rows[0])
		}
	case "menu":
		if pt != nil && len(pt.Out.Buttons) > 0 {
			b := pt.Out.Buttons[0]
			for _, x := range pt.Out.Buttons {
				if x.Axis == model.AxisRows {
					b = x
				}
			}
			a.openPivotMenu(b.Row, b.Col)
		}
	case "chip":
		for _, c := range a.pv.pane.chips {
			if c.area == areaRows && pt != nil {
				a.openChipMenu(pt, c.area, c.idx, image.Pt(c.r.Min.X, c.r.Max.Y))
				break
			}
		}
	}
}

// previewChart sets up the chart state Preview's ChartUI option names.
func (a *App) previewChart(what string) {
	v := &a.state().view
	c, idx := a.curChart()
	if c == nil && what != "alt" && what != "f11" && what != "insert" {
		return
	}
	center := func() f32.Point {
		rc, _ := v.ChartRect(c)
		return f32.Pt(float32(rc.Min.X+rc.Max.X)/2, float32(rc.Min.Y+rc.Max.Y)/2)
	}
	switch what {
	case "insert":
		a.openChartTypeDialog(true)
	case "type", "combo":
		a.openChartTypeDialog(false)
		if what == "combo" {
			a.ch.dlg.pick = edit.ComboColumnLine
			a.ch.dlg.setComboDefault()
		}
	case "data":
		a.openSelectData()
	case "series":
		a.openSelectData()
		a.openEditSeries(0)
	case "move":
		a.openMoveChart()
	case "format":
		a.openFormatElem()
	case "ctx":
		a.chartContextMenu(v, c, center())
	case "drag", "resize":
		if idx < 0 {
			return
		}
		p := center()
		h := -1
		if what == "resize" {
			rc, _ := v.ChartRect(c)
			hp := render.ChartHandles(rc)[4]
			p, h = f32.Pt(float32(hp.X), float32(hp.Y)), 4
		}
		a.startChartDrag(v, c, idx, h, p)
		a.chartDragTo(v, p.Add(f32.Pt(130, 70)), 0)
	case "title":
		v.SelElem = render.ChartElem{Kind: render.ElemTitle}
		a.startTitleEdit("", false)
	case "series1":
		if all := c.AllSeries(); len(all) > 0 {
			v.SelElem = render.ChartElem{Kind: render.ElemSeries, Plot: all[0].Plot, Series: all[0].Series}
		}
	case "alt":
		a.insertChart(edit.ColumnClustered)
	case "f11":
		a.insertChartSheet()
	}
}

// previewUndo makes a few changes around the active cell and opens the
// Undo list, or the Redo list after taking four of them back, with three
// entries highlighted.
func (a *App) previewUndo(redo bool) {
	b, sh := a.book, a.active
	p := a.state().view.Sel.Active
	cell := model.Range{R1: p.Row, C1: p.Col, R2: p.Row, C2: p.Col}
	b.Enter(sh, p.Row, p.Col, "Total")
	func() {
		defer b.Op("Bold")()
		b.Format(sh, cell, func(st *model.Style, _, _ int) { st.Font.Bold = true })
	}()
	func() {
		defer b.Op("Fill Color")()
		b.Format(sh, cell, func(st *model.Style, _, _ int) { st.Fill = colors.RGB(0xFFFF00) })
	}()
	b.InsertRows(sh, p.Row+1, 1)
	b.ResizeCols(sh, []int{p.Col}, 14)
	b.Enter(sh, p.Row+1, p.Col, "=SUM(A1:A3)")
	b.FillDown(sh, model.Range{R1: p.Row + 1, C1: p.Col, R2: p.Row + 3, C2: p.Col})
	a.syncSheets()
	a.changed(calc.Result{})
	hl := &a.home.undoHL
	if redo {
		a.undoN(4)
		hl = &a.home.redoHL
	}
	hl.ShowOpen(3)
}
