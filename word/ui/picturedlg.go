// The picture dialogs of the right-click menu, as Word's: Size and
// Position (Word's Layout dialog: Position, Text Wrapping and Size tabs)
// and Format Picture (Fill & Line, Layout & Properties with the
// alternative text, and Picture with the crop). Each applies all its
// changes as one undoable step, to the picture selected when it opened,
// inline or floating.

package ui

import (
	"math"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// The picture dialogs.
const (
	picLayoutDlg = iota // Size and Position
	picFormatDlg        // Format Picture
)

// The Size and Position dialog's tabs, in Word's order.
const (
	tabPosition = iota
	tabWrapping
	tabSize
)

// The Format Picture dialog's tabs.
const (
	tabFillLine = iota
	tabProperties
	tabPicture
)

// layoutTabs and formatTabs are the dialogs' tab names.
var (
	layoutTabs = []string{"Position", "Text Wrapping", "Size"}
	formatTabs = []string{"Fill & Line", "Layout & Properties", "Picture"}
)

// wrapChoices are the Text Wrapping tab's styles: the radio key, Word's
// label and the model's wrap (inline has none).
var wrapChoices = []struct {
	key, label string
	wrap       model.FloatWrap
}{
	{"inline", "In line with text", 0},
	{"square", "Square", model.WrapSquare},
	{"tight", "Tight", model.WrapTight},
	{"topbottom", "Top and bottom", model.WrapTopBottom},
	{"behind", "Behind text", model.WrapBehind},
	{"front", "In front of text", model.WrapNone},
}

// relChoice is a reference a position is measured from, as the Position
// tab names it.
type relChoice struct {
	label string
	rel   model.FloatRel
}

// The references each of the Position tab's lists offers, as Word's.
var (
	hRels      = []relChoice{{"Column", model.RelColumn}, {"Margin", model.RelMargin}, {"Page", model.RelPage}, {"Character", model.RelCharacter}}
	vAlignRels = []relChoice{{"Margin", model.RelMargin}, {"Page", model.RelPage}, {"Line", model.RelLine}}
	vAbsRels   = []relChoice{{"Paragraph", model.RelParagraph}, {"Margin", model.RelMargin}, {"Page", model.RelPage}, {"Line", model.RelLine}}
	hAligns    = []string{"Left", "Centered", "Right"}
	vAligns    = []string{"Top", "Centered", "Bottom"}
)

// alignKeys are the model's names for the alignment lists' entries.
var (
	hAlignKeys = []string{"left", "center", "right"}
	vAlignKeys = []string{"top", "center", "bottom"}
)

// pictureDialog is the state of the open picture dialog.
type pictureDialog struct {
	open bool
	kind int
	tab  int
	tabs [3]widget.Clickable
	tag  int
	pos  model.Pos // the picture: the character after pos
	err  string

	ok, cancel, reset widget.Clickable

	// Size tab. startW and startH are the size when the dialog opened,
	// origW and origH the picture's own; the scales are of one or the
	// other as relOriginal says.
	height, width, scaleH, scaleW widget.Editor
	lock, relOriginal             widget.Bool
	startW, startH, origW, origH  float64

	// Text Wrapping tab.
	wrap                       widget.Enum
	distT, distB, distL, distR widget.Editor
	hMode, vMode               widget.Enum // "align" or "abs"
	hAlign, vAlign             int         // index in hAligns, vAligns
	hAlignRel, hAbsRel         int         // index in hRels
	vAlignRel                  int         // index in vAlignRels
	vAbsRel                    int         // index in vAbsRels
	hAbs, vAbs                 widget.Editor
	hAlignDD, hAlignRelDD      gui.DropDown
	hAbsRelDD, vAlignDD        gui.DropDown
	vAlignRelDD, vAbsRelDD     gui.DropDown
	floatX, floatY             float64 // a floating picture's offsets, or an inline one's if it floated
	inlineAtOpen               bool
	cropL, cropT, cropR, cropB widget.Editor
	line                       widget.Enum // "none" or "solid"
	lineColor                  gui.ColorPicker
	lineChoice                 gui.ColorChoice
	lineWidth                  widget.Editor
	alt                        widget.Editor
	decorative                 widget.Bool
	fullW, fullH               float64 // the whole image at the picture's scale, before cropping

	// shown is what each box showed when the dialog opened: a value
	// shown rounded and left alone is kept as it was, not rounded.
	shown map[*widget.Editor]string
	// startH... are the position settings at open, kept the same way.
	startHMode, startVMode string
	startHRel, startVRel   int
}

// edited reports whether ed's text differs from what it showed at open.
func (p *pictureDialog) edited(ed *widget.Editor) bool { return ed.Text() != p.shown[ed] }

// openPictureDialog opens dialog kind (picLayoutDlg or picFormatDlg) at
// tab for the selected picture, filled in from it.
func (d *window) openPictureDialog(kind, tab int) {
	r := d.ed.SelectedPictureObj()
	if r == nil {
		return
	}
	p := &d.pic
	a, _ := d.ed.Sel.Ordered()
	*p = pictureDialog{open: true, kind: kind, tab: tab, pos: a,
		lineColor: p.lineColor} // keep the color picker's recent colors
	o := r.Opaque
	w, h := model.PictureSize(o)
	p.startW, p.startH = w, h
	p.origW, p.origH = w, h
	look := *model.PictureLookOf(o)
	if nw, nh, ok := model.PictureImage(o).NaturalSize(); ok {
		// The natural size is of the whole image; the crop shows part of it.
		p.origW, p.origH = nw*(1-look.Crop[0]-look.Crop[2]), nh*(1-look.Crop[1]-look.Crop[3])
	}
	for _, ed := range []*widget.Editor{&p.height, &p.width, &p.scaleH, &p.scaleW, &p.distT, &p.distB, &p.distL, &p.distR,
		&p.hAbs, &p.vAbs, &p.cropL, &p.cropT, &p.cropR, &p.cropB, &p.lineWidth} {
		ed.SingleLine, ed.Submit = true, true
	}
	p.lock.Value, p.relOriginal.Value = true, true
	p.height.SetText(gui.FormatInches(h))
	p.width.SetText(gui.FormatInches(w))
	p.syncScales()

	// Text Wrapping and Position: an inline picture is shown at the
	// offsets it would float at, from its column and paragraph.
	p.wrap.Value = "inline"
	dist := model.FloatDistance
	p.hMode.Value, p.vMode.Value = "abs", "abs"
	if f := o.Float; f != nil {
		for _, c := range wrapChoices[1:] {
			if c.wrap == f.Wrap {
				p.wrap.Value = c.key
			}
		}
		dist = f.Distance
		p.floatX, p.floatY = f.X, f.Y
		p.hAbsRel, p.hAlignRel = relIndex(hRels, f.RelX), relIndex(hRels, f.RelX)
		p.vAbsRel, p.vAlignRel = relIndex(vAbsRels, f.RelY), relIndex(vAlignRels, f.RelY)
		if i := alignIndex(hAlignKeys, f.AlignX); i >= 0 {
			p.hMode.Value, p.hAlign = "align", i
		}
		if i := alignIndex(vAlignKeys, f.AlignY); i >= 0 {
			p.vMode.Value, p.vAlign = "align", i
		}
	} else {
		p.inlineAtOpen = true
		if pg, box, ok := d.ceng().PictureRect(a); ok {
			if opg, x0, y0, ok := d.ceng().FloatOrigin(a); ok && opg == pg {
				p.floatX, p.floatY = box.X0-x0, box.Y0-y0
			}
		}
	}
	p.distT.SetText(gui.FormatInches(dist[1]))
	p.distB.SetText(gui.FormatInches(dist[3]))
	p.distL.SetText(gui.FormatInches(dist[0]))
	p.distR.SetText(gui.FormatInches(dist[2]))
	p.hAbs.SetText(gui.FormatInches(p.floatX))
	p.vAbs.SetText(gui.FormatInches(p.floatY))

	// Format Picture.
	color, lw := model.PictureLine(o)
	p.line.Value = "none"
	p.lineWidth.SetText(strconv.FormatFloat(math.Max(*lw, 0.75), 'f', -1, 64) + " pt")
	p.lineChoice = gui.ColorChoice{Color: colors.RGB(0x000000)}
	if c, ok := colors.ParseHex(*color); ok {
		p.line.Value, p.lineChoice = "solid", gui.ColorChoice{Color: c}
	}
	p.alt.SetText(look.Alt)
	p.decorative.Value = look.Decorative
	fx, fy := 1-look.Crop[0]-look.Crop[2], 1-look.Crop[1]-look.Crop[3]
	p.fullW, p.fullH = w/max(fx, 0.01), h/max(fy, 0.01)
	p.cropL.SetText(gui.FormatInches(look.Crop[0] * p.fullW))
	p.cropT.SetText(gui.FormatInches(look.Crop[1] * p.fullH))
	p.cropR.SetText(gui.FormatInches(look.Crop[2] * p.fullW))
	p.cropB.SetText(gui.FormatInches(look.Crop[3] * p.fullH))
	p.shown = map[*widget.Editor]string{}
	for _, ed := range []*widget.Editor{&p.height, &p.width, &p.hAbs, &p.vAbs, &p.distT, &p.distB, &p.distL, &p.distR,
		&p.cropL, &p.cropT, &p.cropR, &p.cropB} {
		p.shown[ed] = ed.Text()
	}
	p.startHMode, p.startVMode, p.startHRel, p.startVRel = p.hMode.Value, p.vMode.Value, p.hAbsRel, p.vAbsRel
}

// relIndex is the index of rel in list, or 0 (the list's default) when
// it isn't there (a paragraph-relative horizontal position counts as the
// column, as Word shows it).
func relIndex(list []relChoice, rel model.FloatRel) int {
	for i, c := range list {
		if c.rel == rel {
			return i
		}
	}
	return 0
}

// alignIndex is the index of align in keys ("inside" counts as left,
// "outside" as right), or -1 for none.
func alignIndex(keys []string, align string) int {
	switch align {
	case "inside":
		align = "left"
	case "outside":
		align = "right"
	}
	for i, k := range keys {
		if k == align {
			return i
		}
	}
	return -1
}

// scaleBase is the size the Size tab's scales are percentages of.
func (p *pictureDialog) scaleBase() (float64, float64) {
	if p.relOriginal.Value {
		return p.origW, p.origH
	}
	return p.startW, p.startH
}

// syncScales sets the scale boxes from the height and width boxes.
func (p *pictureDialog) syncScales() {
	bw, bh := p.scaleBase()
	if h, ok := gui.ParseLength(p.height.Text()); ok && bh > 0 {
		p.scaleH.SetText(strconv.Itoa(int(math.Round(h/bh*100))) + " %")
	}
	if w, ok := gui.ParseLength(p.width.Text()); ok && bw > 0 {
		p.scaleW.SetText(strconv.Itoa(int(math.Round(w/bw*100))) + " %")
	}
}

// parsePercent reads a typed percentage ("120", "120 %").
func parsePercent(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), 64)
	return v, err == nil
}

