// pivot.go handles PivotTables: creating and changing reports, their contextual Analyze and Design ribbons, and the report cell menu.

package ui

import (
	"errors"
	"fmt"
	"image"
	"slices"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/pivot"
	"officelite/excel/render"
)

// PivotTables: Insert > PivotTable, the contextual PivotTable Analyze
// and Design tabs that show while the active cell is in a report, the
// right-click menu of report cells and the commands they run. The field
// list is in pivotpane.go, the dialogs in pivotdlg.go and the label
// drop-downs in pivotmenu.go.

// The contextual ribbons (data.go numbers the Home and Data ribbons).
const (
	tabPivotAnalyze = 100 + iota
	tabPivotDesign
)

// pivotUI is the PivotTable state of the window.
type pivotUI struct {
	hideList bool // the field list was closed (Field List shows it again)
	pane     fieldPane
	menu     pivotMenu
	an       analyzeRibbon
	de       designRibbon
	dlg      pivotDialog
	ctx      gui.ContextMenu // a submenu of the cell menu (Sort, Summarize Values By, ...)
	ctxActs  []func()
	ctxAt    image.Point // where the cell menu opened, for its submenus
}

// analyzeRibbon is the PivotTable Analyze ribbon's widget state.
type analyzeRibbon struct {
	ribbon                      gui.Ribbon
	refreshDD                   gui.DropDown
	fieldSettings, source       widget.Clickable
	clear, sel, move, fieldList widget.Clickable
	details                     widget.Clickable
	tipsSet                     bool
}

// designRibbon is the Design ribbon's widget state.
type designRibbon struct {
	ribbon                                gui.Ribbon
	subtotalsDD, grandDD, layoutDD, blank gui.DropDown
	tipsSet                               bool
}

// curPivot is the report the active cell is in, and its index.
func (a *App) curPivot() (*model.PivotTable, int) {
	act := a.state().view.Sel.Active
	return a.wb.Sheets[a.active].PivotAt(act.Row, act.Col)
}

// pivotMenus are the contextual tabs, shown while the active cell is in
// a report.
func (a *App) pivotMenus() []gui.Menu {
	if pt, _ := a.curPivot(); pt == nil {
		if a.tab == tabPivotAnalyze || a.tab == tabPivotDesign {
			a.tab = tabHome
		}
		return nil
	}
	return []gui.Menu{
		{Label: "PivotTable Analyze", Tab: true, Selected: a.tab == tabPivotAnalyze},
		{Label: "Design", Tab: true, Selected: a.tab == tabPivotDesign},
	}
}

// pivotMenuChosen switches to a contextual tab (i is 0 or 1 past the
// fixed menus).
func (a *App) pivotMenuChosen(i int) {
	if i == 0 {
		a.tab = tabPivotAnalyze
	} else {
		a.tab = tabPivotDesign
	}
}

// pivotDone updates the view after a report changed.
func (a *App) pivotDone(res calc.Result) {
	a.syncSheets()
	a.changed(res)
	a.grid.focusPending = true
}

// applyPivot lays out pt as report idx of sheet (a new one when idx <
// 0): Excel's "There's already data" question first when it would cover
// other cells, and its columns widened to fit afterwards. then runs
// after it succeeded.
func (a *App) applyPivot(sheet, idx int, pt *model.PivotTable, then func()) {
	if !a.readyToChange() {
		return
	}
	var try func(overwrite bool)
	try = func(overwrite bool) {
		res, err := a.book.SetPivot(sheet, idx, pt, overwrite)
		switch {
		case errors.Is(err, edit.ErrPivotOverwrite):
			a.dialog.Show(appName, fmt.Sprintf("There's already data in [%s]%s. Do you want to replace it?", a.wb.Name, a.wb.Sheets[sheet].Name), "OK", "Cancel")
			a.answer = func(i int) {
				if i == 0 {
					try(true)
				}
			}
			return
		case err != nil:
			a.showError(err)
			return
		}
		a.fitPivot(sheet, idx)
		a.pivotDone(res)
		if then != nil {
			then()
		}
	}
	try(false)
}

