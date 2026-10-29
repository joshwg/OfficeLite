package ui

import (
	"io"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"officelite/excel/model"
	"officelite/excel/render"
)

// gridInput is the grid's input state; its address is the event tag.
type gridInput struct {
	focusPending bool
	dragging     bool
	dragArea     render.Area
	wheelX       float32
	wheelY       float32
	wheelZoom    float32
	lastPress    time.Duration // time of the last press, for double-clicks
	lastHit      render.Hit
	resize       *resizeDrag   // a header divider being dragged
	lastDivider  time.Duration // time of the last press on a divider
	lastEdge     dividerID
	move         *moveDrag // the selection being dragged by its border (mouse.go)
	pointing     bool      // dragging out a reference for a formula (mouse.go)
}

// doubleClick is the longest time between the presses of a double-click.
const doubleClick = 500 * time.Millisecond

var navKeys = []key.Name{
	key.NameUpArrow, key.NameDownArrow, key.NameLeftArrow, key.NameRightArrow,
	key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd,
	key.NameTab, key.NameReturn, key.NameEnter,
}

func (a *App) layoutGrid(gtx layout.Context) layout.Dimensions {
	st := a.state()
	tag := &a.grid
	if a.grid.focusPending {
		a.grid.focusPending = false
		requestFocus(gtx, tag)
	}
	a.gridPointer(gtx, st)
	a.gridKeys(gtx, st)

	size := gtx.Constraints.Max
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	switch r := a.grid.resize; {
	case r != nil && r.cols:
		pointer.CursorColResize.Add(gtx.Ops)
	case r != nil:
		pointer.CursorRowResize.Add(gtx.Ops)
	case a.grid.move != nil:
		pointer.CursorAllScroll.Add(gtx.Ops)
	case a.ch.drag != nil && a.ch.drag.moved:
		a.dragCursor().Add(gtx.Ops) // chart.go
	default:
		pointer.CursorCrosshair.Add(gtx.Ops)
		a.dividerCursors(gtx, &st.view)
		a.filterCursors(gtx, &st.view)
		a.pivotCursors(gtx, &st.view)
		a.outlineCursors(gtx, &st.view)
		a.chartCursors(gtx, &st.view)
	}
	area.Pop()

	cs := clip.Rect{Max: size}.Push(gtx.Ops)
	defer cs.Pop()
	d := st.view.Draw(gtx)
	a.drawEditRefs(gtx, &st.view) // the entry's references, colored (mouse.go)
	a.layoutCellEditor(gtx, &st.view)
	if a.grid.resize == nil && a.grid.move == nil {
		a.borderCursor(gtx, &st.view)
	}
	a.drawAnnots(gtx, &st.view)     // notes, links, fill drag (notes.go)
	a.drawValidation(gtx, &st.view) // list button, input message, circles (validation.go)
	a.drawMoveDrag(gtx, &st.view)
	a.drawChartDrag(gtx, &st.view)   // chart.go
	a.layoutTitleEdit(gtx, &st.view) // a chart title edited in place
	a.layoutGridMenu(gtx)
	a.layoutFilterMenu(gtx) // an AutoFilter drop-down (filter.go)
	a.layoutPivotMenu(gtx)  // a PivotTable drop-down (pivotmenu.go)
	a.layoutPivotSubmenu(gtx)
	a.layoutValidList(gtx) // a validation list (validation.go)
	return d
}

