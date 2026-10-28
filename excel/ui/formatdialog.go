package ui

import (
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
)

// The Format Cells dialog (Ctrl+1) and the small Row Height / Column
// Width / Unhide prompts.

type fmtTab int

const (
	tabNumber fmtTab = iota
	tabAlignment
	tabFont
	tabBorder
	tabFill
)

var tabNames = []string{"Number", "Alignment", "Font", "Border", "Fill"}

var hAlignNames = []string{"General", "Left", "Center", "Right", "Fill", "Justify", "Center Across Selection", "Distributed"}

// vAlignChoices lists vertical alignments in menu order.
var vAlignChoices = []struct {
	name string
	v    model.VAlign
}{
	{"Top", model.VAlignTop}, {"Center", model.VAlignCenter}, {"Bottom", model.VAlignBottom},
	{"Justify", model.VAlignJustify}, {"Distributed", model.VAlignDistributed},
}

// formatDialog is the Format Cells dialog's state: the active cell's
// format when it opened and as edited. Only what was changed is applied
// to the selection.
type formatDialog struct {
	open       bool
	focus      bool
	tab        fmtTab
	tabs       [5]widget.Clickable
	orig, cur  model.Style
	border     int // a BorderKind, or -1 for no change
	key        int // Enter (1) or Esc (-1) pressed, from dialogInput
	ok, cancel widget.Clickable

	numDD, hDD, vDD, fontDD, sizeDD, colorDD, borderDD, fillDD gui.DropDown
	decLess, decMore, indLess, indMore                         widget.Clickable
	wrap, bold, italic, under, strike                          widget.Bool
	block                                                      int // absorbs clicks outside the box
}

func (a *App) openFormatCells(tab fmtTab) {
	if !a.readyToChange() {
		return
	}
	d := &a.fmtDlg
	st := a.activeStyle()
	*d = formatDialog{open: true, focus: true, tab: tab, orig: st, cur: st, border: -1}
}

// applyFormatCells applies what the dialog changed to the selection.
func (a *App) applyFormatCells() {
	d := &a.fmtDlg
	o, c := d.orig, d.cur
	a.ensureFont(c.Font.Family)
	sh := a.wb.Sheets[a.active]
	last := a.book.LastChange()
	a.formatAt(func(st *model.Style, row, col int) {
		if c.Font.Family != o.Font.Family {
			st.Font.Family = c.Font.Family
		}
		if c.Font.SizePt != o.Font.SizePt {
			st.Font.SizePt = c.Font.SizePt
		}
		if c.Font.Bold != o.Font.Bold {
			st.Font.Bold = c.Font.Bold
		}
		if c.Font.Italic != o.Font.Italic {
			st.Font.Italic = c.Font.Italic
		}
		if c.Font.Underline != o.Font.Underline {
			st.Font.Underline = c.Font.Underline
		}
		if c.Font.Strike != o.Font.Strike {
			st.Font.Strike = c.Font.Strike
		}
		if c.Font.Color != o.Font.Color {
			st.Font.Color = c.Font.Color
		}
		if c.Fill != o.Fill {
			st.Fill = c.Fill
		}
		if c.HAlign != o.HAlign {
			st.HAlign = c.HAlign
		}
		if c.VAlign != o.VAlign {
			st.VAlign = c.VAlign
		}
		if c.Wrap != o.Wrap {
			st.Wrap = c.Wrap
		}
		if c.Indent != o.Indent {
			st.Indent = c.Indent
		}
		if c.NumFmtID != o.NumFmtID || c.NumFmtCode != o.NumFmtCode {
			st.NumFmtID, st.NumFmtCode = c.NumFmtID, c.NumFmtCode
		}
		if d.border >= 0 {
			area, _ := edit.FormatArea(sh, a.fmtRange) // the range being formatted
			edit.ApplyBorder(st, edit.BorderKind(d.border), area, row, col)
		}
	})
	if a.book.LastChange() != last {
		a.growRows()
	}
}

// modal draws a dialog box centered over the window, dimming and
// blocking what is behind it.
func (a *App) modal(gtx layout.Context, block any, title string, width unit.Dp, body layout.Widget, buttons ...layout.Widget) {
	size := gtx.Constraints.Max
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, block)
	area.Pop()
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: block, Kinds: pointer.Press}); !ok {
			break
		}
	}
	gui.FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.RGBA(0, 0, 0, 0x30))
	w := min(gtx.Dp(width), size.X-gtx.Dp(16))
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, size.Y)}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(18).Layout(g, func(gtx layout.Context) layout.Dimensions {
		btns := []layout.FlexChild{layout.Flexed(1, layout.Spacer{}.Layout)}
		for _, b := range buttons {
			btns = append(btns, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 8}.Layout(gtx, b)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.ui.Label(16, title, chromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(body),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, btns...)
			}),
		)
	})
	call := m.Stop()
	pos := image.Pt((size.X-dims.Size.X)/2, max((size.Y-dims.Size.Y)/4, 0))
	off := op.Offset(pos).Push(gtx.Ops)
	box := image.Rectangle{Max: dims.Size}
	gui.FillRect(gtx.Ops, box.Add(image.Pt(3, 3)), colors.RGBA(0, 0, 0, 0x28))
	gui.FillRect(gtx.Ops, box.Inset(-1), chromeLine)
	gui.FillRect(gtx.Ops, box, colors.White)
	call.Add(gtx.Ops)
	off.Pop()
}