// showError shows a refusal as Excel does, in a message box.
func (a *App) showError(err error) {
	a.dialog.Show(appName, err.Error(), "OK")
	a.answer = func(int) {}
}

// fitPivot widens the columns of report idx (the last one when idx < 0)
// to their contents, as Excel's "Autofit column widths on update".
func (a *App) fitPivot(sheet, idx int) {
	sh := a.wb.Sheets[sheet]
	if idx < 0 || idx >= len(sh.Pivots) {
		idx = len(sh.Pivots) - 1
	}
	if idx < 0 || a.gtx.Ops == nil {
		return
	}
	st := a.states[sheet]
	if st == nil {
		return
	}
	v := &st.view
	area := sh.Pivots[idx].Out.Area
	var cols []int
	var widths []float32
	for c := area.C1; c <= area.C2; c++ {
		if px, ok := v.FitColWidth(a.gtx, c); ok {
			w := model.PxToColWidth(px+1, v.DigitWidth)
			cols = append(cols, c)
			widths = append(widths, w)
		}
	}
	a.book.WidenPivotCols(sheet, cols, widths)
}

// changePivot applies f to a copy of the active cell's report.
func (a *App) changePivot(f func(pt *model.PivotTable)) {
	pt, idx := a.curPivot()
	if pt == nil {
		return
	}
	p := pt.Clone()
	f(p)
	pivot.Normalize(p)
	a.applyPivot(a.active, idx, p, nil)
}

// refreshPivot updates the active cell's report from its source.
func (a *App) refreshPivot() {
	a.changePivot(func(*model.PivotTable) {})
}

// refreshAllPivots updates every report of the workbook.
func (a *App) refreshAllPivots() {
	if !a.readyToChange() {
		return
	}
	res, err := a.book.RefreshAll(true)
	if err != nil {
		a.showError(err)
	}
	a.pivotDone(res)
}

// clearPivot removes every field from the report (Clear All).
func (a *App) clearPivot() {
	a.changePivot(func(p *model.PivotTable) {
		p.Rows, p.Cols, p.Pages, p.Values = nil, nil, nil, nil
		for i := range p.Fields {
			p.Fields[i] = model.PivotField{Name: p.Fields[i].Name}
		}
	})
}

// selectPivot selects the whole report (Select > Entire PivotTable).
func (a *App) selectPivot() {
	if pt, _ := a.curPivot(); pt != nil {
		a.selectRange(a.active, pt.Out.Area)
		v := &a.state().view
		v.Sel.Active = render.Pos{Row: pt.Out.Body.R1, Col: pt.Out.Body.C1}
	}
}

// toggleFieldList shows or hides the PivotTable Fields pane.
func (a *App) toggleFieldList() { a.pv.hideList = !a.pv.hideList }

// pivotProtected reports, with Excel's message, whether a command that
// changes the cells of r would change a report; Delete on a whole
// report (cleared) is the caller's business.
func (a *App) pivotProtected(r model.Range) bool {
	if edit.PivotOverlap(a.wb.Sheets[a.active], r) == nil {
		return false
	}
	a.showError(edit.ErrPivotChange)
	return true
}

// pivotDelete handles Delete on a selection touching a report: a
// selection holding whole reports deletes them, anything else is
// refused. It reports whether the selection touched a report.
func (a *App) pivotDelete() bool {
	sh := a.wb.Sheets[a.active]
	sel := &a.state().view.Sel
	var hit []int
	for _, r := range sel.Ranges() {
		for i, p := range sh.Pivots {
			if !overlap(p.Out.Area, r) {
				continue
			}
			if !contains(r, p.Out.Area) {
				a.showError(edit.ErrPivotChange)
				return true
			}
			hit = append(hit, i)
		}
	}
	if len(hit) == 0 {
		return false
	}
	if !a.readyToChange() {
		return true
	}
	slices.Sort(hit)
	hit = slices.Compact(hit)
	a.book.BeginGroup()
	var res calc.Result
	for k := len(hit) - 1; k >= 0; k-- {
		res = joinResult(res, a.book.DeletePivot(a.active, hit[k]))
	}
	for _, r := range sel.Ranges() {
		res = joinResult(res, a.book.Clear(a.active, r))
	}
	a.book.EndGroup()
	a.pivotDone(res)
	return true
}

