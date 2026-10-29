// The chart dialogs: Insert Chart and Change Chart Type (the kinds, and
// each series' kind and axis for a combo), Select Data Source (the data
// range, the series and their Edit Series dialog, the category labels,
// hidden and empty cells), Move Chart and Format Chart Area (or of the
// selected part).

package ui

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// chartDlgKind says which chart dialog is open.
type chartDlgKind int

const (
	cdNone chartDlgKind = iota
	cdInsert
	cdType
	cdData
	cdSeries // Edit Series, over Select Data
	cdMove
	cdFormat
)

// chartDialog is the open chart dialog (at most one).
type chartDialog struct {
	kind  chartDlgKind
	focus bool
	key   int // Enter (1) or Esc (-1)
	tag   int // focus tag when no field has the focus
	block int
	err   string

	ok, cancel widget.Clickable

	// Insert Chart, Change Chart Type: the kind picked and a combo's
	// series.
	pick     edit.ChartKind
	kindBtns []widget.Clickable
	kindList widget.List
	comboDD  []gui.DropDown
	comboK   []edit.ChartKind
	comboSec []widget.Bool

	// Select Data: a working copy of the chart.
	work               *model.Chart
	idx                int // the chart's index (-1: a chart sheet's)
	rangeEd, catEd     widget.Editor
	rangeText, catText string // as last applied
	sel                int    // the selected series (AllSeries order)
	serBtns            []widget.Clickable
	add, editS, remove widget.Clickable
	up, down, switchRC widget.Clickable
	blanks             widget.Enum
	hidden             widget.Bool
	serName, serVals   widget.Editor // Edit Series
	serIdx             int           // the series edited; -1 adds one
	where              widget.Enum   // Move Chart: "new" or "object"
	sheetName          widget.Editor
	objDD              gui.DropDown
	objSheet           int
	fillCP, lineCP     gui.ColorPicker // Format
	fill, line         gui.ColorChoice
	fillSet, lineSet   bool
	elem               render.ChartElem
	dataList           widget.List
}

// openChartDialog opens chart dialog k, ending a title edit and closing
// the menus.
func (a *App) openChartDialog(k chartDlgKind) {
	a.menu.Close()
	a.finishTitleEdit(true)
	d := &a.ch.dlg
	d.kind, d.focus, d.err, d.key = k, true, "", 0
	for _, ed := range []*widget.Editor{&d.rangeEd, &d.catEd, &d.serName, &d.serVals, &d.sheetName} {
		ed.SingleLine, ed.Submit = true, true
	}
	d.kindList.Axis = layout.Vertical
	d.dataList.Axis = layout.Vertical
}

// closeChartDialog closes the open chart dialog and gives the grid the
// keyboard.
func (a *App) closeChartDialog() {
	a.ch.dlg.kind = cdNone
	a.grid.focusPending = true
}

// openChartTypeDialog opens Insert Chart (insert) or Change Chart Type
// for the current chart.
func (a *App) openChartTypeDialog(insert bool) {
	d := &a.ch.dlg
	if insert {
		if a.onChartSheet() || !a.readyToChange() {
			return
		}
		a.openChartDialog(cdInsert)
		d.pick = edit.ColumnClustered
		d.work, d.comboK = nil, nil
		return
	}
	c, _ := a.curChart()
	if c == nil || !a.readyToChange() {
		return
	}
	a.openChartDialog(cdType)
	d.work = c
	d.pick = edit.KindOf(c)
	kinds, sec := edit.SeriesKinds(c)
	d.comboK = kinds
	d.comboSec = make([]widget.Bool, len(sec))
	for i := range sec {
		d.comboSec[i].Value = sec[i]
	}
	d.comboDD = make([]gui.DropDown, len(kinds))
	if d.pick != edit.ComboColumnLine {
		d.setComboDefault()
	}
}

// setComboDefault lays the series out as the combo kind does: columns,
// the last one a line on the secondary axis.
func (d *chartDialog) setComboDefault() {
	for i := range d.comboK {
		d.comboK[i] = edit.ColumnClustered
		d.comboSec[i].Value = false
	}
	if n := len(d.comboK); n > 1 {
		d.comboK[n-1], d.comboSec[n-1].Value = edit.LinePlain, true
	}
}

