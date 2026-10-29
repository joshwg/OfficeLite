package ui

import (
	"errors"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// cellEditor is the state of editing a cell: the text shows in the cell
// itself and in the formula bar, either of which can have the caret.
type cellEditor struct {
	active bool
	// enter is Excel's Enter mode (started by typing over a cell): the
	// arrow keys commit the entry and move. Edit mode (F2, double-click,
	// the formula bar) moves the caret instead.
	enter           bool
	inBar           bool // the formula bar has the caret
	sheet, row, col int
	orig            string // the text when editing started
	cell, bar       widget.Editor
	focus           *widget.Editor // editor to focus at the next frame
	barText         string         // what bar shows while not editing
	point           refPoint       // the reference last put in by pointing (mouse.go)
	checked         bool           // the entry passed its validation alert (validation.go)
}

var editSelection = color.NRGBA{R: 0xA8, G: 0x5C, B: 0x00, A: 0x50}

// editText is a cell's content as editing starts with it: the formula,
// the value (percentages as 12%), or the text with its line breaks.
func editText(sh *model.Sheet, p render.Pos) string {
	r, c := p.Row, p.Col
	if m, ok := sh.MergeAt(r, c); ok {
		r, c = m.R1, m.C1
	}
	cell := sh.Get(r, c)
	if cell.Formula == "" {
		// A value spilled from a dynamic array shows the array's formula.
		if ar, ac, ok := sh.SpillAnchor(r, c); ok {
			return "=" + sh.Get(ar, ac).Formula
		}
	}
	switch {
	case cell.Formula != "":
		return "=" + cell.Formula
	case cell.Kind == model.KindNumber && strings.HasSuffix(cell.Text, "%") && looksNumeric(cell.Text):
		return strconv.FormatFloat(cell.Num*100, 'G', 15, 64) + "%"
	case cell.Kind == model.KindNumber && looksNumeric(cell.Text):
		return strconv.FormatFloat(cell.Num, 'G', 15, 64)
	case cell.Kind == model.KindNumber:
		// Dates and times are edited as displayed.
		return cell.Text
	}
	return cell.Text
}

// beginEdit starts editing the active cell with text, the caret at its
// end. enter selects Enter mode; bar puts the caret in the formula bar.
func (a *App) beginEdit(text string, enter, bar bool) {
	if a.onChartSheet() {
		return // no cells to edit
	}
	st := a.state()
	p := st.view.Sel.Active
	if m, ok := st.view.Sheet.MergeAt(p.Row, p.Col); ok {
		p = render.Pos{Row: m.R1, Col: m.C1}
	}
	ed := &a.ed
	ed.active, ed.enter, ed.inBar = true, enter, bar
	ed.sheet, ed.row, ed.col = a.active, p.Row, p.Col
	ed.point = refPoint{}
	ed.orig = editText(st.view.Sheet, p)
	ed.cell.SetText(text)
	ed.bar.SetText(text)
	n := ed.cell.Len()
	ed.cell.SetCaret(n, n)
	ed.bar.SetCaret(n, n)
	ed.focus = &ed.cell
	if bar {
		ed.focus = &ed.bar
	}
	st.view.EnsureVisible(p.Row, p.Col)
	a.errMsg = ""
}

// editingText is the entry being edited.
func (a *App) editingText() string {
	if a.ed.inBar {
		return a.ed.bar.Text()
	}
	return a.ed.cell.Text()
}

// commitEdit stores the entry and moves the active cell by dr, dc. A
// formula that doesn't parse is reported and editing continues.
func (a *App) commitEdit(dr, dc int) bool {
	ed := &a.ed
	if !ed.active {
		return true
	}
	if a.dialog.Visible {
		return false // a data validation alert is waiting for its answer
	}
	text := a.editingText()
	if text != ed.orig {
		if !a.entryAllowed(text, dr, dc) {
			return false // validation.go
		}
		res, err := a.book.Enter(ed.sheet, ed.row, ed.col, text)
		if errors.Is(err, edit.ErrPivotChange) {
			ed.active = false // Excel's message box, and the entry goes
			a.grid.focusPending = true
			a.showError(err)
			return false
		}
		if err != nil {
			a.errMsg = err.Error()
			return false
		}
		a.changed(res) // as in Excel, an entry ends copy mode
	}
	ed.active = false
	a.grid.focusPending = true
	if ed.sheet == a.active && (dr != 0 || dc != 0) {
		a.move(a.state(), dr, dc, false, false)
	}
	return true
}

// cancelEdit leaves the cell as it was (Esc).
func (a *App) cancelEdit() {
	a.ed.active = false
	a.errMsg = ""
	a.grid.focusPending = true
}

// editKeys handles the keys that end or steer editing, before the editors
// see them.
func (a *App) editKeys(gtx layout.Context) {
	ed := &a.ed
	if ed.focus != nil {
		requestFocus(gtx, ed.focus)
		ed.focus = nil
	}
	// A click in the formula bar starts editing there; while editing, it
	// moves the caret between the cell and the bar.
	switch {
	case !ed.active && gtx.Focused(&ed.bar):
		a.beginEdit(ed.bar.Text(), false, true)
		ed.focus = nil
	case ed.active && gtx.Focused(&ed.bar) && !ed.inBar:
		ed.inBar, ed.enter = true, false
	case ed.active && gtx.Focused(&ed.cell) && ed.inBar:
		ed.inBar = false
	}
	if !ed.active {
		return
	}
	for _, tag := range []*widget.Editor{&ed.cell, &ed.bar} {
		filters := []event.Filter{
			key.Filter{Focus: tag, Name: key.NameReturn, Optional: key.ModShift | key.ModAlt | key.ModCtrl},
			key.Filter{Focus: tag, Name: key.NameEnter, Optional: key.ModShift | key.ModAlt | key.ModCtrl},
			key.Filter{Focus: tag, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: tag, Name: key.NameF2},
		}
		if ed.enter && tag == &ed.cell {
			for _, n := range []key.Name{key.NameUpArrow, key.NameDownArrow, key.NameLeftArrow, key.NameRightArrow} {
				filters = append(filters, key.Filter{Focus: tag, Name: n})
			}
		}
		for {
			ev, ok := gtx.Event(filters...)
			if !ok {
				break
			}
			e, ok := ev.(key.Event)
			if !ok || e.State != key.Press {
				continue
			}
			shift := e.Modifiers.Contain(key.ModShift)
			switch e.Name {
			case key.NameReturn, key.NameEnter:
				switch {
				case e.Modifiers.Contain(key.ModAlt):
					tag.Insert("\n") // a line break in the cell
				case e.Modifiers.Contain(key.ModCtrl):
					a.commitEdit(0, 0)
				case shift:
					a.commitEdit(-1, 0)
				default:
					a.commitEdit(1, 0)
				}
			case key.NameTab:
				if shift {
					a.commitEdit(0, -1)
				} else {
					a.commitEdit(0, 1)
				}
			case key.NameF2:
				ed.enter = !ed.enter
			case key.NameUpArrow:
				a.commitEdit(-1, 0)
			case key.NameDownArrow:
				a.commitEdit(1, 0)
			case key.NameLeftArrow:
				a.commitEdit(0, -1)
			case key.NameRightArrow:
				a.commitEdit(0, 1)
			}
			if !ed.active {
				return
			}
		}
	}
	// Let the editor with the caret take the typing now, so that the other
	// one shows it in this frame.
	focused := &ed.cell
	if ed.inBar {
		focused = &ed.bar
	}
	for {
		if _, ok := focused.Update(gtx); !ok {
			break
		}
	}
	a.syncEditors()
}

// syncEditors copies the text of the editor with the caret to the other.
func (a *App) syncEditors() {
	ed := &a.ed
	from, to := &ed.cell, &ed.bar
	if ed.inBar {
		from, to = to, from
	}
	if s := from.Text(); s != to.Text() {
		to.SetText(s)
		n := to.Len()
		to.SetCaret(n, n)
	}
}

// layoutCellEditor draws the entry over the cell being edited, growing to
// the right as Excel's in-cell editor does.
func (a *App) layoutCellEditor(gtx layout.Context, v *render.View) {
	ed := &a.ed
	if !ed.active || ed.sheet != a.active {
		return
	}
	a.syncEditors()
	rc, pane, ok := v.CellRect(ed.row, ed.col)
	if !ok {
		// Scrolled out of view: keep the editor (and its focus) alive,
		// invisibly.
		cs := clip.Rect{}.Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(1, 1))
		ed.cell.Layout(g, a.shaper, gui.UIFont, unit.Sp(11), op.CallOp{}, op.CallOp{})
		cs.Pop()
		return
	}
	cell := v.Sheet.Get(ed.row, ed.col)
	st := a.wb.Styles[0]
	if int(cell.Style) < len(a.wb.Styles) {
		st = a.wb.Styles[cell.Style]
	}
	fnt := font.Font{Typeface: gui.Typeface(st.Font.Family)}
	if st.Font.Bold {
		fnt.Weight = font.Bold
	}
	if st.Font.Italic {
		fnt.Style = font.Italic
	}
	size := unit.Sp(st.Font.SizePt * 96 / 72 * a.zoom)
	pad := max(1, gtx.Dp(unit.Dp(2*a.zoom)))

	// Size the box as Excel does. A wrapping cell, or one tall enough for
	// two lines, keeps its width and wraps the entry; any other grows over
	// whole columns to the right until the entry fits, and wraps at the
	// window's edge. Either way the box grows down to fit the lines.
	measure := func(maxW int, s string) layout.Dimensions {
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(max(maxW, 1), 1<<20)}
		m := op.Record(gtx.Ops)
		d := widget.Label{}.Layout(g, a.shaper, fnt, size, s, op.CallOp{})
		m.Stop()
		return d
	}
	text := ed.cell.Text() + "M" // room for the caret
	line := measure(1<<20, "M").Size.Y
	box := rc
	if !st.Wrap && rc.Dy() < 2*line {
		need := measure(1<<20, text)
		for col := ed.col + 1; box.Dx() < need.Size.X+2*pad && box.Max.X < pane.Max.X; col++ {
			next, _, ok := v.CellRect(ed.row, col)
			if !ok || col >= model.MaxCols {
				break
			}
			box.Max.X = next.Max.X
		}
	}
	box.Max.X = min(box.Max.X, gtx.Constraints.Max.X)
	need := measure(box.Dx()-2*pad, text)
	box.Max.Y = max(box.Max.Y, min(box.Min.Y+need.Size.Y+pad, gtx.Constraints.Max.Y))
	fill := st.Fill
	if fill.A == 0 {
		fill = colors.White
	}
	gui.FillRect(gtx.Ops, box, fill)
	gui.HLine(gtx.Ops, box.Min.X, box.Max.X, box.Max.Y-1, 1, gui.ChromeLine)
	gui.VLine(gtx.Ops, box.Max.X-1, box.Min.Y, box.Max.Y, 1, gui.ChromeLine)
	// The active cell keeps its outline, as in Excel.
	t := max(2, gtx.Dp(2))
	o := image.Rect(rc.Min.X-t/2-1, rc.Min.Y-t/2-1, rc.Max.X+t/2, rc.Max.Y+t/2)
	gui.HLine(gtx.Ops, o.Min.X, o.Max.X, o.Min.Y, t, accent)
	gui.HLine(gtx.Ops, o.Min.X, o.Max.X, o.Max.Y-t, t, accent)
	gui.VLine(gtx.Ops, o.Min.X, o.Min.Y, o.Max.Y, t, accent)
	if box.Max.X <= rc.Max.X {
		gui.VLine(gtx.Ops, o.Max.X-t, o.Min.Y, o.Max.Y, t, accent)
	}

	inner := image.Rect(box.Min.X+pad, box.Min.Y, box.Max.X-pad, box.Max.Y-1)
	cs := clip.Rect(inner).Push(gtx.Ops)
	g := gtx
	g.Constraints = layout.Exact(inner.Size())
	// Single-line entries sit at the bottom of the cell, like its text.
	y := inner.Min.Y
	if need.Size.Y < inner.Dy() {
		y = inner.Max.Y - need.Size.Y - pad/2
		g.Constraints = layout.Exact(image.Pt(inner.Dx(), need.Size.Y))
	}
	off := op.Offset(image.Pt(inner.Min.X, y)).Push(gtx.Ops)
	ed.cell.Layout(g, a.shaper, fnt, size, gui.Material(gtx.Ops, st.Font.Color), gui.Material(gtx.Ops, editSelection))
	off.Pop()
	cs.Pop()
}