// plainButton is a dialog's secondary button.
func (a *App) plainButton(btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return gui.Hoverable(gtx, btn, chromeBG, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 18, Right: 18}.Layout(gtx, a.ui.Label(14, label, chromeText, font.Normal).Layout)
		})
	}
}

func (a *App) accentButton(btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return a.ui.Button(gtx, btn, label) }
}

// dialogKeys handles Enter and Esc for a dialog whose tag has the focus.
// It returns +1 for Enter, -1 for Esc, 0 otherwise.
func dialogKeys(gtx layout.Context, tag any) int {
	res := 0
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Name: key.NameEscape},
			key.Filter{Focus: tag, Name: key.NameReturn},
			key.Filter{Focus: tag, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			if e.Name == key.NameEscape {
				res = -1
			} else {
				res = 1
			}
		}
	}
	return res
}

// dialogInput takes the open dialog's Enter and Esc before the window's
// shortcuts do (the first handler asking for a key gets it).
func (a *App) dialogInput(gtx layout.Context) {
	switch {
	case a.fmtDlg.open:
		if k := dialogKeys(gtx, &a.fmtDlg); k != 0 {
			a.fmtDlg.key = k
		}
	case a.askDlg.open:
		if k := dialogKeys(gtx, a.askDlg.tag()); k != 0 {
			a.askDlg.key = k
		}
	}
}

// layoutDialogs draws whichever of this file's dialogs is open.
func (a *App) layoutDialogs(gtx layout.Context) {
	if a.fmtDlg.open {
		a.layoutFormatCells(gtx)
	}
	if a.askDlg.open {
		a.layoutAsk(gtx)
	}
}

