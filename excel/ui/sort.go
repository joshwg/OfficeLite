package ui

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
)

// Sort A to Z, Z to A and Custom Sort.

// sortRegion is what a sort of the selection sorts: the selection (its
// used part), or for a single cell the data around it.
func (a *App) sortRegion() model.Range {
	v := &a.state().view
	r := v.Sel.Range
	if r.R1 == r.R2 && r.C1 == r.C2 {
		return edit.CurrentRegion(v.Sheet, r.R1, r.C1)
	}
	if r.R2 == model.MaxRows-1 {
		r.R2 = max(v.Sheet.UsedRows()-1, r.R1)
	}
	if r.C2 == model.MaxCols-1 {
		r.C2 = max(v.Sheet.UsedCols()-1, r.C1)
	}
	return r
}

// sortAscending sorts the selection's rows by the active cell's column,
// A to Z (smallest to largest).
func (a *App) sortAscending() { a.quickSort(false) }

// sortDescending sorts Z to A.
func (a *App) sortDescending() { a.quickSort(true) }

func (a *App) quickSort(desc bool) {
	if !a.commitForCommand() {
		return
	}
	r := a.sortRegion()
	col := a.state().view.Sel.Active.Col
	if col < r.C1 || col > r.C2 {
		col = r.C1
	}
	a.runSort(edit.SortSpec{Range: r, Keys: []edit.SortKey{{Col: col, Descending: desc}}, Header: a.book.GuessHeader(a.active, r)})
}

func (a *App) runSort(spec edit.SortSpec) {
	res, err := a.book.Sort(a.active, spec)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(res)
	active := a.state().view.Sel.Active
	a.selectRange(a.active, spec.Range)
	if spec.Range.Contains(active.Row, active.Col) {
		a.state().view.Sel.Active = active
	}
}

// sortDialog is Custom Sort: up to three keys.
type sortDialog struct {
	region  model.Range
	header  widget.Bool
	cols    [3]gui.DropDown
	orders  [3]gui.DropDown
	col     [3]int // 0 = none, else 1 + column offset in the region
	desc    [3]bool
	matchCs widget.Bool
}

// customSortDialog opens Custom Sort for the selection (or the data
// around the active cell).
func (a *App) customSortDialog() {
	if !a.commitForCommand() {
		return
	}
	d := &a.ins.sort
	d.region = a.sortRegion()
	d.header.Value = a.book.GuessHeader(a.active, d.region)
	d.matchCs.Value = false
	d.col, d.desc = [3]int{}, [3]bool{}
	act := a.state().view.Sel.Active.Col
	if act >= d.region.C1 && act <= d.region.C2 {
		d.col[0] = act - d.region.C1 + 1
	} else {
		d.col[0] = 1
	}
	a.showDialog(dlgSort)
}

// colLabels names the region's columns: the header text, or "Column B".
func (a *App) colLabels(d *sortDialog) []string {
	sh := a.wb.Sheets[a.active]
	labels := []string{"(none)"}
	for c := d.region.C1; c <= d.region.C2; c++ {
		s := "Column " + model.ColName(c)
		if d.header.Value {
			if t := sh.Get(d.region.R1, c).Text; t != "" {
				s = t
			}
		}
		labels = append(labels, s)
	}
	return labels
}

func (a *App) layoutSortDialog(gtx layout.Context, k int) {
	d := &a.ins.sort
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
		spec := edit.SortSpec{Range: d.region, Header: d.header.Value, MatchCase: d.matchCs.Value}
		for i := range d.col {
			if d.col[i] > 0 {
				spec.Keys = append(spec.Keys, edit.SortKey{Col: d.region.C1 + d.col[i] - 1, Descending: d.desc[i]})
			}
		}
		a.closeDialog()
		a.runSort(spec)
		return
	}
	a.focusArea(gtx)
	labels := a.colLabels(d)
	orders := []string{"A to Z", "Z to A"}
	th := a.ui
	body := func(gtx layout.Context) layout.Dimensions {
		rows := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 10}.Layout(gtx, th.Label(13, "Sort "+rangeName(d.region), mutedText, font.Normal).Layout)
			}),
		}
		for i := range d.col {
			name := "Sort by"
			if i > 0 {
				name = "Then by"
			}
			rows = append(rows, a.labeled(name, 80, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if j := d.cols[i].Layout(gtx, th, 200, labels[min(d.col[i], len(labels)-1)], dropItems(labels)); j >= 0 {
							d.col[i] = j
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(200), gtx.Dp(24))}
					}),
					layout.Rigid(layout.Spacer{Width: 10}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cur := orders[0]
						if d.desc[i] {
							cur = orders[1]
						}
						if j := d.orders[i].Layout(gtx, th, 100, cur, dropItems(orders)); j >= 0 {
							d.desc[i] = j == 1
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(100), gtx.Dp(24))}
					}),
				)
			}))
		}
		rows = append(rows,
			layout.Rigid(a.checkBox(&d.header, "My data has headers")),
			layout.Rigid(a.checkBox(&d.matchCs, "Case sensitive")),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	a.modal(gtx, &a.ins.block, "Sort", 440, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}