// overlap reports whether ranges a and b share a cell.
func overlap(a, b model.Range) bool {
	return a.R1 <= b.R2 && b.R1 <= a.R2 && a.C1 <= b.C2 && b.C1 <= a.C2
}

// contains reports whether range in lies entirely within outer.
func contains(outer, in model.Range) bool {
	return in.R1 >= outer.R1 && in.R2 <= outer.R2 && in.C1 >= outer.C1 && in.C2 <= outer.C2
}

// pivotPart is what a report cell shows.
type pivotPart struct {
	field int // the row, column or filter field it labels, or -1
	value int // the value field whose value or name it shows, or -1
	axis  model.PivotAxis
	data  bool // a value (Show Details applies)
}

// pivotPartAt works out what cell row, col of report pt shows.
func pivotPartAt(pt *model.PivotTable, row, col int) pivotPart {
	p := pivotPart{field: -1, value: -1}
	for _, h := range pt.Out.Headers {
		if h.Row == row && h.Col == col {
			p.value, p.axis = h.Value, model.AxisValues
			return p
		}
	}
	for _, b := range pt.Out.Buttons {
		if b.Row == row && b.Col == col {
			p.field, p.axis = b.Field, b.Axis
			if b.Field < 0 {
				fs := srcFields(pt.Rows)
				if b.Axis == model.AxisCols {
					fs = srcFields(pt.Cols)
				}
				if len(fs) > 0 {
					p.field = fs[0]
				}
			}
			return p
		}
	}
	o := &pt.Out
	ri, ci := row-o.Body.R1-o.FirstDataRow, col-o.Body.C1-o.FirstDataCol
	rows, cols := srcFields(pt.Rows), srcFields(pt.Cols)
	switch {
	case ri >= 0 && ri < len(o.RowLines) && ci >= 0 && ci < len(o.ColLines):
		rl, cl := o.RowLines[ri], o.ColLines[ci]
		if rl.Kind == model.LineLabel || rl.Kind == model.LineBlank || len(pt.Values) == 0 {
			return p
		}
		d := cl.Data
		if d < 0 {
			d = rl.Data
		}
		if d < 0 {
			d = 0
		}
		p.value, p.axis, p.data = d, model.AxisValues, true
	case ri >= 0 && ri < len(o.RowLines) && ci < 0:
		rl := o.RowLines[ri]
		k := len(rl.Items) - 1
		if pt.Layout != model.LayoutCompact {
			k = col - o.Body.C1
		}
		if rl.Kind != model.LineGrand && k >= 0 && k < len(rows) && k < len(rl.Items) {
			p.field, p.axis = rows[k], model.AxisRows
		}
	case ri < 0 && ci >= 0 && ci < len(o.ColLines):
		k := row - o.Body.R1 - 1
		if o.FirstHeaderRow == 0 {
			k = row - o.Body.R1
		}
		if cl := o.ColLines[ci]; cl.Kind != model.LineGrand && k >= 0 && k < len(cols) {
			p.field, p.axis = cols[k], model.AxisCols
		}
	}
	return p
}

// srcFields are an axis's fields other than Σ Values.
func srcFields(axis []int) []int {
	return slices.DeleteFunc(slices.Clone(axis), func(f int) bool { return f == model.DataField })
}

// removeField takes field f out of every area (and its value fields).
func removeField(p *model.PivotTable, f int) {
	del := func(x int) bool { return x == f }
	p.Rows, p.Cols, p.Pages = slices.DeleteFunc(p.Rows, del), slices.DeleteFunc(p.Cols, del), slices.DeleteFunc(p.Pages, del)
	p.Values = slices.DeleteFunc(p.Values, func(v model.PivotValue) bool { return v.Field == f })
}

