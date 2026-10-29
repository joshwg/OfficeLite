package ui

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Mouse features of the page view, as in Word: the right-click menu,
// selecting lines from the left margin, dragging selected text to move
// or copy it, and dragging table borders to resize columns and rows.

var dropCaretColor = colors.RGB(0x404040)

// mouseState is the page view's mouse interaction in progress.
type mouseState struct {
	hover   f32.Point // last pointer position over the view
	hoverIn bool
	mods    key.Modifiers // modifiers at the last pointer event

	menu  gui.ContextMenu
	items []gui.MenuItem // the context menu's items
	acts  []func()       // and their commands

	// Left-margin selection: the lines (or paragraphs) selected at the
	// press, which a drag extends from.
	margin       int // 0 none, 1 lines, 2 paragraphs
	marginA      model.Pos
	marginB      model.Pos
	marginActive bool

	// Moving text: a press inside the selection (pending) becomes a move
	// once the pointer moves.
	pending, moving bool
	pressAt         f32.Point
	pressPos        model.Pos
	drop            model.Pos
	dropOK          bool

	// Table border drag.
	resizing  bool
	border    wl.TableBorder
	page      int
	rowTop    float64
	borderNow float64 // the dragged border's x or y, page points
	start     f32.Point
}

// track remembers the pointer position and modifiers for the cursor.
func (m *mouseState) track(e pointer.Event) {
	m.mods = e.Modifiers
	switch e.Kind {
	case pointer.Leave:
		m.hoverIn = false
	case pointer.Cancel:
	default:
		m.hover, m.hoverIn = e.Position, true
	}
}

// busy reports whether a margin, move or border drag is in progress.
func (m *mouseState) busy() bool { return m.marginActive || m.pending || m.moving || m.resizing }

// docCursor sets the page view's cursor: the I-beam over text, an arrow
// in the left margin and while moving text, resize arrows on table
// borders, a hand over a link with Ctrl held, and the Format Painter's.
func (d *window) docCursor(gtx layout.Context) {
	m := &d.v.m
	c := pointer.CursorText
	switch {
	case m.moving || m.marginActive:
		c = pointer.CursorDefault
	case m.resizing:
		c = resizeCursor(m.border)
	case d.painter.armed:
		c = pointer.CursorCrosshair
	case !m.hoverIn || m.menu.IsOpen():
	default:
		if b, ok := d.borderAt(m.hover); ok {
			c = resizeCursor(b)
		} else if _, _, ok := d.marginAt(m.hover); ok {
			c = pointer.CursorDefault
		} else if m.mods.Contain(key.ModShortcut) && d.overLink(m.hover) {
			c = pointer.CursorPointer
		}
	}
	c.Add(gtx.Ops)
}

func resizeCursor(b wl.TableBorder) pointer.Cursor {
	if b.Vertical {
		return pointer.CursorColResize
	}
	return pointer.CursorRowResize
}

func (d *window) overLink(p f32.Point) bool {
	_, ok := d.ed.LinkAt(d.viewPos(p))
	return ok && d.overText(p)
}

// Left-margin selection.

// marginAt reports whether a view position is in the left margin of a
// page's body (left of the text column and of any line there) and
// returns the page and page y.
func (d *window) marginAt(p f32.Point) (int, float64, bool) {
	if d.hf.on {
		return 0, 0, false
	}
	page, x, y, ok := d.pointerPage(p)
	if !ok {
		return 0, 0, false
	}
	pg := d.eng.Pages[page]
	if x < 0 || x >= d.eng.PageTextLeft(pg)-2 || y < pg.BodyTop || y > pg.BodyBottom {
		return 0, 0, false
	}
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		if x >= pl.X && y >= pl.Y && y < pl.Y+pl.Line.Height {
			return 0, 0, false // a table or paragraph reaching into the margin
		}
	}
	return page, y, true
}