// editorEvents reports whether ed's text was changed by typing and
// whether Enter was pressed in it since the last frame.
func editorEvents(gtx layout.Context, ed *widget.Editor) (changed, submit bool) {
	for {
		ev, ok := ed.Update(gtx)
		if !ok {
			return
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			changed = true
		case widget.SubmitEvent:
			submit = true
		}
	}
}

// update keeps the Size tab's boxes in step as one is typed in: with the
// aspect ratio locked, height and width follow each other, and the
// scales follow the sizes (and the sizes the scales). It reports whether
// Enter was pressed in any of the dialog's boxes.
func (p *pictureDialog) update(gtx layout.Context) bool {
	submit := false
	ratio := 1.0
	if p.startH > 0 {
		ratio = p.startW / p.startH
	}
	bw, bh := p.scaleBase()
	ch, s1 := editorEvents(gtx, &p.height)
	cw, s2 := editorEvents(gtx, &p.width)
	csh, s3 := editorEvents(gtx, &p.scaleH)
	csw, s4 := editorEvents(gtx, &p.scaleW)
	submit = s1 || s2 || s3 || s4
	switch {
	case ch:
		if h, ok := gui.ParseLength(p.height.Text()); ok && p.lock.Value {
			p.width.SetText(gui.FormatInches(h * ratio))
		}
		p.syncScales()
	case cw:
		if w, ok := gui.ParseLength(p.width.Text()); ok && p.lock.Value && ratio > 0 {
			p.height.SetText(gui.FormatInches(w / ratio))
		}
		p.syncScales()
	case csh:
		if v, ok := parsePercent(p.scaleH.Text()); ok {
			p.height.SetText(gui.FormatInches(bh * v / 100))
			if p.lock.Value {
				p.width.SetText(gui.FormatInches(bh * v / 100 * ratio))
			}
			p.syncScales()
		}
	case csw:
		if v, ok := parsePercent(p.scaleW.Text()); ok {
			p.width.SetText(gui.FormatInches(bw * v / 100))
			if p.lock.Value && ratio > 0 {
				p.height.SetText(gui.FormatInches(bw * v / 100 / ratio))
			}
			p.syncScales()
		}
	}
	if p.relOriginal.Update(gtx) {
		p.syncScales()
	}
	for _, ed := range []*widget.Editor{&p.distT, &p.distB, &p.distL, &p.distR, &p.hAbs, &p.vAbs,
		&p.cropL, &p.cropT, &p.cropR, &p.cropB, &p.lineWidth} {
		if _, s := editorEvents(gtx, ed); s {
			submit = true
		}
	}
	return submit
}