// pivotCellMenu is the right-click menu of a report cell.
func (a *App) pivotCellMenu(pt *model.PivotTable) []ctxItem {
	act := a.state().view.Sel.Active
	part := pivotPartAt(pt, act.Row, act.Col)
	items := []ctxItem{
		{"Copy", "Ctrl+C", "Copy the selection to the Clipboard.", false, func() { a.copy(a.gtx, false) }},
		ctxSep,
		{"Format Cells…", "Ctrl+1", "Set the number format, alignment, font, borders and fill.", false, func() { a.openFormatCells(tabNumber) }},
	}
	if part.value >= 0 {
		v := part.value
		items = append(items, ctxItem{"Number Format…", "", "Change the number format of the value field.", false, func() { a.valueFieldDialog(v, true) }})
	}
	items = append(items,
		ctxItem{"Refresh", "", "Update the report from its source data.", false, a.refreshPivot},
		ctxSep,
	)
	if part.field >= 0 && part.axis != model.AxisPages {
		f := part.field
		items = append(items, ctxItem{"Sort", "▸", "Sort the field's items.", false, func() {
			a.pivotSubmenu([]ctxItem{
				{"Sort A to Z", "", "Sort the items from lowest to highest.", false, func() { a.sortPivotField(f, model.SortAscending) }},
				{"Sort Z to A", "", "Sort the items from highest to lowest.", false, func() { a.sortPivotField(f, model.SortDescending) }},
			})
		}})
	}
	if part.field >= 0 {
		f := part.field
		items = append(items, ctxItem{`Remove "` + pt.Fields[f].Label() + `"`, "", "Take the field out of the report.", false, func() {
			a.changePivot(func(p *model.PivotTable) { removeField(p, f) })
		}})
	}
	if part.value >= 0 {
		v := part.value
		items = append(items,
			ctxItem{`Remove "` + pt.Values[v].Name + `"`, "", "Take the value field out of the report.", false, func() {
				a.changePivot(func(p *model.PivotTable) { p.Values = slices.Delete(p.Values, v, v+1) })
			}},
			ctxSep,
			ctxItem{"Summarize Values By", "▸", "Choose how the values are summarized.", false, func() { a.pivotSubmenu(a.summarizeItems(v)) }},
			ctxItem{"Show Values As", "▸", "Show the values as percentages of totals.", false, func() { a.pivotSubmenu(a.showAsItems(v)) }},
		)
		if part.data {
			r, c := act.Row, act.Col
			items = append(items, ctxItem{"Show Details", "", "Put the source rows behind this value on a new sheet.", false, func() { a.showPivotDetails(r, c) }})
		}
		items = append(items, ctxSep, ctxItem{"Value Field Settings…", "", "Change the value field's name, function and number format.", false, func() { a.valueFieldDialog(v, false) }})
	} else if part.field >= 0 {
		f := part.field
		items = append(items, ctxSep, ctxItem{"Field Settings…", "", "Change the field's name and subtotals.", false, func() { a.fieldSettingsDialog(f) }})
	}
	list := "Show Field List"
	if !a.pv.hideList {
		list = "Hide Field List"
	}
	items = append(items, ctxSep, ctxItem{list, "", "Show or hide the PivotTable Fields pane.", false, a.toggleFieldList})
	return items
}

// pivotSubmenu opens a submenu of the cell menu where the menu was.
func (a *App) pivotSubmenu(items []ctxItem) {
	openCtx(&a.pv.ctx, &a.pv.ctxActs, a.pv.ctxAt, items)
}

// summarizeItems is the Summarize Values By submenu of value field v.
func (a *App) summarizeItems(v int) []ctxItem {
	pt, _ := a.curPivot()
	var items []ctxItem
	for i, n := range model.FuncNames {
		fn := model.PivotFunc(i)
		label := n
		if pt != nil && pt.Values[v].Func == fn {
			label = "✓ " + n
		}
		items = append(items, ctxItem{label, "", "Summarize the values with " + n + ".", false, func() {
			a.changePivot(func(p *model.PivotTable) { setValueFunc(p, v, fn) })
		}})
	}
	return items
}