func (a *App) gridPointer(gtx layout.Context, st *sheetState) {
	v := &st.view
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &a.grid,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll | pointer.Move,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if e.Buttons.Contain(pointer.ButtonSecondary) {
				if c := v.ChartAt(e.Position.X, e.Position.Y); c != nil || a.onChartSheet() {
					a.chartContextMenu(v, c, e.Position) // chartribbon.go
					continue
				}
				a.gridContextMenu(st, e.Position) // ctxmenu.go
				continue
			}
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			// While a formula is typed, a click can point at a cell
			// (mouse.go); otherwise a click elsewhere ends editing
			// (unless the entry is wrong).
			if a.ed.active && a.pointPress(st, e.Position, e.Modifiers.Contain(key.ModShift)) {
				continue
			}
			if a.ed.active && !a.commitEdit(0, 0) {
				continue
			}
			if a.rename.active {
				a.finishRename(true)
			}
			requestFocus(gtx, &a.grid)
			if a.chartPress(v, e) {
				a.grid.lastPress = 0
				continue // a chart, over the cells (chart.go)
			}
			if a.beginResize(st, e.Position) {
				// A double-click on a divider fits the column (row) to its
				// contents, as in Excel.
				d := a.grid.resize
				id := dividerID{d.cols, d.edge}
				if a.grid.lastDivider != 0 && e.Time-a.grid.lastDivider < doubleClick && a.grid.lastEdge == id {
					a.endResize(st, false)
					a.autoFit(gtx, st, d)
					a.grid.lastDivider = 0
				} else {
					a.grid.lastDivider, a.grid.lastEdge = e.Time, id
				}
				a.grid.lastPress = 0
				continue
			}
			if a.outlinePress(v, e.Position) {
				a.grid.lastPress = 0
				continue // an outline button (outline.go)
			}
			if a.validPress(v, e.Position) {
				a.grid.lastPress = 0
				continue // a validation list's button (validation.go)
			}
			if a.filterPress(v, e.Position.X, e.Position.Y) {
				a.grid.lastPress = 0
				continue // an AutoFilter button (filter.go)
			}
			if a.pivotPress(v, e.Position.X, e.Position.Y) {
				a.grid.lastPress = 0
				continue // a PivotTable's button (pivotmenu.go)
			}
			if a.fillPress(v, e.Position.Round()) {
				continue // dragging the fill handle (fill.go)
			}
			if a.movePress(v, e.Position) {
				continue // dragging the selection by its border (mouse.go)
			}
			hit := v.HitTest(e.Position.X, e.Position.Y)
			ctrl, shift := e.Modifiers.Contain(key.ModShortcut), e.Modifiers.Contain(key.ModShift)
			if ctrl && a.followLinkAt(v, hit) {
				continue // Ctrl+click on a hyperlink (notes.go)
			}
			double := e.Time-a.grid.lastPress < doubleClick && hit == a.grid.lastHit
			a.grid.lastPress, a.grid.lastHit = e.Time, hit
			if double && hit.Area == render.AreaCell && !e.Modifiers.Contain(key.ModShift) {
				a.grid.lastPress = 0
				a.pointAt(st, hit, false)
				if a.pivotDouble(hit.Row, hit.Col) {
					continue // Show Details on a report's value (pivot.go)
				}
				a.beginEdit(editText(v.Sheet, v.Sel.Active), false, false)
				continue
			}
			a.grid.dragging = true
			a.grid.dragArea = hit.Area
			// Ctrl+click (or drag) adds a range to the selection.
			extra := v.Sel.Extra
			if ctrl && !shift && hit.Area != render.AreaCorner {
				extra = append(extra[:len(extra):len(extra)], v.Sel.Range)
			}
			a.pointAt(st, hit, shift)
			if ctrl && !shift && hit.Area != render.AreaCorner {
				v.Sel.Extra = extra
			}
		case pointer.Drag:
			if a.ch.drag != nil {
				a.chartDragTo(v, e.Position, e.Modifiers) // chart.go
				continue
			}
			if a.grid.move != nil {
				a.moveTo(v, e.Position)
				continue
			}
			if a.grid.pointing {
				a.pointDrag(st, e.Position)
				continue
			}
			if a.ins.fill.active {
				a.fillMove(v, e.Position.Round())
				continue
			}
			if a.grid.resize != nil {
				a.dragResize(gtx, st, e.Position)
				continue
			}
			if a.grid.dragging {
				hit := v.HitTest(e.Position.X, e.Position.Y)
				hit.Area = a.grid.dragArea
				a.pointAt(st, hit, true)
				v.EnsureVisible(st.selEnd.Row, st.selEnd.Col)
			}
		case pointer.Release, pointer.Cancel:
			if d := a.ch.drag; d != nil {
				// As for a selection dragged by its border: a Cancel
				// just before the Release (Windows) doesn't end it.
				if e.Kind == pointer.Release {
					a.chartRelease()
				} else {
					d.done = true
				}
			}
			a.grid.dragging = false
			a.grid.pointing = false
			if m := a.grid.move; m != nil {
				// Windows sends a Cancel as the button goes up, just
				// before the Release; a Cancel alone (the drag was
				// taken away) ends the drag at the next move.
				if e.Kind == pointer.Release {
					a.moveRelease(e.Modifiers.Contain(key.ModShortcut))
				} else {
					m.done = true
				}
			}
			if a.ins.fill.active {
				a.fillRelease(e.Kind == pointer.Release)
			}
			if a.grid.resize != nil {
				a.endResize(st, e.Kind == pointer.Release)
			}
		case pointer.Scroll:
			a.wheel(gtx, st, e)
		case pointer.Move:
			if m := a.grid.move; m != nil && m.done {
				a.grid.move = nil // cancelled
				a.notice = ""
			}
			if d := a.ch.drag; d != nil && (d.done || !e.Buttons.Contain(pointer.ButtonPrimary)) {
				a.ch.drag = nil
			}
			a.gridHover(v, e.Position.Round()) // notes show on hover (notes.go)
		}
	}
}