// layout draws the open picture dialog and applies or drops it.
func (p *pictureDialog) layout(gtx layout.Context, d *window) {
	if d.ed.PictureObjAt(p.pos) == nil {
		p.open = false // the picture is gone (undone, say)
		return
	}
	for i := range p.tabs {
		for p.tabs[i].Clicked(gtx) {
			p.tab = i
		}
	}
	for p.reset.Clicked(gtx) {
		p.height.SetText(gui.FormatInches(p.origH))
		p.width.SetText(gui.FormatInches(p.origW))
		p.syncScales()
	}
	submit := p.update(gtx)
	for p.ok.Clicked(gtx) {
		submit = true
	}
	if submit {
		if err := p.apply(d); err != "" {
			p.err = err
		} else {
			p.open = false
			d.v.focusPending = true
			return
		}
	}
	for p.cancel.Clicked(gtx) {
		p.open = false
		d.v.focusPending = true
		return
	}
	th := d.th
	title, names := "Layout", layoutTabs
	if p.kind == picFormatDlg {
		title, names = "Format Picture", formatTabs
	}
	gui.Modal(gtx, &p.tag, 560, func(gtx layout.Context) layout.Dimensions {
		var tabs []layout.FlexChild
		for i, n := range names {
			tabs = append(tabs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx, th, &p.tabs[i], nil, n, p.tab == i, colors.Transparent)
			}))
		}
		kids := []layout.FlexChild{dialogTitle(th, title),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Flex{}.Layout(gtx, tabs...) }),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(300)
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, p.page(th)...)
			}),
		}
		if p.err != "" {
			kids = append(kids, layout.Rigid(th.Label(12, p.err, gui.ErrorText, font.Normal).Layout))
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &p.ok, "OK") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &p.cancel, "Cancel") }),
				)
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// picHeading is a section heading inside a tab.
func picHeading(th *gui.Theme, s string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 4, Bottom: 6}.Layout(gtx, th.Label(13, s, gui.ChromeText, font.SemiBold).Layout)
	})
}