// openSelectData opens Select Data Source for the current chart.
func (a *App) openSelectData() {
	c, idx := a.curChart()
	if c == nil || !a.readyToChange() {
		return
	}
	if !chartEditable(c) {
		a.showError(errChartKind)
		return
	}
	a.openChartDialog(cdData)
	d := &a.ch.dlg
	d.work, d.idx, d.sel = c.Clone(), idx, 0
	home := a.wb.Sheets[a.active].Name
	d.rangeText = ""
	if src, ok := edit.ChartDataSource(a.wb, c, home); ok {
		d.rangeText = "=" + src.SourceRef()
	}
	d.rangeEd.SetText(d.rangeText)
	d.catText = a.catRefText(d.work)
	d.catEd.SetText(d.catText)
	d.blanks.Value = []string{"gap", "zero", "span"}[min(int(c.DispBlanks), 2)]
	d.hidden.Value = !c.PlotVisOnly
}

// catRefText is the category labels' reference of the first series.
func (a *App) catRefText(c *model.Chart) string {
	if all := c.AllSeries(); len(all) > 0 {
		if r := c.Plots[all[0].Plot].Series[all[0].Series].Cat.Ref; r != "" {
			return "=" + r
		}
	}
	return ""
}

// openMoveChart opens Move Chart for the current chart.
func (a *App) openMoveChart() {
	c, idx := a.curChart()
	if c == nil || !a.readyToChange() {
		return
	}
	a.openChartDialog(cdMove)
	d := &a.ch.dlg
	d.idx = idx
	d.where.Value = "new"
	d.sheetName.SetText(edit.NextChartSheetName(a.wb))
	d.objSheet = -1
	for i, sh := range a.wb.Sheets {
		if !sh.IsChartSheet() && !sh.Hidden && (d.objSheet < 0 || i == a.active) {
			d.objSheet = i
		}
	}
	if idx < 0 {
		d.where.Value = "object"
	}
}

// openFormatElem opens Format Chart Area (or of the selected part):
// its fill and outline.
func (a *App) openFormatElem() {
	c, _ := a.curChart()
	if c == nil || !chartEditable(c) || !a.readyToChange() {
		return
	}
	a.openChartDialog(cdFormat)
	d := &a.ch.dlg
	d.elem = a.state().view.SelElem
	if d.elem.Kind < render.ElemChartArea {
		d.elem = render.ChartElem{Kind: render.ElemChartArea}
	}
	d.fillSet, d.lineSet = false, false
	d.fill, d.line = gui.ColorChoice{None: true}, gui.ColorChoice{None: true}
	if d.elem.Kind == render.ElemChartArea {
		d.fill, d.line = paintChoice(c.ChartArea.Fill), paintChoice(c.ChartArea.Line)
	}
}

// paintChoice is a chart paint as the color picker shows it.
func paintChoice(p model.Paint) gui.ColorChoice {
	if p.Kind == model.FillSolid {
		return gui.ColorChoice{Color: p.Color}
	}
	return gui.ColorChoice{None: true}
}

// parseChartRef reads a reference typed in a dialog: =Sheet1!$A$1:$A$5,
// Sheet1!A1:A5 or A1:A5 (on the chart's sheet), made absolute.
func (a *App) parseChartRef(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "=")
	if s == "" {
		return "", nil
	}
	home := a.wb.Sheets[a.active].Name
	if a.onChartSheet() {
		home = ""
	}
	rs, ok := model.ParseRefList(s, home)
	if !ok || len(rs) == 0 {
		return "", fmt.Errorf("The reference %q isn't valid", s)
	}
	var parts []string
	for _, r := range rs {
		si := slices.IndexFunc(a.wb.Sheets, func(sh *model.Sheet) bool { return strings.EqualFold(sh.Name, r.Sheet) })
		if si < 0 || a.wb.Sheets[si].IsChartSheet() {
			return "", fmt.Errorf("The reference %q isn't valid", s)
		}
		parts = append(parts, edit.ChartSource{Sheet: a.wb.Sheets[si].Name, Range: r.Range}.SourceRef())
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "(" + strings.Join(parts, ",") + ")", nil
}

