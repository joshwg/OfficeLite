package ui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/fileio"
	"officelite/word/fileio/docfile"
	"officelite/word/model"
)

// modalOpen reports whether a dialog takes the input.
func (d *window) modalOpen() bool { return d.dlg.open || d.confirm.show || d.lossy.Visible }

// checkLossy runs save at once, or, when saving doc as .doc loses
// something, after the user agrees.
func (d *window) checkLossy(f fileio.Format, save func()) {
	var lost []string
	if f == fileio.DOC {
		lost = docfile.Warnings(d.doc)
	}
	if len(lost) == 0 {
		save()
		return
	}
	msg := "Some of this document's content can't be kept in the Word 97-2003 format:\n"
	for _, w := range lost {
		msg += "\n• " + w
	}
	d.lossyGo = save
	d.lossy.Show("Save as Word 97-2003 Document?", msg, "Save", "Cancel")
}

func (d *window) lossyLayout(gtx layout.Context) {
	switch d.lossy.Clicked(gtx) {
	case 0:
		if f := d.lossyGo; f != nil {
			d.lossyGo = nil
			f()
		}
	case 1:
		d.lossyGo = nil
		d.closeAfterSave = false
		d.v.focusPending = true
	}
	d.lossy.Layout(gtx, d.th)
}

// modal dims the window, swallows clicks outside the panel (tag) and
// draws body in a white panel width wide, centered.
func modal(gtx layout.Context, tag event.Tag, width unit.Dp, body layout.Widget) {
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}}); !ok {
			break
		}
	}
	size := gtx.Constraints.Max
	gui.FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.RGBA(0, 0, 0, 0x50))
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(gtx.Dp(width), 0), Max: image.Pt(min(gtx.Dp(width), size.X), size.Y)}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(20).Layout(g, body)
	call := m.Stop()
	pos := image.Pt((size.X-dims.Size.X)/2, max((size.Y-dims.Size.Y)/3, 0))
	off := op.Offset(pos).Push(gtx.Ops)
	gui.FillRect(gtx.Ops, image.Rect(3, 3, dims.Size.X+4, dims.Size.Y+4), colors.RGBA(0, 0, 0, 0x30))
	gui.FillRect(gtx.Ops, image.Rect(-1, -1, dims.Size.X+1, dims.Size.Y+1), gui.ChromeLine)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: dims.Size}, colors.White)
	call.Add(gtx.Ops)
	off.Pop()
}

// plainButton is an outlined secondary button.
func plainButton(gtx layout.Context, th *gui.Theme, btn *widget.Clickable, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return gui.Hoverable(gtx, btn, gui.ChromeBG, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
		l := th.Label(14, label, gui.ChromeText, font.Normal)
		d := layout.Inset{Top: 8, Bottom: 8, Left: 22, Right: 22}.Layout(gtx, l.Layout)
		r := image.Rectangle{Max: d.Size}
		gui.HLine(gtx.Ops, 0, r.Max.X, 0, 1, gui.ChromeLine)
		gui.HLine(gtx.Ops, 0, r.Max.X, r.Max.Y-1, 1, gui.ChromeLine)
		gui.VLine(gtx.Ops, 0, 0, r.Max.Y, 1, gui.ChromeLine)
		gui.VLine(gtx.Ops, r.Max.X-1, 0, r.Max.Y, 1, gui.ChromeLine)
		return d
	})
}

// confirmState is the "save changes?" prompt shown when a window with
// unsaved changes is closed.
type confirmState struct {
	show                   bool
	save, dontSave, cancel widget.Clickable
	tag                    int
}

func (c *confirmState) layout(gtx layout.Context, d *window) {
	for c.save.Clicked(gtx) {
		c.show = false
		d.closeAfterSave = true
		d.save()
	}
	for c.dontSave.Clicked(gtx) {
		c.show = false
		d.allowClose = true
		d.close()
	}
	for c.cancel.Clicked(gtx) {
		c.show = false
		d.v.focusPending = true
	}
	th := d.th
	modal(gtx, &c.tag, 440, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(18, appName, gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(th.Paragraph("Do you want to save changes to "+d.doc.Name+"?", gui.ChromeText)),
			layout.Rigid(layout.Spacer{Height: 20}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &c.save, "Save") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return plainButton(gtx, th, &c.dontSave, "Don't Save") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return plainButton(gtx, th, &c.cancel, "Cancel") }),
				)
			}),
		)
	})
}

var (
	spaceChoices = []float64{0, 3, 4, 6, 8, 10, 12, 18, 24}
	lineChoices  = []float64{1, 1.08, 1.15, 1.5, 2, 2.5, 3}
)

