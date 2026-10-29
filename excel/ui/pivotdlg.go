// pivotdlg.go implements the PivotTable dialogs: Create, Value Field Settings, Field Settings, Move and Change Data Source.

package ui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/pivot"
)

// The PivotTable dialogs: Create PivotTable, Value Field Settings, Field
// Settings, Move PivotTable and Change PivotTable Data Source.

// pivotDlgKind says which PivotTable dialog is open.
type pivotDlgKind int

const (
	pdCreate pivotDlgKind = iota
	pdValue
	pdField
	pdMove
	pdSource
)

// pivotDialog is the open PivotTable dialog (at most one).
type pivotDialog struct {
	open  bool
	kind  pivotDlgKind
	focus bool
	key   int // Enter (1) or Esc (-1)
	tag   int // focus tag when no field has the focus
	block int
	err   string

	ok, cancel widget.Clickable

	source widget.Editor // Create, Change Data Source
	where  widget.Enum   // "new" or "existing" (Create, Move)
	loc    widget.Editor

	value     int           // Value Field Settings: the value field
	name      widget.Editor // custom name
	fn        widget.Enum   // the function, as its index
	showDD    gui.DropDown
	showAs    int
	fmtDD     gui.DropDown
	numFmt    int
	fnList    widget.List
	field     int         // Field Settings: the field
	subtotals widget.Enum // "auto" or "none"
}

// valueFormats are the number formats Value Field Settings offers, as
// built-in format ids.
var valueFormats = []struct {
	label string
	id    int
}{
	{"General", 0}, {"Number (0)", 1}, {"Number (0.00)", 2}, {"Number (#,##0)", 3}, {"Number (#,##0.00)", 4},
	{"Currency", 7}, {"Accounting", 44}, {"Percentage (0%)", 9}, {"Percentage (0.00%)", 10},
	{"Date", 14}, {"Scientific", 11},
}

// openPivotDialog shows dialog k.
func (a *App) openPivotDialog(k pivotDlgKind) {
	a.menu.Close()
	d := &a.pv.dlg
	d.open, d.kind, d.focus, d.err, d.key = true, k, true, "", 0
	for _, ed := range []*widget.Editor{&d.source, &d.loc, &d.name} {
		ed.SingleLine, ed.Submit = true, true
	}
	d.fnList.Axis = layout.Vertical
}

// closePivotDialog closes the open PivotTable dialog and gives the grid the
// focus back.
func (a *App) closePivotDialog() {
	a.pv.dlg.open = false
	a.grid.focusPending = true
}

// createPivotDialog opens Create PivotTable with the data around the
// active cell (or the selection) as the source.
func (a *App) createPivotDialog() {
	v := &a.state().view
	sh := v.Sheet
	r := v.Sel.Range
	if r.R1 == r.R2 && r.C1 == r.C2 {
		r = edit.CurrentRegion(sh, r.R1, r.C1)
	}
	r.R2 = min(r.R2, max(sh.UsedRows()-1, r.R1))
	r.C2 = min(r.C2, max(sh.UsedCols()-1, r.C1))
	a.openPivotDialog(pdCreate)
	d := &a.pv.dlg
	d.source.SetText(a.book.SourceText(a.active, r))
	d.where.Value = "new"
	d.loc.SetText("")
}

// valueFieldDialog opens Value Field Settings for value field v of the
// active cell's report. The bool argument is ignored.
func (a *App) valueFieldDialog(v int, _ bool) {
	pt, _ := a.curPivot()
	if pt == nil || v < 0 || v >= len(pt.Values) {
		return
	}
	a.openPivotDialog(pdValue)
	d := &a.pv.dlg
	pv := pt.Values[v]
	d.value = v
	d.name.SetText(pv.Name)
	d.fn.Value = strconv.Itoa(int(pv.Func))
	d.showAs = int(pv.ShowAs)
	d.numFmt = 0
	for i, f := range valueFormats {
		if f.id == pv.NumFmtID && pv.NumFmtCode == "" {
			d.numFmt = i
		}
	}
}

// fieldSettingsDialog opens Field Settings for field f.
func (a *App) fieldSettingsDialog(f int) {
	pt, _ := a.curPivot()
	if pt == nil || f < 0 || f >= len(pt.Fields) {
		return
	}
	a.openPivotDialog(pdField)
	d := &a.pv.dlg
	d.field = f
	d.name.SetText(pt.Fields[f].Label())
	d.subtotals.Value = "auto"
	if pt.Fields[f].NoSubtotal {
		d.subtotals.Value = "none"
	}
}