// applyDataRange lays the working chart out from the data range typed.
func (a *App) applyDataRange() { a.tryDataRange(false) }

// tryDataRange applies the data range typed; quiet (while it is typed)
// it leaves one that isn't a range yet for later.
func (a *App) tryDataRange(quiet bool) {
	d := &a.ch.dlg
	t := d.rangeEd.Text()
	if t == d.rangeText {
		return
	}
	ref, err := a.parseChartRef(t)
	if err != nil || ref == "" || strings.HasPrefix(ref, "(") {
		if quiet {
			return
		}
		d.rangeText = t
		if err == nil {
			err = fmt.Errorf("The chart data range must be one block of cells")
		}
		d.err = err.Error()
		return
	}
	d.rangeText = t
	rs, _ := model.ParseRefList(ref, "")
	sh := a.wb.SheetByName(rs[0].Sheet)
	src := edit.DetectSource(a.wb, sh, rs[0].Range)
	if len(src.Series(a.wb, false)) == 0 {
		d.err = "The chart data range has no data"
		return
	}
	d.err = ""
	edit.SetChartSource(a.wb, d.work, a.wb.Sheets[a.active].Name, src)
	d.sel = 0
	d.catText = a.catRefText(d.work)
	d.catEd.SetText(d.catText)
}

// runChartDialog carries out the open dialog's OK; false keeps it open
// (with d.err set).
func (a *App) runChartDialog() bool {
	d := &a.ch.dlg
	home := a.wb.Sheets[a.active].Name
	switch d.kind {
	case cdInsert:
		a.closeChartDialog()
		a.insertChart(d.pick)
		if d.pick == edit.ComboColumnLine && len(d.comboK) > 0 {
			a.applyComboKinds()
		}
		return true
	case cdType:
		c, _ := a.curChart()
		if c == nil {
			a.closeChartDialog()
			return true
		}
		a.closeChartDialog()
		if d.pick == edit.ComboColumnLine {
			a.applyComboKinds()
			return true
		}
		a.changeChart(func(c *model.Chart) error {
			edit.SetChartKind(a.wb, c, home, d.pick)
			return nil
		})
		return true
	case cdData:
		a.applyDataRange()
		if d.err != "" {
			return false
		}
		if len(d.work.AllSeries()) == 0 {
			d.err = "A chart needs at least one series"
			return false
		}
		d.work.DispBlanks = model.BlanksAs(slices.Index([]string{"gap", "zero", "span"}, d.blanks.Value))
		d.work.PlotVisOnly = !d.hidden.Value
		work := d.work
		a.closeChartDialog()
		a.changeChart(func(c *model.Chart) error {
			*c = *work.Clone()
			c.Anchor = a.anchorOf()
			return nil
		})
		return true
	case cdSeries:
		return a.runEditSeries()
	case cdMove:
		return a.runMoveChart()
	case cdFormat:
		e := d.elem
		a.closeChartDialog()
		if d.fillSet {
			a.colorElem(e, true, d.fill, false)
		}
		if d.lineSet {
			a.colorElem(e, false, d.line, false)
		}
		return true
	}
	a.closeChartDialog()
	return true
}

// anchorOf is the current chart's anchor (kept when Select Data replaces
// the chart with its working copy).
func (a *App) anchorOf() model.Anchor {
	c, _ := a.curChart()
	if c == nil {
		return model.Anchor{}
	}
	return c.Anchor
}

// applyComboKinds sets the current chart's series' kinds and axes as
// the dialog's combo rows say.
func (a *App) applyComboKinds() {
	d := &a.ch.dlg
	home := a.wb.Sheets[a.active].Name
	kinds := slices.Clone(d.comboK)
	sec := make([]bool, len(d.comboSec))
	for i := range sec {
		sec[i] = d.comboSec[i].Value
	}
	a.changeChart(func(c *model.Chart) error {
		if n := len(c.AllSeries()); len(kinds) != n {
			edit.SetChartKind(a.wb, c, home, edit.ComboColumnLine)
			return nil
		}
		edit.SetComboKinds(a.wb, c, home, kinds, sec)
		return nil
	})
}

