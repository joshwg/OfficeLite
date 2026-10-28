package ui

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Find and Replace (Ctrl+F, Ctrl+H) and Go To (Ctrl+G, F5).

type findDialog struct {
	replace    bool // the Replace tab
	what, with widget.Editor
	within     gui.DropDown
	lookIn     gui.DropDown
	workbook   bool
	values     bool // look in values (Find only)
	matchCase  widget.Bool
	whole      widget.Bool
	tabFind    widget.Clickable
	tabReplace widget.Clickable
	findAll    widget.Clickable
	findNext   widget.Clickable
	replOne    widget.Clickable
	replAll    widget.Clickable
	close      widget.Clickable
	status     string
}

// showFind opens Find (Ctrl+F).
func (a *App) showFind() { a.openFind(false) }

// showReplace opens Replace (Ctrl+H).
func (a *App) showReplace() { a.openFind(true) }

func (a *App) openFind(replace bool) {
	if !a.commitForCommand() {
		return
	}
	d := &a.ins.find
	d.replace = replace
	d.what.SingleLine, d.what.Submit = true, true
	d.with.SingleLine, d.with.Submit = true, true
	d.what.SetCaret(d.what.Len(), 0)
	d.status = ""
	a.showDialog(dlgFind)
}

// findOptions are the dialog's settings.
func (a *App) findOptions() edit.FindOptions {
	d := &a.ins.find
	o := edit.FindOptions{
		What:        d.what.Text(),
		Replacement: d.with.Text(),
		Workbook:    d.workbook,
		Formulas:    !d.values || d.replace,
		MatchCase:   d.matchCase.Value,
		WholeCell:   d.whole.Value,
	}
	if sel := a.state().view.Sel.Range; !o.Workbook && (sel.R1 != sel.R2 || sel.C1 != sel.C2) {
		o.Within = &sel
	}
	return o
}

// findNextCell selects the next match after the active cell.
func (a *App) findNextCell() bool {
	st := a.state()
	p := st.view.Sel.Active
	f, err := a.book.FindNext(edit.Found{Sheet: a.active, Row: p.Row, Col: p.Col}, a.findOptions(), false)
	if err != nil {
		a.ins.find.status = err.Error()
		return false
	}
	a.goToFound(f)
	a.ins.find.status = ""
	return true
}

// goToFound makes a match the active cell, keeping a multi-cell
// selection being searched.
func (a *App) goToFound(f edit.Found) {
	st := a.state()
	sel := st.view.Sel.Range
	if f.Sheet == a.active && (sel.R1 != sel.R2 || sel.C1 != sel.C2) && sel.Contains(f.Row, f.Col) {
		st.view.Sel.Active = render.Pos{Row: f.Row, Col: f.Col}
		st.view.EnsureVisible(f.Row, f.Col)
		return
	}
	a.selectRange(f.Sheet, model.Range{R1: f.Row, C1: f.Col, R2: f.Row, C2: f.Col})
	a.state().view.EnsureVisible(f.Row, f.Col)
}