// marginLine returns the range a margin selection covers at (page, y):
// the line there, with the paragraph mark when it is the paragraph's
// last, or (paras) the whole paragraph.
func (d *window) marginLine(page int, y float64, paras bool) (model.Pos, model.Pos, bool) {
	a, b, ok := d.eng.LineAtY(page, y)
	if !ok {
		return a, b, false
	}
	ed := d.ed
	if paras {
		a.Off, b.Off = 0, ed.Para(b.Para).Len()
	}
	if b.Off >= ed.Para(b.Para).Len() && b.Para+1 < ed.NumParas() {
		locs := ed.Locs()
		if locs[b.Para+1].Cell == locs[b.Para].Cell && locs[b.Para+1].Blocks == locs[b.Para].Blocks {
			b = model.Pos{Para: b.Para + 1}
		}
	}
	return a, b, true
}

// pressMargin handles a primary press in the left margin: a click
// selects the line (a table row), a double-click the paragraph, a
// triple-click or Ctrl+click the document; Shift extends by lines.
func (d *window) pressMargin(e pointer.Event, clicks int) bool {
	page, y, ok := d.marginAt(e.Position)
	if !ok {
		return false
	}
	m := &d.v.m
	ed := d.ed
	m.marginActive = true
	if clicks >= 3 || e.Modifiers.Contain(key.ModShortcut) {
		ed.SelectAll()
		m.margin = 0
		return true
	}
	a, b, ok := d.marginLine(page, y, clicks == 2)
	if !ok {
		return true
	}
	m.margin = clicks
	if e.Modifiers.Contain(key.ModShift) {
		// Extend the selection as it is: its whole range stays, whichever
		// side of it the line clicked is on.
		m.marginA, m.marginB = ed.Sel.Ordered()
		d.extendMargin(a, b)
		return true
	}
	if _, inTable := ed.CellAt(a); inTable && clicks == 1 {
		// Beside a table the whole row is selected.
		ed.SetCaret(a, false)
		ed.SelectRow()
		m.marginA, m.marginB = ed.Sel.Ordered()
		return true
	}
	m.marginA, m.marginB = a, b
	ed.Select(a, b)
	return true
}

// extendMargin selects from the margin selection's start to the range
// [a, b) the pointer is on, whole lines (or paragraphs) either way.
func (d *window) extendMargin(a, b model.Pos) {
	m := &d.v.m
	if a.Less(m.marginA) {
		d.ed.Select(m.marginB, a)
	} else {
		d.ed.Select(m.marginA, b)
	}
	d.ed.Sel = d.ed.SnapToCells(d.ed.Sel)
}

// dragMargin continues a margin selection; it reports true while one is
// in progress.
func (d *window) dragMargin(e pointer.Event) bool {
	m := &d.v.m
	if !m.marginActive {
		return false
	}
	if m.margin == 0 {
		return true
	}
	page, _, py, ok := d.pointerPage(e.Position)
	if !ok {
		return true
	}
	if a, b, ok := d.marginLine(page, py, m.margin == 2); ok {
		d.extendMargin(a, b)
	}
	return true
}

// releaseMargin ends a margin selection.
func (d *window) releaseMargin() bool {
	m := &d.v.m
	active := m.marginActive
	m.marginActive = false
	return active
}

// Moving text.

// pressInSelection starts a possible text move when a click lands in the
// selection; it reports true when it took the press.
func (d *window) pressInSelection(e pointer.Event, pos model.Pos) bool {
	ed := d.ed
	if ed.Sel.Empty() || d.painter.armed || !ed.CanMoveSelection() || !d.overText(e.Position) {
		return false
	}
	s, t := ed.Sel.Ordered()
	if pos.Less(s) || !pos.Less(t) {
		return false
	}
	m := &d.v.m
	m.pending, m.moving, m.pressAt, m.pressPos = true, false, e.Position, pos
	return true
}

// dragMove continues a text move; it reports true while one is pending
// or in progress.
func (d *window) dragMove(e pointer.Event) bool {
	m := &d.v.m
	if !m.pending {
		return false
	}
	if !m.moving {
		dp := e.Position.Sub(m.pressAt)
		if dp.X*dp.X+dp.Y*dp.Y < 16 {
			return true
		}
		m.moving = true
	}
	m.drop = d.viewPos(e.Position)
	s, t := d.ed.Sel.Ordered()
	m.dropOK = m.drop.Less(s) || t.Less(m.drop)
	return true
}