// pointAt selects (or extends the selection to) whatever was clicked.
func (a *App) pointAt(st *sheetState, hit render.Hit, extend bool) {
	v := &st.view
	if !extend {
		v.Sel.Extra = nil // a plain click selects just one range
	}
	switch hit.Area {
	case render.AreaCorner:
		v.Sel.Range = model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}
		v.Sel.Extra = nil
		return
	case render.AreaColHeader:
		top := max(v.TopRow, 0)
		if v.Sheet.FreezeRows > 0 {
			top = 0
		}
		if !extend {
			v.Sel.Active = render.Pos{Row: top, Col: hit.Col}
		}
		st.selEnd = render.Pos{Row: model.MaxRows - 1, Col: hit.Col}
		v.Sel.Range = model.Range{R1: 0, C1: min(v.Sel.Active.Col, hit.Col), R2: model.MaxRows - 1, C2: max(v.Sel.Active.Col, hit.Col)}
		return
	case render.AreaRowHeader:
		left := max(v.LeftCol, 0)
		if v.Sheet.FreezeCols > 0 {
			left = 0
		}
		if !extend {
			v.Sel.Active = render.Pos{Row: hit.Row, Col: left}
		}
		st.selEnd = render.Pos{Row: hit.Row, Col: model.MaxCols - 1}
		v.Sel.Range = model.Range{R1: min(v.Sel.Active.Row, hit.Row), C1: 0, R2: max(v.Sel.Active.Row, hit.Row), C2: model.MaxCols - 1}
		return
	case render.AreaCell:
		p := render.Pos{Row: hit.Row, Col: hit.Col}
		if extend {
			st.selEnd = p
		} else {
			v.Sel.Active, st.selEnd = p, p
		}
		v.Sel.Range = expandMerges(v.Sheet, rangeOf(v.Sel.Active, st.selEnd))
	}
}

func rangeOf(a, b render.Pos) model.Range {
	return model.Range{R1: min(a.Row, b.Row), C1: min(a.Col, b.Col), R2: max(a.Row, b.Row), C2: max(a.Col, b.Col)}
}

// expandMerges grows r until no merged area is partly inside it, as Excel
// does when selecting across merged cells.
func expandMerges(sh *model.Sheet, r model.Range) model.Range {
	for changed := true; changed; {
		changed = false
		for _, m := range sh.Merges {
			if m.R1 > r.R2 || m.R2 < r.R1 || m.C1 > r.C2 || m.C2 < r.C1 {
				continue
			}
			n := model.Range{R1: min(r.R1, m.R1), C1: min(r.C1, m.C1), R2: max(r.R2, m.R2), C2: max(r.C2, m.C2)}
			if n != r {
				r, changed = n, true
			}
		}
	}
	return r
}

func (a *App) wheel(gtx layout.Context, st *sheetState, e pointer.Event) {
	v := &st.view
	g := &a.grid
	if e.Modifiers.Contain(key.ModShortcut) {
		g.wheelZoom += e.Scroll.Y
		step := float32(gtx.Dp(40))
		for g.wheelZoom >= step {
			g.wheelZoom -= step
			a.setZoom(a.zoom - 0.1)
		}
		for g.wheelZoom <= -step {
			g.wheelZoom += step
			a.setZoom(a.zoom + 0.1)
		}
		return
	}
	dx, dy := e.Scroll.X, e.Scroll.Y
	if e.Modifiers.Contain(key.ModShift) {
		dx, dy = dy, dx
	}
	rowStep := float32(gtx.Dp(20)) * a.zoom
	colStep := float32(gtx.Dp(64)) * a.zoom
	g.wheelY += dy
	g.wheelX += dx
	n := int(g.wheelY / rowStep)
	g.wheelY -= float32(n) * rowStep
	v.TopRow = stepVisible(v.Geo.Rows, v.TopRow, n, v.Sheet.FreezeRows)
	n = int(g.wheelX / colStep)
	g.wheelX -= float32(n) * colStep
	v.LeftCol = stepVisible(v.Geo.Cols, v.LeftCol, n, v.Sheet.FreezeCols)
}

