package ui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Editing headers and footers, as in Word: double-click the top or bottom
// margin of a page to edit its header or footer (the body dims and a
// Header & Footer bar shows); Esc, Close or a double-click in the body
// goes back. While a header is edited d.ed is an editor over a document
// holding the header's blocks (model.Document.SubDocument) and d.hf.view
// lays it out where it shows on the page, for the caret and hit testing.

// hfEdit is the header/footer editing state.
type hfEdit struct {
	on     bool
	page   int // page whose header/footer is edited
	footer bool
	kind   model.HFKind
	hf     *model.HeaderFooter
	view   *wl.Engine    // the edited header laid out on its page
	body   *model.Editor // the body's editor, while d.ed edits the header
	synced int           // header editor version synced into the document
	eds    map[*model.HeaderFooter]*model.Editor

	pageNum, numPages, firstPg, oddEven, gotoOther, closeBtn widget.Clickable
}

// bodyEd is the body's editor (d.ed unless a header is being edited).
func (d *window) bodyEd() *model.Editor {
	if d.hf.on {
		return d.hf.body
	}
	return d.ed
}

// ceng is the engine positions of d.ed are laid out in: the header's view
// while editing one, else the page layout.
func (d *window) ceng() *wl.Engine {
	if d.hf.on && d.hf.view != nil {
		return d.hf.view
	}
	return d.eng
}

// enterHF starts editing the header (or footer) shown on page i, creating
// it when the page has none.
func (d *window) enterHF(i int, footer bool) {
	if d.hf.on {
		d.exitHF()
	}
	doc := d.doc
	kind := doc.HF.KindFor(i)
	hf := *doc.HF.Slot(kind, footer)
	if hf == nil {
		n := len(doc.Styles.Styles)
		hf = doc.NewHeaderFooter(kind, footer)
		d.ed.Version++ // a change to save
		d.eng.Invalidate(nil, len(doc.Styles.Styles) != n)
	}
	if d.hf.eds == nil {
		d.hf.eds = map[*model.HeaderFooter]*model.Editor{}
	}
	ed := d.hf.eds[hf]
	if ed == nil {
		ed = model.NewEditor(doc.SubDocument(hf.Blocks))
		d.hf.eds[hf] = ed
	}
	sub := ed.Doc
	sub.Styles, sub.Lists, sub.Body = doc.Styles, doc.Lists, hf.Blocks
	if len(sub.Body) == 0 {
		sub.Body = []model.Block{&model.Paragraph{}}
	}
	ed.Doc.Page = doc.Page
	d.hf.on, d.hf.page, d.hf.footer, d.hf.kind, d.hf.hf = true, i, footer, kind, hf
	d.hf.body, d.hf.synced = d.ed, ed.Version
	d.ed = ed
	d.ed.SetCaret(model.Pos{}, false)
	d.painterOff()
	d.laidOut = -1
	d.eng.InvalidateHF()
	d.edited()
}

// exitHF goes back to editing the body.
func (d *window) exitHF() {
	if !d.hf.on {
		return
	}
	d.hfSync()
	d.ed = d.hf.body
	d.hf.on, d.hf.view, d.hf.body = false, nil, nil
	d.laidOut = -1
	d.edited()
}

// hfSync copies header edits into the document; it runs before every
// layout while a header is edited.
func (d *window) hfSync() {
	if !d.hf.on {
		return
	}
	sub := d.ed.Doc
	d.hf.hf.Blocks = sub.Body
	d.doc.Styles, d.doc.Lists = sub.Styles, sub.Lists
	if d.ed.Version != d.hf.synced {
		d.hf.synced = d.ed.Version
		d.hf.body.Version++ // the document changed
	}
	d.eng.InvalidateHF()
}

// hfAfterLayout lays the edited header out on its page again.
func (d *window) hfAfterLayout() {
	if !d.hf.on {
		return
	}
	i := min(d.hf.page, len(d.eng.Pages)-1)
	d.hf.page = i
	d.hf.view = d.eng.EditView(i, d.hf.footer, d.ed.Doc)
}