// releaseMove ends a text move: the text moves (Ctrl copies) to the drop
// position; a click without a drag places the caret.
func (d *window) releaseMove(e pointer.Event) bool {
	m := &d.v.m
	if !m.pending {
		return false
	}
	moving := m.moving
	m.pending, m.moving = false, false
	if !moving {
		d.ed.SetCaret(m.pressPos, false)
		d.ins.anchor = m.pressPos
		return true
	}
	if m.dropOK && d.ed.MoveText(m.drop, e.Modifiers.Contain(key.ModShortcut)) {
		d.edited()
	}
	return true
}

// cancelMouse abandons a text move or border drag (Esc).
func (d *window) cancelMouse() bool {
	m := &d.v.m
	if !m.pending && !m.resizing {
		return false
	}
	m.pending, m.moving, m.resizing = false, false, false
	d.v.dragging = false
	return true
}

// drawDropCaret draws the dotted caret where moved text would go.
func (d *window) drawDropCaret(gtx layout.Context, px int) {
	m := &d.v.m
	if !m.moving || !m.dropOK {
		return
	}
	c, ok := d.ceng().CaretAt(m.drop)
	if !ok {
		return
	}
	v := &d.v
	pg := d.ceng().Pages[c.Page]
	x := px + int(math.Round((pg.X+c.X)*v.scale))
	y0 := int(math.Round((pg.Top + c.Y - v.scrollY) * v.scale))
	y1 := int(math.Round((pg.Top + c.Y + c.H - v.scrollY) * v.scale))
	gui.DashedLine(gtx.Ops, x, y0, x, y1, max(1, gtx.Dp(1)), 2, 2, dropCaretColor)
}

// Table borders.

// borderAt returns the table border under a view position.
func (d *window) borderAt(p f32.Point) (wl.TableBorder, bool) {
	if d.hf.on {
		return wl.TableBorder{}, false
	}
	page, x, y, ok := d.pointerPage(p)
	if !ok {
		return wl.TableBorder{}, false
	}
	return d.eng.TableBorderAt(page, x, y, 3/d.v.scale)
}

// pressBorder starts dragging a table border.
func (d *window) pressBorder(e pointer.Event) bool {
	b, ok := d.borderAt(e.Position)
	if !ok {
		return false
	}
	m := &d.v.m
	page, _, _, _ := d.pointerPage(e.Position)
	m.resizing, m.border, m.page, m.borderNow, m.start = true, b, page, b.At, e.Position
	if !b.Vertical {
		m.rowTop, _ = d.eng.RowTop(page, b.Table, b.Row)
	}
	return true
}

// dragBorder moves the dragged border's guide; it reports true while a
// border drag is in progress.
func (d *window) dragBorder(e pointer.Event) bool {
	m := &d.v.m
	if !m.resizing {
		return false
	}
	b := m.border
	if b.Vertical {
		lo, hi := model.ColumnBorderRange(b.Table, b.Grid)
		dx := float64(e.Position.X-m.start.X) / d.v.scale
		m.borderNow = b.At + min(max(dx, lo), hi)
	} else {
		dy := float64(e.Position.Y-m.start.Y) / d.v.scale
		m.borderNow = max(b.At+dy, m.rowTop+model.MinRowHeight)
	}
	return true
}

// releaseBorder applies a border drag.
func (d *window) releaseBorder() bool {
	m := &d.v.m
	if !m.resizing {
		return false
	}
	m.resizing = false
	b := m.border
	changed := false
	if b.Vertical {
		changed = d.ed.MoveColumnBorder(b.Table, b.Grid, m.borderNow-b.At)
	} else if math.Abs(m.borderNow-b.At) >= 0.25 {
		changed = d.ed.SetRowHeight(b.Table, b.Row, m.borderNow-m.rowTop)
	}
	if changed {
		d.edited()
	}
	return true
}

