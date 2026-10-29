package ui

import (
	"image"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/pointer"
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
	"officelite/excel/render"
)

// The Data ribbon's Outline group: Group and Ungroup (with Excel's Rows /
// Columns dialog for a cell range), Clear Outline, Subtotal and its
// dialog, Show Detail and Hide Detail, and clicks on the outline's +/-
// and level buttons (drawn by render/outline.go).

// The Ungroup button's menu.
var ungroupItems = []string{"Ungroup…", "Clear Outline"}

// groupDialog is the Group (or Ungroup) dialog: rows or columns.
type groupDialog struct {
	ungroup bool
	axis    widget.Enum // "rows" or "cols"
	r       model.Range
}

// subtotalDialog is the Subtotal dialog.
type subtotalDialog struct {
	region         model.Range
	byDD, fnDD     gui.DropDown
	by, fn         int // column of the region, index into edit.SubtotalFuncs
	cols           []widget.Bool
	replace, below widget.Bool
	removeAll      widget.Clickable
	list           widget.List
}

// ungroupMenu runs an item of the Ungroup button's menu.
func (a *App) ungroupMenu(i int) {
	switch i {
	case 0:
		a.ungroupCmd()
	case 1:
		a.clearOutline()
	}
}

// groupCmd groups the selected rows or columns (Data > Group,
// Shift+Alt+Right); for a cell range it asks which.
func (a *App) groupCmd() { a.groupOrUngroup(false) }

// ungroupCmd ungroups them (Data > Ungroup, Shift+Alt+Left).
func (a *App) ungroupCmd() { a.groupOrUngroup(true) }

func (a *App) groupOrUngroup(ungroup bool) {
	if !a.readyToChange() || !a.singleRange() {
		return
	}
	a.menu.Close()
	r := a.state().view.Sel.Range
	switch {
	case wholeRows(r) && !wholeCols(r):
		a.runGroup(ungroup, true, r)
	case wholeCols(r) && !wholeRows(r):
		a.runGroup(ungroup, false, r)
	default:
		d := &a.ins.grp
		*d = groupDialog{ungroup: ungroup, r: r}
		d.axis.Value = "rows"
		a.showDialog(dlgGroup)
	}
}

// runGroup groups (ungroups) the rows or the columns of r.
func (a *App) runGroup(ungroup, rows bool, r model.Range) {
	lo, hi := r.R1, r.R2
	if !rows {
		lo, hi = r.C1, r.C2
	}
	var ok bool
	if ungroup {
		ok = a.book.Ungroup(a.active, rows, lo, hi)
	} else {
		ok = a.book.Group(a.active, rows, lo, hi)
	}
	if ok {
		a.afterOutline()
	}
}

func (a *App) layoutGroupDialog(gtx layout.Context, k int) {
	d := &a.ins.grp
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if k != 0 {
		a.closeDialog()
		if k > 0 && a.readyToChange() {
			a.runGroup(d.ungroup, d.axis.Value != "cols", d.r)
		}
		return
	}
	a.focusArea(gtx)
	title := "Group"
	if d.ungroup {
		title = "Ungroup"
	}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.ui.Radio(&d.axis, "rows", "Rows")),
			layout.Rigid(a.ui.Radio(&d.axis, "cols", "Columns")),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, title, 240, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// clearOutline removes the sheet's outline (Ungroup > Clear Outline).
func (a *App) clearOutline() {
	if !a.readyToChange() {
		return
	}
	if a.book.ClearOutline(a.active) {
		a.afterOutline()
	}
}

// showDetail and hideDetail expand or collapse the groups of the
// selection (Data > Show Detail, Hide Detail).
func (a *App) showDetail() { a.detail(false) }
func (a *App) hideDetail() { a.detail(true) }

func (a *App) detail(hide bool) {
	if !a.readyToChange() || !a.singleRange() {
		return
	}
	r := a.state().view.Sel.Range
	var ok bool
	if hide {
		ok = a.book.HideDetail(a.active, r)
	} else {
		ok = a.book.ShowDetail(a.active, r)
	}
	if ok {
		a.afterOutline()
	}
}

// outlinePress runs a click on an outline button: a level button shows
// that many levels, a group's - collapses it and + expands it.
func (a *App) outlinePress(v *render.View, p f32.Point) bool {
	b, ok := v.OutlineButtonAt(p.X, p.Y)
	if !ok {
		return false
	}
	if !a.readyToChange() {
		return true
	}
	if b.Level > 0 {
		a.book.ShowLevels(a.active, b.Rows, b.Level)
	} else {
		a.book.SetGroupCollapsed(a.active, b.Group, !b.Group.Collapsed)
	}
	a.afterOutline()
	return true
}

// outlineCursors shows the arrow over the outline's buttons.
func (a *App) outlineCursors(gtx layout.Context, v *render.View) {
	for _, b := range v.OutlineButtons() {
		area := clip.Rect(b.Rect).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
	}
}

