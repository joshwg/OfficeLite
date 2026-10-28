package edit

import (
	"image/color"
	"slices"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Cell formatting (the Home ribbon's Font, Alignment, Number and Styles
// groups): each command changes the styles of the selected cells as one
// undoable change. Cells without content get a styled empty cell. A
// selection of whole columns or rows only formats their used part, as
// row and column default styles are not modeled.

// sheetChange is a change of sheet properties that has no cells of its
// own: merged areas, visibility, the standard column width.
type sheetChange struct {
	sheet         int
	before, after sheetProps
	rows          *dimChange // row heights that grew with a format change
}

type sheetProps struct {
	merges    []model.Range
	hidden    bool
	baseWidth float32
	defWidth  float32
}

func propsOf(sh *model.Sheet) sheetProps {
	return sheetProps{merges: slices.Clone(sh.Merges), hidden: sh.Hidden, baseWidth: sh.BaseColWidth, defWidth: sh.DefaultColWidth}
}

func (c *sheetChange) set(wb *model.Workbook, after bool) {
	p := c.before
	if after {
		p = c.after
	}
	sh := wb.Sheets[c.sheet]
	sh.Merges = slices.Clone(p.merges)
	sh.Hidden, sh.BaseColWidth, sh.DefaultColWidth = p.hidden, p.baseWidth, p.defWidth
	if c.rows != nil {
		c.rows.set(wb, after)
	}
}

// GrowRows sets the height of rows (pixels) as part of the last change,
// for rows that a larger font or wrapped text made taller, as Excel
// does for rows without a height of their own.
func (b *Book) GrowRows(sheet int, rows []int, heights []float32) {
	if len(rows) == 0 {
		return
	}
	n := len(b.undo)
	if n == 0 || b.undo[n-1].st != nil || b.undo[n-1].dims != nil || b.undo[n-1].group != nil ||
		b.undo[n-1].sheet != nil && (b.undo[n-1].sheet.sheet != sheet || b.undo[n-1].sheet.rows != nil) {
		b.SetRowHeights(sheet, rows, heights)
		return
	}
	last := b.undo[n-1]
	if last.sheet == nil {
		p := propsOf(b.WB.Sheets[sheet])
		last.sheet = &sheetChange{sheet: sheet, before: p, after: p}
	}
	d := &dimChange{sheet: sheet, idx: rows, after: heights}
	m := d.sizes(b.WB)
	d.before = make([]dimValue, len(rows))
	for i, x := range rows {
		v, ok := m[x]
		d.before[i] = dimValue{v, ok}
	}
	d.set(b.WB, true)
	last.sheet.rows = d
}

// FormatArea is the part of r that formatting touches: a selection of
// whole columns (rows) stops at the last used row (column). ok is false
// when nothing is left.
func FormatArea(sh *model.Sheet, r model.Range) (model.Range, bool) {
	if r.R1 == 0 && r.R2 == model.MaxRows-1 {
		r.R2 = min(r.R2, sh.UsedRows()-1)
	}
	if r.C1 == 0 && r.C2 == model.MaxCols-1 {
		r.C2 = min(r.C2, sh.UsedCols()-1)
	}
	return r, r.R1 <= r.R2 && r.C1 <= r.C2
}

// Format changes the style of every cell of r on sheet with f, which
// gets each cell's style and position, as one undoable change.
func (b *Book) Format(sheet int, r model.Range, f func(st *model.Style, row, col int)) calc.Result {
	area, ok := FormatArea(b.WB.Sheets[sheet], r)
	if !ok {
		return calc.Result{}
	}
	return b.apply(Where{sheet, r}, b.restyle(sheet, area, f))
}

// restyle lists the cell changes f makes to the styles of area's cells.
// Cells whose style stays the same are left out.
func (b *Book) restyle(sheet int, area model.Range, f func(st *model.Style, row, col int)) []cellChange {
	sh := b.WB.Sheets[sheet]
	child := spillChildren(sh)
	var cells []cellChange
	for row := area.R1; row <= area.R2; row++ {
		for col := area.C1; col <= area.C2; col++ {
			cur := sh.Get(row, col)
			st := b.style(cur.Style)
			f(&st, row, col)
			idx := b.addStyle(st)
			if idx == cur.Style {
				continue
			}
			before := snapshot(sh, child, row, col)
			cells = append(cells, cellChange{sheet, row, col, before, b.withStyle(before, idx)})
		}
	}
	return cells
}

// withStyle is c with style idx, its displayed number redone in the new
// number format (formula results are redone by the recalculation).
func (b *Book) withStyle(c model.Cell, idx uint32) model.Cell {
	c.Style = idx
	if c.Kind == model.KindNumber && c.Formula == "" {
		st := b.style(idx)
		c.Text = b.nf.Format(c.Num, st.NumFmtID, st.NumFmtCode)
	}
	return c
}

// Border presets of the Borders button.
type BorderKind int

const (
	BorderBottomEdge BorderKind = iota
	BorderTopEdge
	BorderLeftEdge
	BorderRightEdge
	BordersNone
	BordersAll
	BordersOutside
	BordersThickOutside
	BorderBottomDouble
	BorderBottomThick
)

// BorderNames are the presets' menu labels, in BorderKind order.
var BorderNames = []string{
	"Bottom Border", "Top Border", "Left Border", "Right Border", "No Border",
	"All Borders", "Outside Borders", "Thick Outside Borders",
	"Bottom Double Border", "Thick Bottom Border",
}

// Borders applies a border preset to r. No Border also clears the edges
// the cells around r draw against it, as Excel does.
func (b *Book) Borders(sheet int, r model.Range, kind BorderKind) calc.Result {
	sh := b.WB.Sheets[sheet]
	area, ok := FormatArea(sh, r)
	if !ok {
		return calc.Result{}
	}
	in := area
	if kind == BordersNone {
		area = model.Range{R1: max(area.R1-1, 0), C1: max(area.C1-1, 0),
			R2: min(area.R2+1, model.MaxRows-1), C2: min(area.C2+1, model.MaxCols-1)}
		area.R2 = min(area.R2, max(in.R2, sh.UsedRows()-1))
		area.C2 = min(area.C2, max(in.C2, sh.UsedCols()-1))
	}
	f := func(st *model.Style, row, col int) {
		if !in.Contains(row, col) {
			// A neighbor of No Border: drop the edge facing the range.
			switch {
			case row == in.R1-1 && col >= in.C1 && col <= in.C2:
				st.Border[model.Bottom] = model.Edge{}
			case row == in.R2+1 && col >= in.C1 && col <= in.C2:
				st.Border[model.Top] = model.Edge{}
			case col == in.C1-1 && row >= in.R1 && row <= in.R2:
				st.Border[model.Right] = model.Edge{}
			case col == in.C2+1 && row >= in.R1 && row <= in.R2:
				st.Border[model.Left] = model.Edge{}
			}
			return
		}
		ApplyBorder(st, kind, in, row, col)
	}
	return b.apply(Where{sheet, r}, b.restyle(sheet, area, f))
}

// ApplyBorder applies a border preset to the style of the cell at row,
// col of the range in.
func ApplyBorder(st *model.Style, kind BorderKind, in model.Range, row, col int) {
	top, bottom, left, right := row == in.R1, row == in.R2, col == in.C1, col == in.C2
	edge := func(s model.BorderStyle) model.Edge {
		return model.Edge{Style: s, Color: model.DefaultStyle().Font.Color}
	}
	set := func(side int, on bool, s model.BorderStyle) {
		if on {
			st.Border[side] = edge(s)
		}
	}
	switch kind {
	case BorderBottomEdge:
		set(model.Bottom, bottom, model.BorderThin)
	case BorderTopEdge:
		set(model.Top, top, model.BorderThin)
	case BorderLeftEdge:
		set(model.Left, left, model.BorderThin)
	case BorderRightEdge:
		set(model.Right, right, model.BorderThin)
	case BordersNone:
		st.Border = [4]model.Edge{}
	case BordersAll:
		for side := range st.Border {
			st.Border[side] = edge(model.BorderThin)
		}
	case BordersOutside, BordersThickOutside:
		s := model.BorderThin
		if kind == BordersThickOutside {
			s = model.BorderThick
		}
		set(model.Top, top, s)
		set(model.Bottom, bottom, s)
		set(model.Left, left, s)
		set(model.Right, right, s)
	case BorderBottomDouble:
		set(model.Bottom, bottom, model.BorderDouble)
	case BorderBottomThick:
		set(model.Bottom, bottom, model.BorderThick)
	}
}

// MergeKind is a Merge & Center menu choice.
type MergeKind int

const (
	MergeCenter MergeKind = iota // merge into one cell and center it
	MergeAcross                  // merge each row of the selection
	MergeCells                   // merge into one cell
	Unmerge
)

// Merge merges (or, for Unmerge, unmerges) the cells of r. Merged areas
// overlapping r are unmerged first; merging keeps only the upper-left
// value of each new area, and its cells take the upper-left cell's
// format. lost reports whether other values were dropped.
func (b *Book) Merge(sheet int, r model.Range, kind MergeKind) (res calc.Result, lost bool) {
	sh := b.WB.Sheets[sheet]
	area, ok := FormatArea(sh, r)
	if !ok {
		return calc.Result{}, false
	}
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	after := propsOf(sh)
	after.merges = slices.DeleteFunc(after.merges, func(m model.Range) bool { return overlaps(m, area) })
	var parts []model.Range
	switch kind {
	case MergeCenter, MergeCells:
		parts = []model.Range{area}
	case MergeAcross:
		for row := area.R1; row <= area.R2; row++ {
			parts = append(parts, model.Range{R1: row, C1: area.C1, R2: row, C2: area.C2})
		}
	}
	child := spillChildren(sh)
	var cells []cellChange
	for _, m := range parts {
		if m.R1 == m.R2 && m.C1 == m.C2 {
			continue
		}
		after.merges = append(after.merges, m)
		tl := sh.Get(m.R1, m.C1)
		st := b.style(tl.Style)
		if kind == MergeCenter {
			st.HAlign = model.HAlignCenter
		}
		idx := b.addStyle(st)
		for row := m.R1; row <= m.R2; row++ {
			for col := m.C1; col <= m.C2; col++ {
				before := snapshot(sh, child, row, col)
				var c model.Cell
				if row == m.R1 && col == m.C1 {
					c = b.withStyle(before, idx)
				} else {
					lost = lost || before.Formula != "" || before.Kind != model.KindEmpty
					c = model.Cell{Style: idx}
				}
				if c != before {
					cells = append(cells, cellChange{sheet, row, col, before, c})
				}
			}
		}
	}
	ch.after = after
	if slices.Equal(ch.before.merges, after.merges) && len(cells) == 0 {
		return calc.Result{}, false
	}
	return b.applySheet(Where{sheet, r}, ch, cells), lost
}

func overlaps(a, b model.Range) bool {
	return a.R1 <= b.R2 && b.R1 <= a.R2 && a.C1 <= b.C2 && b.C1 <= a.C2
}

// applySheet makes a sheet change with cell changes, records them and
// recalculates.
func (b *Book) applySheet(where Where, ch *sheetChange, cells []cellChange) calc.Result {
	ch.set(b.WB, true)
	res := b.setCells(cells, true)
	b.record(&change{where: where, cells: cells, sheet: ch})
	return res
}

// ClearFormats gives the cells of r the default format and unmerges
// them; ClearAll empties them too (Clear All).
func (b *Book) ClearFormats(sheet int, r model.Range, all bool) calc.Result {
	sh := b.WB.Sheets[sheet]
	area, ok := FormatArea(sh, r)
	if !ok {
		return calc.Result{}
	}
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.merges = slices.DeleteFunc(ch.after.merges, func(m model.Range) bool { return overlaps(m, area) })
	child := spillChildren(sh)
	var cells []cellChange
	for row := area.R1; row <= area.R2; row++ {
		for col := area.C1; col <= min(area.C2, sh.RowLen(row)-1); col++ {
			before := snapshot(sh, child, row, col)
			c := model.Cell{}
			if !all {
				c = b.withStyle(before, 0)
			}
			if c != before {
				cells = append(cells, cellChange{sheet, row, col, before, c})
			}
		}
	}
	return b.applySheet(Where{sheet, r}, ch, cells)
}

// HideSheet hides or shows a sheet. where is what Undo selects.
func (b *Book) HideSheet(sheet int, hidden bool, where Where) {
	sh := b.WB.Sheets[sheet]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.hidden = hidden
	b.applySheet(where, ch, nil)
}

// SetStandardWidth sets the width of columns without their own width,
// in characters excluding padding (Format > Default Width).
func (b *Book) SetStandardWidth(sheet int, chars float32) {
	sh := b.WB.Sheets[sheet]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.baseWidth, ch.after.defWidth = chars, 0
	b.applySheet(Where{sheet, model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}}, ch, nil)
}

// Built-in number formats as codes (en-US), for the Number group's
// increase/decrease decimal buttons.
var builtinCodes = map[int]string{
	1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00",
	5: `"$"#,##0_);\("$"#,##0\)`, 6: `"$"#,##0_);[Red]\("$"#,##0\)`,
	7: `"$"#,##0.00_);\("$"#,##0.00\)`, 8: `"$"#,##0.00_);[Red]\("$"#,##0.00\)`,
	9: "0%", 10: "0.00%", 11: "0.00E+00", 37: "#,##0_);(#,##0)", 38: "#,##0_);[Red](#,##0)",
	39: "#,##0.00_);(#,##0.00)", 40: "#,##0.00_);[Red](#,##0.00)",
	41: `_(* #,##0_);_(* \(#,##0\);_(* "-"_);_(@_)`,
	42: `_("$"* #,##0_);_("$"* \(#,##0\);_("$"* "-"_);_(@_)`,
	43: `_(* #,##0.00_);_(* \(#,##0.00\);_(* "-"??_);_(@_)`,
	44: `_("$"* #,##0.00_);_("$"* \(#,##0.00\);_("$"* "-"??_);_(@_)`,
	48: "##0.0E+0",
}

// FormatCode is the code of a style's number format ("" for General or
// a built-in format without a known code, such as a date).
func FormatCode(id int, code string) string {
	if code != "" {
		if numfmt.IsGeneral(id, code) {
			return ""
		}
		return code
	}
	return builtinCodes[id]
}

// StepDecimals is the number format with one more (delta > 0) or one
// fewer decimal place than id/code, as Excel's Increase/Decrease Decimal
// buttons do; shown is the cell's displayed text, which decides the
// decimals of a General cell. ok is false when the format has no
// digits to change (dates, text).
func StepDecimals(id int, code, shown string, delta int) (string, bool) {
	c := FormatCode(id, code)
	if c == "" {
		if !numfmt.IsGeneral(id, code) {
			return "", false
		}
		n := 0
		if i := strings.IndexByte(shown, '.'); i >= 0 {
			for _, r := range shown[i+1:] {
				if r < '0' || r > '9' {
					break
				}
				n++
			}
		}
		n = max(n+delta, 0)
		if n == 0 {
			return "0", true
		}
		return "0." + strings.Repeat("0", n), true
	}
	changed := false
	var out strings.Builder
	for _, sec := range splitSections(c) {
		if out.Len() > 0 {
			out.WriteByte(';')
		}
		s, ok := stepSection(sec, delta)
		changed = changed || ok
		out.WriteString(s)
	}
	return out.String(), changed
}

// splitSections splits a format code at the ';' outside quotes.
func splitSections(c string) []string {
	var secs []string
	start := 0
	for i := 0; i < len(c); i++ {
		switch c[i] {
		case '"':
			for i++; i < len(c) && c[i] != '"'; i++ {
			}
		case '\\', '_', '*':
			i++
		case ';':
			secs = append(secs, c[start:i])
			start = i + 1
		}
	}
	return append(secs, c[start:])
}

// stepSection adds or removes a decimal place in one section: after the
// last digit placeholder of its number part.
func stepSection(s string, delta int) (string, bool) {
	dot, last := -1, -1 // the decimal point; the last digit placeholder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
			}
		case '[':
			for i++; i < len(s) && s[i] != ']'; i++ {
			}
		case '\\', '_', '*':
			i++
		case 'E', 'e':
			i = len(s) // the exponent's digits stay
		case '.':
			if dot < 0 {
				dot = i
			}
		case '0', '#', '?':
			last = i
		case '/', 'y', 'm', 'd', 'h', 's', '@':
			return s, false // fractions, dates and text have no decimals
		}
	}
	if last < 0 {
		return s, false
	}
	if delta > 0 {
		if dot < 0 {
			return s[:last+1] + ".0" + s[last+1:], true
		}
		return s[:last+1] + "0" + s[last+1:], true
	}
	if dot < 0 || last < dot {
		return s, false
	}
	if last == dot+1 {
		return s[:dot] + s[last+1:], true // the last decimal goes with its point
	}
	return s[:last] + s[last+1:], true
}

