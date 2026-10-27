package ui

import (
	"image"
	"image/color"
	"slices"
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

// commonFonts are offered in the font list besides the document's own.
var commonFonts = []string{
	"Aptos", "Arial", "Calibri", "Calibri Light", "Cambria", "Candara", "Consolas",
	"Constantia", "Corbel", "Courier New", "Garamond", "Georgia", "Segoe UI",
	"Tahoma", "Times New Roman", "Trebuchet MS", "Verdana",
}

var fontSizes = []float64{8, 9, 10, 10.5, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72}

// palette is the font color menu: name and RRGGBB ("" = automatic).
var palette = []struct{ name, hex string }{
	{"Automatic", ""}, {"Black", "000000"}, {"Dark Gray", "595959"}, {"Gray", "A6A6A6"},
	{"Dark Red", "C00000"}, {"Red", "FF0000"}, {"Orange", "FFC000"}, {"Yellow", "FFFF00"},
	{"Light Green", "92D050"}, {"Green", "00B050"}, {"Light Blue", "00B0F0"}, {"Blue", "0070C0"},
	{"Dark Blue", "002060"}, {"Purple", "7030A0"}, {"Blue Accent", "4472C4"}, {"Orange Accent", "ED7D31"},
}

var lineSpacings = []float64{1, 1.08, 1.15, 1.5, 2, 2.5, 3}

type toolbar struct {
	styleDD, fontDD, sizeDD, colorDD, spacingDD gui.DropDown

	undo, redo, bold, italic, under, strike   widget.Clickable
	alignL, alignC, alignR, alignJ            widget.Clickable
	bullets, numbers, indLess, indMore, clear widget.Clickable
	modify                                    widget.Clickable
	state                                     toolState
	stateKey                                  toolKey
	styleIDs                                  []string
}

// toolState is the selection's formatting shown by the controls.
type toolState struct {
	style                       string // display name of the paragraph style
	font                        string
	size                        float64
	bold, italic, under, strike bool
	color                       string
	align                       model.Align
	list                        string
}

type toolKey struct {
	version int
	sel     model.Selection
	pending model.Run
	hasPend bool
}

// toolState computes (or reuses) the formatting the controls reflect.
func (d *window) toolState() *toolState {
	ed := d.ed
	k := toolKey{version: ed.Version, sel: ed.Sel}
	if ed.Pending != nil {
		k.pending, k.hasPend = *ed.Pending, true
	}
	t := &d.tb
	if k == t.stateKey && t.state.font != "" {
		return &t.state
	}
	t.stateKey = k
	cf := ed.CaretFormat()
	p := ed.Para(ed.Sel.Caret.Para)
	pp := d.doc.ResolvePara(p)
	t.state = toolState{
		style:  styleName(d.doc.Styles.Find(ed.ParaStyleID(ed.Sel.Caret.Para))),
		font:   cf.Font,
		size:   cf.Size,
		bold:   ed.AllSelected(func(r model.RunProps) bool { return r.Bold.Bool(false) }),
		italic: ed.AllSelected(func(r model.RunProps) bool { return r.Italic.Bool(false) }),
		under: ed.AllSelected(func(r model.RunProps) bool {
			return r.Underline != model.UnderlineUnset && r.Underline != model.UnderlineNone
		}),
		strike: ed.AllSelected(func(r model.RunProps) bool { return r.Strike.Bool(false) }),
		color:  cf.Color,
		align:  pp.Align,
		list:   ed.ListKind(p),
	}
	if t.state.align == model.AlignUnset {
		t.state.align = model.AlignLeft
	}
	return &t.state
}

// styleName is a style's display name: Word shows built-in names
// capitalized ("heading 1" as "Heading 1").
func styleName(st *model.Style) string {
	if st == nil {
		return "Normal"
	}
	n := st.Name
	if n == "" {
		n = st.ID
	}
	if n != "" && n[0] >= 'a' && n[0] <= 'z' {
		n = strings.ToUpper(n[:1]) + n[1:]
	}
	return n
}

// styleItems lists the paragraph styles (default first, then headings in
// order, then the rest by name) and the character styles.
func (d *window) styleItems() ([]gui.DropItem, []string) {
	doc := d.doc
	var paras, chars []*model.Style
	for _, st := range doc.Styles.Styles {
		if st.Hidden {
			continue
		}
		switch st.Type {
		case model.ParagraphStyle:
			paras = append(paras, st)
		case model.CharacterStyle:
			if !st.Default {
				chars = append(chars, st)
			}
		}
	}
	rank := func(st *model.Style) string {
		n := strings.ToLower(styleName(st))
		switch {
		case st.Default:
			return "0"
		case strings.HasPrefix(n, "heading ") && len(n) == 9:
			return "1" + n
		case n == "title" || n == "subtitle":
			return "2" + n
		}
		return "3" + n
	}
	slices.SortStableFunc(paras, func(a, b *model.Style) int { return strings.Compare(rank(a), rank(b)) })
	slices.SortStableFunc(chars, func(a, b *model.Style) int { return strings.Compare(styleName(a), styleName(b)) })
	var items []gui.DropItem
	var ids []string
	sample := func(st *model.Style, r model.RunProps) gui.DropItem {
		f := font.Font{Typeface: gui.Typeface(r.Font)}
		if r.Bold.Bool(false) {
			f.Weight = font.Bold
		}
		if r.Italic.Bool(false) {
			f.Style = font.Italic
		}
		size := float32(min(max(r.Size, 9), 18))
		return gui.DropItem{Label: styleName(st), Font: f, Size: size, Color: colors.HexOr(r.Color, gui.ChromeText)}
	}
	for _, st := range paras {
		items = append(items, sample(st, doc.ResolveRun(&model.Paragraph{Style: st.ID}, &model.Run{})))
		ids = append(ids, st.ID)
	}
	if len(chars) > 0 {
		items = append(items, gui.DropItem{Separator: true}, gui.DropItem{Label: "Character styles", Header: true})
		ids = append(ids, "", "")
		for _, st := range chars {
			items = append(items, sample(st, doc.ResolveRun(&model.Paragraph{}, &model.Run{Style: st.ID})))
			ids = append(ids, st.ID)
		}
	}
	return items, ids
}

func fmtSize(s float64) string { return strconv.FormatFloat(s, 'f', -1, 64) }

func (d *window) toolbar(gtx layout.Context) layout.Dimensions {
	t := &d.tb
	ed := d.ed
	st := d.toolState()
	h := gtx.Dp(38)
	w := gtx.Constraints.Max.X
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), gui.ChromeBG)
	gui.HLine(gtx.Ops, 0, w, h-1, 1, gui.ChromeLine)

	did := false
	click := func(b *widget.Clickable, f func()) {
		for b.Clicked(gtx) {
			f()
			did = true
		}
	}
	click(&t.undo, func() { ed.Undo() })
	click(&t.redo, func() { ed.Redo() })
	click(&t.bold, ed.ToggleBold)
	click(&t.italic, ed.ToggleItalic)
	click(&t.under, ed.ToggleUnderline)
	click(&t.strike, ed.ToggleStrike)
	click(&t.alignL, func() { ed.SetAlign(model.AlignLeft) })
	click(&t.alignC, func() { ed.SetAlign(model.AlignCenter) })
	click(&t.alignR, func() { ed.SetAlign(model.AlignRight) })
	click(&t.alignJ, func() { ed.SetAlign(model.AlignJustify) })
	click(&t.bullets, func() { ed.ToggleList(true) })
	click(&t.numbers, func() { ed.ToggleList(false) })
	click(&t.indLess, func() { ed.Indent(-1) })
	click(&t.indMore, func() { ed.Indent(1) })
	click(&t.clear, ed.ClearAllFormatting)
	click(&t.modify, func() { d.dlg.openFor(d, ed.ParaStyleID(ed.Sel.Caret.Para)) })

	btn := func(b *widget.Clickable, ic *widget.Icon, checked bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, d.th, b, ic, "", checked, color.NRGBA{})
		})
	}
	disabled := func(b *widget.Clickable, ic *widget.Icon, off bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if off {
				gtx = gtx.Disabled()
			}
			return gui.ToolButton(gtx, d.th, b, ic, "", false, color.NRGBA{})
		})
	}
	sep := layout.Rigid(gui.ToolSeparator)
	gap := layout.Rigid(layout.Spacer{Width: 4}.Layout)

	styleItems, styleIDs := d.styleItems()
	t.styleIDs = styleIDs
	fonts := slices.Clone(commonFonts)
	for _, f := range d.fams {
		if !slices.Contains(fonts, f) && f != "Symbol" {
			fonts = append(fonts, f)
		}
	}
	slices.Sort(fonts)
	fontItems := make([]gui.DropItem, len(fonts))
	for i, f := range fonts {
		fontItems[i] = gui.DropItem{Label: f}
	}
	sizeItems := make([]gui.DropItem, len(fontSizes))
	for i, s := range fontSizes {
		sizeItems[i] = gui.DropItem{Label: fmtSize(s)}
	}
	colorItems := make([]gui.DropItem, len(palette))
	for i, c := range palette {
		sw := colors.HexOr(c.hex, colors.RGB(0))
		colorItems[i] = gui.DropItem{Label: c.name, Swatch: sw}
	}
	spacingItems := []gui.DropItem{}
	for _, s := range lineSpacings {
		spacingItems = append(spacingItems, gui.DropItem{Label: strconv.FormatFloat(s, 'f', -1, 64)})
	}
	spacingItems = append(spacingItems, gui.DropItem{Separator: true},
		gui.DropItem{Label: "Add Space Before Paragraph"}, gui.DropItem{Label: "Remove Space Before Paragraph"},
		gui.DropItem{Label: "Add Space After Paragraph"}, gui.DropItem{Label: "Remove Space After Paragraph"})

	combo := func(dd *gui.DropDown, width unit.Dp, cur string, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var chosen int
			dd.ListWidth = listW
			dims := gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				chosen = dd.Layout(gtx, d.th, width, cur, items)
				return layout.Dimensions{Size: image.Pt(gtx.Dp(width), gtx.Dp(24))}
			})
			if chosen >= 0 {
				apply(chosen)
				did = true
			}
			return dims
		})
	}
	iconDD := func(dd *gui.DropDown, ic *widget.Icon, bar color.NRGBA, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var b widget.Clickable // the drop-down's own button handles clicks
			chosen := dd.LayoutButton(gtx, d.th, listW, items, func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx.Disabled(), d.th, &b, ic, "", dd.IsOpen(), bar)
			})
			if chosen >= 0 {
				apply(chosen)
				did = true
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(28), gtx.Dp(28))}
		})
	}

	cur := colors.HexOr(st.color, colors.RGB(0))
	layout.Inset{Left: 6, Top: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			disabled(&t.undo, gui.IconUndo, !ed.CanUndo()),
			disabled(&t.redo, gui.IconRedo, !ed.CanRedo()),
			sep,
			combo(&t.styleDD, 150, st.style, styleItems, 240, func(i int) {
				if id := t.styleIDs[i]; id != "" {
					ed.ApplyStyle(id)
				}
			}),
			gap,
			combo(&t.fontDD, 140, st.font, fontItems, 200, func(i int) {
				d.ensureFont(fonts[i])
				ed.SetFont(fonts[i])
			}),
			gap,
			combo(&t.sizeDD, 52, fmtSize(st.size), sizeItems, 60, func(i int) { ed.SetSize(fontSizes[i]) }),
			sep,
			btn(&t.bold, gui.IconBold, st.bold),
			btn(&t.italic, gui.IconItalic, st.italic),
			btn(&t.under, gui.IconUnderline, st.under),
			btn(&t.strike, gui.IconStrike, st.strike),
			iconDD(&t.colorDD, gui.IconTextColor, cur, colorItems, 170, func(i int) { ed.SetColor(palette[i].hex) }),
			sep,
			btn(&t.alignL, gui.IconAlignLeft, st.align == model.AlignLeft),
			btn(&t.alignC, gui.IconAlignCenter, st.align == model.AlignCenter),
			btn(&t.alignR, gui.IconAlignRight, st.align == model.AlignRight),
			btn(&t.alignJ, gui.IconAlignJustify, st.align == model.AlignJustify),
			sep,
			btn(&t.bullets, gui.IconBullets, st.list == "bullet"),
			btn(&t.numbers, gui.IconNumbering, st.list != "" && st.list != "bullet"),
			btn(&t.indLess, gui.IconIndentLess, false),
			btn(&t.indMore, gui.IconIndentMore, false),
			iconDD(&t.spacingDD, gui.IconLineSpacing, color.NRGBA{}, spacingItems, 230, func(i int) {
				switch {
				case i < len(lineSpacings):
					ed.SetLineSpacing(lineSpacings[i])
				case i == len(lineSpacings)+1:
					ed.SetSpacing(model.F(12), nil)
				case i == len(lineSpacings)+2:
					ed.SetSpacing(model.F(0), nil)
				case i == len(lineSpacings)+3:
					ed.SetSpacing(nil, model.F(12))
				case i == len(lineSpacings)+4:
					ed.SetSpacing(nil, model.F(0))
				}
			}),
			sep,
			btn(&t.clear, gui.IconClearFormat, false),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx, d.th, &t.modify, nil, "Modify Style…", false, color.NRGBA{})
			}),
		)
	})
	if did {
		d.edited()
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}