// stepVisible moves i by n entries along ax, skipping hidden ones and never
// going below lo.
func stepVisible(ax *render.Axis, i, n, lo int) int {
	for ; n > 0; n-- {
		j := i + 1
		for j < ax.Len()-1 && ax.Hidden(j) {
			j++
		}
		if j >= ax.Len() {
			break
		}
		i = j
	}
	for ; n < 0; n++ {
		j := i - 1
		for j > lo && ax.Hidden(j) {
			j--
		}
		if j < lo {
			break
		}
		i = j
	}
	return i
}

func (a *App) gridKeys(gtx layout.Context, st *sheetState) {
	tag := &a.grid
	filters := []event.Filter{
		key.FocusFilter{Target: tag},
		transfer.TargetFilter{Target: tag, Type: "application/text"},
		key.Filter{Focus: tag, Name: "C", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "X", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "V", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "Z", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "Y", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "A", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "+", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Focus: tag, Name: "=", Required: key.ModShortcut | key.ModShift},
		key.Filter{Focus: tag, Name: "-", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: key.NameF2},
		key.Filter{Focus: tag, Name: key.NameDeleteForward},
		key.Filter{Focus: tag, Name: key.NameDeleteBackward},
	}
	for _, n := range navKeys {
		filters = append(filters, key.Filter{Focus: tag, Name: n, Optional: key.ModShift | key.ModShortcut})
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.EditEvent:
			if !a.ed.active && (a.onChartSheet() || st.view.SelChart != nil) {
				if st.view.SelChart != nil && st.view.SelElem.Kind == render.ElemTitle && e.Text != "" {
					a.startTitleEdit(e.Text, true) // typing replaces the selected title
				}
				continue // a chart takes no other typing
			}
			// Typing replaces the cell's content (Enter mode); characters
			// typed before the editor took the focus are added to it.
			if a.ed.active {
				a.ed.cell.Insert(e.Text)
			} else if e.Text != "" {
				a.beginEdit(e.Text, true, false)
			}
			continue
		case transfer.DataEvent:
			if a.onChartSheet() {
				continue
			}
			if r := e.Open(); r != nil {
				data, _ := io.ReadAll(io.LimitReader(r, 64<<20))
				r.Close()
				a.paste(string(data))
			}
			continue
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		shift := e.Modifiers.Contain(key.ModShift)
		ctrl := e.Modifiers.Contain(key.ModShortcut)
		v := &st.view
		pageKey := ctrl && (e.Name == key.NamePageUp || e.Name == key.NamePageDown)
		if !pageKey && (e.Name != "C" && e.Name != "X" && e.Name != "V" || !ctrl) && a.chartKey(e) {
			continue // arrows, Delete, F2 for a selected chart (chart.go)
		}
		if a.onChartSheet() && e.Name != "Z" && e.Name != "Y" && e.Name != "C" && !pageKey {
			continue // a chart sheet has no cells (chart.go)
		}
		if v.SelChart != nil && e.Name != "Z" && e.Name != "Y" && e.Name != "C" && e.Name != "X" && e.Name != "V" && !pageKey {
			v.SelChart, v.SelElem = nil, render.ChartElem{} // a key for the cells drops the chart selection
		}
		switch e.Name {
		case "C", "X":
			a.copy(gtx, e.Name == "X")
		case "V":
			a.requestPaste(gtx)
		case "Z":
			a.undo()
		case "Y":
			a.redo()
		case "+", "=":
			a.insertDelete(true)
		case "-":
			a.insertDelete(false)
		case key.NameF2:
			a.beginEdit(editText(v.Sheet, v.Sel.Active), false, false)
		case key.NameDeleteForward:
			a.clearSelection()
		case key.NameDeleteBackward:
			a.beginEdit("", true, false)
		case "A":
			selectAll(v)
		case key.NameUpArrow:
			a.move(st, -1, 0, shift, ctrl)
		case key.NameDownArrow:
			a.move(st, 1, 0, shift, ctrl)
		case key.NameLeftArrow:
			a.move(st, 0, -1, shift, ctrl)
		case key.NameRightArrow:
			a.move(st, 0, 1, shift, ctrl)
		case key.NameTab:
			a.move(st, 0, map[bool]int{false: 1, true: -1}[shift], false, false)
		case key.NameReturn, key.NameEnter:
			a.move(st, map[bool]int{false: 1, true: -1}[shift], 0, false, false)
		case key.NamePageUp, key.NamePageDown:
			dir := 1
			if e.Name == key.NamePageUp {
				dir = -1
			}
			if ctrl {
				a.switchSheet(dir)
				continue
			}
			n := v.VisibleRows()
			v.TopRow = stepVisible(v.Geo.Rows, v.TopRow, dir*n, v.Sheet.FreezeRows)
			a.move(st, dir*n, 0, shift, false)
		case key.NameHome:
			to := render.Pos{Row: v.Sel.Active.Row, Col: v.Sheet.FreezeCols}
			if ctrl {
				to = render.Pos{Row: v.Sheet.FreezeRows, Col: v.Sheet.FreezeCols}
			}
			a.goTo(st, to, shift)
		case key.NameEnd:
			if ctrl {
				a.goTo(st, render.Pos{Row: max(v.Sheet.UsedRows()-1, 0), Col: max(v.Sheet.UsedCols()-1, 0)}, shift)
			}
		}
	}
}

