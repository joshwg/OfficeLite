// mouse.go handles grid mouse features: dragging the selection's border, pointing at cells in a formula, and reference outlines.

package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Mouse features of the grid: dragging the selection's border to move
// or copy cells, pointing at cells while typing a formula, and the
// colored outlines of the references of the formula being edited.

// moveDrag is a drag of the selection's border.
type moveDrag struct {
	src        model.Range
	grab       render.Pos // the cell under the pointer when the drag began
	dRow, dCol int        // how far the cells would move
	done       bool       // the button went up (Windows sends Cancel first)
}

// borderBand is the selection's border area for moving it, in grid
// coordinates: the outline's rectangle and its thickness each side.
func (a *App) borderBand(gtx layout.Context, v *render.View) (box image.Rectangle, band int, ok bool) {
	r := v.Sel.Range
	if a.ed.active || v.Sel.Multiple() || r == (model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}) || gtx.Ops == nil {
		return box, 0, false
	}
	box, ok = v.RangeRect(r)
	return box, max(gtx.Dp(3), 2), ok
}

// onBorder reports whether pos is on the selection's border, but not on
// the fill handle.
func (a *App) onBorder(gtx layout.Context, v *render.View, pos image.Point) bool {
	box, band, ok := a.borderBand(gtx, v)
	if !ok || !pos.In(box.Inset(-band)) || pos.In(box.Inset(band)) {
		return false
	}
	if hr, ok := a.fillHandleRect(gtx, v); ok && pos.In(hr) {
		return false
	}
	hw, hh := v.HeaderSize()
	return pos.X >= hw && pos.Y >= hh
}

// borderCursor shows the move cursor over the selection's border.
func (a *App) borderCursor(gtx layout.Context, v *render.View) {
	box, band, ok := a.borderBand(gtx, v)
	if !ok {
		return
	}
	o := box.Inset(-band)
	for _, r := range []image.Rectangle{
		image.Rect(o.Min.X, o.Min.Y, o.Max.X, box.Min.Y+band),
		image.Rect(o.Min.X, box.Max.Y-band, o.Max.X, o.Max.Y),
		image.Rect(o.Min.X, o.Min.Y, box.Min.X+band, o.Max.Y),
		image.Rect(box.Max.X-band, o.Min.Y, o.Max.X, o.Max.Y),
	} {
		area := clip.Rect(r).Push(gtx.Ops)
		pointer.CursorAllScroll.Add(gtx.Ops)
		area.Pop()
	}
}

// movePress starts dragging the selection when the press is on its
// border, reporting whether it did.
func (a *App) movePress(v *render.View, pos f32.Point) bool {
	if !a.onBorder(a.gtx, v, pos.Round()) {
		return false
	}
	hit := v.HitTest(pos.X, pos.Y)
	src := v.Sel.Range
	// The grabbed cell, kept inside the selection: the border band
	// reaches a little into the cells around it.
	grab := render.Pos{Row: min(max(hit.Row, src.R1), src.R2), Col: min(max(hit.Col, src.C1), src.C2)}
	a.grid.move = &moveDrag{src: src, grab: grab}
	return true
}

// moveTo follows the pointer while the selection is dragged.
func (a *App) moveTo(v *render.View, pos f32.Point) {
	m := a.grid.move
	hit := v.HitTest(pos.X, pos.Y)
	m.dRow, m.dCol = hit.Row-m.grab.Row, hit.Col-m.grab.Col
	if wholeCols(m.src) {
		m.dRow = 0
	}
	if wholeRows(m.src) {
		m.dCol = 0
	}
	m.dRow = min(max(m.dRow, -m.src.R1), model.MaxRows-1-m.src.R2)
	m.dCol = min(max(m.dCol, -m.src.C1), model.MaxCols-1-m.src.C2)
	v.EnsureVisible(min(max(hit.Row, 0), model.MaxRows-1), min(max(hit.Col, 0), model.MaxCols-1))
	if dest, ok := edit.MoveTarget(m.src, m.dRow, m.dCol); ok && (m.dRow != 0 || m.dCol != 0) {
		a.notice = "Drag to " + rangeName(dest) + "; hold Ctrl to copy"
	} else {
		a.notice = ""
	}
}