// openEditSeries opens Edit Series for series i of the working chart
// (-1: a new series).
func (a *App) openEditSeries(i int) {
	d := &a.ch.dlg
	a.applyDataRange()
	d.kind, d.focus, d.err, d.serIdx = cdSeries, true, "", i
	d.serName.SetText("")
	d.serVals.SetText("={1}")
	if all := d.work.AllSeries(); i >= 0 && i < len(all) {
		s := &d.work.Plots[all[i].Plot].Series[all[i].Series]
		switch {
		case s.Name.Ref != "":
			d.serName.SetText("=" + s.Name.Ref)
		case s.Name.Literal != "":
			d.serName.SetText(s.Name.Literal)
		}
		d.serVals.SetText("=" + s.Val.Ref)
	}
}

// runEditSeries applies Edit Series to the working chart and goes back
// to Select Data.
func (a *App) runEditSeries() bool {
	d := &a.ch.dlg
	vals, err := a.parseChartRef(d.serVals.Text())
	if err == nil && vals == "" {
		err = fmt.Errorf("Enter the series values as a reference, e.g. =Sheet1!$B$2:$B$5")
	}
	if err != nil {
		d.err = err.Error()
		return false
	}
	var name model.ChartData
	if t := strings.TrimSpace(d.serName.Text()); t != "" {
		if ref, err := a.parseChartRef(t); err == nil && strings.HasPrefix(t, "=") {
			name = model.ChartData{Ref: ref, Text: true}
		} else {
			name = model.ChartData{Literal: strings.Trim(strings.TrimPrefix(t, "="), `"`)}
		}
	}
	home := a.wb.Sheets[a.active].Name
	ser := a.workSeries()
	if d.serIdx < 0 || d.serIdx >= len(ser) {
		s := model.ChartSeries{Name: name, Val: model.ChartData{Ref: vals}}
		if len(ser) > 0 {
			s.Cat = ser[0].Cat
		}
		ser = append(ser, s)
		d.sel = len(ser) - 1
	} else {
		ser[d.serIdx].Name, ser[d.serIdx].Val = name, model.ChartData{Ref: vals}
	}
	edit.SetChartSeries(a.wb, d.work, home, ser)
	d.kind, d.err, d.focus = cdData, "", true
	d.rangeText = ""
	if src, ok := edit.ChartDataSource(a.wb, d.work, home); ok {
		d.rangeText = "=" + src.SourceRef()
	}
	d.rangeEd.SetText(d.rangeText)
	return false
}

// workSeries are the working chart's series in order.
func (a *App) workSeries() []model.ChartSeries {
	d := &a.ch.dlg
	var out []model.ChartSeries
	for _, sr := range d.work.AllSeries() {
		out = append(out, d.work.Plots[sr.Plot].Series[sr.Series])
	}
	return out
}

// setWorkSeries gives the working chart new series (Remove, Move Up and
// Down).
func (a *App) setWorkSeries(ser []model.ChartSeries) {
	d := &a.ch.dlg
	home := a.wb.Sheets[a.active].Name
	edit.SetChartSeries(a.wb, d.work, home, ser)
	d.rangeText = ""
	if src, ok := edit.ChartDataSource(a.wb, d.work, home); ok {
		d.rangeText = "=" + src.SourceRef()
	}
	d.rangeEd.SetText(d.rangeText)
}