// move steps the active cell (or, with extend, the selection's far corner).
// jump moves to the edge of the current block of data, like Ctrl+arrow.
func (a *App) move(st *sheetState, dr, dc int, extend, jump bool) {
	v := &st.view
	from := v.Sel.Active
	if extend {
		from = st.selEnd
	}
	// Leave a merged cell from its far edge.
	if m, ok := v.Sheet.MergeAt(from.Row, from.Col); ok && !extend {
		switch {
		case dr > 0:
			from.Row = m.R2
		case dr < 0:
			from.Row = m.R1
		case dc > 0:
			from.Col = m.C2
		case dc < 0:
			from.Col = m.C1
		}
	}
	var to render.Pos
	if jump {
		to = jumpTarget(v.Sheet, from, dr, dc)
	} else {
		to = render.Pos{
			Row: stepVisible(v.Geo.Rows, from.Row, dr, 0),
			Col: stepVisible(v.Geo.Cols, from.Col, dc, 0),
		}
	}
	a.goTo(st, to, extend)
}

func (a *App) goTo(st *sheetState, to render.Pos, extend bool) {
	v := &st.view
	to.Row = min(max(to.Row, 0), model.MaxRows-1)
	to.Col = min(max(to.Col, 0), model.MaxCols-1)
	if extend {
		st.selEnd = to
	} else {
		v.Sel.Extra = nil
		if m, ok := v.Sheet.MergeAt(to.Row, to.Col); ok {
			to = render.Pos{Row: m.R1, Col: m.C1}
		}
		v.Sel.Active, st.selEnd = to, to
	}
	v.Sel.Range = expandMerges(v.Sheet, rangeOf(v.Sel.Active, st.selEnd))
	v.EnsureVisible(to.Row, to.Col)
}

// jumpTarget implements Ctrl+arrow: from a filled cell with a filled
// neighbor, go to the last filled cell of the block; otherwise go to the next
// filled cell, or the sheet edge if there is none.
func jumpTarget(sh *model.Sheet, p render.Pos, dr, dc int) render.Pos {
	filled := func(q render.Pos) bool { return sh.Get(q.Row, q.Col).Text != "" }
	in := func(q render.Pos) bool {
		return q.Row >= 0 && q.Col >= 0 && q.Row < model.MaxRows && q.Col < model.MaxCols
	}
	next := func(q render.Pos) render.Pos { return render.Pos{Row: q.Row + dr, Col: q.Col + dc} }
	// Past the used area every cell is empty: go straight to the edge.
	limitR, limitC := sh.UsedRows(), sh.UsedCols()
	edge := func(q render.Pos) render.Pos {
		switch {
		case dr > 0:
			q.Row = model.MaxRows - 1
		case dr < 0:
			q.Row = 0
		case dc > 0:
			q.Col = model.MaxCols - 1
		case dc < 0:
			q.Col = 0
		}
		return q
	}
	n := next(p)
	if !in(n) {
		return p
	}
	if filled(p) && filled(n) {
		for in(next(n)) && filled(next(n)) {
			n = next(n)
		}
		return n
	}
	for in(n) && !filled(n) {
		if (dr > 0 && n.Row >= limitR) || (dc > 0 && n.Col >= limitC) {
			return edge(p)
		}
		n = next(n)
	}
	if !in(n) {
		return edge(p)
	}
	return n
}

// selectAll selects the whole sheet.
func selectAll(v *render.View) {
	v.Sel.Extra = nil
	v.Sel.Range = model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}
}
