package ui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/fileio"
	"officelite/word/fileio/docfile"
	"officelite/word/model"
)

// modalOpen reports whether a dialog takes the input.
func (d *window) modalOpen() bool {
	return d.dlg.open || d.confirm.show || d.lossy.Visible || d.lk.dlg.Visible || d.ins.modal() || d.extraModal()
}

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
		d.ar.saver.Close() // the changes and their recovery copy go (recover.go)
		d.close()
	}
	for c.cancel.Clicked(gtx) {
		c.show = false
		d.v.focusPending = true
	}
	th := d.th
	gui.Modal(gtx, &c.tag, 440, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(18, appName, gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(th.Paragraph("Do you want to save changes to "+d.doc.Name+"?", gui.ChromeText)),
			layout.Rigid(layout.Spacer{Height: 20}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &c.save, "Save") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &c.dontSave, "Don't Save") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &c.cancel, "Cancel") }),
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

	styleDD, fontDD, sizeDD, beforeDD, afterDD, lineDD gui.DropDown
	colorCP                                            gui.ColorPicker

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
		sizeItems[i] = gui.DropItem{Label: gui.FormatPt(v)}
	}
	spaceItems := make([]gui.DropItem, len(spaceChoices))
	for i, v := range spaceChoices {
		spaceItems[i] = gui.DropItem{Label: gui.FormatPt(v) + " pt"}
	}
	lineItems := make([]gui.DropItem, len(lineChoices))
	for i, v := range lineChoices {
		lineItems[i] = gui.DropItem{Label: strconv.FormatFloat(v, 'f', -1, 64)}
	}
	ptOf := func(p *float64) string {
		if p == nil {
			return "0 pt"
		}
		return gui.FormatPt(*p) + " pt"
	}
	lineText := "Single"
	switch para.LineRule {
	case model.LineAuto:
		lineText = strconv.FormatFloat(para.Line, 'f', -1, 64)
	case model.LineExact:
		lineText = "Exactly " + gui.FormatPt(para.Line) + " pt"
	case model.LineAtLeast:
		lineText = "At least " + gui.FormatPt(para.Line) + " pt"
	}

	form := gui.Form{Th: th, LabelW: 110, LabelColor: gui.MutedText, RowH: 28}
	label, row, combo := form.Label, form.Row, form.Combo
	tool := func(b *widget.Clickable, ic *gui.Icon, on bool) layout.FlexChild {
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

	gui.Modal(gtx, &s.tag, 520, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(18, "Modify Style", gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 14}.Layout),
			row(label("Style"), combo(&s.styleDD, 220, cur, paraItems, func(i int) { s.openFor(d, s.ids[i]) })),
			row(label("Font"), combo(&s.fontDD, 170, run.Font, fontItems, func(i int) { s.run.Font = commonFonts[i] }),
				gap, combo(&s.sizeDD, 60, gui.FormatPt(run.Size), sizeItems, func(i int) { s.run.Size = fontSizes[i] })),
			row(label("Font style"), tool(&s.bold, gui.IconBold, run.Bold.Bool(false)),
				tool(&s.italic, gui.IconItalic, run.Italic.Bool(false)),
				tool(&s.under, gui.IconUnderline, run.Underline != model.UnderlineUnset && run.Underline != model.UnderlineNone),
				gap, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					auto := run.Color == "" || run.Color == "auto"
					cur := gui.ColorChoice{Color: colors.HexOr(run.Color, colors.RGB(0)), None: auto}
					return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
						if c, ok, _ := s.colorCP.Combo(gtx, th, 140, cur, "Automatic"); ok {
							s.run.Color = choiceHex(c)
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(140), gtx.Dp(24))}
					})
				})),
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
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &s.cancel, "Cancel") }),
				)
			}),
		)
	})
}
