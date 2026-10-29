package ui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// The Custom AutoFilter dialog (two conditions joined by And or Or, with
// * and ? wildcards) and the Top 10 AutoFilter dialog.

// Custom AutoFilter's comparisons, as Excel lists them; begins with,
// contains, ... are equals with wildcards.
const (
	opNone = iota
	opEquals
	opNotEquals
	opGreater
	opGreaterEq
	opLess
	opLessEq
	opBegins
	opNotBegins
	opEnds
	opNotEnds
	opContains
	opNotContains
)

var customOps = []string{
	"", "equals", "does not equal", "is greater than", "is greater than or equal to", "is less than",
	"is less than or equal to", "begins with", "does not begin with", "ends with", "does not end with",
	"contains", "does not contain",
}

type customFilterDialog struct {
	col int
	ops [2]gui.DropDown
	op  [2]int
	val [2]widget.Editor
	and widget.Enum
}

type top10Dialog struct {
	col           int
	topDD, unitDD gui.DropDown
	bottom, pct   bool
	n             widget.Editor
}

// customFilterDialog opens Custom AutoFilter for column col, with the
// comparisons op1 and op2 (op1 -1: the column's current conditions).
func (a *App) customFilterDialog(col, op1, op2 int, and bool) {
	d := &a.ins.cf
	d.col = col
	d.op = [2]int{op1, op2}
	for i := range d.val {
		d.val[i].SingleLine, d.val[i].Submit = true, true
		d.val[i].SetText("")
	}
	d.and.Value = "and"
	if !and {
		d.and.Value = "or"
	}
	if op1 < 0 {
		d.op = [2]int{opEquals, opNone}
		if fc, ok := a.wb.Sheets[a.active].AutoFilter.Column(col); ok && fc.Kind == model.FilterCustom {
			for i, cd := range fc.Conds[:min(len(fc.Conds), 2)] {
				op, text := a.condToDialog(cd)
				d.op[i] = op
				d.val[i].SetText(text)
			}
			if !fc.And && len(fc.Conds) > 1 {
				d.and.Value = "or"
			}
		}
	}
	a.showDialog(dlgCustomFilter)
}

// condToDialog is how the dialog shows a condition: "a*" is begins with
// a, a date column's serial number a date.
func (a *App) condToDialog(cd model.FilterCond) (int, string) {
	v := cd.Val
	inner := func(s string) bool { return s != "" && !formula.HasWildcards(s) }
	eq := cd.Op == model.OpEqual
	switch {
	case cd.Op != model.OpEqual && cd.Op != model.OpNotEqual:
		return opGreater + int(cd.Op-model.OpGreater), a.dialogValue(v)
	case len(v) > 2 && strings.HasPrefix(v, "*") && strings.HasSuffix(v, "*") && inner(v[1:len(v)-1]):
		return pick(eq, opContains, opNotContains), v[1 : len(v)-1]
	case strings.HasSuffix(v, "*") && inner(v[:len(v)-1]):
		return pick(eq, opBegins, opNotBegins), v[:len(v)-1]
	case strings.HasPrefix(v, "*") && inner(v[1:]):
		return pick(eq, opEnds, opNotEnds), v[1:]
	}
	return pick(eq, opEquals, opNotEquals), a.dialogValue(v)
}

func pick(c bool, a, b int) int {
	if c {
		return a
	}
	return b
}

