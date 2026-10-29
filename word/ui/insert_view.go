package ui

import (
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Page view interactions for inserted content: clicking a picture
// selects it and its handles resize it, Ctrl+click follows a link, a drag
// across table cells selects whole cells, Tab moves between cells and
// Ctrl+K opens Insert Link.

var resizeOutline = colors.RGB(0x404040)

// insertKeys handles the Insert shortcuts in the page view.
func (d *window) insertKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &d.v, Name: "K", Required: key.ModShortcut})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press && !d.modalOpen() {
			d.openLinkDialog()
		}
	}
}

// tableTab is Tab (Shift+Tab) in a table: the next (previous) cell. It
// reports false outside tables.
func (d *window) tableTab(back bool) bool {
	dir := 1
	if back {
		dir = -1
	}
	return d.ed.NextCell(dir)
}

// pointerPage converts a view pointer position to a page and page point.
func (d *window) pointerPage(p f32.Point) (int, float64, float64, bool) {
	v := &d.v
	if len(d.eng.Pages) == 0 || v.scale == 0 {
		return 0, 0, 0, false
	}
	x := (float64(p.X) - float64(d.pageX())) / v.scale
	y := v.scrollY + float64(p.Y)/v.scale
	page, py := d.eng.PageAt(y)
	return page, x - d.eng.Pages[page].X, py, true
}

// pressObject handles a primary-button press in the page view before
// the usual caret placement; it reports true when it took the press.
func (d *window) pressObject(e pointer.Event, clicks int) bool {
	s := &d.ins
	s.resizing = false
	page, x, y, ok := d.pointerPage(e.Position)
	if !ok {
		return false
	}
	// A handle of the selected picture starts a resize.
	if r := d.ed.SelectedPicture(); r != nil && clicks == 1 {
		a, _ := d.ed.Sel.Ordered()
		if pg, box, ok := d.eng.PictureRect(a); ok && pg == page {
			for i, h := range wl.PictureHandles(box) {
				grow := 3.0 / d.v.scale // easier to hit when zoomed out
				if x >= h.X0-grow && x <= h.X1+grow && y >= h.Y0-grow && y <= h.Y1+grow {
					s.resizing, s.handle, s.resPos, s.resBox, s.resNow, s.resPage, s.resStart = true, i, a, box, box, pg, e.Position
					return true
				}
			}
		}
	}
	shift := e.Modifiers.Contain(key.ModShift)
	if e.Modifiers.Contain(key.ModShortcut) && clicks == 1 && !shift {
		if l, ok := d.ed.LinkAt(d.viewPos(e.Position)); ok && d.overText(e.Position) {
			d.followLink(l)
			return true
		}
	}
	if pos, _, ok := d.eng.PictureAt(page, x, y); ok && !shift {
		d.ed.Select(pos, model.Pos{Para: pos.Para, Off: pos.Off + 1})
		s.anchor = pos
		return true
	}
	s.anchor = d.ed.Sel.Anchor
	if !shift {
		s.anchor = d.viewPos(e.Position)
	}
	return false
}

// overText reports whether a view position is on a line's text (not in
// the margin beside it), for following links.
func (d *window) overText(p f32.Point) bool {
	pos := d.viewPos(p)
	c, ok := d.eng.CaretAt(pos)
	if !ok {
		return false
	}
	page, x, y, ok := d.pointerPage(p)
	return ok && page == c.Page && math.Abs(x-c.X) < 24 && y >= c.Y-2 && y <= c.Y+c.H+2
}