// drawBorderGuide draws the dotted line where a dragged border goes.
func (d *window) drawBorderGuide(gtx layout.Context, px int) {
	m := &d.v.m
	if !m.resizing || m.page >= len(d.eng.Pages) {
		return
	}
	v := &d.v
	pg := d.eng.Pages[m.page]
	vx := func(x float64) int { return px + int(math.Round((pg.X+x)*v.scale)) }
	vy := func(y float64) int { return int(math.Round((pg.Top + y - v.scrollY) * v.scale)) }
	b := m.border
	w := max(1, gtx.Dp(1))
	if b.Vertical {
		// Word's guide runs the height of the window.
		gui.DashedLine(gtx.Ops, vx(m.borderNow), 0, vx(m.borderNow), v.size.Y, w, 2, 2, dropCaretColor)
		return
	}
	gui.DashedLine(gtx.Ops, 0, vy(m.borderNow), v.size.X, vy(m.borderNow), w, 2, 2, dropCaretColor)
}

// Context menu.

// contextPress opens the context menu for a right-click: outside the
// selection the caret moves there first (a picture is selected).
func (d *window) contextPress(e pointer.Event) {
	ed := d.ed
	page, x, y, ok := d.pointerPage(e.Position)
	if pos, _, pic := d.ceng().PictureAt(page, x, y); ok && pic {
		ed.Select(pos, model.Pos{Para: pos.Para, Off: pos.Off + 1})
	} else if ok {
		pos := d.viewPos(e.Position)
		s, t := ed.Sel.Ordered()
		if ed.Sel.Empty() || pos.Less(s) || t.Less(pos) {
			ed.SetCaret(pos, false)
		}
	}
	d.v.hasGoal = false
	d.v.resetBlink()
	d.openContextMenu(image.Pt(int(e.Position.X), int(e.Position.Y)))
}

// openContextMenu shows the right-click menu at pos (page view pixels)
// with the commands that apply where the caret is, as Word's does.
func (d *window) openContextMenu(pos image.Point) {
	ed := d.ed
	m := &d.v.m
	m.items, m.acts = nil, nil
	add := func(it gui.MenuItem, f func()) {
		m.items = append(m.items, it)
		m.acts = append(m.acts, f)
	}
	sep := func() { add(gui.MenuItem{Separator: true}, nil) }
	sel := !ed.Sel.Empty()
	add(gui.MenuItem{Label: "Cut", Shortcut: "Ctrl+X", Disabled: !sel, Tip: "Remove the selection and put it on the Clipboard."},
		func() { d.copy(d.gtx, true) })
	add(gui.MenuItem{Label: "Copy", Shortcut: "Ctrl+C", Disabled: !sel, Tip: "Put a copy of the selection on the Clipboard."},
		func() { d.copy(d.gtx, false) })
	add(gui.MenuItem{Label: "Paste", Shortcut: "Ctrl+V", Tip: "Paste the Clipboard, keeping its formatting."},
		func() { d.paste(d.gtx) })
	add(gui.MenuItem{Label: "Paste Special…", Shortcut: "Ctrl+Alt+V", Tip: "Choose how to paste the Clipboard: formatted or plain text."},
		func() { d.homeExtra(homePasteSpecial) })
	add(gui.MenuItem{Label: "Keep Text Only", Tip: "Paste only the Clipboard's text, in the formatting where it goes."},
		func() { d.homeExtra(homePastePlain) })
	sep()
	if !d.hf.on {
		add(gui.MenuItem{Label: "Modify Style…", Tip: "Change the paragraph's style; every paragraph in it follows."},
			func() { d.dlg.openFor(d, ed.ParaStyleID(ed.Sel.Caret.Para)) })
		add(gui.MenuItem{Label: "Styles", Shortcut: "Ctrl+Alt+Shift+S", Checked: d.pane.open, Tip: "Show or hide the Styles pane."},
			func() { d.homeExtra(homeStylesPane) })
		sep()
	}
	if l, ok := ed.SelectionLink(); ok {
		add(gui.MenuItem{Label: "Edit Link…", Shortcut: "Ctrl+K", Tip: "Change the link's text or address."},
			func() { d.insertChosen(insLink) })
		add(gui.MenuItem{Label: "Open Link", Tip: "Open the link's address (Ctrl+click also opens it)."},
			func() { d.followLink(l) })
		add(gui.MenuItem{Label: "Remove Link", Tip: "Keep the text but remove the link."},
			func() { d.insertChosen(insRemoveLink) })
	} else {
		add(gui.MenuItem{Label: "Link…", Shortcut: "Ctrl+K", Tip: "Make the selection a link to a web page or e-mail address."},
			func() { d.insertChosen(insLink) })
	}
	if ed.InTable() && !d.hf.on {
		sep()
		for _, it := range []struct {
			item       int
			label, tip string
		}{
			{insRowAbove, "Insert Row Above", "Add a row above the current one."},
			{insRowBelow, "Insert Row Below", "Add a row below the current one."},
			{insColLeft, "Insert Column Left", "Add a column to the left of the current one."},
			{insColRight, "Insert Column Right", "Add a column to the right of the current one."},
			{insDelRow, "Delete Row", "Delete the selected rows."},
			{insDelCol, "Delete Column", "Delete the selected columns."},
			{insDelTable, "Delete Table", "Delete the whole table."},
			{insSelectTable, "Select Table", "Select the whole table."},
		} {
			add(gui.MenuItem{Label: it.label, Tip: it.tip}, func() { d.insertChosen(it.item) })
		}
	}
	m.menu.Open(pos, m.items)
}