// runMoveChart moves the chart to a new chart sheet or onto a sheet.
func (a *App) runMoveChart() bool {
	d := &a.ch.dlg
	c, idx := a.curChart()
	if c == nil {
		a.closeChartDialog()
		return true
	}
	if d.where.Value == "new" {
		name := strings.TrimSpace(d.sheetName.Text())
		at, err := a.book.ChartToSheet(a.active, idx, name)
		if err != nil {
			d.err = err.Error()
			return false
		}
		a.closeChartDialog()
		a.syncSheets()
		a.changed(calc.Result{})
		a.active = at
		a.tab = tabChartDesign
		return true
	}
	to := d.objSheet
	if to < 0 || to >= len(a.wb.Sheets) {
		d.err = "Pick a sheet to put the chart on"
		return false
	}
	if to == a.active && idx >= 0 {
		a.closeChartDialog()
		return true
	}
	// The same size, at the top-left of the target's visible cells.
	w, h := float64(newChartW), float64(newChartH)
	if idx >= 0 {
		x0, y0, x1, y1 := render.ChartBox(c, a.state().view.Geo)
		w, h = x1-x0, y1-y0
	}
	from := a.active
	a.active = to // the target's view state, for its layout
	tv := &a.state().view
	a.active = from
	x0, y0 := tv.Geo.Cols.Start(tv.LeftCol)+20, tv.Geo.Rows.Start(tv.TopRow)+20
	anchor := render.ChartAnchor(model.Anchor{Kind: model.AnchorTwoCell}, tv.Geo, x0, y0, x0+w, y0+h)
	sheet, i, err := a.book.ChartToObject(from, idx, to, anchor)
	if err != nil {
		d.err = err.Error()
		return false
	}
	a.closeChartDialog()
	a.syncSheets()
	a.changed(calc.Result{})
	a.active = sheet
	a.selectChart(i, render.ChartElem{Kind: render.ElemChartArea})
	a.tab = tabChartDesign
	return true
}

// chartDialogInput takes Enter and Esc for the open dialog.
func (a *App) chartDialogInput(gtx layout.Context) {
	d := &a.ch.dlg
	if d.kind == cdNone {
		return
	}
	for _, t := range []any{&d.rangeEd, &d.catEd, &d.serName, &d.serVals, &d.sheetName, &d.tag} {
		if k := dialogKeys(gtx, t); k != 0 {
			d.key = k
		}
	}
}

// layoutChartDialog draws the open chart dialog.
func (a *App) layoutChartDialog(gtx layout.Context) {
	d := &a.ch.dlg
	if d.kind == cdNone {
		return
	}
	k := d.key
	d.key = 0
	for d.ok.Clicked(gtx) {
		k = 1
	}
	for d.cancel.Clicked(gtx) {
		k = -1
	}
	if k != 0 {
		gtx.Execute(op.InvalidateCmd{})
	}
	if k < 0 {
		if d.kind == cdSeries {
			d.kind, d.err, d.focus = cdData, "", true // back to Select Data
		} else {
			a.closeChartDialog()
			return
		}
	}
	if k > 0 && a.runChartDialog() {
		return
	}
	if d.kind == cdNone {
		return
	}
	if d.focus {
		d.focus = false
		switch d.kind {
		case cdData:
			gtx.Execute(key.FocusCmd{Tag: &d.rangeEd})
		case cdSeries:
			gtx.Execute(key.FocusCmd{Tag: &d.serName})
		case cdMove:
			gtx.Execute(key.FocusCmd{Tag: &d.sheetName})
		default:
			gtx.Execute(key.FocusCmd{Tag: &d.tag})
		}
	}
	var title string
	var body []layout.FlexChild
	width := 520
	switch d.kind {
	case cdInsert, cdType:
		title = "Change Chart Type"
		if d.kind == cdInsert {
			title = "Insert Chart"
		}
		body = a.chartTypeBody(gtx)
		width = 660
	case cdData:
		title = "Select Data Source"
		body = a.selectDataBody(gtx)
		width = 560
	case cdSeries:
		title = "Edit Series"
		body = a.editSeriesBody()
		width = 440
	case cdMove:
		title = "Move Chart"
		body = a.moveChartBody(gtx)
		width = 460
	case cdFormat:
		c, _ := a.curChart()
		title = "Format " + d.elem.Name(c, a.wb, a.wb.Sheets[a.active].Name)
		body = a.formatElemBody(gtx)
		width = 400
	}
	th := a.ui
	body = append(body, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if d.err == "" {
			return layout.Dimensions{}
		}
		l := th.Label(12, d.err, errorText, font.Normal)
		l.MaxLines = 3
		return layout.Inset{Top: 6}.Layout(gtx, l.Layout)
	}))
	content := func(gtx layout.Context) layout.Dimensions {
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		event.Op(gtx.Ops, &d.tag)
		area.Pop()
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, body...)
	}
	a.ui.ModalDialog(gtx, &d.block, title, unit.Dp(width), content, a.accentButton(&d.ok, "OK"), a.plainButton(&d.cancel, "Cancel"))
}