// dialogValue shows a stored value: a date column's serial numbers as
// dates.
func (a *App) dialogValue(v string) string {
	if a.book.FilterColumnKind(a.active, a.ins.cf.col) != edit.ColumnDate {
		return v
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	t := formula.SerialToTime(n, a.wb.Date1904)
	return strconv.Itoa(int(t.Month())) + "/" + strconv.Itoa(t.Day()) + "/" + strconv.Itoa(t.Year())
}

// dialogCond turns a comparison and the text typed for it into a
// condition: numbers, dates and percentages are compared as numbers.
func (a *App) dialogCond(op int, text string) (model.FilterCond, bool) {
	text = strings.TrimSpace(text)
	value := text
	if in, err := edit.ParseInput(text, a.wb.Date1904); err == nil && in.Kind == model.KindNumber {
		value = strconv.FormatFloat(in.Num, 'g', -1, 64)
	}
	switch op {
	case opEquals:
		return model.FilterCond{Op: model.OpEqual, Val: value}, true
	case opNotEquals:
		return model.FilterCond{Op: model.OpNotEqual, Val: value}, true
	case opGreater, opGreaterEq, opLess, opLessEq:
		return model.FilterCond{Op: model.OpGreater + model.FilterOp(op-opGreater), Val: value}, true
	case opBegins, opNotBegins:
		return model.FilterCond{Op: model.FilterOp(op - opBegins), Val: text + "*"}, true
	case opEnds, opNotEnds:
		return model.FilterCond{Op: model.FilterOp(op - opEnds), Val: "*" + text}, true
	case opContains, opNotContains:
		return model.FilterCond{Op: model.FilterOp(op - opContains), Val: "*" + text + "*"}, true
	}
	return model.FilterCond{}, false
}

func (a *App) layoutCustomFilterDialog(gtx layout.Context, k int) {
	d := &a.ins.cf
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		fc := &model.FilterColumn{Kind: model.FilterCustom, And: d.and.Value == "and"}
		for i := range d.op {
			if cd, ok := a.dialogCond(d.op[i], d.val[i].Text()); ok {
				fc.Conds = append(fc.Conds, cd)
			}
		}
		a.closeDialog()
		if len(fc.Conds) == 0 {
			a.setFilterColumn(d.col, nil)
			return
		}
		a.setFilterColumn(d.col, fc)
		return
	}
	th := a.ui
	labels := customOps
	condRow := func(i int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if j := d.ops[i].Layout(gtx, th, 190, labels[d.op[i]], dropItems(labels)); j >= 0 {
							d.op[i] = j
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(190), gtx.Dp(24))}
					}),
					layout.Rigid(layout.Spacer{Width: 10}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return gui.TextField(gtx, a.ui, &d.val[i], 220, 1, "")
					}),
				)
			})
		})
	}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 10}.Layout(gtx, th.Label(13, "Show rows where: "+a.headerName(d.col), chromeText, font.Normal).Layout)
			}),
			condRow(0),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 8, Left: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(a.ui.Radio(&d.and, "and", "And")),
						layout.Rigid(layout.Spacer{Width: 20}.Layout),
						layout.Rigid(a.ui.Radio(&d.and, "or", "Or")),
					)
				})
			}),
			condRow(1),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 6}.Layout(gtx, th.Label(12, "Use ? to represent any single character", mutedText, font.Normal).Layout)
			}),
			layout.Rigid(th.Label(12, "Use * to represent any series of characters", mutedText, font.Normal).Layout),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Custom AutoFilter", 480, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// top10Dialog opens Top 10 AutoFilter for column col.
func (a *App) top10Dialog(col int) {
	d := &a.ins.top
	d.col, d.bottom, d.pct = col, false, false
	d.n.SingleLine, d.n.Submit = true, true
	d.n.SetText("10")
	if fc, ok := a.wb.Sheets[a.active].AutoFilter.Column(col); ok && fc.Kind == model.FilterTop10 {
		d.bottom, d.pct = !fc.Top10.Top, fc.Top10.Percent
		d.n.SetText(strconv.FormatFloat(fc.Top10.N, 'g', -1, 64))
	}
	a.showDialog(dlgTop10)
}

func (a *App) layoutTop10Dialog(gtx layout.Context, k int) {
	d := &a.ins.top
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		n, err := strconv.ParseFloat(strings.TrimSpace(d.n.Text()), 64)
		limit := 500.0
		if d.pct {
			limit = 100
		}
		if err != nil || n < 1 || n > limit || n != float64(int(n)) {
			a.ins.err = "Enter a whole number from 1 to " + strconv.Itoa(int(limit)) + "."
			return
		}
		a.closeDialog()
		a.setFilterColumn(d.col, &model.FilterColumn{Kind: model.FilterTop10, Top10: model.Top10{Top: !d.bottom, Percent: d.pct, N: n}})
		return
	}
	th := a.ui
	tops, units := []string{"Top", "Bottom"}, []string{"Items", "Percent"}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 10}.Layout(gtx, th.Label(13, "Show", chromeText, font.Normal).Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if j := d.topDD.Layout(gtx, th, 100, tops[pick(d.bottom, 1, 0)], dropItems(tops)); j >= 0 {
							d.bottom = j == 1
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(100), gtx.Dp(24))}
					}),
					layout.Rigid(layout.Spacer{Width: 10}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return gui.TextField(gtx, a.ui, &d.n, 70, 1, "")
					}),
					layout.Rigid(layout.Spacer{Width: 10}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if j := d.unitDD.Layout(gtx, th, 100, units[pick(d.pct, 1, 0)], dropItems(units)); j >= 0 {
							d.pct = j == 1
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(100), gtx.Dp(24))}
					}),
				)
			}),
			a.errorLine(),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Top 10 AutoFilter", 400, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}