// picBox is a bordered single-line text box w wide.
func picBox(th *gui.Theme, ed *widget.Editor, w unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gui.TextField(gtx, th, ed, w, 1, "")
	})
}

// picGap is a horizontal space between controls.
func picGap(w unit.Dp) layout.FlexChild { return layout.Rigid(layout.Spacer{Width: w}.Layout) }

// picLabel is the label of a row's second control, a fixed width so the
// controls after it line up from row to row.
func picLabel(th *gui.Theme, s string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(64)
		return gui.Middle(gtx, th.Label(13, s, gui.ChromeText, font.Normal).Layout)
	})
}

// picText is a plain label in a row.
func picText(th *gui.Theme, s string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gui.Middle(gtx, th.Label(13, s, gui.ChromeText, font.Normal).Layout)
	})
}

// floats reports whether the dialog's Text Wrapping choice makes the
// picture float.
func (p *pictureDialog) floats() bool { return p.wrap.Value != "inline" }

// page returns the rows of the current tab.
func (p *pictureDialog) page(th *gui.Theme) []layout.FlexChild {
	f := gui.Form{Th: th, LabelW: 150, RowH: 28}
	if p.kind == picFormatDlg {
		switch p.tab {
		case tabFillLine:
			return []layout.FlexChild{picHeading(th, "Line"),
				f.Row(layout.Rigid(th.Radio(&p.line, "none", "No line"))),
				f.Row(layout.Rigid(th.Radio(&p.line, "solid", "Solid line"))),
				f.Row(f.Label("Color"), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					c, ok, dims := p.lineColor.Combo(gtx, th, 120, p.lineChoice, "Automatic")
					if ok {
						p.lineChoice, p.line.Value = c, "solid"
						if c.None {
							p.lineChoice = gui.ColorChoice{Color: colors.RGB(0x000000)}
						}
					}
					return dims
				})),
				f.Row(f.Label("Width"), picBox(th, &p.lineWidth, 90)),
			}
		case tabProperties:
			return []layout.FlexChild{picHeading(th, "Alt Text"),
				layout.Rigid(th.Label(12, "How would you describe this picture to someone who is blind?", gui.MutedText, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Height: 6}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return gui.TextField(gtx, th, &p.alt, 500, 4, "Description")
				}),
				layout.Rigid(layout.Spacer{Height: 8}.Layout),
				f.Row(layout.Rigid(th.CheckBox(&p.decorative, "Mark as decorative"))),
				layout.Rigid(th.Label(12, "Decorative pictures add visual interest but aren't informative; screen readers skip them.", gui.MutedText, font.Normal).Layout),
			}
		default:
			return []layout.FlexChild{picHeading(th, "Crop"),
				layout.Rigid(th.Label(12, "How much to cut off each edge, at the picture's size.", gui.MutedText, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Height: 6}.Layout),
				f.Row(f.Label("Left"), picBox(th, &p.cropL, 90), picGap(24), picLabel(th, "Top"), picBox(th, &p.cropT, 90)),
				f.Row(f.Label("Right"), picBox(th, &p.cropR, 90), picGap(24), picLabel(th, "Bottom"), picBox(th, &p.cropB, 90)),
			}
		}
	}
	switch p.tab {
	case tabWrapping:
		kids := []layout.FlexChild{picHeading(th, "Wrapping style")}
		for _, c := range wrapChoices {
			kids = append(kids, layout.Rigid(th.Radio(&p.wrap, c.key, c.label)))
		}
		kids = append(kids, layout.Rigid(th.Label(12, "WordLite keeps text above and below a picture for Square and Tight, as for Top and bottom.", gui.MutedText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			picHeading(th, "Distance from text"),
			f.Row(f.Label("Top"), picBox(th, &p.distT, 90), picGap(24), picLabel(th, "Left"), picBox(th, &p.distL, 90)),
			f.Row(f.Label("Bottom"), picBox(th, &p.distB, 90), picGap(24), picLabel(th, "Right"), picBox(th, &p.distR, 90)))
		return kids
	case tabSize:
		return []layout.FlexChild{picHeading(th, "Height"),
			f.Row(f.Label("Absolute"), picBox(th, &p.height, 100)),
			picHeading(th, "Width"),
			f.Row(f.Label("Absolute"), picBox(th, &p.width, 100)),
			picHeading(th, "Scale"),
			f.Row(f.Label("Height"), picBox(th, &p.scaleH, 80), picGap(24), picLabel(th, "Width"), picBox(th, &p.scaleW, 80)),
			f.Row(layout.Rigid(th.CheckBox(&p.lock, "Lock aspect ratio"))),
			f.Row(layout.Rigid(th.CheckBox(&p.relOriginal, "Relative to original picture size"))),
			picHeading(th, "Original size"),
			f.Row(f.Label("Height:  "+gui.FormatInches(p.origH)), picText(th, "Width:  "+gui.FormatInches(p.origW)), picGap(24),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &p.reset, "Reset") })),
		}
	}
	if !p.floats() {
		return []layout.FlexChild{layout.Rigid(th.Label(13, "An inline picture moves with its line of text. To place it on the page, choose a wrapping style other than In line with text on the Text Wrapping tab.", gui.MutedText, font.Normal).Layout)}
	}
	labels := func(rs []relChoice) []gui.DropItem {
		out := make([]gui.DropItem, len(rs))
		for i, r := range rs {
			out[i] = gui.DropItem{Label: r.label}
		}
		return out
	}
	names := func(ns []string) []gui.DropItem {
		out := make([]gui.DropItem, len(ns))
		for i, n := range ns {
			out[i] = gui.DropItem{Label: n}
		}
		return out
	}
	return []layout.FlexChild{picHeading(th, "Horizontal"),
		f.Row(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(150)
			return th.Radio(&p.hMode, "align", "Alignment")(gtx)
		}), f.Combo(&p.hAlignDD, 110, hAligns[p.hAlign], names(hAligns), func(i int) { p.hAlign, p.hMode.Value = i, "align" }),
			picGap(12), picText(th, "relative to"), picGap(12),
			f.Combo(&p.hAlignRelDD, 110, hRels[p.hAlignRel].label, labels(hRels), func(i int) { p.hAlignRel, p.hMode.Value = i, "align" })),
		f.Row(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(150)
			return th.Radio(&p.hMode, "abs", "Absolute position")(gtx)
		}), picBox(th, &p.hAbs, 110), picGap(12), picText(th, "to the right of"), picGap(12),
			f.Combo(&p.hAbsRelDD, 110, hRels[p.hAbsRel].label, labels(hRels), func(i int) { p.hAbsRel, p.hMode.Value = i, "abs" })),
		picHeading(th, "Vertical"),
		f.Row(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(150)
			return th.Radio(&p.vMode, "align", "Alignment")(gtx)
		}), f.Combo(&p.vAlignDD, 110, vAligns[p.vAlign], names(vAligns), func(i int) { p.vAlign, p.vMode.Value = i, "align" }),
			picGap(12), picText(th, "relative to"), picGap(12),
			f.Combo(&p.vAlignRelDD, 110, vAlignRels[p.vAlignRel].label, labels(vAlignRels), func(i int) { p.vAlignRel, p.vMode.Value = i, "align" })),
		f.Row(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(150)
			return th.Radio(&p.vMode, "abs", "Absolute position")(gtx)
		}), picBox(th, &p.vAbs, 110), picGap(12), picText(th, "below"), picGap(12),
			f.Combo(&p.vAbsRelDD, 110, vAbsRels[p.vAbsRel].label, labels(vAbsRels), func(i int) { p.vAbsRel, p.vMode.Value = i, "abs" })),
	}
}