func (a *App) layoutFormatCells(gtx layout.Context) {
	d := &a.fmtDlg
	th := a.ui
	if d.focus {
		d.focus = false
		gtx.Execute(key.FocusCmd{Tag: d})
	}
	// Polling here too registers the keys in the frame the dialog opens,
	// which the focus needs.
	done := d.key + dialogKeys(gtx, d)
	d.key = 0
	for d.ok.Clicked(gtx) {
		done = 1
	}
	for d.cancel.Clicked(gtx) {
		done = -1
	}
	if done != 0 {
		d.open = false
		if done > 0 {
			a.applyFormatCells()
		}
		a.grid.focusPending = true
		return
	}
	for i := range d.tabs {
		for d.tabs[i].Clicked(gtx) {
			d.tab = fmtTab(i)
			gtx.Execute(key.FocusCmd{Tag: d})
		}
	}
	c := &d.cur

	label := func(s string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(130)
			return layout.Inset{Top: 4}.Layout(gtx, th.Label(13, s, chromeText, font.Normal).Layout)
		}
	}
	field := func(name string, w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(label(name)), layout.Rigid(w))
			})
		})
	}
	combo := func(dd *gui.DropDown, cur string, items []gui.DropItem, apply func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if i := dd.Layout(gtx, th, 200, cur, items); i >= 0 {
				apply(i)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(200), gtx.Dp(24))}
		}
	}
	small := func(b *widget.Clickable, s string, f func()) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			for b.Clicked(gtx) {
				f()
			}
			return gui.ToolButton(gtx, th, b, nil, s, false, color.NRGBA{})
		}
	}
	check := func(b *widget.Bool, v *bool, s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b.Value = *v
			if b.Update(gtx) {
				*v = b.Value
			}
			cb := material.CheckBox(th.Theme, b, s)
			cb.Font = gui.UIFont
			cb.TextSize = 13
			cb.IconColor = th.Accent
			return layout.Inset{Bottom: 4}.Layout(gtx, cb.Layout)
		})
	}
	text := func(s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 8}.Layout(gtx, th.Label(12, s, mutedText, font.Normal).Layout)
		})
	}

	var page []layout.FlexChild
	switch d.tab {
	case tabNumber:
		names := make([]string, len(edit.NumberFormats))
		for i, f := range edit.NumberFormats {
			names[i] = f.Name
		}
		page = append(page,
			field("Category:", combo(&d.numDD, edit.NumberFormatName(c.NumFmtID, c.NumFmtCode), menuItems(names), func(i int) {
				f := edit.NumberFormats[i]
				c.NumFmtID, c.NumFmtCode = f.ID, f.Code
			})),
			field("Decimal places:", func(gtx layout.Context) layout.Dimensions {
				step := func(n int) {
					if code, ok := edit.StepDecimals(c.NumFmtID, c.NumFmtCode, "", n); ok {
						c.NumFmtID, c.NumFmtCode = 0, code
					}
				}
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(small(&d.decLess, "−", func() { step(-1) })),
					layout.Rigid(small(&d.decMore, "+", func() { step(1) })),
				)
			}),
		)
		code := edit.FormatCode(c.NumFmtID, c.NumFmtCode)
		if code == "" {
			code = edit.NumberFormatName(c.NumFmtID, c.NumFmtCode)
		}
		page = append(page, text("Format code: "+code))
	case tabAlignment:
		vName := "Bottom"
		for _, v := range vAlignChoices {
			if v.v == c.VAlign {
				vName = v.name
			}
		}
		vNames := make([]string, len(vAlignChoices))
		for i, v := range vAlignChoices {
			vNames[i] = v.name
		}
		page = append(page,
			field("Horizontal:", combo(&d.hDD, hAlignNames[min(int(c.HAlign), len(hAlignNames)-1)], menuItems(hAlignNames), func(i int) {
				c.HAlign = model.HAlign(i)
			})),
			field("Vertical:", combo(&d.vDD, vName, menuItems(vNames), func(i int) { c.VAlign = vAlignChoices[i].v })),
			field("Indent:", func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(small(&d.indLess, "−", func() { c.Indent = max(c.Indent-1, 0) })),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(30)
						return layout.Center.Layout(gtx, th.Label(13, strconv.Itoa(c.Indent), chromeText, font.Normal).Layout)
					}),
					layout.Rigid(small(&d.indMore, "+", func() { c.Indent = min(c.Indent+1, 15) })),
				)
			}),
			check(&d.wrap, &c.Wrap, "Wrap text"),
		)
	case tabFont:
		fonts := slices.Clone(commonFonts)
		if !slices.Contains(fonts, c.Font.Family) {
			fonts = append(fonts, c.Font.Family)
			slices.Sort(fonts)
		}
		sizes := make([]string, len(fontSizes))
		for i, s := range fontSizes {
			sizes[i] = fmtSize(s)
		}
		colorName := "Custom"
		for i, p := range palette {
			if i > 0 && p.c == c.Font.Color {
				colorName = p.name
			}
		}
		if c.Font.Color == a.wb.Styles[0].Font.Color {
			colorName = "Automatic"
		}
		page = append(page,
			field("Font:", combo(&d.fontDD, c.Font.Family, menuItems(fonts), func(i int) { c.Font.Family = fonts[i] })),
			field("Size:", combo(&d.sizeDD, fmtSize(c.Font.SizePt), menuItems(sizes), func(i int) { c.Font.SizePt = fontSizes[i] })),
			field("Color:", combo(&d.colorDD, colorName, colorItems("Automatic"), func(i int) {
				c.Font.Color = palette[i].c
				if i == 0 {
					c.Font.Color = a.wb.Styles[0].Font.Color
				}
			})),
			check(&d.bold, &c.Font.Bold, "Bold"),
			check(&d.italic, &c.Font.Italic, "Italic"),
			check(&d.under, &c.Font.Underline, "Underline"),
			check(&d.strike, &c.Font.Strike, "Strikethrough"),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Preview in the chosen font (when loaded).
				l := material.Label(th.Theme, unit.Sp(min(max(c.Font.SizePt, 8), 28)), "AaBbCcYyZz")
				l.Font = font.Font{Typeface: gui.Typeface(c.Font.Family)}
				if c.Font.Bold {
					l.Font.Weight = font.Bold
				}
				if c.Font.Italic {
					l.Font.Style = font.Italic
				}
				l.Color = c.Font.Color
				return layout.Inset{Top: 6}.Layout(gtx, l.Layout)
			}),
		)
	case tabBorder:
		cur := "(no change)"
		if d.border >= 0 {
			cur = edit.BorderNames[d.border]
		}
		page = append(page,
			field("Border:", combo(&d.borderDD, cur, menuItems(edit.BorderNames), func(i int) { d.border = i })),
			text("The border is applied to the selected cells as a whole."),
		)
	case tabFill:
		name := "Custom"
		for i, p := range palette {
			if p.c == c.Fill {
				name = p.name
				if i == 0 {
					name = "No Fill"
				}
			}
		}
		page = append(page,
			field("Background color:", combo(&d.fillDD, name, colorItems("No Fill"), func(i int) { c.Fill = palette[i].c })),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				r := image.Rect(0, 0, gtx.Dp(200), gtx.Dp(40))
				gui.FillRect(gtx.Ops, r, colors.White)
				if c.Fill.A > 0 {
					gui.FillRect(gtx.Ops, r, c.Fill)
				}
				gui.HLine(gtx.Ops, 0, r.Max.X, 0, 1, chromeLine)
				gui.HLine(gtx.Ops, 0, r.Max.X, r.Max.Y-1, 1, chromeLine)
				return layout.Dimensions{Size: r.Max}
			}),
		)
	}

	body := func(gtx layout.Context) layout.Dimensions {
		var tabs []layout.FlexChild
		for i, n := range tabNames {
			tabs = append(tabs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx, th, &d.tabs[i], nil, n, d.tab == fmtTab(i), color.NRGBA{})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Flex{}.Layout(gtx, tabs...) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gui.HLine(gtx.Ops, 0, gtx.Constraints.Max.X, 2, 1, chromeLine)
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(14))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(230)
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, page...)
			}),
		)
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, d)
	area.Pop()
	a.modal(gtx, &d.block, "Format Cells", 440, body, a.accentButton(&d.ok, "OK"), a.plainButton(&d.cancel, "Cancel"))
}