// movePivotDialog opens Move PivotTable.
func (a *App) movePivotDialog() {
	pt, _ := a.curPivot()
	if pt == nil {
		return
	}
	a.openPivotDialog(pdMove)
	d := &a.pv.dlg
	d.where.Value = "existing"
	d.loc.SetText(model.CellName(pt.Out.Area.R1, pt.Out.Area.C1))
}

// changeSourceDialog opens Change PivotTable Data Source.
func (a *App) changeSourceDialog() {
	pt, _ := a.curPivot()
	if pt == nil {
		return
	}
	a.openPivotDialog(pdSource)
	a.pv.dlg.source.SetText(pt.Source)
}

// parseLocation reads a location: A3, or 'Sheet 2'!$A$3.
func (a *App) parseLocation(s string) (sheet, row, col int, ok bool) {
	name, r, ok := pivot.SplitSource(s)
	if !ok {
		return 0, 0, 0, false
	}
	sheet = a.active
	if name != "" {
		sheet = -1
		for i, sh := range a.wb.Sheets {
			if strings.EqualFold(sh.Name, name) {
				sheet = i
			}
		}
		if sheet < 0 {
			return 0, 0, 0, false
		}
	}
	return sheet, r.R1, r.C1, true
}

// sourceText checks a source typed in a dialog and returns how the
// report keeps it: a reference with its sheet, or a defined name.
func (a *App) sourceText(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "=")
	si, r, err := a.book.PivotSourceRange(a.active, s)
	if err != nil {
		return "", err
	}
	if _, err := pivot.ReadSource(a.wb.Sheets[si], si, r, nil); err != nil {
		return "", err
	}
	if _, _, ok := pivot.SplitSource(s); ok {
		return a.book.SourceText(si, r), nil
	}
	return s, nil
}

// runPivotDialog carries out the open dialog's OK; false keeps it open
// (with d.err set).
func (a *App) runPivotDialog() bool {
	d := &a.pv.dlg
	switch d.kind {
	case pdCreate:
		src, err := a.sourceText(d.source.Text())
		if err != nil {
			d.err = err.Error()
			return false
		}
		if d.where.Value == "new" {
			a.closePivotDialog()
			a.newSheetPivot(src)
			return true
		}
		sheet, row, col, ok := a.parseLocation(d.loc.Text())
		if !ok {
			d.err = "Reference isn't valid."
			return false
		}
		pt, err := a.book.NewPivot(sheet, src, row, col)
		if err != nil {
			d.err = err.Error()
			return false
		}
		a.closePivotDialog()
		a.pv.hideList = false
		a.applyPivot(sheet, -1, pt, func() {
			a.selectRange(sheet, model.Range{R1: pt.Row, C1: pt.Col, R2: pt.Row, C2: pt.Col})
		})
	case pdValue:
		name := strings.TrimSpace(d.name.Text())
		fn, _ := strconv.Atoi(d.fn.Value)
		v, show, nf := d.value, model.PivotShowAs(d.showAs), valueFormats[d.numFmt].id
		pt, _ := a.curPivot()
		if pt == nil {
			break
		}
		for i, pv := range pt.Values {
			if i != v && strings.EqualFold(pv.Name, name) {
				d.err = "PivotTable field name already exists."
				return false
			}
		}
		a.closePivotDialog()
		a.changePivot(func(p *model.PivotTable) {
			pv := &p.Values[v]
			setValueFunc(p, v, model.PivotFunc(fn))
			if name != "" && name != pt.Values[v].Name {
				pv.Name = name
			}
			pv.ShowAs = show
			pv.NumFmtID, pv.NumFmtCode = nf, ""
		})
	case pdField:
		name := strings.TrimSpace(d.name.Text())
		f, none := d.field, d.subtotals.Value == "none"
		a.closePivotDialog()
		a.changePivot(func(p *model.PivotTable) {
			pf := &p.Fields[f]
			pf.Caption = ""
			if name != "" && name != pf.Name {
				pf.Caption = name
			}
			pf.NoSubtotal = none
		})
	case pdMove:
		pt, idx := a.curPivot()
		if pt == nil {
			break
		}
		if d.where.Value == "new" {
			a.closePivotDialog()
			a.movePivotToNewSheet(idx)
			return true
		}
		sheet, row, col, ok := a.parseLocation(d.loc.Text())
		if !ok {
			d.err = "Reference isn't valid."
			return false
		}
		// The location is the report's top-left, filters included.
		row += pt.Out.Body.R1 - pt.Out.Area.R1
		a.closePivotDialog()
		a.movePivot(idx, sheet, row, col, false)
	case pdSource:
		src, err := a.sourceText(d.source.Text())
		if err != nil {
			d.err = err.Error()
			return false
		}
		a.closePivotDialog()
		a.changePivot(func(p *model.PivotTable) { p.Source = src })
	}
	a.closePivotDialog()
	return true
}

