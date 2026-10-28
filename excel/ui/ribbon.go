package ui

import (
	"image"
	"image/color"
	"slices"
	"strconv"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
)

// The Home ribbon under the menu bar, as Excel's Home tab: Clipboard,
// Font, Alignment, Number, Styles, Cells and Editing groups. The controls
// show the active cell's format; the commands are in format.go.

// commonFonts are offered in the font list besides the workbook's own.
var commonFonts = []string{
	"Aptos", "Aptos Narrow", "Arial", "Calibri", "Calibri Light", "Cambria", "Candara",
	"Consolas", "Constantia", "Corbel", "Courier New", "Garamond", "Georgia", "Segoe UI",
	"Tahoma", "Times New Roman", "Trebuchet MS", "Verdana",
}

var fontSizes = []float32{8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72}

// palette is the color menus' choices: name and color (zero = automatic
// font color or no fill).
var palette = []struct {
	name string
	c    color.NRGBA
}{
	{"", color.NRGBA{}}, {"Black", colors.RGB(0x000000)}, {"White", colors.RGB(0xFFFFFF)},
	{"Dark Gray", colors.RGB(0x595959)}, {"Gray", colors.RGB(0xA6A6A6)}, {"Light Gray", colors.RGB(0xD9D9D9)},
	{"Dark Red", colors.RGB(0xC00000)}, {"Red", colors.RGB(0xFF0000)}, {"Orange", colors.RGB(0xFFC000)},
	{"Yellow", colors.RGB(0xFFFF00)}, {"Light Green", colors.RGB(0x92D050)}, {"Green", colors.RGB(0x00B050)},
	{"Light Blue", colors.RGB(0x00B0F0)}, {"Blue", colors.RGB(0x0070C0)}, {"Dark Blue", colors.RGB(0x002060)},
	{"Purple", colors.RGB(0x7030A0)}, {"Blue Accent", colors.RGB(0xDDEBF7)}, {"Orange Accent", colors.RGB(0xFCE4D6)},
	{"Gold Accent", colors.RGB(0xFFF2CC)}, {"Green Accent", colors.RGB(0xE2EFDA)},
}

// Menus of the drop-down buttons.
var (
	pasteItems  = []string{"Paste", "Paste Values", "Paste Formulas", "Paste Formatting"}
	mergeItems  = []string{"Merge & Center", "Merge Across", "Merge Cells", "Unmerge Cells"}
	insertItems = []string{"Insert Cells…", "Insert Sheet Rows", "Insert Sheet Columns", "Insert Sheet"}
	deleteItems = []string{"Delete Cells…", "Delete Sheet Rows", "Delete Sheet Columns", "Delete Sheet"}
	formatItems = []string{"Row Height…", "AutoFit Row Height", "Column Width…", "AutoFit Column Width",
		"Default Width…", "", "Hide Rows", "Hide Columns", "Hide Sheet", "Unhide Rows", "Unhide Columns",
		"Unhide Sheet…", "", "Rename Sheet", "Format Cells…"}
	sumItems   = []string{"Sum", "Average", "Count Numbers", "Max", "Min"}
	fillItems  = []string{"Down", "Right"}
	clearItems = []string{"Clear All", "Clear Formats", "Clear Contents", "Clear Notes", "Clear Hyperlinks"}
	sortItems  = []string{"Sort A to Z", "Sort Z to A", "Custom Sort…"}
	findItems  = []string{"Find…", "Replace…", "Go To…"}
)

// homeRibbon is the ribbon's widget state.
type homeRibbon struct {
	ribbon gui.Ribbon

	pasteDD                          gui.DropDown
	cut, copy, painter               widget.Clickable
	fontDD, sizeDD                   gui.DropDown
	bold, italic, under, strike      widget.Clickable
	border, fill, color              widget.Clickable
	borderDD, fillDD, colorDD        gui.DropDown
	vTop, vMid, vBot, wrap           widget.Clickable
	left, center, right              widget.Clickable
	indLess, indMore, merge          widget.Clickable
	mergeDD                          gui.DropDown
	numDD                            gui.DropDown
	acct, pct, comma, decMore, decLs widget.Clickable
	stylesDD                         gui.DropDown
	insertDD, deleteDD, formatDD     gui.DropDown
	sumDD, fillCmdDD, clearDD        gui.DropDown
	sortDD, findDD                   gui.DropDown
	fontLaunch, alignLaunch, numLnch widget.Clickable

	// The Borders, Fill Color and Font Color buttons repeat the last
	// choice, as in Excel.
	lastBorder edit.BorderKind
	lastFill   color.NRGBA
	lastColor  color.NRGBA

	// The Format Painter: the copied formats, and whether it stays on
	// after painting (double-click).
	painterClip   *edit.Clip
	painterSticky bool
	painterDrag   bool // a selection drag is under way while painting

	fams []string // font families loaded into the shaper
}