// styleDialog is Modify Style: font, size, bold, italic, underline, color,
// alignment and spacing of a paragraph style.
type styleDialog struct {
	open bool
	id   string
	run  model.RunProps  // the style's own properties being edited
	para model.ParaProps // (a copy)

	styleDD, fontDD, sizeDD, colorDD, beforeDD, afterDD, lineDD gui.DropDown

	bold, italic, under            widget.Clickable
	alignL, alignC, alignR, alignJ widget.Clickable
	ok, cancel                     widget.Clickable
	tag                            int
	ids                            []string
}

// openFor shows the dialog for paragraph style id.
func (s *styleDialog) openFor(d *window, id string) {
	st := d.doc.Styles.Find(id)
	if st == nil || st.Type != model.ParagraphStyle {
		st = d.doc.Styles.Default(model.ParagraphStyle)
	}
	if st == nil {
		return
	}
	s.open = true
	s.id = st.ID
	s.run = st.Run
	s.para = st.Para
	d.menu.Close()
}

// base returns the formatting the style inherits (its BasedOn chain, or
// the document defaults).
func (s *styleDialog) base(doc *model.Document) (model.RunProps, model.ParaProps) {
	st := doc.Styles.Find(s.id)
	if st != nil && st.BasedOn != "" && doc.Styles.Find(st.BasedOn) != nil {
		p := &model.Paragraph{Style: st.BasedOn}
		return doc.ResolveRun(p, &model.Run{}), doc.ResolvePara(p)
	}
	r := doc.Styles.DefaultRun
	if r.Font == "" {
		r.Font = "Calibri"
	}
	if r.Size == 0 {
		r.Size = 11
	}
	return r, doc.Styles.DefaultPara
}