// newSheetPivot creates an empty report at A3 of a new sheet before the
// active one, as one undo step, and selects it.
func (a *App) newSheetPivot(src string) {
	if !a.readyToChange() {
		return
	}
	at := a.active
	a.book.BeginGroup()
	_, err := a.book.InsertSheet(at)
	var res calc.Result
	if err == nil {
		var pt *model.PivotTable
		if pt, err = a.book.NewPivot(at, src, 2, 0); err == nil {
			res, err = a.book.SetPivot(at, -1, pt, true)
		}
	}
	a.book.EndGroup()
	if err != nil {
		a.book.Undo()
		a.syncSheets()
		a.showError(err)
		return
	}
	a.pv.hideList = false
	a.pivotDone(res)
	a.active = at
	a.selectRange(at, model.Range{R1: 2, R2: 2})
}

// movePivot moves report idx of the active sheet to row, col of sheet.
func (a *App) movePivot(idx, sheet, row, col int, overwrite bool) {
	if !a.readyToChange() {
		return
	}
	res, err := a.book.MovePivot(a.active, idx, sheet, row, col, overwrite)
	switch {
	case err == edit.ErrPivotOverwrite:
		a.dialog.Show(appName, "There's already data in ["+a.wb.Name+"]"+a.wb.Sheets[sheet].Name+". Do you want to replace it?", "OK", "Cancel")
		a.answer = func(i int) {
			if i == 0 {
				a.movePivot(idx, sheet, row, col, true)
			}
		}
		return
	case err != nil:
		a.showError(err)
		return
	}
	a.pivotDone(res)
	pt := a.wb.Sheets[sheet].Pivots[len(a.wb.Sheets[sheet].Pivots)-1]
	if sheet == a.active {
		pt = a.wb.Sheets[sheet].Pivots[idx]
	}
	a.selectRange(sheet, model.Range{R1: pt.Out.Body.R1, C1: pt.Out.Body.C1, R2: pt.Out.Body.R1, C2: pt.Out.Body.C1})
}

// movePivotToNewSheet moves report idx to A3 of a new sheet.
func (a *App) movePivotToNewSheet(idx int) {
	if !a.readyToChange() {
		return
	}
	from := a.active
	a.book.BeginGroup()
	_, err := a.book.InsertSheet(from)
	var res calc.Result
	if err == nil {
		res, err = a.book.MovePivot(from+1, idx, from, 2, 0, true)
	}
	a.book.EndGroup()
	if err != nil {
		a.book.Undo()
		a.syncSheets()
		a.showError(err)
		return
	}
	a.pivotDone(res)
	a.active = from
	a.selectRange(from, model.Range{R1: 2, R2: 2})
}

// pivotDialogInput takes Enter and Esc for the open dialog.
func (a *App) pivotDialogInput(gtx layout.Context) {
	d := &a.pv.dlg
	if !d.open {
		return
	}
	for _, t := range []any{&d.source, &d.loc, &d.name, &d.tag} {
		if k := dialogKeys(gtx, t); k != 0 {
			d.key = k
		}
	}
}