// askDialog is a one-value prompt: a number (Row Height, Column Width)
// or a choice from a list (Unhide sheet).
type askDialog struct {
	open       bool
	focus      bool
	title      string
	prompt     string
	choices    []string // a list instead of a number
	choice     int
	dd         gui.DropDown
	ed         widget.Editor
	ok, cancel widget.Clickable
	number     func(float64) bool // returns false for a value out of range
	chosen     func(int)
	block      int
	bad        bool
	key        int // Enter (1) or Esc (-1) pressed, from dialogInput
}

// tag is what has the keyboard: the number box, or the dialog.
func (d *askDialog) tag() any {
	if d.choices == nil {
		return &d.ed
	}
	return d
}

// ask prompts for a number, starting from value.
func (a *App) ask(title, prompt, value string, f func(float64) bool) {
	d := &a.askDlg
	*d = askDialog{open: true, focus: true, title: title, prompt: prompt, number: f}
	d.ed.SingleLine, d.ed.Submit = true, true
	d.ed.SetText(value)
	d.ed.SetCaret(len(value), 0)
}

// choose prompts for one of choices.
func (a *App) choose(title, prompt string, choices []string, f func(int)) {
	d := &a.askDlg
	*d = askDialog{open: true, focus: true, title: title, prompt: prompt, choices: choices, chosen: f}
}

func (a *App) layoutAsk(gtx layout.Context) {
	d := &a.askDlg
	th := a.ui
	if d.focus {
		d.focus = false
		gtx.Execute(key.FocusCmd{Tag: d.tag()})
	}
	done := d.key + dialogKeys(gtx, d.tag())
	d.key = 0
	for {
		ev, ok := d.ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			done = 1
		}
	}
	for d.ok.Clicked(gtx) {
		done = 1
	}
	for d.cancel.Clicked(gtx) {
		done = -1
	}
	if done < 0 {
		d.open = false
		a.grid.focusPending = true
		return
	}
	if done > 0 {
		if d.choices != nil {
			d.open = false
			d.chosen(d.choice)
			a.grid.focusPending = true
			return
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(d.ed.Text()), 64)
		if err == nil && d.number(x) {
			d.open = false
			a.grid.focusPending = true
			return
		}
		d.bad = true
	}
	body := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(th.Label(13, d.prompt, chromeText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
		}
		if d.choices != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if i := d.dd.Layout(gtx, th, 240, d.choices[d.choice], menuItems(d.choices)); i >= 0 {
					d.choice = i
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(240), gtx.Dp(24))}
			}))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w, h := gtx.Dp(120), gtx.Dp(26)
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
				gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
				g := gtx
				g.Constraints = layout.Constraints{Min: image.Pt(w-12, 0), Max: image.Pt(w-12, h)}
				off := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(4))).Push(gtx.Ops)
				e := material.Editor(th.Theme, &d.ed, "")
				e.Font = gui.UIFont
				e.TextSize = 13
				e.Layout(g)
				off.Pop()
				return layout.Dimensions{Size: image.Pt(w, h)}
			}))
			if d.bad {
				children = append(children, layout.Rigid(th.Label(12, "Enter a number in the allowed range.", errorText, font.Normal).Layout))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	if d.choices != nil {
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		event.Op(gtx.Ops, d)
		area.Pop()
	}
	a.modal(gtx, &d.block, d.title, 320, body, a.accentButton(&d.ok, "OK"), a.plainButton(&d.cancel, "Cancel"))
}
