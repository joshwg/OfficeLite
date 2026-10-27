package ui

import (
	"image"
	"io"
	"math"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

var (
	viewBG     = colors.RGB(0xE3E3E3)
	pageShadow = colors.RGBA(0, 0, 0, 0x30)
	selColor   = colors.RGB(0xC5DBF6)
	caretColor = colors.RGB(0x000000)
)

const blinkPeriod = 530 * time.Millisecond

// docView is the page view's state; its address is the input tag.
type docView struct {
	zoom         float64
	scrollY      float64 // points from the top of the page view
	scrollX      int     // pixels, when the page is wider than the view
	focusPending bool
	focused      bool
	ensureCaret  bool
	goalX        float64 // remembered caret x for Up/Down, page points
	hasGoal      bool
	dragging     bool
	clicks       int
	lastClick    time.Time
	lastPos      f32.Point
	blinkStart   time.Time
	wheelZoom    float32
	size         image.Point
	scale        float64 // pixels per point
	skipSpace    bool    // Ctrl+Space also types a space; drop it

	vbar                           gui.Scrollbar
	zoomInBtn, zoomOutBtn, zoomBtn widget.Clickable
}

func (v *docView) resetBlink() { v.blinkStart = time.Now() }

// pageX is the left edge of the pages in view pixels.
func (d *window) pageX() int {
	v := &d.v
	pw := int(math.Round(d.eng.Pages[0].Width * v.scale))
	if pw+16 <= v.size.X {
		return (v.size.X - pw) / 2
	}
	return 8 - v.scrollX
}

func (d *window) layoutDoc(gtx layout.Context) layout.Dimensions {
	v := &d.v
	v.size = gtx.Constraints.Max
	v.scale = v.zoom * float64(gtx.Metric.PxPerDp) * 96 / 72
	tag := &d.v
	if v.focusPending && !d.dlg.open && !d.confirm.show {
		v.focusPending = false
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
	d.docPointer(gtx)
	d.docKeys(gtx)
	d.relayout()
	if v.ensureCaret {
		v.ensureCaret = false
		d.scrollToCaret()
	}
	viewH := float64(v.size.Y) / v.scale
	v.scrollY = min(max(v.scrollY, 0), max(d.eng.Height-viewH, 0))

	size := v.size
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	gui.FillRect(gtx.Ops, image.Rectangle{Max: size}, viewBG)
	event.Op(gtx.Ops, tag)
	pointer.CursorText.Add(gtx.Ops)
	key.InputHintOp{Tag: tag, Hint: key.HintText}.Add(gtx.Ops)

	px := d.pageX()
	for _, pg := range d.eng.Pages {
		top := (pg.Top - v.scrollY) * v.scale
		h := pg.Height * v.scale
		if top+h < 0 || top > float64(size.Y) {
			continue
		}
		r := image.Rect(px, int(math.Round(top)), px+int(math.Round(pg.Width*v.scale)), int(math.Round(top+h)))
		gui.FillRect(gtx.Ops, r.Add(image.Pt(2, 2)), pageShadow)
		off := op.Offset(r.Min).Push(gtx.Ops)
		d.eng.DrawPage(gtx.Ops, pg, wl.DrawOptions{
			Scale:    v.scale,
			Sel:      d.ed.Sel,
			SelColor: selColor,
			Y0:       max(-top/v.scale, 0),
			Y1:       (float64(size.Y) - top) / v.scale,
		})
		off.Pop()
	}
	d.drawCaret(gtx, px)
	return layout.Dimensions{Size: size}
}

func (d *window) drawCaret(gtx layout.Context, px int) {
	v := &d.v
	if !v.focused || !d.ed.Sel.Empty() || d.back.Visible {
		return
	}
	since := time.Since(v.blinkStart)
	phase := since / blinkPeriod
	gtx.Execute(op.InvalidateCmd{At: v.blinkStart.Add((phase + 1) * blinkPeriod)})
	if phase%2 == 1 {
		return
	}
	c, ok := d.eng.CaretAt(d.ed.Sel.Caret)
	if !ok {
		return
	}
	pg := d.eng.Pages[c.Page]
	x := px + int(math.Round(c.X*v.scale))
	y := (pg.Top + c.Y - v.scrollY) * v.scale
	w := max(1, gtx.Dp(1))
	gui.FillRect(gtx.Ops, image.Rect(x, int(math.Round(y)), x+w, int(math.Round(y+c.H*v.scale))), caretColor)
}

// scrollToCaret scrolls the least amount that shows the caret.
func (d *window) scrollToCaret() {
	v := &d.v
	c, ok := d.eng.CaretAt(d.ed.Sel.Caret)
	if !ok || v.scale == 0 {
		return
	}
	top := d.eng.Pages[c.Page].Top + c.Y
	viewH := float64(v.size.Y) / v.scale
	margin := min(24.0, viewH/4)
	if top-margin < v.scrollY {
		v.scrollY = top - margin
	} else if top+c.H+margin > v.scrollY+viewH {
		v.scrollY = top + c.H + margin - viewH
	}
	if pw := int(d.eng.Pages[0].Width * v.scale); pw+16 > v.size.X {
		x := int(c.X*v.scale) + 8
		if x-v.scrollX < 16 {
			v.scrollX = max(x-16, 0)
		} else if x-v.scrollX > v.size.X-16 {
			v.scrollX = x - v.size.X + 16
		}
	} else {
		v.scrollX = 0
	}
}

// viewPos converts a pointer position in the page view to a document
// position.
func (d *window) viewPos(p f32.Point) model.Pos {
	v := &d.v
	x := (float64(p.X) - float64(d.pageX())) / v.scale
	y := v.scrollY + float64(p.Y)/v.scale
	return d.eng.HitView(x, y)
}

func (d *window) docPointer(gtx layout.Context) {
	v := &d.v
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  v,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
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
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			gtx.Execute(key.FocusCmd{Tag: v})
			now := time.Now()
			if now.Sub(v.lastClick) < 500*time.Millisecond && e.Position.Sub(v.lastPos).X*e.Position.Sub(v.lastPos).X+e.Position.Sub(v.lastPos).Y*e.Position.Sub(v.lastPos).Y < 25 {
				v.clicks = v.clicks%3 + 1
			} else {
				v.clicks = 1
			}
			v.lastClick, v.lastPos = now, e.Position
			pos := d.viewPos(e.Position)
			switch {
			case v.clicks == 2:
				d.ed.SelectWord(pos)
			case v.clicks == 3:
				d.ed.SelectParagraph(pos.Para)
			default:
				d.ed.SetCaret(pos, e.Modifiers.Contain(key.ModShift))
			}
			v.dragging = true
			v.hasGoal = false
			v.resetBlink()
		case pointer.Drag:
			if !v.dragging {
				continue
			}
			// Dragging past the edges scrolls.
			if e.Position.Y < 0 {
				v.scrollY += float64(e.Position.Y) / v.scale / 2
			} else if e.Position.Y > float32(v.size.Y) {
				v.scrollY += float64(e.Position.Y-float32(v.size.Y)) / v.scale / 2
			}
			pos := d.viewPos(e.Position)
			if v.clicks == 1 {
				d.ed.SetCaret(pos, true)
			} else {
				// Extend by words or paragraphs from the original selection.
				a, b := d.ed.Sel.Ordered()
				lo, hi := d.ed.WordAt(pos)
				if v.clicks == 3 {
					lo, hi = model.Pos{Para: pos.Para}, model.Pos{Para: pos.Para, Off: d.ed.Para(pos.Para).Len()}
				}
				if pos.Less(a) {
					d.ed.Select(b, lo)
				} else {
					d.ed.Select(a, hi)
				}
			}
			v.resetBlink()
		case pointer.Release, pointer.Cancel:
			v.dragging = false
		case pointer.Scroll:
			d.wheel(gtx, e)
		}
	}
}