// layoutPivotDialog draws the open dialog.
func (a *App) layoutPivotDialog(gtx layout.Context) {
	d := &a.pv.dlg
	if !d.open {
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
		// The dialog is drawn last: the rest of the window shows the
		// change at the next frame.
		gtx.Execute(op.InvalidateCmd{})
	}
	if k < 0 {
		a.closePivotDialog()
		return
	}
	if k > 0 && a.runPivotDialog() {
		return
	}
	if d.focus {
		d.focus = false
		switch d.kind {
		case pdCreate, pdSource:
			gtx.Execute(key.FocusCmd{Tag: &d.source})
		case pdValue, pdField:
			gtx.Execute(key.FocusCmd{Tag: &d.name})
		case pdMove:
			gtx.Execute(key.FocusCmd{Tag: &d.loc})
		}
	}
	th := a.ui
	para := func(s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 6}.Layout(gtx, th.Label(13, s, chromeText, font.SemiBold).Layout)
		})
	}
	form := gui.Form{Th: a.ui, LabelW: 110}
	field := func(ed *widget.Editor) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return gui.TextField(gtx, a.ui, ed, 300, 1, "") }
	}
	space := layout.Rigid(layout.Spacer{Height: 8}.Layout)
	whereRows := func() []layout.FlexChild {
		return []layout.FlexChild{
			layout.Rigid(a.ui.Radio(&d.where, "new", "New Worksheet")),
			layout.Rigid(a.ui.Radio(&d.where, "existing", "Existing Worksheet")),
			space,
			form.Field("Location:", func(gtx layout.Context) layout.Dimensions {
				if d.where.Value == "new" {
					gtx = gtx.Disabled()
				}
				return field(&d.loc)(gtx)
			}),
		}
	}
	var title string
	var body []layout.FlexChild
	pt, _ := a.curPivot()
	switch d.kind {
	case pdCreate:
		title = "Create PivotTable"
		body = append([]layout.FlexChild{
			para("Choose the data that you want to analyze"),
			form.Field("Table/Range:", field(&d.source)),
			space,
			para("Choose where you want the PivotTable report to be placed"),
		}, whereRows()...)
	case pdMove:
		title = "Move PivotTable"
		body = append([]layout.FlexChild{para("Choose where you want the PivotTable to be placed")}, whereRows()...)
	case pdSource:
		title = "Change PivotTable Data Source"
		body = []layout.FlexChild{
			para("Choose the data that you want to analyze"),
			form.Field("Table/Range:", field(&d.source)),
		}
	case pdValue:
		title = "Value Field Settings"
		src := ""
		if pt != nil && d.value < len(pt.Values) {
			src = pt.Fields[pt.Values[d.value].Field].Name
		}
		body = []layout.FlexChild{
			form.Field("Source Name:", th.Label(13, src, chromeText, font.Normal).Layout),
			form.Field("Custom Name:", field(&d.name)),
			para("Summarize value field by"),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.pivotFnList(gtx)
			}),
			space,
			form.Field("Show values as:", func(gtx layout.Context) layout.Dimensions {
				if i := d.showDD.Layout(gtx, th, 300, model.ShowAsNames[d.showAs], dropItems(model.ShowAsNames)); i >= 0 {
					d.showAs = i
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(300), gtx.Dp(24))}
			}),
			form.Field("Number Format:", func(gtx layout.Context) layout.Dimensions {
				labels := make([]string, len(valueFormats))
				for i, f := range valueFormats {
					labels[i] = f.label
				}
				if i := d.fmtDD.Layout(gtx, th, 300, labels[d.numFmt], dropItems(labels)); i >= 0 {
					d.numFmt = i
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(300), gtx.Dp(24))}
			}),
		}
	case pdField:
		title = "Field Settings"
		src := ""
		if pt != nil && d.field < len(pt.Fields) {
			src = pt.Fields[d.field].Name
		}
		body = []layout.FlexChild{
			form.Field("Source Name:", th.Label(13, src, chromeText, font.Normal).Layout),
			form.Field("Custom Name:", field(&d.name)),
			para("Subtotals"),
			layout.Rigid(a.ui.Radio(&d.subtotals, "auto", "Automatic")),
			layout.Rigid(a.ui.Radio(&d.subtotals, "none", "None")),
		}
	}
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
	a.ui.ModalDialog(gtx, &d.block, title, 460, content, a.accentButton(&d.ok, "OK"), a.plainButton(&d.cancel, "Cancel"))
}

// pivotFnList is Value Field Settings' list of functions.
func (a *App) pivotFnList(gtx layout.Context) layout.Dimensions {
	d := &a.pv.dlg
	w, h := gtx.Constraints.Max.X, gtx.Dp(150)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	g := gtx
	g.Constraints = layout.Exact(image.Pt(w-8, h-4))
	o := op.Offset(image.Pt(4, 2)).Push(gtx.Ops)
	cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
	material.List(a.th, &d.fnList).Layout(g, len(model.FuncNames), func(gtx layout.Context, i int) layout.Dimensions {
		return a.ui.Radio(&d.fn, strconv.Itoa(i), model.FuncNames[i])(gtx)
	})
	cs.Pop()
	o.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}