// setValueFunc changes value field v's function, renaming it when it
// had its default name.
func setValueFunc(p *model.PivotTable, v int, fn model.PivotFunc) {
	pv := &p.Values[v]
	src := p.Fields[pv.Field].Name
	if pv.Name == model.FuncNames[pv.Func]+" of "+src {
		pv.Name = model.FuncNames[fn] + " of " + src
	}
	pv.Func = fn
}

// showAsItems is the Show Values As submenu of value field v.
func (a *App) showAsItems(v int) []ctxItem {
	var items []ctxItem
	for i, n := range model.ShowAsNames {
		s := model.PivotShowAs(i)
		items = append(items, ctxItem{n, "", "Show the values as " + n + ".", false, func() {
			a.changePivot(func(p *model.PivotTable) { p.Values[v].ShowAs = s })
		}})
	}
	return items
}

// sortPivotField sorts field f's items.
func (a *App) sortPivotField(f int, s model.PivotSort) {
	a.changePivot(func(p *model.PivotTable) { p.Fields[f].Sort = s })
}

// showPivotDetails puts the records behind the value at row, col on a
// new sheet (a double-click on a value, or Show Details).
func (a *App) showPivotDetails(row, col int) {
	if !a.readyToChange() {
		return
	}
	si, err := a.book.ShowDetails(a.active, row, col)
	if err != nil {
		a.showError(err)
		return
	}
	a.pivotDone(calc.Result{})
	a.active = si
	a.selectRange(si, model.Range{})
}

// pivotDouble handles a double-click on a report cell: Show Details on
// a value; other cells aren't edited. It reports whether the cell was
// in a report.
func (a *App) pivotDouble(row, col int) bool {
	pt, _ := a.wb.Sheets[a.active].PivotAt(row, col)
	if pt == nil {
		return false
	}
	if pivotPartAt(pt, row, col).data {
		a.showPivotDetails(row, col)
	}
	return true
}

// insertPivot opens Create PivotTable (Insert > PivotTable).
func (a *App) insertPivot() {
	if !a.readyToChange() {
		return
	}
	a.createPivotDialog()
}

// layoutPivotRibbon draws the contextual ribbon of the tab showing.
func (a *App) layoutPivotRibbon(gtx layout.Context) layout.Dimensions {
	pt, _ := a.curPivot()
	if pt == nil {
		a.tab = tabHome
		return layout.Dimensions{}
	}
	if a.tab == tabPivotDesign {
		return a.layoutDesignRibbon(gtx, pt)
	}
	return a.layoutAnalyzeRibbon(gtx, pt)
}

// refreshItems is the Refresh button's menu.
var refreshItems = []string{"Refresh", "Refresh All"}

// setTips sets the tooltips of the Analyze ribbon's buttons.
func (r *analyzeRibbon) setTips() {
	tip := gui.SetTip
	tip(&r.fieldSettings, "Field Settings", "", "Show the settings of the selected field: its name, subtotals or how its values are summarized.")
	tip(&r.refreshDD, "Refresh", "Alt+F5", "Get the latest data from the source of this PivotTable, or of every PivotTable of the workbook.")
	tip(&r.source, "Change Data Source", "", "Change the source data for this PivotTable.")
	tip(&r.clear, "Clear All", "", "Remove all fields and filters from the PivotTable.")
	tip(&r.sel, "Select", "", "Select the entire PivotTable.")
	tip(&r.move, "Move PivotTable", "", "Move this PivotTable to another location in the workbook.")
	tip(&r.fieldList, "Field List", "", "Show or hide the Field List. The Field List lets you add and remove fields from your PivotTable.")
	tip(&r.details, "Show Details", "", "Show the source rows behind the selected value on a new sheet.")
}