func (d *window) wheel(gtx layout.Context, e pointer.Event) {
	v := &d.v
	if e.Modifiers.Contain(key.ModShortcut) {
		v.wheelZoom += e.Scroll.Y
		step := float32(gtx.Dp(40))
		for v.wheelZoom >= step {
			v.wheelZoom -= step
			d.v.zoom = max(d.v.zoom-0.1, 0.1)
		}
		for v.wheelZoom <= -step {
			v.wheelZoom += step
			d.v.zoom = min(d.v.zoom+0.1, 5)
		}
		return
	}
	dx, dy := e.Scroll.X, e.Scroll.Y
	if e.Modifiers.Contain(key.ModShift) {
		dx, dy = dy, dx
	}
	v.scrollY += float64(dy) / v.scale
	v.scrollX = max(v.scrollX+int(dx), 0)
}

func (d *window) vScrollbar(gtx layout.Context) layout.Dimensions {
	v := &d.v
	w := gtx.Dp(14)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	if v.scale > 0 {
		viewH := float64(gtx.Constraints.Max.Y) / v.scale
		v.scrollY = v.vbar.Layout(gtx, v.scrollY, viewH, d.eng.Height)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

var navKeys = []key.Name{
	key.NameUpArrow, key.NameDownArrow, key.NameLeftArrow, key.NameRightArrow,
	key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd,
	key.NameDeleteBackward, key.NameDeleteForward, key.NameReturn, key.NameEnter, key.NameTab,
}

// ctrlKeys are the editing shortcuts (with Ctrl).
var ctrlKeys = []key.Name{"A", "B", "I", "U", "C", "X", "V", "Z", "Y", "L", "E", "R", "J", key.NameSpace}

func (d *window) docKeys(gtx layout.Context) {
	tag := &d.v
	filters := []event.Filter{
		key.FocusFilter{Target: tag},
		transfer.TargetFilter{Target: tag, Type: "application/text"},
		key.Filter{Focus: tag, Name: "N", Required: key.ModShortcut | key.ModShift},
		key.Filter{Focus: tag, Name: "Z", Required: key.ModShortcut | key.ModShift},
	}
	for _, n := range navKeys {
		filters = append(filters, key.Filter{Focus: tag, Name: n, Optional: key.ModShift | key.ModShortcut})
	}
	for _, n := range ctrlKeys {
		filters = append(filters, key.Filter{Focus: tag, Name: n, Required: key.ModShortcut})
	}
	for _, n := range []key.Name{"1", "2", "3"} {
		filters = append(filters, key.Filter{Focus: tag, Name: n, Required: key.ModShortcut | key.ModAlt})
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.FocusEvent:
			d.v.focused = e.Focus
			d.v.resetBlink()
		case key.EditEvent:
			if d.modalOpen() {
				continue
			}
			if d.v.skipSpace && e.Text == " " {
				d.v.skipSpace = false
				continue
			}
			d.v.skipSpace = false
			d.ed.InsertText(e.Text)
			d.v.hasGoal = false
			d.edited()
		case transfer.DataEvent:
			r := e.Open()
			b, err := io.ReadAll(r)
			r.Close()
			if err == nil {
				d.pasteData(string(b))
			}
		case key.Event:
			if e.State != key.Press || d.modalOpen() {
				continue
			}
			d.v.skipSpace = false
			d.key(gtx, e)
		}
	}
}

// key runs a key command in the page view.
func (d *window) key(gtx layout.Context, e key.Event) {
	ed := d.ed
	shift := e.Modifiers.Contain(key.ModShift)
	ctrl := e.Modifiers.Contain(key.ModShortcut)
	keepGoal := false
	switch e.Name {
	case key.NameLeftArrow:
		ed.Move(-1, ctrl, shift)
	case key.NameRightArrow:
		ed.Move(1, ctrl, shift)
	case key.NameUpArrow, key.NameDownArrow:
		dir := 1
		if e.Name == key.NameUpArrow {
			dir = -1
		}
		if ctrl {
			d.paragraphMove(dir, shift)
			break
		}
		d.verticalMove(dir, shift)
		keepGoal = true
	case key.NameHome, key.NameEnd:
		var to model.Pos
		switch {
		case ctrl && e.Name == key.NameHome:
			to = model.Pos{}
		case ctrl:
			last := ed.NumParas() - 1
			to = model.Pos{Para: last, Off: ed.Para(last).Len()}
		default:
			a, b := d.eng.LineBounds(ed.Sel.Caret)
			to = a
			if e.Name == key.NameEnd {
				to = b
			}
		}
		ed.SetCaret(to, shift)
	case key.NamePageUp, key.NamePageDown:
		d.pageMove(e.Name == key.NamePageDown, shift)
		keepGoal = true
	case key.NameDeleteBackward:
		if ctrl && ed.Sel.Empty() {
			ed.Move(-1, true, true)
		}
		ed.Backspace()
	case key.NameDeleteForward:
		if ctrl && ed.Sel.Empty() {
			ed.Move(1, true, true)
		}
		ed.Delete()
	case key.NameReturn, key.NameEnter:
		switch {
		case ctrl:
			ed.InsertSpecial(model.RunPageBreak)
		case shift:
			ed.InsertSpecial(model.RunBreak)
		default:
			ed.Enter()
		}
	case key.NameTab:
		// In a list, Tab at the start of an item changes its level.
		if p := ed.Para(ed.Sel.Caret.Para); ed.ListKind(p) != "" && ed.Sel.Empty() && ed.Sel.Caret.Off == 0 {
			if shift {
				ed.Indent(-1)
			} else {
				ed.Indent(1)
			}
		} else if !shift {
			ed.InsertSpecial(model.RunTab)
		}
	case "A":
		ed.SelectAll()
	case "B":
		ed.ToggleBold()
	case "I":
		ed.ToggleItalic()
	case "U":
		ed.ToggleUnderline()
	case "C":
		d.copy(gtx, false)
	case "X":
		d.copy(gtx, true)
	case "V":
		d.paste(gtx)
	case "Z":
		if shift {
			ed.Redo()
		} else {
			ed.Undo()
		}
	case "Y":
		ed.Redo()
	case "L":
		ed.SetAlign(model.AlignLeft)
	case "E":
		ed.SetAlign(model.AlignCenter)
	case "R":
		ed.SetAlign(model.AlignRight)
	case "J":
		ed.SetAlign(model.AlignJustify)
	case key.NameSpace:
		ed.ClearCharFormatting()
		d.v.skipSpace = true
	case "N":
		d.applyStyle(d.normalID())
	case "1", "2", "3":
		d.applyHeading(int(e.Name[0] - '0'))
	}
	if !keepGoal {
		d.v.hasGoal = false
	}
	d.edited()
}

func (d *window) verticalMove(dir int, extend bool) {
	d.relayout()
	ed := d.ed
	c, ok := d.eng.CaretAt(ed.Sel.Caret)
	if !ok {
		return
	}
	if !d.v.hasGoal {
		d.v.goalX, d.v.hasGoal = c.X, true
	}
	to, ok := d.eng.Vertical(ed.Sel.Caret, dir, d.v.goalX)
	if !ok {
		if dir < 0 {
			to = model.Pos{}
		} else {
			last := ed.NumParas() - 1
			to = model.Pos{Para: last, Off: ed.Para(last).Len()}
		}
	}
	ed.SetCaret(to, extend)
}

// paragraphMove is Ctrl+Up/Down: to the start of this or the previous
// paragraph, or of the next one.
func (d *window) paragraphMove(dir int, extend bool) {
	ed := d.ed
	p := ed.Sel.Caret
	switch {
	case dir < 0 && p.Off > 0:
		p.Off = 0
	case dir < 0:
		p = model.Pos{Para: max(p.Para-1, 0)}
	case p.Para+1 < ed.NumParas():
		p = model.Pos{Para: p.Para + 1}
	default:
		p.Off = ed.Para(p.Para).Len()
	}
	ed.SetCaret(p, extend)
}

// pageMove is PageUp/PageDown: scroll a screen and move the caret by the
// same distance.
func (d *window) pageMove(down bool, extend bool) {
	d.relayout()
	v := &d.v
	c, ok := d.eng.CaretAt(d.ed.Sel.Caret)
	if !ok || v.scale == 0 {
		return
	}
	viewH := float64(v.size.Y) / v.scale
	dy := viewH * 0.9
	if !down {
		dy = -dy
	}
	if !v.hasGoal {
		v.goalX, v.hasGoal = c.X, true
	}
	y := d.eng.Pages[c.Page].Top + c.Y + c.H/2 + dy
	v.scrollY += dy
	to := d.eng.HitView(v.goalX, min(max(y, 0), d.eng.Height))
	d.ed.SetCaret(to, extend)
}

// Clipboard.

func (d *window) copy(gtx layout.Context, cut bool) {
	if d.ed.Sel.Empty() {
		return
	}
	f := d.ed.Copy()
	d.clip, d.clipText = f, f.Text()
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(d.clipText))})
	if cut {
		d.ed.DeleteSelection()
	}
}