// chartTypeBody lists the kinds (two columns of icons and names) and,
// for a combo, each series' kind and axis.
func (a *App) chartTypeBody(gtx layout.Context) []layout.FlexChild {
	d := &a.ch.dlg
	th := a.ui
	if len(d.kindBtns) != len(edit.ChartKinds) {
		d.kindBtns = make([]widget.Clickable, len(edit.ChartKinds))
	}
	for i := range d.kindBtns {
		for d.kindBtns[i].Clicked(gtx) {
			k := edit.ChartKinds[i].Kind
			if k == edit.ComboColumnLine && d.pick != k {
				d.setComboDefault()
			}
			d.pick = k
		}
	}
	kindBtn := func(i int) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			info := edit.ChartKinds[i]
			bg := colors.White
			if info.Kind == d.pick {
				bg = th.AccentTint
			}
			return gui.Hoverable(gtx, &d.kindBtns[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
				if info.Kind == d.pick {
					gui.FillRect(gtx.Ops, image.Rect(0, 0, gtx.Dp(3), gtx.Dp(30)), accent)
				}
				gtx.Constraints.Min.Y = gtx.Dp(30)
				return layout.Inset{Left: 8, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(22)
							return kindIcon(info.Kind).Layout(gtx, accentDark)
						}),
						layout.Rigid(layout.Spacer{Width: 8}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							l := th.Label(13, info.Name, chromeText, font.Normal)
							l.MaxLines = 1
							return gui.Middle(gtx, l.Layout)
						}),
					)
				})
			})
		}
	}
	half := (len(edit.ChartKinds) + 1) / 2
	col := func(from, to int) layout.FlexChild {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			var rows []layout.FlexChild
			for i := from; i < to; i++ {
				rows = append(rows, layout.Rigid(kindBtn(i)))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	}
	body := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, col(0, half), layout.Rigid(layout.Spacer{Width: 8}.Layout), col(half, len(edit.ChartKinds)))
		}),
	}
	if d.pick != edit.ComboColumnLine || len(d.comboK) == 0 {
		return body
	}
	// The combo's rows: each series' name, kind and axis.
	c := d.work
	home := a.wb.Sheets[a.active].Name
	body = append(body, layout.Rigid(layout.Spacer{Height: 10}.Layout),
		layout.Rigid(th.Label(13, "Choose the chart type and axis for your data series:", chromeText, font.SemiBold).Layout),
		layout.Rigid(layout.Spacer{Height: 6}.Layout))
	labels := make([]string, len(edit.ComboKinds))
	for i, k := range edit.ComboKinds {
		labels[i] = k.String()
	}
	for i := range d.comboK {
		name := fmt.Sprintf("Series%d", i+1)
		if c != nil {
			if all := c.AllSeries(); i < len(all) {
				name = a.wb.SeriesName(&c.Plots[all[i].Plot].Series[all[i].Series], home, i)
			}
		}
		body = append(body, gui.Form{Th: th, LabelW: 170}.Row(
			gui.Form{Th: th, LabelW: 170}.Label(name),
			gui.Form{Th: th}.Combo(&d.comboDD[i], 230, d.comboK[i].String(), dropItems(labels), func(j int) { d.comboK[i] = edit.ComboKinds[j] }),
			layout.Rigid(layout.Spacer{Width: 10}.Layout),
			layout.Rigid(th.CheckBox(&d.comboSec[i], "Secondary Axis")),
		))
	}
	return body
}