// apply makes the dialog's changes to the picture as one undoable step,
// or returns what is wrong with a value typed in.
func (p *pictureDialog) apply(d *window) string {
	length := func(ed *widget.Editor, what string, min float64) (float64, string) {
		v, ok := gui.ParseLength(ed.Text())
		if !ok || v < min {
			return 0, what + ` must be a length such as 1.5" or 3 cm.`
		}
		return v, ""
	}
	if p.kind == picFormatDlg {
		return p.applyFormat(d, length)
	}
	h, err := length(&p.height, "The height", 1)
	if err != "" {
		return err
	}
	w, err := length(&p.width, "The width", 1)
	if err != "" {
		return err
	}
	var dist [4]float64
	for i, ed := range []*widget.Editor{&p.distL, &p.distT, &p.distR, &p.distB} {
		if dist[i], err = length(ed, "A distance from text", 0); err != "" {
			return err
		}
	}
	x, err := length(&p.hAbs, "The horizontal position", -1e6)
	if err != "" {
		return err
	}
	y, err := length(&p.vAbs, "The vertical position", -1e6)
	if err != "" {
		return err
	}
	var wrap model.FloatWrap
	for _, c := range wrapChoices {
		if c.key == p.wrap.Value {
			wrap = c.wrap
		}
	}
	// Values left as shown keep their exact amounts.
	if !p.edited(&p.height) && !p.edited(&p.width) {
		w, h = p.startW, p.startH
	}
	if !p.edited(&p.hAbs) {
		x = p.floatX
	}
	if !p.edited(&p.vAbs) {
		y = p.floatY
	}
	keepDist := true
	for _, ed := range []*widget.Editor{&p.distL, &p.distT, &p.distR, &p.distB} {
		keepDist = keepDist && !p.edited(ed)
	}
	keepH := !p.inlineAtOpen && p.hMode.Value == p.startHMode && p.hMode.Value == "abs" && p.hAbsRel == p.startHRel && !p.edited(&p.hAbs)
	keepV := !p.inlineAtOpen && p.vMode.Value == p.startVMode && p.vMode.Value == "abs" && p.vAbsRel == p.startVRel && !p.edited(&p.vAbs)
	floats := p.floats()
	d.ed.EditPicture(p.pos, "Layout", func(o *model.Opaque) {
		if !floats {
			model.InlinePicture(o)
			model.SetPictureSize(o, w, h)
			return
		}
		model.FloatPicture(o, wrap, x, y)
		f := o.Float
		f.W, f.H = w, h
		if !keepDist || p.inlineAtOpen {
			f.Distance = dist
		}
		switch {
		case keepH:
		case p.hMode.Value == "align":
			f.AlignX, f.RelX = hAlignKeys[p.hAlign], hRels[p.hAlignRel].rel
		default:
			f.AlignX, f.X, f.RelX = "", x, hRels[p.hAbsRel].rel
		}
		switch {
		case keepV:
		case p.vMode.Value == "align":
			f.AlignY, f.RelY = vAlignKeys[p.vAlign], vAlignRels[p.vAlignRel].rel
		default:
			f.AlignY, f.Y, f.RelY = "", y, vAbsRels[p.vAbsRel].rel
		}
	})
	d.ed.Select(p.pos, model.Pos{Para: p.pos.Para, Off: p.pos.Off + 1})
	d.edited()
	return ""
}