// moveRelease moves (copy: copies) the dragged cells when the button
// goes up, asking first when that replaces data.
func (a *App) moveRelease(copy bool) {
	m := a.grid.move
	a.grid.move = nil
	a.notice = ""
	if m.dRow == 0 && m.dCol == 0 || !a.readyToChange() {
		return
	}
	dest := model.Range{R1: m.src.R1 + m.dRow, C1: m.src.C1 + m.dCol, R2: m.src.R2 + m.dRow, C2: m.src.C2 + m.dCol}
	if !copy && a.pivotProtected(m.src) || a.pivotProtected(dest) {
		return
	}
	run := func() {
		dest, res, err := a.book.MoveCells(a.active, m.src, m.dRow, m.dCol, copy)
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.clip = nil
		a.changed(res)
		a.selectRange(a.active, dest)
	}
	if a.book.MoveOverwrites(a.active, m.src, m.dRow, m.dCol, copy) {
		a.dialog.Show(appName, "There's already data here. Do you want to replace it?", "OK", "Cancel")
		a.answer = func(i int) {
			if i == 0 {
				run()
			}
		}
		return
	}
	run()
}

// drawMoveDrag outlines where dragged cells would go.
func (a *App) drawMoveDrag(gtx layout.Context, v *render.View) {
	m := a.grid.move
	if m == nil || m.dRow == 0 && m.dCol == 0 {
		return
	}
	dest, ok := edit.MoveTarget(m.src, m.dRow, m.dCol)
	if !ok {
		return
	}
	box, ok := v.RangeRect(dest)
	if !ok {
		return
	}
	t := max(gtx.Dp(2), 2)
	on, off := gtx.Dp(4), gtx.Dp(2)
	c := colors.RGB(0x7F7F7F)
	gui.DashedLine(gtx.Ops, box.Min.X-1, box.Min.Y-1, box.Max.X, box.Min.Y-1, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X-1, box.Max.Y-t+1, box.Max.X, box.Max.Y-t+1, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X-1, box.Min.Y-1, box.Min.X-1, box.Max.Y, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Max.X-t+1, box.Min.Y-1, box.Max.X-t+1, box.Max.Y, t, on, off, c)
}

// Pointing: while a formula is typed, a click on a cell puts its
// reference into the entry instead of ending it, as in Excel.

// refPoint is the reference last put into the entry by pointing, which
// the next click (or a drag, or Shift+click) replaces.
type refPoint struct {
	active     bool
	start, end int        // its rune offsets in the entry
	anchor     render.Hit // where the pointing started
	text       string     // the entry right after it went in
}

// caretEditor is the editor with the caret.
func (a *App) caretEditor() *widget.Editor {
	if a.ed.inBar {
		return &a.ed.bar
	}
	return &a.ed.cell
}

// refCanGo reports whether a reference can go at rune offset caret of a
// formula: after "=", an operator, "(" or ",".
func refCanGo(text string, caret int) bool {
	if !strings.HasPrefix(text, "=") {
		return false
	}
	rs := []rune(text)
	if caret > len(rs) {
		return false
	}
	i := caret - 1
	for i >= 0 && unicode.IsSpace(rs[i]) {
		i--
	}
	return i >= 0 && strings.ContainsRune("=+-*/^&(,:<>;{", rs[i])
}

// pointPress puts the reference of the clicked cell or header into the
// formula being edited; false when the entry can't take one there (the
// click then ends editing, as usual).
func (a *App) pointPress(st *sheetState, pos f32.Point, extend bool) bool {
	ed := &a.ed
	if ed.sheet != a.active {
		return false
	}
	v := &st.view
	hit := v.HitTest(pos.X, pos.Y)
	if hit.Area == render.AreaCorner || hit.Area == render.AreaNone {
		return false
	}
	e := a.caretEditor()
	text := e.Text()
	s, t := e.Selection()
	s, t = min(s, t), max(s, t)
	p := &ed.point
	again := p.active && p.text == text && s == t && t == p.end
	switch {
	case again:
		s, t = p.start, p.end
		if !extend {
			p.anchor = hit
		}
	case refCanGo(text, s):
		p.anchor = hit
	default:
		return false
	}
	a.putRef(e, s, t, hit)
	a.grid.pointing = true
	return true
}

// pointDrag stretches the pointed reference to a range.
func (a *App) pointDrag(st *sheetState, pos f32.Point) {
	p := &a.ed.point
	if !p.active {
		return
	}
	v := &st.view
	// Over a header, corner or outline gutter the hit has no row or column
	// of its own (it reads 0): keep the pointer on the cells.
	pos.X, pos.Y = max(pos.X, float32(v.HdrW())), max(pos.Y, float32(v.HdrH()))
	hit := v.HitTest(pos.X, pos.Y)
	a.putRef(a.caretEditor(), p.start, p.end, hit)
	v.EnsureVisible(min(max(hit.Row, 0), model.MaxRows-1), min(max(hit.Col, 0), model.MaxCols-1))
}