func (d *window) paste(gtx layout.Context) {
	gtx.Execute(clipboard.ReadCmd{Tag: &d.v})
}

// pasteData pastes clipboard text: what WordLite copied keeps its
// formatting, anything else is plain text.
func (d *window) pasteData(s string) {
	norm := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	if d.clip != nil && norm(s) == norm(d.clipText) {
		d.ed.Paste(d.clip)
	} else {
		d.ed.PasteText(s)
	}
	d.edited()
}

// Styles.

func (d *window) normalID() string {
	if st := d.doc.Styles.Default(model.ParagraphStyle); st != nil {
		return st.ID
	}
	return "Normal"
}

// applyHeading applies Heading n, adding the style if the document lacks
// it.
func (d *window) applyHeading(n int) {
	id := headingID(d.doc, n)
	if d.doc.Styles.Find(id) == nil {
		// Documents without the style get Word's built-in definition.
		for _, st := range model.DefaultStyles().Styles {
			if st.ID == "Heading"+itoa(n) {
				c := *st
				c.BasedOn = d.normalID()
				c.Next = d.normalID()
				d.ed.ModifyStyleSheet(func(ss *model.StyleSheet) { ss.Styles = append(ss.Styles, &c) })
				id = c.ID
			}
		}
	}
	d.applyStyle(id)
}

// headingID finds the style for "heading n" by name, as documents may use
// other IDs (localized or renamed).
func headingID(doc *model.Document, n int) string {
	want := "heading " + itoa(n)
	for _, st := range doc.Styles.Styles {
		if st.Type == model.ParagraphStyle && strings.EqualFold(st.Name, want) {
			return st.ID
		}
	}
	return "Heading" + itoa(n)
}

func (d *window) applyStyle(id string) {
	d.ed.ApplyStyle(id)
	d.edited()
}