// hfPress handles a press in the page view for headers and footers and
// reports whether it consumed it: a double-click in a page's top or
// bottom margin edits its header or footer; while editing, presses in the
// body do nothing and a double-click there returns to the body.
func (d *window) hfPress(p f32.Point, clicks int) bool {
	v := &d.v
	if v.scale == 0 || len(d.eng.Pages) == 0 {
		return false
	}
	y := v.scrollY + float64(p.Y)/v.scale
	page, py := d.eng.PageAt(y)
	footer, inHF := d.eng.InHFArea(page, py)
	if !d.hf.on {
		if clicks == 2 && inHF {
			d.enterHF(page, footer)
			d.relayout()
			d.ed.SetCaret(d.viewPos(p), false)
			v.clicks = 0
			return true
		}
		return false
	}
	if !inHF {
		if clicks == 2 {
			d.exitHF()
			d.relayout()
			d.ed.SetCaret(d.viewPos(p), false)
			v.clicks = 0
		}
		return true
	}
	if page != d.hf.page || footer != d.hf.footer {
		d.enterHF(page, footer)
		d.relayout()
	}
	return false
}

// hfCommand runs a Header & Footer command.
func (d *window) hfInsertField(instr string) {
	if !d.hf.on {
		d.enterHF(d.eng.PageOfPos(d.ed.Sel.Caret), true)
	}
	v, _ := wl.FieldValue(instr, d.hf.page+1, len(d.eng.Pages))
	d.ed.InsertSimpleField(instr, v)
	d.edited()
}

// setHFSwitch turns Different First Page or Different Odd & Even Pages
// on or off (one change to the document; not on the undo stack).
func (d *window) setHFSwitch(first bool, on bool) {
	hf := &d.doc.HF
	if first {
		hf.TitlePage = on
	} else {
		hf.EvenOdd = on
	}
	d.bodyEd().Version++
	d.eng.InvalidateHF()
	d.laidOut = -1
	if d.hf.on {
		// Edit the header that page now shows.
		page, footer := d.hf.page, d.hf.footer
		d.exitHF()
		d.relayout()
		d.enterHF(page, footer)
	}
	d.edited()
}

// removeHF deletes the header (or footer) of the kind the caret's page
// shows.
func (d *window) removeHF(footer bool) {
	page := d.eng.PageOfPos(d.bodyEd().Sel.Caret)
	if d.hf.on {
		page = d.hf.page
		d.exitHF()
	}
	slot := d.doc.HF.Slot(d.doc.HF.KindFor(page), footer)
	if *slot == nil {
		return
	}
	*slot = nil
	d.ed.Version++
	d.eng.InvalidateHF()
	d.laidOut = -1
	d.edited()
}