// applyFormat makes Format Picture's changes: the border, the
// alternative text and the crop (which keeps the picture's scale, so
// cutting more off makes it smaller).
func (p *pictureDialog) applyFormat(d *window, length func(*widget.Editor, string, float64) (float64, string)) string {
	lw := 0.0
	if p.line.Value == "solid" {
		v, ok := gui.ParseLength(strings.TrimSuffix(strings.TrimSpace(p.lineWidth.Text()), "pt") + "pt")
		if !ok || v <= 0 || v > 1584 {
			return "The line width must be a number of points, such as 1.5 pt."
		}
		lw = v
	}
	var crop [4]float64
	var err string
	for i, ed := range []*widget.Editor{&p.cropL, &p.cropT, &p.cropR, &p.cropB} {
		if crop[i], err = length(ed, "A crop", 0); err != "" {
			return err
		}
	}
	l, t, r, b := crop[0]/p.fullW, crop[1]/p.fullH, crop[2]/p.fullW, crop[3]/p.fullH
	if l+r >= 0.99 || t+b >= 0.99 {
		return "The crop would leave nothing of the picture to show."
	}
	keepCrop := true
	for _, ed := range []*widget.Editor{&p.cropL, &p.cropT, &p.cropR, &p.cropB} {
		keepCrop = keepCrop && !p.edited(ed)
	}
	alt := strings.TrimSpace(p.alt.Text())
	d.ed.EditPicture(p.pos, "Format Picture", func(o *model.Opaque) {
		color, width := model.PictureLine(o)
		if p.line.Value == "solid" {
			*color, *width = colors.Hex(p.lineChoice.Color), lw
		} else {
			*color, *width = "", 0
		}
		look := model.PictureLookOf(o)
		look.Alt, look.Decorative = alt, p.decorative.Value
		if look.Decorative {
			look.Alt = "" // a decorative picture needs no description
		}
		if !keepCrop {
			look.Crop = [4]float64{l, t, r, b}
			model.SetPictureSize(o, p.fullW*(1-l-r), p.fullH*(1-t-b))
		}
	})
	d.ed.Select(p.pos, model.Pos{Para: p.pos.Para, Off: p.pos.Off + 1})
	d.edited()
	return ""
}