// selectDataBody is Select Data Source: the range, the series list and
// its buttons, the category labels, hidden and empty cells.
func (a *App) selectDataBody(gtx layout.Context) []layout.FlexChild {
	d := &a.ch.dlg
	th := a.ui
	home := a.wb.Sheets[a.active].Name
	for {
		ev, ok := d.rangeEd.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			a.applyDataRange()
		case widget.ChangeEvent:
			a.tryDataRange(true) // the chart follows the range as it is typed
		}
	}
	if !gtx.Focused(&d.rangeEd) && d.rangeEd.Text() != d.rangeText {
		a.applyDataRange()
	}
	if t := d.catEd.Text(); t != d.catText && !gtx.Focused(&d.catEd) {
		d.catText = t
		if ref, err := a.parseChartRef(t); err != nil {
			d.err = err.Error()
		} else {
			d.err = ""
			ser := a.workSeries()
			for i := range ser {
				ser[i].Cat = model.ChartData{Ref: ref}
			}
			a.setWorkSeries(ser)
		}
	}
	ser := a.workSeries()
	if len(d.serBtns) < len(ser) {
		d.serBtns = make([]widget.Clickable, len(ser)+4)
	}
	for i := range ser {
		for d.serBtns[i].Clicked(gtx) {
			d.sel = i
		}
	}
	d.sel = min(max(d.sel, 0), max(len(ser)-1, 0))
	for d.add.Clicked(gtx) {
		a.openEditSeries(-1)
	}
	for d.editS.Clicked(gtx) {
		if len(ser) > 0 {
			a.openEditSeries(d.sel)
		}
	}
	for d.remove.Clicked(gtx) {
		if len(ser) > 0 {
			a.setWorkSeries(slices.Delete(slices.Clone(ser), d.sel, d.sel+1))
			ser = a.workSeries()
		}
	}
	for d.up.Clicked(gtx) {
		if d.sel > 0 {
			ser[d.sel-1], ser[d.sel] = ser[d.sel], ser[d.sel-1]
			a.setWorkSeries(ser)
			d.sel--
		}
	}
	for d.down.Clicked(gtx) {
		if d.sel+1 < len(ser) {
			ser[d.sel+1], ser[d.sel] = ser[d.sel], ser[d.sel+1]
			a.setWorkSeries(ser)
			d.sel++
		}
	}
	for d.switchRC.Clicked(gtx) {
		a.applyDataRange()
		if err := edit.SwitchRowColumn(a.wb, d.work, home); err != nil {
			d.err = err.Error()
		} else {
			d.err = ""
			d.catText = a.catRefText(d.work)
			d.catEd.SetText(d.catText)
		}
	}
	ser = a.workSeries()
	form := gui.Form{Th: th, LabelW: 130}
	field := func(ed *widget.Editor, w int) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return gui.TextField(gtx, th, ed, unit.Dp(w), 1, "") }
	}
	btn := func(b *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 6}.Layout(gtx, a.plainButton(b, label))
		})
	}
	list := func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Constraints.Max.X, gtx.Dp(150)
		gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
		gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(w-4, h-4))
		o := op.Offset(image.Pt(2, 2)).Push(gtx.Ops)
		cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
		material.List(a.th, &d.dataList).Layout(g, len(ser), func(gtx layout.Context, i int) layout.Dimensions {
			bg := colors.White
			if i == d.sel {
				bg = gui.HoverBG
			}
			return gui.Hoverable(gtx, &d.serBtns[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: 6, Top: 4, Bottom: 4}.Layout(gtx, th.Label(13, a.wb.SeriesName(&ser[i], home, i), chromeText, font.Normal).Layout)
			})
		})
		cs.Pop()
		o.Pop()
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
	return []layout.FlexChild{
		form.Field("Chart data range:", field(&d.rangeEd, 360)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx, layout.Flexed(1, layout.Spacer{}.Layout), btn(&d.switchRC, "Switch Row/Column"))
			})
		}),
		layout.Rigid(th.Label(13, "Legend Entries (Series)", chromeText, font.SemiBold).Layout),
		layout.Rigid(layout.Spacer{Height: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, btn(&d.add, "Add"), btn(&d.editS, "Edit"), btn(&d.remove, "Remove"),
				btn(&d.up, "Move Up"), btn(&d.down, "Move Down"))
		}),
		layout.Rigid(layout.Spacer{Height: 6}.Layout),
		layout.Rigid(list),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		form.Field("Category labels:", field(&d.catEd, 360)),
		layout.Rigid(th.Label(13, "Hidden and Empty Cells", chromeText, font.SemiBold).Layout),
		layout.Rigid(layout.Spacer{Height: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(th.Label(13, "Show empty cells as:  ", chromeText, font.Normal).Layout),
				layout.Rigid(th.Radio(&d.blanks, "gap", "Gaps")),
				layout.Rigid(th.Radio(&d.blanks, "zero", "Zero")),
				layout.Rigid(th.Radio(&d.blanks, "span", "Connect with line")))
		}),
		layout.Rigid(th.CheckBox(&d.hidden, "Show data in hidden rows and columns")),
	}
}