// contextMenuLayout draws the open context menu over the page view and
// runs the chosen command.
func (d *window) contextMenuLayout(gtx layout.Context) {
	m := &d.v.m
	if i := m.menu.Layout(gtx, d.th); i >= 0 && i < len(m.acts) && m.acts[i] != nil {
		m.acts[i]()
		d.edited()
	}
}

// previewMouse sets up the wordshot preview of these features: the
// context menu (in the first table, if any), dragging the first table's
// first column border 30pt right, or dragging the first word to the end
// of its line.
func (d *window) previewMouse(what string) {
	d.relayout()
	ed := d.ed
	m := &d.v.m
	var tb *model.Table
	for _, b := range d.doc.Body {
		if t, ok := b.(*model.Table); ok {
			tb = t
			break
		}
	}
	caretPt := func(p model.Pos) image.Point {
		c, _ := d.eng.CaretAt(p)
		pg := d.eng.Pages[c.Page]
		return image.Pt(d.pageX()+int((pg.X+c.X)*d.v.scale), int((pg.Top+c.Y+c.H-d.v.scrollY)*d.v.scale))
	}
	switch what {
	case "context":
		if tb != nil {
			if a, _, ok := ed.CellBounds(tb.Rows[0].Cells[0]); ok {
				ed.SetCaret(a, false)
			}
		}
		d.v.ensureCaret = true
		d.scrollToCaret()
		d.openContextMenu(caretPt(ed.Sel.Caret).Add(image.Pt(20, 0)))
	case "colresize":
		for i, pg := range d.eng.Pages {
			for _, rb := range pg.TableRows {
				if rb.Table == tb && len(rb.Edges) > 1 {
					m.resizing, m.page = true, i
					m.border = wl.TableBorder{Table: tb, Vertical: true, Grid: rb.Edges[1].Grid, Row: -1, At: rb.Edges[1].X}
					m.borderNow = rb.Edges[1].X + 30
					d.v.scrollY = max(pg.Top+rb.Y0-72, 0)
					return
				}
			}
		}
	case "dragtext":
		a, b := ed.WordAt(model.Pos{})
		ed.Select(a, b)
		_, end, _ := d.eng.LineBounds(model.Pos{}, false)
		m.pending, m.moving, m.drop, m.dropOK = true, true, end, b.Less(end)
	}
}
