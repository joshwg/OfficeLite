package ui

import (
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// The Styles pane (Home > Styles Pane, Ctrl+Alt+Shift+S): every paragraph
// and character style with a preview in its own formatting. A click
// selects a style, a double-click (or Apply) applies it; New Style…,
// Modify…, Update to Match Selection and Delete (custom styles only) work
// on the selected one.

type stylesPane struct {
	open     bool
	selected string // style ID
	list     widget.List
	rows     []widget.Clickable
	ids      []string

	apply, newBtn, modify, update, del, closeBtn widget.Clickable
}

// paneStyles lists the paragraph styles (Normal and headings first, as in
// the gallery) then the character styles.
func paneStyles(doc *model.Document) []*model.Style {
	var paras, chars []*model.Style
	for _, st := range doc.Styles.Styles {
		switch st.Type {
		case model.ParagraphStyle:
			paras = append(paras, st)
		case model.CharacterStyle:
			chars = append(chars, st)
		}
	}
	rank := func(st *model.Style) string {
		n := strings.ToLower(styleName(st))
		h := "0"
		if st.Hidden {
			h = "1"
		}
		switch {
		case st.Default:
			return "0"
		case strings.HasPrefix(n, "heading ") && len(n) == 9:
			return h + "1" + n
		case n == "title" || n == "subtitle":
			return h + "2" + n
		}
		return h + "3" + n
	}
	sortFn := func(a, b *model.Style) int { return strings.Compare(rank(a), rank(b)) }
	slices.SortStableFunc(paras, sortFn)
	slices.SortStableFunc(chars, sortFn)
	return append(paras, chars...)
}

// styleSample is the label of a style's preview, in its formatting.
func (d *window) styleSample(st *model.Style, size float32) sampleFmt {
	var r model.RunProps
	if st.Type == model.CharacterStyle {
		r = d.doc.ResolveRun(&model.Paragraph{}, &model.Run{Style: st.ID})
	} else {
		r = d.doc.ResolveRun(&model.Paragraph{Style: st.ID}, &model.Run{})
	}
	f := font.Font{Typeface: gui.Typeface(r.Font)}
	if r.Bold.Bool(false) {
		f.Weight = font.Bold
	}
	if r.Italic.Bool(false) {
		f.Style = font.Italic
	}
	c := colors.HexOr(r.Color, gui.ChromeText)
	if st.Hidden {
		c = colors.Mix(c, colors.White, 0.4)
	}
	if size == 0 {
		size = float32(min(max(r.Size, 9), 16))
	}
	return sampleFmt{font: f, size: size, color: c}
}

type sampleFmt struct {
	font  font.Font
	size  float32
	color color.NRGBA
}

func (d *window) stylesPaneLayout(gtx layout.Context) layout.Dimensions {
	p := &d.pane
	if !p.open {
		return layout.Dimensions{}
	}
	th := d.th
	styles := paneStyles(d.doc)
	if len(p.rows) < len(styles) {
		p.rows = make([]widget.Clickable, len(styles)+8)
	}
	p.ids = p.ids[:0]
	for _, st := range styles {
		p.ids = append(p.ids, st.ID)
	}
	cur := d.ed.ParaStyleID(d.ed.Sel.Caret.Para)
	if p.selected == "" || d.doc.Styles.Find(p.selected) == nil {
		p.selected = cur
	}
	sel := d.doc.Styles.Find(p.selected)
	for i := range styles {
		for {
			c, ok := p.rows[i].Update(gtx)
			if !ok {
				break
			}
			p.selected = p.ids[i]
			if c.NumClicks >= 2 {
				d.ed.ApplyStyle(p.selected)
				d.edited()
			}
		}
	}
	for p.apply.Clicked(gtx) {
		d.ed.ApplyStyle(p.selected)
		d.edited()
	}
	for p.newBtn.Clicked(gtx) {
		d.newStyle.openFor(d)
	}
	for p.modify.Clicked(gtx) {
		d.dlg.openFor(d, p.selected)
	}
	for p.update.Clicked(gtx) {
		d.ed.UpdateStyleToMatch(p.selected)
		d.edited()
	}
	for p.del.Clicked(gtx) {
		if err := d.ed.DeleteStyle(p.selected); err != nil {
			d.notice = err.Error()
		} else {
			d.notice = "Deleted style " + styleName(sel)
			p.selected = ""
		}
		d.edited()
	}
	for p.closeBtn.Clicked(gtx) {
		p.open = false
		d.v.focusPending = true
	}
	if !p.open {
		return layout.Dimensions{}
	}

	w := gtx.Dp(250)
	h := gtx.Constraints.Max.Y
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), colors.White)
	gui.VLine(gtx.Ops, 0, 0, h, 1, gui.ChromeLine)

	text := func(b *widget.Clickable, s string, off bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if off {
				gtx = gtx.Disabled()
			}
			return gui.ToolButton(gtx, th, b, nil, s, false, color.NRGBA{})
		})
	}
	isPara := sel != nil && sel.Type == model.ParagraphStyle
	canDelete := sel != nil && sel.Custom && !sel.Default
	p.list.Axis = layout.Vertical
	return layout.Inset{Left: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 10, Right: 4, Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, th.Label(15, "Styles", gui.ChromeText, font.SemiBold).Layout),
						text(&p.closeBtn, "✕", false),
					)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return paneList(gtx, th, &p.list, len(styles), func(gtx layout.Context, i int) layout.Dimensions {
					st := styles[i]
					m := d.styleSample(st, 0)
					bg := color.NRGBA{}
					if st.ID == p.selected {
						bg = th.AccentTint
					}
					return gui.Hoverable(gtx, &p.rows[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Left: 10, Right: 8, Top: 4, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							mark := "¶"
							if st.Type == model.CharacterStyle {
								mark = "a"
							}
							if st.ID == cur {
								mark = "▸ " + mark
							}
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									l := th.Label(m.size, styleName(st), m.color, font.Normal)
									l.Font = m.font
									return l.Layout(gtx)
								}),
								layout.Rigid(th.Label(12, mark, gui.MutedText, font.Normal).Layout),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gui.HLine(gtx.Ops, 0, gtx.Constraints.Max.X, 0, 1, gui.ChromeLine)
				return layout.Inset{Left: 6, Right: 6, Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								text(&p.apply, "Apply", sel == nil),
								text(&p.newBtn, "New Style…", false),
								text(&p.modify, "Modify…", !isPara),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								text(&p.update, "Update to Match", sel == nil),
								text(&p.del, "Delete", !canDelete),
							)
						}),
					)
				})
			}),
		)
	})
}