// Number format choices of the Number group's list, in the order shown.
var NumberFormats = []struct {
	Name string
	ID   int
	Code string
}{
	{"General", 0, ""},
	{"Number", 2, ""},
	{"Currency", 0, `"$"#,##0.00`},
	{"Accounting", 44, ""},
	{"Short Date", 14, ""},
	{"Long Date", 0, "dddd, mmmm d, yyyy"},
	{"Time", 0, "h:mm:ss AM/PM"},
	{"Percentage", 10, ""},
	{"Fraction", 12, ""},
	{"Scientific", 11, ""},
	{"Text", 49, ""},
}

// NumberFormatName is the Number group's name for a format, or "Custom".
func NumberFormatName(id int, code string) string {
	for _, f := range NumberFormats {
		if f.ID == id && f.Code == code || f.Code != "" && f.Code == code {
			return f.Name
		}
	}
	switch {
	case numfmt.IsGeneral(id, code):
		return "General"
	case code == "@":
		return "Text"
	}
	return "Custom"
}

// CellStyle is a built-in cell style of the Styles group.
type CellStyle struct {
	Name  string
	Apply func(st *model.Style, normal model.Style)
}

// CellStyles are Excel's common built-in cell styles. Normal resets the
// whole format; the others set only what they define, as Excel's styles
// do.
var CellStyles = func() []CellStyle {
	rgb := func(v uint32) model.Edge {
		return model.Edge{Style: model.BorderThin, Color: hex(v)}
	}
	font := func(c uint32, bold bool, size float32) func(*model.Style, model.Style) {
		return func(st *model.Style, n model.Style) {
			st.Font.Color, st.Font.Bold = hex(c), bold
			if size > 0 {
				st.Font.SizePt = size
			}
		}
	}
	fillFont := func(fill, c uint32, bold bool) func(*model.Style, model.Style) {
		return func(st *model.Style, n model.Style) {
			st.Fill, st.Font.Color, st.Font.Bold = hex(fill), hex(c), bold
		}
	}
	boxed := func(fill, c, line uint32, bold bool) func(*model.Style, model.Style) {
		return func(st *model.Style, n model.Style) {
			fillFont(fill, c, bold)(st, n)
			for i := range st.Border {
				st.Border[i] = rgb(line)
			}
		}
	}
	heading := func(size float32, line uint32, ls model.BorderStyle) func(*model.Style, model.Style) {
		return func(st *model.Style, n model.Style) {
			font(0x44546A, true, size)(st, n)
			if ls != model.BorderNone {
				st.Border[model.Bottom] = model.Edge{Style: ls, Color: hex(line)}
			}
		}
	}
	numFmt := func(id int) func(*model.Style, model.Style) {
		return func(st *model.Style, n model.Style) { st.NumFmtID, st.NumFmtCode = id, "" }
	}
	return []CellStyle{
		{"Normal", func(st *model.Style, n model.Style) { *st = n }},
		{"Good", fillFont(0xC6EFCE, 0x006100, false)},
		{"Bad", fillFont(0xFFC7CE, 0x9C0006, false)},
		{"Neutral", fillFont(0xFFEB9C, 0x9C5700, false)},
		{"Title", func(st *model.Style, n model.Style) {
			st.Font.Family = "Calibri Light"
			font(0x44546A, false, 18)(st, n)
		}},
		{"Heading 1", heading(15, 0x4472C4, model.BorderThick)},
		{"Heading 2", heading(13, 0xA2B8E1, model.BorderThick)},
		{"Heading 3", heading(11, 0x8EA9DB, model.BorderMedium)},
		{"Heading 4", heading(11, 0, model.BorderNone)},
		{"Total", func(st *model.Style, n model.Style) {
			st.Font.Bold = true
			st.Border[model.Top] = model.Edge{Style: model.BorderThin, Color: hex(0x4472C4)}
			st.Border[model.Bottom] = model.Edge{Style: model.BorderDouble, Color: hex(0x4472C4)}
		}},
		{"Note", boxed(0xFFFFCC, 0x000000, 0xB2B2B2, false)},
		{"Input", boxed(0xFFCC99, 0x3F3F76, 0x7F7F7F, false)},
		{"Output", boxed(0xF2F2F2, 0x3F3F3F, 0x3F3F3F, true)},
		{"Calculation", boxed(0xF2F2F2, 0xFA7D00, 0x7F7F7F, true)},
		{"Check Cell", func(st *model.Style, n model.Style) {
			fillFont(0xA5A5A5, 0xFFFFFF, true)(st, n)
			for i := range st.Border {
				st.Border[i] = model.Edge{Style: model.BorderDouble, Color: hex(0x3F3F3F)}
			}
		}},
		{"Comma", numFmt(43)},
		{"Currency", numFmt(44)},
		{"Percent", numFmt(9)},
	}
}()

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// LastChange identifies the change Undo would take back (nil if none),
// to tell whether a command recorded one.
func (b *Book) LastChange() any {
	if len(b.undo) == 0 {
		return nil
	}
	return b.undo[len(b.undo)-1]
}