// dragObject continues a picture resize; it reports true while one is
// in progress.
func (d *window) dragObject(e pointer.Event) bool {
	s := &d.ins
	if !s.resizing {
		return false
	}
	dx := float64(e.Position.X-s.resStart.X) / d.v.scale
	dy := float64(e.Position.Y-s.resStart.Y) / d.v.scale
	b := s.resBox
	w0, h0 := b.X1-b.X0, b.Y1-b.Y0
	w, h := w0, h0
	// Handles clockwise from the top-left; the opposite edge stays.
	switch s.handle {
	case 0:
		w, h = w0-dx, h0-dy
	case 1:
		h = h0 - dy
	case 2:
		w, h = w0+dx, h0-dy
	case 3:
		w = w0 + dx
	case 4:
		w, h = w0+dx, h0+dy
	case 5:
		h = h0 + dy
	case 6:
		w, h = w0-dx, h0+dy
	case 7:
		w = w0 - dx
	}
	if s.handle%2 == 0 && w0 > 0 && h0 > 0 {
		// Corners keep the aspect ratio.
		k := max(w/w0, h/h0)
		w, h = w0*k, h0*k
	}
	w, h = max(w, 4), max(h, 4)
	r := wl.Rect{X0: b.X0, Y0: b.Y1 - h, X1: b.X0 + w, Y1: b.Y1}
	switch s.handle {
	case 0, 6, 7:
		r.X0, r.X1 = b.X1-w, b.X1
	}
	s.resNow = r
	return true
}

// releaseObject ends a picture resize, applying the new size.
func (d *window) releaseObject() {
	s := &d.ins
	if !s.resizing {
		return
	}
	s.resizing = false
	w, h := s.resNow.X1-s.resNow.X0, s.resNow.Y1-s.resNow.Y0
	if math.Abs(w-(s.resBox.X1-s.resBox.X0)) < 0.5 && math.Abs(h-(s.resBox.Y1-s.resBox.Y0)) < 0.5 {
		return
	}
	d.ed.ResizePicture(s.resPos, math.Round(w*4)/4, math.Round(h*4)/4)
	d.ed.Select(s.resPos, model.Pos{Para: s.resPos.Para, Off: s.resPos.Off + 1})
	d.edited()
}

// dragTo extends a drag selection to pos; once it leaves the cell it
// started in, whole cells are selected.
func (d *window) dragTo(pos model.Pos) {
	d.ed.Select(d.ins.anchor, pos)
	d.ed.Sel = d.ed.SnapToCells(d.ed.Sel)
}

// drawInsertOverlay draws the outline of a picture being resized.
func (d *window) drawInsertOverlay(gtx layout.Context, px int) {
	s := &d.ins
	if !s.resizing || s.resPage >= len(d.eng.Pages) {
		return
	}
	v := &d.v
	pg := d.eng.Pages[s.resPage]
	r := s.resNow
	x0 := px + int(math.Round((pg.X+r.X0)*v.scale))
	x1 := px + int(math.Round((pg.X+r.X1)*v.scale))
	y0 := int(math.Round((pg.Top + r.Y0 - v.scrollY) * v.scale))
	y1 := int(math.Round((pg.Top + r.Y1 - v.scrollY) * v.scale))
	gui.DashedLine(gtx.Ops, x0, y0, x1, y0, 1, 4, 3, resizeOutline)
	gui.DashedLine(gtx.Ops, x0, y1, x1, y1, 1, 4, 3, resizeOutline)
	gui.DashedLine(gtx.Ops, x0, y0, x0, y1, 1, 4, 3, resizeOutline)
	gui.DashedLine(gtx.Ops, x1, y0, x1, y1, 1, 4, 3, resizeOutline)
}

// followLink opens a link's address with the system's handler (web
// browser, mail program); links to bookmarks move the caret there if
// the bookmark is known, otherwise the status bar says so.
func (d *window) followLink(l model.Link) {
	url := l.Span.URL
	if url == "" {
		d.notice = "Links to places in the document (#" + l.Span.Anchor + ") aren't supported yet."
		return
	}
	low := strings.ToLower(url)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") &&
		!strings.HasPrefix(low, "mailto:") && !strings.HasPrefix(low, "ftp://") {
		d.notice = "WordLite only opens web and e-mail links: " + url
		return
	}
	if err := gui.OpenURL(url); err != nil {
		d.errMsg = "Can't open " + url + ": " + err.Error()
		return
	}
	d.notice = "Opened " + url
}