// layoutBarEditor lays out the formula bar's editor in an area w×h. When
// not editing it shows the active cell's content and starts editing when
// clicked.
func (a *App) layoutBarEditor(gtx layout.Context, content string, w, h int) {
	ed := &a.ed
	if !ed.active {
		if ed.barText != content || ed.bar.Text() != content {
			ed.barText = content
			ed.bar.SetText(content)
		}
	} else {
		a.syncEditors()
	}
	ed.bar.SingleLine = false
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, h)}
	m := op.Record(gtx.Ops)
	d := ed.bar.Layout(g, a.th.Shaper, gui.UIFont, unit.Sp(13), gui.Material(gtx.Ops, chromeText), gui.Material(gtx.Ops, editSelection))
	call := m.Stop()
	cs := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
	off := op.Offset(image.Pt(0, max((h-d.Size.Y)/2, 0))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	cs.Pop()
}

// changed updates the window after an edit: the title, the status bar,
// cached counts.
func (a *App) changed(res calc.Result) {
	a.modified = true
	a.ar.rev++ // AutoRecover copies it again (recover.go)
	a.stats = statsCache{}
	for _, st := range a.states {
		st.extent = extent{}
	}
	a.bs.info = infoCache{}
	a.errMsg = ""
	a.notice = ""
	// As in Excel, any change ends cut or copy mode (paste keeps a copy).
	a.clip = nil
	if w := circularWarning(res); w != nil {
		a.notice = w[0]
	}
	if a.win != nil {
		a.win.Option(windowTitle(a.wb.Name))
	}
	a.refreshCircles() // validation.go
}