// afterOutline updates the view after the outline changed: the rows it
// hides, and the active cell, which moves off a row (column) that was
// hidden to the summary of the collapsed group, as in Excel.
func (a *App) afterOutline() {
	a.syncSheets()
	a.changed(calc.Result{})
	v := &a.state().view
	sh := v.Sheet
	at := []int{v.Sel.Active.Row, v.Sel.Active.Col}
	moved := false
	for k, rows := range []bool{true, false} {
		i := &at[k]
		if !sh.IndexHidden(rows, *i) {
			continue
		}
		moved = true
		for _, g := range sh.Groups(rows) {
			if g.Collapsed && *i >= g.Lo && *i <= g.Hi && g.HasSummary() && !sh.IndexHidden(rows, g.Summary) {
				*i = g.Summary
				break
			}
		}
		for *i > 0 && sh.IndexHidden(rows, *i) {
			*i--
		}
	}
	if moved {
		a.selectRange(a.active, model.Range{R1: at[0], C1: at[1], R2: at[0], C2: at[1]})
	}
	a.grid.focusPending = true
}

// subtotalDialog opens Subtotal for the selection (or the list around
// the active cell).
func (a *App) subtotalDialog() {
	if !a.commitForCommand() || !a.singleRange() {
		return
	}
	sh := a.wb.Sheets[a.active]
	r := edit.SubtotalRange(sh, a.state().view.Sel.Range)
	if r.R2 <= r.R1 {
		a.dialog.Show(appName, edit.ErrSubtotalRange.Error(), "OK")
		a.answer = func(int) {}
		return
	}
	d := &a.ins.sub
	*d = subtotalDialog{region: r}
	d.cols = make([]widget.Bool, r.C2-r.C1+1)
	d.cols[len(d.cols)-1].Value = true // Excel checks the last column
	d.replace.Value, d.below.Value = true, true
	d.list.Axis = layout.Vertical
	a.selectRange(a.active, r)
	a.showDialog(dlgSubtotal)
}

// subtotalLabels names the list's columns by their headers.
func (a *App) subtotalLabels(d *subtotalDialog) []string {
	sh := a.wb.Sheets[a.active]
	var out []string
	for c := d.region.C1; c <= d.region.C2; c++ {
		s := sh.Get(d.region.R1, c).Text
		if s == "" {
			s = "Column " + model.ColName(c)
		}
		out = append(out, s)
	}
	return out
}

func (a *App) layoutSubtotalDialog(gtx layout.Context, k int) {
	d := &a.ins.sub
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if d.removeAll.Clicked(gtx) {
		a.closeDialog()
		if a.readyToChange() {
			list, res, err := a.book.RemoveSubtotals(a.active, d.region)
			a.afterSubtotal(list, res, err)
		}
		return
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		spec := edit.SubtotalSpec{Range: d.region, GroupBy: d.region.C1 + d.by, Func: d.fn,
			Replace: d.replace.Value, Above: !d.below.Value}
		for i := range d.cols {
			if d.cols[i].Value {
				spec.Cols = append(spec.Cols, d.region.C1+i)
			}
		}
		if len(spec.Cols) == 0 {
			a.ins.err = "Select at least one column to add a subtotal to."
			return
		}
		a.closeDialog()
		if !a.readyToChange() {
			return
		}
		list, res, err := a.book.Subtotal(a.active, spec)
		a.afterSubtotal(list, res, err)
		return
	}
	a.focusArea(gtx)
	th := a.ui
	labels := a.subtotalLabels(d)
	fnNames := make([]string, len(edit.SubtotalFuncs))
	for i, f := range edit.SubtotalFuncs {
		fnNames[i] = f.Name
	}
	heading := func(s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 4}.Layout(gtx, th.Label(13, s, chromeText, font.Normal).Layout)
		})
	}
	drop := func(dd *gui.DropDown, cur *int, items []string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Constraints.Max.X
			if j := dd.Layout(gtx, th, gtx.Metric.PxToDp(w), items[min(*cur, len(items)-1)], dropItems(items)); j >= 0 {
				*cur = j
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(w, gtx.Dp(24))}
			})
		})
	}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			heading("At each change in:"),
			drop(&d.byDD, &d.by, labels),
			heading("Use function:"),
			drop(&d.fnDD, &d.fn, fnNames),
			heading("Add subtotal to:"),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w, h := gtx.Constraints.Max.X, gtx.Dp(130)
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
				gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
				g := gtx
				g.Constraints = layout.Exact(image.Pt(w-8, h-4))
				o := op.Offset(image.Pt(4, 2)).Push(gtx.Ops)
				cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
				material.List(a.th, &d.list).Layout(g, len(d.cols), func(gtx layout.Context, i int) layout.Dimensions {
					return a.ui.CheckBox(&d.cols[i], labels[i])(gtx)
				})
				cs.Pop()
				o.Pop()
				return layout.Dimensions{Size: image.Pt(w, h)}
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(a.ui.CheckBox(&d.replace, "Replace current subtotals")),
			layout.Rigid(a.ui.CheckBox(&d.below, "Summary below data")),
			a.errorLine(),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Subtotal", 340, body,
		a.plainButton(&d.removeAll, "Remove All"), a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// afterSubtotal shows the result of Subtotal or Remove All: the list
// selected, or the error.
func (a *App) afterSubtotal(list model.Range, res calc.Result, err error) {
	if err != nil {
		a.dialog.Show(appName, err.Error(), "OK")
		a.answer = func(int) {}
		return
	}
	a.syncSheets()
	a.changed(res)
	a.selectRange(a.active, list)
}