func (a *App) layoutFindDialog(gtx layout.Context, k int) {
	d := &a.ins.find
	for _, e := range []*widget.Editor{&d.what, &d.with} {
		for {
			ev, ok := e.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				k = 1
			}
		}
	}
	for d.tabFind.Clicked(gtx) {
		d.replace = false
	}
	for d.tabReplace.Clicked(gtx) {
		d.replace = true
	}
	for d.close.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	for d.findNext.Clicked(gtx) {
		k = 1
	}
	if k > 0 {
		a.findNextCell()
	}
	for d.findAll.Clicked(gtx) {
		all, err := a.book.FindAll(a.active, a.findOptions())
		switch {
		case err != nil:
			d.status = err.Error()
		case len(all) == 0:
			d.status = edit.ErrNotFound.Error()
		default:
			d.status = fmt.Sprintf("%d cell(s) found", len(all))
			a.goToFound(all[0])
		}
	}
	for d.replOne.Clicked(gtx) {
		// Replace the active cell when it matches, then find the next.
		p := a.state().view.Sel.Active
		res, ok, err := a.book.Replace(edit.Found{Sheet: a.active, Row: p.Row, Col: p.Col}, a.findOptions())
		switch {
		case err != nil:
			d.status = err.Error()
		case ok:
			a.changed(res)
			a.findNextCell()
		default:
			a.findNextCell()
		}
	}
	for d.replAll.Clicked(gtx) {
		n, res, err := a.book.ReplaceAll(a.active, a.findOptions())
		switch {
		case err != nil:
			d.status = err.Error()
		case n == 0:
			d.status = edit.ErrNotFound.Error()
		default:
			a.changed(res)
			d.status = fmt.Sprintf("All done. We made %d replacements.", n)
		}
	}

	th := a.ui
	tab := func(btn *widget.Clickable, label string, on bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				c, w := mutedText, font.Normal
				if on {
					c, w = accent, font.SemiBold
				}
				d := layout.Inset{Top: 2, Bottom: 6, Right: 16}.Layout(gtx, th.Label(13, label, c, w).Layout)
				if on {
					gui.HLine(gtx.Ops, 0, d.Size.X-gtx.Dp(16), d.Size.Y-gtx.Dp(2), gtx.Dp(2), accent)
				}
				return d
			})
		})
	}
	drop := func(dd *gui.DropDown, cur string, items []string, set func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if i := dd.Layout(gtx, th, 110, cur, dropItems(items)); i >= 0 {
				set(i)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(110), gtx.Dp(24))}
		}
	}
	within := []string{"Sheet", "Workbook"}
	lookIn := []string{"Formulas", "Values"}
	body := func(gtx layout.Context) layout.Dimensions {
		rows := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx, tab(&d.tabFind, "Find", !d.replace), tab(&d.tabReplace, "Replace", d.replace))
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			a.labeled("Find what:", 100, func(gtx layout.Context) layout.Dimensions {
				return a.textField(gtx, &d.what, 300, 1, "")
			}),
		}
		if d.replace {
			rows = append(rows, a.labeled("Replace with:", 100, func(gtx layout.Context) layout.Dimensions {
				return a.textField(gtx, &d.with, 300, 1, "")
			}))
		}
		cur := lookIn[0]
		if d.values && !d.replace {
			cur = lookIn[1]
		}
		rows = append(rows,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(th.Label(13, "Within:", chromeText, font.Normal).Layout),
					layout.Rigid(layout.Spacer{Width: 6}.Layout),
					layout.Rigid(drop(&d.within, within[map[bool]int{false: 0, true: 1}[d.workbook]], within, func(i int) { d.workbook = i == 1 })),
					layout.Rigid(layout.Spacer{Width: 14}.Layout),
					layout.Rigid(th.Label(13, "Look in:", chromeText, font.Normal).Layout),
					layout.Rigid(layout.Spacer{Width: 6}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if d.replace {
							return th.Label(13, "Formulas", mutedText, font.Normal).Layout(gtx)
						}
						return drop(&d.lookIn, cur, lookIn, func(i int) { d.values = i == 1 })(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(a.checkBox(&d.matchCase, "Match case")),
					layout.Rigid(layout.Spacer{Width: 16}.Layout),
					layout.Rigid(a.checkBox(&d.whole, "Match entire cell contents")),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.status == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, th.Label(12, d.status, mutedText, font.Normal).Layout)
			}),
			layout.Rigid(layout.Spacer{Height: 10}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var btns []layout.FlexChild
				add := func(w layout.Widget) {
					btns = append(btns, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: 6}.Layout(gtx, w)
					}))
				}
				if d.replace {
					add(a.smallButton(&d.replAll, "Replace All"))
					add(a.smallButton(&d.replOne, "Replace"))
				}
				add(a.smallButton(&d.findAll, "Find All"))
				add(a.smallButton(&d.findNext, "Find Next"))
				add(a.smallButton(&d.close, "Close"))
				return layout.Flex{}.Layout(gtx, btns...)
			}),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	title := "Find and Replace"
	a.panel(gtx, title, 470, body)
}

// smallButton is a compact secondary button.
func (a *App) smallButton(btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return gui.Hoverable(gtx, btn, chromeBG, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, a.ui.Label(13, label, chromeText, font.Normal).Layout)
		})
	}
}

// --- Go To ---

type gotoDialog struct {
	ref   widget.Editor
	list  widget.List
	names []widget.Clickable
}

// showGoTo opens Go To (Ctrl+G, F5): a reference (B12, Sheet2!A1:C5) or
// a defined name.
func (a *App) showGoTo() {
	if !a.commitForCommand() {
		return
	}
	d := &a.ins.gt
	d.ref.SingleLine, d.ref.Submit = true, true
	d.ref.SetText("")
	a.showDialog(dlgGoTo)
}

// gotoNames are the defined names Go To lists.
func (a *App) gotoNames() []string {
	var out []string
	for _, n := range a.wb.Names {
		if n.Sheet < 0 || n.Sheet == a.active {
			out = append(out, n.Name)
		}
	}
	return out
}

func (a *App) layoutGoToDialog(gtx layout.Context, k int) {
	d := &a.ins.gt
	for {
		ev, ok := d.ref.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			k = 1
		}
	}
	names := a.gotoNames()
	if len(d.names) < len(names) {
		d.names = make([]widget.Clickable, len(names))
	}
	for i, n := range names {
		for {
			c, ok := d.names[i].Update(gtx)
			if !ok {
				break
			}
			d.ref.SetText(n)
			if c.NumClicks >= 2 {
				k = 1
			}
		}
	}
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
		sheet, r, err := a.book.ResolveRef(d.ref.Text(), a.active)
		if err != nil {
			a.ins.err = err.Error()
		} else {
			a.closeDialog()
			a.selectRange(sheet, r)
			return
		}
	}
	th := a.ui
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(13, "Go to:", chromeText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(140)
				w := gtx.Constraints.Max.X
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
				gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
				g := gtx
				g.Constraints = layout.Exact(image.Pt(w-2, h-2))
				d.list.Axis = layout.Vertical
				off := opOffset(gtx, 1, 1)
				material.List(a.th, &d.list).Layout(g, len(names), func(gtx layout.Context, i int) layout.Dimensions {
					return gui.Hoverable(gtx, &d.names[i], colors.Transparent, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Top: 3, Bottom: 3, Left: 8}.Layout(gtx, th.Label(13, names[i], chromeText, font.Normal).Layout)
					})
				})
				off.Pop()
				return layout.Dimensions{Size: image.Pt(w, h)}
			}),
			layout.Rigid(layout.Spacer{Height: 10}.Layout),
			a.labeled("Reference:", 80, func(gtx layout.Context) layout.Dimensions {
				return a.textField(gtx, &d.ref, 260, 1, "B12, Sheet2!A1:C5 or a name")
			}),
			a.errorLine(),
		)
	}
	a.modal(gtx, &a.ins.block, "Go To", 380, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}