func (s *styleDialog) layout(gtx layout.Context, d *window) {
	th := d.th
	baseRun, basePara := s.base(d.doc)
	run := baseRun.Merge(s.run)
	para := basePara.Merge(s.para)

	for s.bold.Clicked(gtx) {
		s.run.Bold = model.TriOf(!run.Bold.Bool(false))
	}
	for s.italic.Clicked(gtx) {
		s.run.Italic = model.TriOf(!run.Italic.Bool(false))
	}
	for s.under.Clicked(gtx) {
		s.run.Underline = model.UnderlineSingle
		if u := run.Underline; u != model.UnderlineUnset && u != model.UnderlineNone {
			s.run.Underline = model.UnderlineNone
		}
	}
	for i, b := range []*widget.Clickable{&s.alignL, &s.alignC, &s.alignR, &s.alignJ} {
		for b.Clicked(gtx) {
			s.para.Align = model.AlignLeft + model.Align(i)
		}
	}
	for s.ok.Clicked(gtx) {
		s.open = false
		r, p := s.run, s.para
		d.ensureFont(r.Font)
		d.ed.ModifyStyle(s.id, func(st *model.Style) { st.Run, st.Para = r, p })
		d.edited()
	}
	for s.cancel.Clicked(gtx) {
		s.open = false
		d.v.focusPending = true
	}
	run = baseRun.Merge(s.run)
	para = basePara.Merge(s.para)

	// Choices.
	styleItems, ids := d.styleItems()
	var paraItems []gui.DropItem
	s.ids = s.ids[:0]
	for i, it := range styleItems {
		if ids[i] == "" {
			break // character styles follow
		}
		if st := d.doc.Styles.Find(ids[i]); st != nil && st.Type == model.ParagraphStyle {
			paraItems = append(paraItems, it)
			s.ids = append(s.ids, ids[i])
		}
	}
	fontItems := make([]gui.DropItem, len(commonFonts))
	for i, f := range commonFonts {
		fontItems[i] = gui.DropItem{Label: f}
	}
	sizeItems := make([]gui.DropItem, len(fontSizes))
	for i, v := range fontSizes {
		sizeItems[i] = gui.DropItem{Label: fmtSize(v)}
	}
	colorItems := make([]gui.DropItem, len(palette))
	for i, c := range palette {
		colorItems[i] = gui.DropItem{Label: c.name, Swatch: colors.HexOr(c.hex, colors.RGB(0))}
	}
	spaceItems := make([]gui.DropItem, len(spaceChoices))
	for i, v := range spaceChoices {
		spaceItems[i] = gui.DropItem{Label: fmtSize(v) + " pt"}
	}
	lineItems := make([]gui.DropItem, len(lineChoices))
	for i, v := range lineChoices {
		lineItems[i] = gui.DropItem{Label: strconv.FormatFloat(v, 'f', -1, 64)}
	}
	ptOf := func(p *float64) string {
		if p == nil {
			return "0 pt"
		}
		return fmtSize(*p) + " pt"
	}
	lineText := "Single"
	switch para.LineRule {
	case model.LineAuto:
		lineText = strconv.FormatFloat(para.Line, 'f', -1, 64)
	case model.LineExact:
		lineText = "Exactly " + fmtSize(para.Line) + " pt"
	case model.LineAtLeast:
		lineText = "At least " + fmtSize(para.Line) + " pt"
	}
	colorName := "Automatic"
	for _, c := range palette {
		if c.hex != "" && c.hex == run.Color {
			colorName = c.name
		}
	}
	if colorName == "Automatic" && run.Color != "" && run.Color != "auto" {
		colorName = "#" + run.Color
	}

	label := func(s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(110)
			return gui.Middle(gtx, th.Label(13, s, gui.MutedText, font.Normal).Layout)
		})
	}
	row := func(children ...layout.FlexChild) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(28)
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
			})
		})
	}
	combo := func(dd *gui.DropDown, w unit.Dp, cur string, items []gui.DropItem, apply func(int)) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var dims layout.Dimensions
			dims = gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				if i := dd.Layout(gtx, th, w, cur, items); i >= 0 {
					apply(i)
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(w), gtx.Dp(24))}
			})
			return dims
		})
	}
	tool := func(b *widget.Clickable, ic *widget.Icon, on bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, th, b, ic, "", on, color.NRGBA{})
		})
	}
	gap := layout.Rigid(layout.Spacer{Width: 6}.Layout)
	align := para.Align
	if align == model.AlignUnset {
		align = model.AlignLeft
	}
	cur := styleName(d.doc.Styles.Find(s.id))

	modal(gtx, &s.tag, 520, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(18, "Modify Style", gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 14}.Layout),
			row(label("Style"), combo(&s.styleDD, 220, cur, paraItems, func(i int) { s.openFor(d, s.ids[i]) })),
			row(label("Font"), combo(&s.fontDD, 170, run.Font, fontItems, func(i int) { s.run.Font = commonFonts[i] }),
				gap, combo(&s.sizeDD, 60, fmtSize(run.Size), sizeItems, func(i int) { s.run.Size = fontSizes[i] })),
			row(label("Font style"), tool(&s.bold, gui.IconBold, run.Bold.Bool(false)),
				tool(&s.italic, gui.IconItalic, run.Italic.Bool(false)),
				tool(&s.under, gui.IconUnderline, run.Underline != model.UnderlineUnset && run.Underline != model.UnderlineNone),
				gap, combo(&s.colorDD, 140, colorName, colorItems, func(i int) { s.run.Color = palette[i].hex })),
			row(label("Alignment"), tool(&s.alignL, gui.IconAlignLeft, align == model.AlignLeft),
				tool(&s.alignC, gui.IconAlignCenter, align == model.AlignCenter),
				tool(&s.alignR, gui.IconAlignRight, align == model.AlignRight),
				tool(&s.alignJ, gui.IconAlignJustify, align == model.AlignJustify)),
			row(label("Spacing before"), combo(&s.beforeDD, 90, ptOf(para.SpaceBefore), spaceItems, func(i int) { s.para.SpaceBefore = model.F(spaceChoices[i]) }),
				gap, layout.Rigid(th.Label(13, "after", gui.MutedText, font.Normal).Layout), gap,
				combo(&s.afterDD, 90, ptOf(para.SpaceAfter), spaceItems, func(i int) { s.para.SpaceAfter = model.F(spaceChoices[i]) })),
			row(label("Line spacing"), combo(&s.lineDD, 130, lineText, lineItems, func(i int) {
				s.para.Line, s.para.LineRule = lineChoices[i], model.LineAuto
			})),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// A sample in the style's font.
				return layout.Inset{Top: 6, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := th.Label(float32(min(run.Size, 28)*96/72), "AaBbCc "+cur, colors.HexOr(run.Color, colors.RGB(0)), font.Normal)
					l.Font = font.Font{Typeface: gui.Typeface(run.Font)}
					if run.Bold.Bool(false) {
						l.Font.Weight = font.Bold
					}
					if run.Italic.Bool(false) {
						l.Font.Style = font.Italic
					}
					return l.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &s.ok, "OK") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return plainButton(gtx, th, &s.cancel, "Cancel") }),
				)
			}),
		)
	})
}