// layoutAnalyzeRibbon draws the PivotTable Analyze ribbon for report pt and
// runs its clicked commands.
func (a *App) layoutAnalyzeRibbon(gtx layout.Context, pt *model.PivotTable) layout.Dimensions {
	r := &a.pv.an
	if !r.tipsSet {
		r.setTips()
		r.tipsSet = true
	}
	act := a.state().view.Sel.Active
	part := pivotPartAt(pt, act.Row, act.Col)
	th := a.ui
	k := &gui.RibbonKit{Th: th}
	click := func(b *widget.Clickable, f func()) { k.Click(gtx, b, f) }
	click(&r.fieldSettings, func() {
		switch {
		case part.value >= 0:
			a.valueFieldDialog(part.value, false)
		case part.field >= 0:
			a.fieldSettingsDialog(part.field)
		}
	})
	click(&r.source, a.changeSourceDialog)
	click(&r.clear, a.clearPivot)
	click(&r.sel, a.selectPivot)
	click(&r.move, a.movePivotDialog)
	click(&r.fieldList, a.toggleFieldList)
	click(&r.details, func() { a.showPivotDetails(act.Row, act.Col) })

	big := func(b *widget.Clickable, ic *gui.Icon, label string, checked, enabled bool) layout.Widget {
		return gui.Enabled(enabled, k.BigBtn(b, ic, label, checked))
	}
	small := func(b *widget.Clickable, ic *gui.Icon, label string, enabled bool) layout.Widget {
		return gui.Enabled(enabled, k.Small(b, ic, label))
	}
	row, stack := gui.RibbonRow, gui.RibbonStack
	groups := []gui.RibbonGroup{
		{Caption: "Active Field", Content: big(&r.fieldSettings, gui.IconValueSettings, "Field Settings", false, part.field >= 0 || part.value >= 0)},
		{Caption: "Data", Content: gui.RibbonSide(2,
			k.BigDrop(&r.refreshDD, gui.IconReapply, "Refresh", gui.MenuItems(refreshItems), 140, func(i int) {
				if i == 0 {
					a.refreshPivot()
				} else {
					a.refreshAllPivots()
				}
			}),
			big(&r.source, gui.IconDataSource, "Change Data Source", false, true),
		)},
		{Caption: "Actions", Content: gui.RibbonSide(2,
			// Two rows fit above the caption.
			stack(
				row(small(&r.clear, gui.IconClearAll, "Clear All", true), small(&r.sel, gui.IconSelectPivot, "Select", true)),
				row(small(&r.details, gui.IconShowDetails, "Show Details", part.data)),
			),
			big(&r.move, gui.IconMove, "Move PivotTable", false, true),
		)},
		{Caption: "Show", Content: big(&r.fieldList, gui.IconFieldList, "Field List", !a.pv.hideList, true)},
	}
	dims := r.ribbon.Layout(gtx, th, groups...)
	if k.Did && !a.ed.active && !a.pv.dlg.open && !a.dialog.Visible {
		a.grid.focusPending = true
	}
	return dims
}

// The Design ribbon's drop-down menus.
var (
	subtotalItems = []string{"Do Not Show Subtotals", "Show All Subtotals at Bottom of Group", "Show All Subtotals at Top of Group"}
	grandItems    = []string{"Off for Rows and Columns", "On for Rows and Columns", "On for Rows Only", "On for Columns Only"}
	layoutItems   = []string{"Show in Compact Form", "Show in Outline Form", "Show in Tabular Form", "", "Repeat All Item Labels", "Do Not Repeat Item Labels"}
	blankItems    = []string{"Insert Blank Line after Each Item", "Remove Blank Line after Each Item"}
)

// setTips sets the tooltips of the Design ribbon's drop-downs.
func (r *designRibbon) setTips() {
	tip := gui.SetTip
	tip(&r.subtotalsDD, "Subtotals", "", "Show or hide subtotals, at the top or the bottom of their group.")
	tip(&r.grandDD, "Grand Totals", "", "Show or hide grand totals for rows and columns.")
	tip(&r.layoutDD, "Report Layout", "", "Adjust the report layout: compact, outline or tabular form, and whether item labels repeat.")
	tip(&r.blank, "Blank Rows", "", "Emphasize groups by adding a blank line between each grouped item.")
}