// paneList lays out a scrolling list of n rows.
func paneList(gtx layout.Context, th *gui.Theme, l *widget.List, n int, row func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	l.Axis = layout.Vertical
	return l.List.Layout(gtx, n, row)
}

// newStyleDialog is New Style (Create New Style from Formatting): name,
// type, based on and the style for the following paragraph; the style
// takes the selection's formatting and is applied to it.
type newStyleDialog struct {
	open                 bool
	name                 widget.Editor
	typ                  model.StyleType
	basedOn, next        string
	typeDD, baseDD, nxDD gui.DropDown
	ok, cancel           widget.Clickable
	tag                  int
	err                  string
	focus                bool
}

func (s *newStyleDialog) openFor(d *window) {
	s.open, s.typ, s.err, s.focus = true, model.ParagraphStyle, "", true
	s.basedOn = d.ed.ParaStyleID(d.ed.Sel.Caret.Para)
	s.next = ""
	n := 1
	for d.doc.Styles.FindStyleByName("Style"+strconv.Itoa(n)) != nil {
		n++
	}
	s.name.SetText("Style" + strconv.Itoa(n))
	s.name.SetCaret(s.name.Len(), 0)
	d.menu.Close()
}

func (s *newStyleDialog) layout(gtx layout.Context, d *window) {
	th := d.th
	if s.focus {
		if gtx.Focused(&s.name) {
			s.focus = false
		} else {
			gtx.Execute(key.FocusCmd{Tag: &s.name})
		}
	}
	submit := false
	for {
		ev, ok := s.name.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submit = true
		}
	}
	for s.ok.Clicked(gtx) {
		submit = true
	}
	if submit {
		id, err := d.ed.NewStyle(model.StyleSpec{Name: s.name.Text(), Type: s.typ, BasedOn: s.basedOn, Next: s.next})
		if err != nil {
			s.err = err.Error()
			s.err = strings.ToUpper(s.err[:1]) + s.err[1:] + "."
		} else {
			s.open = false
			d.pane.selected = id
			d.edited()
		}
	}
	for s.cancel.Clicked(gtx) {
		s.open = false
		d.v.focusPending = true
	}
	if !s.open {
		return
	}
	s.name.Submit = true

	// Choices.
	var bases []*model.Style
	for _, st := range paneStyles(d.doc) {
		if st.Type == s.typ {
			bases = append(bases, st)
		}
	}
	baseItems := []gui.DropItem{}
	baseIDs := []string{}
	if s.typ == model.CharacterStyle {
		baseItems = append(baseItems, gui.DropItem{Label: "(underlying properties)"})
		baseIDs = append(baseIDs, "")
	}
	var nextItems []gui.DropItem
	nextIDs := []string{""}
	nextItems = append(nextItems, gui.DropItem{Label: "(this style)"})
	for _, st := range bases {
		m := d.styleSample(st, 13)
		it := gui.DropItem{Label: styleName(st), Font: m.font, Color: m.color}
		baseItems = append(baseItems, it)
		baseIDs = append(baseIDs, st.ID)
		if s.typ == model.ParagraphStyle {
			nextItems = append(nextItems, it)
			nextIDs = append(nextIDs, st.ID)
		}
	}
	nameOf := func(id, none string) string {
		if st := d.doc.Styles.Find(id); st != nil && id != "" {
			return styleName(st)
		}
		return none
	}
	typeName := "Paragraph"
	if s.typ == model.CharacterStyle {
		typeName = "Character"
	}

	form := gui.Form{Th: th, LabelW: 180, LabelColor: gui.MutedText, RowH: 28}
	label, row, combo := form.Label, form.Row, form.Combo
	gui.Modal(gtx, &s.tag, 500, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(th.Label(18, "Create New Style from Formatting", gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 14}.Layout),
			row(label("Name"), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.textBox(gtx, &s.name, 240, "")
			})),
			row(label("Style type"), combo(&s.typeDD, 240, typeName, []gui.DropItem{{Label: "Paragraph"}, {Label: "Character"}}, func(i int) {
				s.typ = model.ParagraphStyle
				s.basedOn = d.ed.ParaStyleID(d.ed.Sel.Caret.Para)
				if i == 1 {
					s.typ, s.basedOn = model.CharacterStyle, ""
				}
			})),
			row(label("Style based on"), combo(&s.baseDD, 240, nameOf(s.basedOn, "(underlying properties)"), baseItems, func(i int) { s.basedOn = baseIDs[i] })),
		}
		if s.typ == model.ParagraphStyle {
			children = append(children, row(label("Style for following paragraph"),
				combo(&s.nxDD, 240, nameOf(s.next, "(this style)"), nextItems, func(i int) { s.next = nextIDs[i] })))
		}
		children = append(children,
			layout.Rigid(th.Paragraph("The new style takes the formatting of the selection and is applied to it.", gui.MutedText)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.err == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 8}.Layout(gtx, th.Label(13, s.err, gui.ErrorText, font.Normal).Layout)
			}),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &s.ok, "OK") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &s.cancel, "Cancel") }),
				)
			}),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