// putRef replaces runes s..t of e with the reference from the pointing
// anchor to hit.
func (a *App) putRef(e *widget.Editor, s, t int, hit render.Hit) {
	p := &a.ed.point
	ref := a.pointedRef(p.anchor, hit)
	e.SetCaret(s, t)
	e.Insert(ref)
	n := s + utf8.RuneCountInString(ref)
	e.SetCaret(n, n)
	p.active, p.start, p.end, p.text = true, s, n, e.Text()
	a.ed.enter = false
	a.syncEditors()
	a.ed.focus = e
}

// pointedRef is the reference text for pointing from anchor to hit: a
// cell or range (merged areas whole), or whole columns or rows when
// pointing at headers.
func (a *App) pointedRef(anchor, hit render.Hit) string {
	sh := a.state().view.Sheet
	switch anchor.Area {
	case render.AreaColHeader:
		c1, c2 := min(anchor.Col, hit.Col), max(anchor.Col, hit.Col)
		return model.ColName(c1) + ":" + model.ColName(c2)
	case render.AreaRowHeader:
		r1, r2 := min(anchor.Row, hit.Row), max(anchor.Row, hit.Row)
		return strconv.Itoa(r1+1) + ":" + strconv.Itoa(r2+1)
	}
	r := expandMerges(sh, rangeOf(render.Pos{Row: anchor.Row, Col: anchor.Col}, render.Pos{Row: hit.Row, Col: hit.Col}))
	if m, ok := sh.MergeAt(r.R1, r.C1); ok && m == r {
		return model.CellName(r.R1, r.C1) // a merged cell is named by its first cell
	}
	return rangeName(r)
}

// refColors are the colors of the references of the formula being
// edited, in Excel's order.
var refColors = []color.NRGBA{
	colors.RGB(0x4472C4), colors.RGB(0xE0301E), colors.RGB(0x7030A0), colors.RGB(0x00A050),
	colors.RGB(0xC55A11), colors.RGB(0x0097A7), colors.RGB(0xC00080), colors.RGB(0x806000),
}

// editRefs lists the references of the formula being edited with their
// colors: the same reference text always gets the same color.
func (a *App) editRefs() ([]formula.RefSpan, []color.NRGBA) {
	if !a.ed.active {
		return nil, nil
	}
	text := a.editingText()
	if !strings.HasPrefix(text, "=") {
		return nil, nil
	}
	spans := formula.RefSpans(text[1:])
	cols := make([]color.NRGBA, len(spans))
	seen := map[string]int{}
	for i, s := range spans {
		k := strings.ToUpper(strings.ReplaceAll(text[1+s.Start:1+s.End], "$", ""))
		n, ok := seen[k]
		if !ok {
			n = len(seen)
			seen[k] = n
		}
		cols[i] = refColors[n%len(refColors)]
	}
	return spans, cols
}

// drawEditRefs outlines the ranges the formula being edited refers to,
// each in its color, with Excel's small squares at the corners.
func (a *App) drawEditRefs(gtx layout.Context, v *render.View) {
	if !a.ed.active || a.ed.sheet != a.active {
		return
	}
	spans, cols := a.editRefs()
	t := max(gtx.Dp(1), 1)
	sq := max(gtx.Dp(5), 4)
	for i, s := range spans {
		if s.Sheet != "" && !strings.EqualFold(s.Sheet, v.Sheet.Name) {
			continue
		}
		box, ok := v.RangeRect(s.Range)
		if !ok {
			continue
		}
		c := cols[i]
		o := box.Inset(-1)
		for k := 0; k < 2; k++ { // two pixels wide at 100%, like Excel's
			gui.HLine(gtx.Ops, o.Min.X, o.Max.X, o.Min.Y, t, c)
			gui.HLine(gtx.Ops, o.Min.X, o.Max.X, o.Max.Y-t, t, c)
			gui.VLine(gtx.Ops, o.Min.X, o.Min.Y, o.Max.Y, t, c)
			gui.VLine(gtx.Ops, o.Max.X-t, o.Min.Y, o.Max.Y, t, c)
			o = o.Inset(t)
		}
		for _, p := range []image.Point{box.Min, {box.Max.X, box.Min.Y}, {box.Min.X, box.Max.Y}, box.Max} {
			gui.FillRect(gtx.Ops, image.Rect(p.X-sq/2, p.Y-sq/2, p.X+sq/2+1, p.Y+sq/2+1), c)
		}
	}
}