func menuItems(labels []string) []gui.DropItem {
	items := make([]gui.DropItem, len(labels))
	for i, l := range labels {
		if l == "" {
			items[i] = gui.DropItem{Separator: true}
		} else {
			items[i] = gui.DropItem{Label: l}
		}
	}
	return items
}

func colorItems(none string) []gui.DropItem {
	items := make([]gui.DropItem, len(palette))
	for i, p := range palette {
		items[i] = gui.DropItem{Label: p.name, Swatch: p.c}
		if i == 0 {
			items[i].Label = none
			items[i].Swatch = color.NRGBA{}
		}
	}
	return items
}

func fmtSize(s float32) string { return strconv.FormatFloat(float64(s), 'f', -1, 32) }

// ribbonHeight is the ribbon's height in dp, for tests.
const ribbonHeight = int(gui.RibbonHeight)

// layoutRibbon draws the Home ribbon and runs its commands.
func (a *App) layoutRibbon(gtx layout.Context) layout.Dimensions {
	h := &a.home
	a.painterUpdate(gtx)
	a.formatKeys(gtx)
	st := a.activeStyle()
	sh := a.wb.Sheets[a.active]
	act := a.state().view.Sel.Active
	_, merged := sh.MergeAt(act.Row, act.Col)
	th := a.ui

	did := false // a command ran: give the keyboard back to the grid
	click := func(b *widget.Clickable, f func()) {
		for b.Clicked(gtx) {
			f()
			did = true
		}
	}
	click(&h.cut, func() { a.copy(gtx, true) })
	click(&h.copy, func() { a.copy(gtx, false) })
	for {
		c, ok := h.painter.Update(gtx)
		if !ok {
			break
		}
		a.startPainter(c.NumClicks > 1)
	}
	click(&h.bold, func() { a.format(func(s *model.Style) { s.Font.Bold = !st.Font.Bold }) })
	click(&h.italic, func() { a.format(func(s *model.Style) { s.Font.Italic = !st.Font.Italic }) })
	click(&h.under, func() { a.format(func(s *model.Style) { s.Font.Underline = !st.Font.Underline }) })
	click(&h.strike, func() { a.format(func(s *model.Style) { s.Font.Strike = !st.Font.Strike }) })
	click(&h.border, func() { a.borders(h.lastBorder) })
	click(&h.fill, func() { a.format(func(s *model.Style) { s.Fill = h.lastFill }) })
	click(&h.color, func() { a.setFontColor(h.lastColor) })
	click(&h.vTop, func() { a.format(func(s *model.Style) { s.VAlign = model.VAlignTop }) })
	click(&h.vMid, func() { a.format(func(s *model.Style) { s.VAlign = model.VAlignCenter }) })
	click(&h.vBot, func() { a.format(func(s *model.Style) { s.VAlign = model.VAlignBottom }) })
	click(&h.wrap, func() { a.formatGrow(func(s *model.Style) { s.Wrap = !st.Wrap }) })
	click(&h.left, func() { a.setHAlign(st, model.HAlignLeft) })
	click(&h.center, func() { a.setHAlign(st, model.HAlignCenter) })
	click(&h.right, func() { a.setHAlign(st, model.HAlignRight) })
	click(&h.indLess, func() { a.indent(-1) })
	click(&h.indMore, func() { a.indent(1) })
	click(&h.merge, func() {
		if merged {
			a.merge(edit.Unmerge)
		} else {
			a.merge(edit.MergeCenter)
		}
	})
	click(&h.acct, func() { a.setNumFmt(44, "") })
	click(&h.pct, func() { a.setNumFmt(9, "") })
	click(&h.comma, func() { a.setNumFmt(43, "") })
	click(&h.decMore, func() { a.stepDecimals(1) })
	click(&h.decLs, func() { a.stepDecimals(-1) })
	click(&h.fontLaunch, func() { a.openFormatCells(tabFont) })
	click(&h.alignLaunch, func() { a.openFormatCells(tabAlignment) })
	click(&h.numLnch, func() { a.openFormatCells(tabNumber) })

	// Lists.
	fonts := slices.Clone(commonFonts)
	for _, s := range a.wb.Styles {
		if f := s.Font.Family; f != "" && !slices.Contains(fonts, f) {
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
	numItems := make([]gui.DropItem, len(edit.NumberFormats))
	for i, f := range edit.NumberFormats {
		numItems[i] = gui.DropItem{Label: f.Name}
	}
	styleItems := make([]gui.DropItem, len(edit.CellStyles))
	for i, cs := range edit.CellStyles {
		s := a.wb.Styles[0]
		cs.Apply(&s, a.wb.Styles[0])
		styleItems[i] = gui.DropItem{Label: cs.Name, Swatch: s.Fill, Color: s.Font.Color}
		if s.Fill.A == 0 {
			styleItems[i].Swatch = colors.White
		}
	}

	// Layout helpers.
	row := gui.RibbonRow
	stack := gui.RibbonStack
	btn := func(b *widget.Clickable, ic *widget.Icon, checked bool) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, th, b, ic, "", checked, color.NRGBA{})
		}
	}
	text := func(b *widget.Clickable, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, th, b, nil, label, false, color.NRGBA{})
		}
	}
	gap := func(w unit.Dp) layout.Widget { return layout.Spacer{Width: w}.Layout }
	sep := gui.ToolSeparator
	chosen := func(i int, f func(int)) {
		if i >= 0 {
			f(i)
			did = true
		}
	}
	combo := func(dd *gui.DropDown, width unit.Dp, cur string, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			dd.ListWidth = listW
			gtx.Constraints.Min.Y = gtx.Dp(gui.RibbonRowHeight)
			var i int
			d := gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				i = dd.Layout(gtx, th, width, cur, items)
				return layout.Dimensions{Size: image.Pt(gtx.Dp(width), gtx.Dp(24))}
			})
			chosen(i, apply)
			return d
		}
	}
	drop := func(dd *gui.DropDown, ic *widget.Icon, label string, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			i, d := gui.DropButton(gtx, th, dd, ic, label, color.NRGBA{}, items, listW)
			chosen(i, apply)
			return d
		}
	}
	split := func(b *widget.Clickable, dd *gui.DropDown, ic *widget.Icon, bar color.NRGBA, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			i, d := gui.SplitButton(gtx, th, b, dd, ic, bar, items, listW)
			chosen(i, apply)
			return d
		}
	}
	big := func(dd *gui.DropDown, ic *widget.Icon, label string, items []gui.DropItem, listW unit.Dp, apply func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			i, d := gui.BigDropButton(gtx, th, dd, ic, label, items, listW)
			chosen(i, apply)
			return d
		}
	}

	numName := edit.NumberFormatName(st.NumFmtID, st.NumFmtCode)
	family := st.Font.Family
	fillBar := h.lastFill
	if fillBar.A == 0 {
		fillBar = colors.RGB(0xFFFF00)
		h.lastFill = fillBar
	}
	colorBar := h.lastColor
	if colorBar.A == 0 {
		colorBar = colors.RGB(0xFF0000)
		h.lastColor = colorBar
	}

	groups := []gui.RibbonGroup{
		{Caption: "Clipboard", Content: gui.RibbonSide(2,
			big(&h.pasteDD, gui.IconContentPaste, "Paste", menuItems(pasteItems), 170, func(i int) { a.pasteSpecial(edit.PasteMode(i)) }),
			stack(
				row(btn(&h.cut, gui.IconContentCut, false), btn(&h.copy, gui.IconContentCopy, false)),
				row(btn(&h.painter, gui.IconPainter, h.painterClip != nil)),
			),
		)},
		{Caption: "Font", Launcher: &h.fontLaunch, Content: stack(
			row(
				combo(&h.fontDD, 130, family, fontItems, 190, func(i int) { a.setFont(fonts[i]) }),
				gap(4),
				combo(&h.sizeDD, 48, fmtSize(st.Font.SizePt), sizeItems, 60, func(i int) {
					a.formatGrow(func(s *model.Style) { s.Font.SizePt = fontSizes[i] })
				}),
			),
			row(
				btn(&h.bold, gui.IconBold, st.Font.Bold),
				btn(&h.italic, gui.IconItalic, st.Font.Italic),
				btn(&h.under, gui.IconUnderline, st.Font.Underline),
				btn(&h.strike, gui.IconStrike, st.Font.Strike),
				sep,
				split(&h.border, &h.borderDD, gui.IconBorderAll, color.NRGBA{}, menuItems(edit.BorderNames), 190, func(i int) {
					h.lastBorder = edit.BorderKind(i)
					a.borders(h.lastBorder)
				}),
				split(&h.fill, &h.fillDD, gui.IconFill, fillBar, colorItems("No Fill"), 170, func(i int) {
					if i > 0 {
						h.lastFill = palette[i].c
					}
					a.format(func(s *model.Style) { s.Fill = palette[i].c })
				}),
				split(&h.color, &h.colorDD, gui.IconTextColor, colorBar, colorItems("Automatic"), 170, func(i int) {
					if i > 0 {
						h.lastColor = palette[i].c
					}
					a.setFontColor(palette[i].c)
				}),
			),
		)},
		{Caption: "Alignment", Launcher: &h.alignLaunch, Content: stack(
			row(
				btn(&h.vTop, gui.IconVAlignTop, st.VAlign == model.VAlignTop),
				btn(&h.vMid, gui.IconVAlignMid, st.VAlign == model.VAlignCenter),
				btn(&h.vBot, gui.IconVAlignBot, st.VAlign == model.VAlignBottom),
				sep,
				btn(&h.wrap, gui.IconWrap, st.Wrap),
			),
			row(
				btn(&h.left, gui.IconAlignLeft, st.HAlign == model.HAlignLeft),
				btn(&h.center, gui.IconAlignCenter, st.HAlign == model.HAlignCenter),
				btn(&h.right, gui.IconAlignRight, st.HAlign == model.HAlignRight),
				sep,
				btn(&h.indLess, gui.IconIndentLess, false),
				btn(&h.indMore, gui.IconIndentMore, false),
				sep,
				split(&h.merge, &h.mergeDD, gui.IconMerge, color.NRGBA{}, menuItems(mergeItems), 160, func(i int) {
					a.merge(edit.MergeKind(i))
				}),
			),
		)},
		{Caption: "Number", Launcher: &h.numLnch, Content: stack(
			row(combo(&h.numDD, 132, numName, numItems, 150, func(i int) {
				f := edit.NumberFormats[i]
				a.setNumFmt(f.ID, f.Code)
			})),
			row(
				text(&h.acct, "$"), text(&h.pct, "%"), text(&h.comma, ","),
				sep,
				text(&h.decMore, ".0→.00"), text(&h.decLs, ".00→.0"),
			),
		)},
		{Caption: "Styles", Content: big(&h.stylesDD, gui.IconStyles, "Cell Styles", styleItems, 170, func(i int) {
			cs := edit.CellStyles[i]
			if cs.Name == "Title" {
				a.ensureFont("Calibri Light")
			}
			normal := a.wb.Styles[0]
			a.formatGrow(func(s *model.Style) { cs.Apply(s, normal) })
		})},
		{Caption: "Cells", Content: stack(
			row(
				drop(&h.insertDD, gui.IconInsertCells, "Insert", menuItems(insertItems), 180, a.insertMenu),
				drop(&h.deleteDD, gui.IconDeleteCells, "Delete", menuItems(deleteItems), 180, a.deleteMenu),
			),
			row(drop(&h.formatDD, gui.IconFormatCells, "Format", menuItems(formatItems), 190, func(i int) { a.formatMenu(gtx, i) })),
		)},
		{Caption: "Editing", Content: stack(
			row(
				drop(&h.sumDD, gui.IconFunctions, "AutoSum", menuItems(sumItems), 150, a.autoSum),
				drop(&h.fillCmdDD, gui.IconFillDown, "Fill", menuItems(fillItems), 130, a.fillMenu),
				drop(&h.clearDD, gui.IconClear, "Clear", menuItems(clearItems), 150, a.clearMenu),
			),
			row(
				drop(&h.sortDD, gui.IconSort, "Sort & Filter", menuItems(sortItems), 150, a.sortMenu),
				drop(&h.findDD, gui.IconSearch, "Find & Select", menuItems(findItems), 140, a.findMenu),
			),
		)},
	}
	dims := h.ribbon.Layout(gtx, th, groups...)
	if did && !a.ed.active && !a.rename.active && !a.fmtDlg.open && !a.askDlg.open {
		a.grid.focusPending = true
	}
	return dims
}