// hfBar is the Header & Footer bar shown while a header or footer is
// edited (nothing otherwise).
func (d *window) hfBar(gtx layout.Context) layout.Dimensions {
	if !d.hf.on {
		return layout.Dimensions{}
	}
	h := &d.hf
	th := d.th
	for h.pageNum.Clicked(gtx) {
		d.hfInsertField(model.FieldPage)
	}
	for h.numPages.Clicked(gtx) {
		d.hfInsertField(model.FieldNumPages)
	}
	for h.firstPg.Clicked(gtx) {
		d.setHFSwitch(true, !d.doc.HF.TitlePage)
	}
	for h.oddEven.Clicked(gtx) {
		d.setHFSwitch(false, !d.doc.HF.EvenOdd)
	}
	for h.gotoOther.Clicked(gtx) {
		d.enterHF(h.page, !h.footer)
	}
	for h.closeBtn.Clicked(gtx) {
		d.exitHF()
	}
	if !h.on {
		return layout.Dimensions{}
	}
	ht := gtx.Dp(34)
	w := gtx.Constraints.Max.X
	gtx.Constraints = layout.Exact(image.Pt(w, ht))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, ht), th.AccentTint)
	gui.HLine(gtx.Ops, 0, w, ht-1, 1, gui.ChromeLine)
	what := "Header"
	other := "Go to Footer"
	if h.footer {
		what, other = "Footer", "Go to Header"
	}
	switch h.kind {
	case model.HFFirst:
		what = "First Page " + what
	case model.HFEven:
		what = "Even Page " + what
	default:
		if d.doc.HF.EvenOdd {
			what = "Odd Page " + what
		}
	}
	text := func(b *widget.Clickable, label string, checked bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, th, b, nil, label, checked, color.NRGBA{})
		})
	}
	check := func(b *widget.Clickable, label string, on bool) layout.FlexChild {
		mark := "☐ "
		if on {
			mark = "☑ "
		}
		return text(b, mark+label, false)
	}
	return layout.Inset{Left: 10, Right: 10, Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(th.Label(13, "Header & Footer", th.AccentDark, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Width: 10}.Layout),
			layout.Rigid(th.Label(12, "Editing: "+what, gui.MutedText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Width: 6}.Layout),
			layout.Rigid(gui.ToolSeparator),
			text(&h.pageNum, "# Page Number", false),
			text(&h.numPages, "Number of Pages", false),
			layout.Rigid(gui.ToolSeparator),
			check(&h.firstPg, "Different First Page", d.doc.HF.TitlePage),
			check(&h.oddEven, "Different Odd & Even Pages", d.doc.HF.EvenOdd),
			layout.Rigid(gui.ToolSeparator),
			text(&h.gotoOther, other, false),
			layout.Flexed(1, layout.Spacer{}.Layout),
			text(&h.closeBtn, "Close Header and Footer", false),
		)
	})
}

var (
	hfSelColor = colors.RGBA(0x80, 0x60, 0xE0, 0x50)
	findColor  = colors.RGBA(0xFF, 0xD7, 0x00, 0x70)
	findCur    = colors.RGBA(0xFF, 0x8C, 0x00, 0x80)
	hfTagBG    = colors.RGB(0xDDE4F0)
	hfTagText  = colors.RGB(0x2B579A)
)

// drawPageExtras draws over page pg (at the current origin): the find
// matches, and while a header is edited its selection and the Header /
// Footer tags.
func (d *window) drawPageExtras(gtx layout.Context, pg *wl.Page) {
	s := d.v.scale
	px := func(v float64) int { return int(math.Round(v * s)) }
	rect := func(r wl.Rect) image.Rectangle {
		return image.Rect(px(r.X0), px(r.Y0), max(px(r.X1), px(r.X0)+1), max(px(r.Y1), px(r.Y0)+1))
	}
	eng := d.ceng()
	if pg.Index < len(eng.Pages) {
		cpg := eng.Pages[pg.Index]
		d.drawMatches(gtx, eng, cpg, rect)
		if d.hf.on && pg.Index == d.hf.page && !d.ed.Sel.Empty() {
			a, b := d.ed.Sel.Ordered()
			for _, r := range eng.SelectionRects(cpg, a, b) {
				gui.FillRect(gtx.Ops, rect(r), hfSelColor)
			}
		}
	}
	if !d.hf.on {
		return
	}
	// Tags at the left of the boundaries, as Word labels them.
	tag := func(label string, y int, above bool) {
		gtx := gtx
		gtx.Constraints.Min = image.Point{}
		m := op.Record(gtx.Ops)
		dims := layout.Inset{Left: 6, Right: 6, Top: 2, Bottom: 2}.Layout(gtx, d.th.Label(11, label, hfTagText, font.Normal).Layout)
		call := m.Stop()
		if above {
			y -= dims.Size.Y
		}
		o := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		gui.FillRect(gtx.Ops, image.Rectangle{Max: dims.Size}, hfTagBG)
		call.Add(gtx.Ops)
		o.Pop()
	}
	names := [3]string{"", "First Page ", "Even Page "}
	k := d.doc.HF.KindFor(pg.Index)
	pre := names[k]
	if k == model.HFDefault && d.doc.HF.EvenOdd {
		pre = "Odd Page "
	}
	tag(pre+"Header", px(pg.BodyTop)+1, false)
	tag(pre+"Footer", px(pg.BodyBottom), true)
}