// layoutDesignRibbon draws the Design ribbon for report pt and runs its chosen
// menu items.
func (a *App) layoutDesignRibbon(gtx layout.Context, pt *model.PivotTable) layout.Dimensions {
	r := &a.pv.de
	if !r.tipsSet {
		r.setTips()
		r.tipsSet = true
	}
	th := a.ui
	k := &gui.RibbonKit{Th: th}
	drop := func(dd *gui.DropDown, ic *gui.Icon, label string, items []string, width unit.Dp, run func(int)) layout.Widget {
		return k.BigDrop(dd, ic, label, gui.MenuItems(items), width, run)
	}
	groups := []gui.RibbonGroup{
		{Caption: "Layout", Content: gui.RibbonSide(2,
			drop(&r.subtotalsDD, gui.IconSubtotals, "Subtotals", subtotalItems, 260, a.setSubtotals),
			drop(&r.grandDD, gui.IconGrandTotals, "Grand Totals", grandItems, 200, a.setGrandTotals),
			drop(&r.layoutDD, gui.IconReportLayout, "Report Layout", layoutItems, 200, a.setReportLayout),
			drop(&r.blank, gui.IconBlankRows, "Blank Rows", blankItems, 240, func(i int) {
				a.changePivot(func(p *model.PivotTable) { p.BlankRows = i == 0 })
			}),
		)},
	}
	dims := r.ribbon.Layout(gtx, th, groups...)
	if k.Did && !a.ed.active && !a.pv.dlg.open && !a.dialog.Visible {
		a.grid.focusPending = true
	}
	return dims
}

// setSubtotals runs a Subtotals menu item.
func (a *App) setSubtotals(i int) {
	a.changePivot(func(p *model.PivotTable) {
		for f := range p.Fields {
			p.Fields[f].NoSubtotal = i == 0
		}
		p.SubtotalsBottom = i == 1
	})
}

// setGrandTotals runs a Grand Totals menu item.
func (a *App) setGrandTotals(i int) {
	a.changePivot(func(p *model.PivotTable) {
		// Excel's "for rows" is the Grand Total column.
		p.NoRowGrand = i == 0 || i == 3
		p.NoColGrand = i == 0 || i == 2
	})
}

// setReportLayout runs a Report Layout menu item.
func (a *App) setReportLayout(i int) {
	a.changePivot(func(p *model.PivotTable) {
		switch i {
		case 0:
			p.Layout = model.LayoutCompact
		case 1:
			p.Layout = model.LayoutOutline
		case 2:
			p.Layout = model.LayoutTabular
		case 4:
			p.RepeatLabels = true
		case 5:
			p.RepeatLabels = false
		}
	})
}

// layoutPivotSubmenu draws a submenu of the cell menu and runs a chosen
// item.
func (a *App) layoutPivotSubmenu(gtx layout.Context) {
	if i := a.pv.ctx.Layout(gtx, a.ui); i >= 0 && i < len(a.pv.ctxActs) && a.pv.ctxActs[i] != nil {
		a.pv.ctxActs[i]()
		a.afterCtxCommand()
	}
}

// sortPivot sorts the field of the active report cell (Sort A to Z or Z
// to A with the active cell in a report); false when it isn't in one.
func (a *App) sortPivot(desc bool) bool {
	pt, _ := a.curPivot()
	if pt == nil {
		return false
	}
	act := a.state().view.Sel.Active
	part := pivotPartAt(pt, act.Row, act.Col)
	if part.field < 0 || part.axis == model.AxisPages {
		a.showError(edit.ErrPivotChange)
		return true
	}
	s := model.SortAscending
	if desc {
		s = model.SortDescending
	}
	a.sortPivotField(part.field, s)
	return true
}