// editSeriesBody is Edit Series: the name and the values.
func (a *App) editSeriesBody() []layout.FlexChild {
	d := &a.ch.dlg
	th := a.ui
	form := gui.Form{Th: th, LabelW: 110}
	field := func(ed *widget.Editor) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return gui.TextField(gtx, th, ed, 280, 1, "") }
	}
	return []layout.FlexChild{
		form.Field("Series name:", field(&d.serName)),
		form.Field("Series values:", field(&d.serVals)),
		layout.Rigid(th.Label(12, "A name is a cell (=Sheet1!$B$1) or text; the values a range (=Sheet1!$B$2:$B$9).", mutedText, font.Normal).Layout),
	}
}

// moveChartBody is Move Chart: a new chart sheet or a sheet.
func (a *App) moveChartBody(gtx layout.Context) []layout.FlexChild {
	d := &a.ch.dlg
	th := a.ui
	var names []string
	var idx []int
	cur := ""
	for i, sh := range a.wb.Sheets {
		if !sh.IsChartSheet() && !sh.Hidden {
			names, idx = append(names, sh.Name), append(idx, i)
			if i == d.objSheet {
				cur = sh.Name
			}
		}
	}
	return []layout.FlexChild{
		layout.Rigid(th.Label(13, "Choose where you want the chart to be placed:", chromeText, font.SemiBold).Layout),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		gui.Form{Th: th}.Row(
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(120)
				return th.Radio(&d.where, "new", "New sheet:")(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.where.Value != "new" {
					gtx = gtx.Disabled()
				}
				return gui.TextField(gtx, th, &d.sheetName, 260, 1, "")
			}),
		),
		gui.Form{Th: th}.Row(
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(120)
				return th.Radio(&d.where, "object", "Object in:")(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.where.Value != "object" {
					gtx = gtx.Disabled()
				}
				return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
					if i := d.objDD.Layout(gtx, th, 260, cur, dropItems(names)); i >= 0 {
						d.objSheet = idx[i]
					}
					return layout.Dimensions{Size: image.Pt(gtx.Dp(260), gtx.Dp(24))}
				})
			}),
		),
	}
}

// formatElemBody is Format …: the fill and outline colors.
func (a *App) formatElemBody(gtx layout.Context) []layout.FlexChild {
	d := &a.ch.dlg
	th := a.ui
	form := gui.Form{Th: th, LabelW: 90}
	pick := func(cp *gui.ColorPicker, cur *gui.ColorChoice, set *bool, none string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				ch, ok, dims := cp.Combo(gtx, th, 200, *cur, none)
				if ok {
					*cur, *set = ch, true
				}
				return dims
			})
		}
	}
	return []layout.FlexChild{
		form.Row(form.Label("Fill:"), layout.Rigid(pick(&d.fillCP, &d.fill, &d.fillSet, "No Fill"))),
		form.Row(form.Label("Border:"), layout.Rigid(pick(&d.lineCP, &d.line, &d.lineSet, "No Line"))),
	}
}
