// PDF export of worksheets: pagination, cell fills, text, borders and charts.

package render

import (
	"errors"
	"image/color"
	"math"
	"strings"

	"officelite/common/colors"
	"officelite/common/pdf"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// PDFOptions control ExportPDF.
type PDFOptions struct {
	Sheets    []int    // indexes of the sheets to export, in order
	Paper     pdf.Size // zero: Letter
	Gridlines bool     // print the gridlines (Excel doesn't by default)
	// FitWidth scales each sheet down so all its columns fit across one
	// page ("Fit All Columns on One Page").
	FitWidth bool
	// DigitWidth is the maximum digit width (pixels) of the workbook's
	// default font, as the grid uses it; 0 measures it with the PDF font.
	DigitWidth float64
}

// ErrNothingToPrint is returned when the exported sheets are all empty.
var ErrNothingToPrint = errors.New("there is nothing to print: the sheets are empty")

// Excel's "Normal" margins, in points.
const (
	marginLR = 0.7 * 72
	marginTB = 0.75 * 72
	pxToPt   = 72.0 / 96
)

var printGridColor = colors.RGB(0xA6A6A6)

// ExportPDF adds the sheets to d as Excel prints them: the used range
// (cells with values, fills or borders, and charts), paginated down then over onto
// pages of the paper size inside Excel's normal margins, with column
// widths and row heights, cell text as displayed (number formats applied),
// fonts, fills, borders, alignment, wrapping and merged cells. Hidden
// rows and columns are skipped; nothing repeats on each page.
func ExportPDF(d *pdf.Document, wb *model.Workbook, o PDFOptions) error {
	if o.Paper.W <= 0 || o.Paper.H <= 0 {
		o.Paper = pdf.Letter
	}
	x := &sheetExport{d: d, wb: wb, o: o, fonts: map[model.Font]*pdf.Font{}}
	defer func() {
		if x.nf != nil {
			x.nf.Close()
		}
	}()
	start := d.Pages()
	for _, i := range o.Sheets {
		if i >= 0 && i < len(wb.Sheets) {
			x.sheet(wb.Sheets[i])
		}
	}
	if d.Pages() == start {
		return ErrNothingToPrint
	}
	return nil
}

// sheetExport holds the state of one ExportPDF run.
type sheetExport struct {
	d     *pdf.Document
	wb    *model.Workbook
	o     PDFOptions
	fonts map[model.Font]*pdf.Font

	// The sheet being exported.
	sh   *model.Sheet
	geo  *Geometry
	mdw  float64
	used model.Range
	p    *pdf.Page
	band model.Range       // the cells on the current page
	nf   *numfmt.Formatter // charts' number formats (chartpdf.go)
}

// font returns the PDF font for f, embedding it on first use.
func (x *sheetExport) font(f model.Font) *pdf.Font {
	if pf, ok := x.fonts[f]; ok {
		return pf
	}
	pf := x.d.Font(pdf.FontSpec{Family: f.Family, Bold: f.Bold, Italic: f.Italic})
	x.fonts[f] = pf
	return pf
}

// style returns cell c's style, or the default style when its index is out of range.
func (x *sheetExport) style(c *model.Cell) *model.Style {
	if int(c.Style) < len(x.wb.Styles) {
		return &x.wb.Styles[c.Style]
	}
	return &x.wb.Styles[0]
}

// defaultFont is the workbook's Normal style font (Calibri 11 without styles).
func (x *sheetExport) defaultFont() model.Font {
	if len(x.wb.Styles) > 0 {
		return x.wb.Styles[0].Font
	}
	return model.Font{Family: "Calibri", SizePt: 11}
}

// usedRange is the print area Excel uses by default: every cell with a
// value, a fill or a border, merges holding one, and charts.
func (x *sheetExport) usedRange() (model.Range, bool) {
	sh := x.sh
	r := model.Range{R1: math.MaxInt, C1: math.MaxInt, R2: -1, C2: -1}
	add := func(row, col int) {
		r.R1, r.C1 = min(r.R1, row), min(r.C1, col)
		r.R2, r.C2 = max(r.R2, row), max(r.C2, col)
	}
	for row := range sh.UsedRows() {
		for col := range sh.RowLen(row) {
			c := sh.Get(row, col)
			if c.Text != "" {
				add(row, col)
				continue
			}
			if c.Style == 0 {
				continue
			}
			st := x.style(c)
			if st.Fill.A > 0 || st.Border[0].Style+st.Border[1].Style+st.Border[2].Style+st.Border[3].Style != 0 {
				add(row, col)
			}
		}
	}
	for _, m := range sh.Merges {
		if m.R1 >= r.R1 && m.R1 <= r.R2 && m.C1 >= r.C1 && m.C1 <= r.C2 {
			add(m.R2, m.C2)
		}
	}
	// Charts print where they are, as in Excel.
	for _, c := range sh.Charts {
		cells := x.chartCells(c)
		add(cells.R1, cells.C1)
		add(cells.R2, cells.C2)
	}
	return r, r.R2 >= 0
}

// bands splits [first, last] of an axis into pages of at most avail
// points (at scale s); an entry wider than a page gets a page of its own.
func bands(ax *Axis, first, last int, avail, s float64) [][2]int {
	var out [][2]int
	start := first
	for start <= last {
		end := start
		w := ax.Size(start) * pxToPt * s
		for end+1 <= last {
			nw := ax.Size(end+1) * pxToPt * s
			if w+nw > avail+0.01 {
				break
			}
			w += nw
			end++
		}
		// Don't start a page with hidden entries only.
		if ax.Span(start, end) > 0 {
			out = append(out, [2]int{start, end})
		}
		start = end + 1
	}
	return out
}

// sheet exports one sheet: a chart sheet as its chart, otherwise its used
// range split into pages (scaled to the page width with FitWidth).
func (x *sheetExport) sheet(sh *model.Sheet) {
	x.sh = sh
	if sh.IsChartSheet() {
		x.chartSheet(sh)
		return
	}
	x.mdw = x.o.DigitWidth
	def := x.defaultFont()
	if x.mdw <= 0 {
		f := x.font(def)
		for r := '0'; r <= '9'; r++ {
			x.mdw = max(x.mdw, f.Advance(r, float64(def.SizePt)*96/72))
		}
		x.mdw = max(math.Round(x.mdw), 1)
	}
	x.geo = NewGeometry(sh, x.mdw)
	used, ok := x.usedRange()
	if !ok {
		return
	}
	x.used = used
	availW := x.o.Paper.W - 2*marginLR
	availH := x.o.Paper.H - 2*marginTB
	s := 1.0
	if x.o.FitWidth {
		if w := x.geo.Cols.Span(used.C1, used.C2) * pxToPt; w > availW {
			s = availW / w
		}
	}
	cols := bands(x.geo.Cols, used.C1, used.C2, availW, s)
	rows := bands(x.geo.Rows, used.R1, used.R2, availH, s)
	for _, cb := range cols {
		for _, rb := range rows {
			x.page(model.Range{R1: rb[0], C1: cb[0], R2: rb[1], C2: cb[1]}, s)
		}
	}
}

// colX is column c's left edge on the current page, in unscaled points
// relative to the band's top-left corner (the page applies the margin and scale).
func (x *sheetExport) colX(c int) float64 {
	return (x.geo.Cols.Start(c) - x.geo.Cols.Start(x.band.C1)) * pxToPt
}

// rowY is row r's top edge on the current page, as colX.
func (x *sheetExport) rowY(r int) float64 {
	return (x.geo.Rows.Start(r) - x.geo.Rows.Start(x.band.R1)) * pxToPt
}

// rect is a rectangle in page points.
type rect struct{ x0, y0, x1, y1 float64 }

// w and h are the rectangle's width and height.
func (r rect) w() float64 { return r.x1 - r.x0 }
func (r rect) h() float64 { return r.y1 - r.y0 }

// cellRect is cell r, c's rectangle on the current page.
func (x *sheetExport) cellRect(r, c int) rect {
	return rect{x.colX(c), x.rowY(r), x.colX(c + 1), x.rowY(r + 1)}
}

// rangeRect is range m's rectangle on the current page.
func (x *sheetExport) rangeRect(m model.Range) rect {
	return rect{x.colX(m.C1), x.rowY(m.R1), x.colX(m.C2 + 1), x.rowY(m.R2 + 1)}
}

// fill paints r in color c.
func (x *sheetExport) fill(r rect, c color.NRGBA) {
	x.p.FillRect(r.x0, r.y0, r.w(), r.h(), c)
}

// page adds a page showing the cells of band at scale s: gridlines, fills,
// merged areas, text, borders, then charts.
func (x *sheetExport) page(band model.Range, s float64) {
	sh, g := x.sh, x.geo
	x.band = band
	p := x.d.AddPage(x.o.Paper)
	x.p = p
	p.Save()
	p.Transform(s, 0, 0, s, marginLR, marginTB)
	area := x.rangeRect(band)
	p.ClipRect(area.x0-2, area.y0-2, area.w()+4, area.h()+4) // room for edge borders

	var merges []model.Range
	for _, m := range sh.Merges {
		if m.R2 >= band.R1 && m.R1 <= band.R2 && m.C2 >= band.C1 && m.C1 <= band.C2 {
			merges = append(merges, m)
		}
	}
	visible := func(r, c int) bool { return !g.Rows.Hidden(r) && !g.Cols.Hidden(c) }

	// 1. Gridlines, which cell fills then cover.
	if x.o.Gridlines {
		for c := band.C1; c <= band.C2+1; c++ {
			if c == band.C2+1 || !g.Cols.Hidden(c) {
				p.Line(x.colX(c), area.y0, x.colX(c), area.y1, 0.25, printGridColor)
			}
		}
		for r := band.R1; r <= band.R2+1; r++ {
			if r == band.R2+1 || !g.Rows.Hidden(r) {
				p.Line(area.x0, x.rowY(r), area.x1, x.rowY(r), 0.25, printGridColor)
			}
		}
	}

	// 2. Fills.
	for r := band.R1; r <= band.R2; r++ {
		for c := band.C1; c <= band.C2; c++ {
			cell := sh.Get(r, c)
			if cell.Style == 0 || !visible(r, c) {
				continue
			}
			if _, ok := sh.MergeAt(r, c); ok {
				continue
			}
			if f := x.style(cell).Fill; f.A > 0 {
				x.fill(x.cellRect(r, c), f)
			}
		}
	}
	// 3. Merged areas are one cell, hiding interior gridlines.
	for _, m := range merges {
		bg := x.style(sh.Get(m.R1, m.C1)).Fill
		rc := x.rangeRect(m)
		if bg.A == 0 {
			if !x.o.Gridlines {
				continue
			}
			bg = colors.White
			rc.x0, rc.y0 = rc.x0+0.2, rc.y0+0.2
			rc.x1, rc.y1 = rc.x1-0.2, rc.y1-0.2
		}
		x.fill(rc, bg)
	}

	// 4. Text.
	for _, m := range merges {
		if cell := sh.Get(m.R1, m.C1); cell.Text != "" {
			x.cellText(cell, x.rangeRect(m), nil)
		}
	}
	for r := band.R1; r <= band.R2; r++ {
		if g.Rows.Hidden(r) {
			continue
		}
		for c := band.C1; c <= min(band.C2, sh.RowLen(r)-1); c++ {
			cell := sh.Get(r, c)
			if cell.Text == "" || g.Cols.Hidden(c) {
				continue
			}
			if _, ok := sh.MergeAt(r, c); ok {
				continue
			}
			x.cellText(cell, x.cellRect(r, c), &Pos{r, c})
		}
	}

	// 5. Borders.
	for r := band.R1; r <= band.R2; r++ {
		for c := band.C1; c <= band.C2; c++ {
			cell := sh.Get(r, c)
			if cell.Style == 0 || !visible(r, c) {
				continue
			}
			st := x.style(cell)
			for side, e := range st.Border {
				if e.Style != model.BorderNone {
					x.edge(side, x.cellRect(r, c), e)
				}
			}
		}
	}
	// 6. Charts, over the cells.
	x.pageCharts()
	p.Restore()
}

// cellText draws a cell's text inside rc, as drawCellText does on screen.
// at is the cell's position when it is a plain (unmerged) cell, which
// lets text overflow into empty neighbors.
func (x *sheetExport) cellText(cell *model.Cell, rc rect, at *Pos) {
	st := x.style(cell)
	size := float64(st.Font.SizePt)
	if size <= 0 {
		return
	}
	f := x.font(st.Font)
	pad := 2 * pxToPt
	inner := rect{rc.x0 + pad, rc.y0, rc.x1 - pxToPt - pad, rc.y1 - pxToPt}
	if inner.w() <= 0 || inner.h() <= 0 {
		return
	}
	align := st.HAlign
	if align == model.HAlignGeneral {
		switch cell.Kind {
		case model.KindNumber:
			align = model.HAlignRight
		case model.KindBool, model.KindError:
			align = model.HAlignCenter
		default:
			align = model.HAlignLeft
		}
	}
	indent := float64(st.Indent) * 3 * x.mdw * pxToPt
	asc, desc := f.Ascent(size), f.Descent(size)
	pitch := asc + desc // as the grid's labels space lines
	p := x.p

	if st.Wrap || align == model.HAlignJustify || align == model.HAlignDistributed {
		lines := wrapText(f, size, cell.Text, inner.w()-indent)
		h := float64(len(lines)) * pitch
		top := x.textTop(st, inner, h)
		p.Save()
		p.ClipRect(inner.x0, inner.y0, inner.w(), inner.h())
		for i, l := range lines {
			w := f.Measure(l, size)
			lx := inner.x0
			switch align {
			case model.HAlignRight:
				lx = inner.x1 - w
			case model.HAlignCenter, model.HAlignCenterContinuous:
				lx = inner.x0 + (inner.w()-w)/2
			default:
				lx += indent
			}
			base := top + float64(i)*pitch + (pitch-asc-desc)/2 + asc
			x.drawLine(st, f, l, lx, base, w, 1)
		}
		p.Restore()
		return
	}

	s := strings.ReplaceAll(strings.ReplaceAll(cell.Text, "\r\n", " "), "\n", " ")
	tw := f.Measure(s, size)
	clip := inner
	squeeze := 1.0
	if tw > inner.w()-indent {
		switch {
		case at == nil:
			// Merged: clip to the merge.
		case cell.Kind == model.KindNumber:
			if tw <= inner.w()-indent+2*pad {
				// Numbers may use the cell padding before they count as
				// too wide.
				clip = rect{rc.x0, inner.y0, rc.x1 - pxToPt, inner.y1}
				if avail := clip.w() - indent; tw > avail {
					squeeze = avail / tw
					tw = avail
				}
				break
			}
			// Numbers never overflow; Excel shows ### instead.
			n := max(int(inner.w()/max(f.Measure("#", size), 0.1)), 1)
			s = strings.Repeat("#", n)
			tw = f.Measure(s, size)
		default:
			clip = x.overflow(at, rc, align, tw+2*pad+indent)
			clip = rect{clip.x0 + pad, inner.y0, clip.x1 - pxToPt - pad, inner.y1}
		}
	}
	var tx float64
	switch align {
	case model.HAlignRight:
		tx = inner.x1 - tw - indent
	case model.HAlignCenter, model.HAlignCenterContinuous:
		tx = rc.x0 + (rc.w()-tw)/2
	default:
		tx = inner.x0 + indent
	}
	if squeeze < 1 {
		tx = max(tx, clip.x0)
		if align == model.HAlignRight {
			tx = clip.x1 - tw - indent
		}
	}
	top := x.textTop(st, inner, asc+desc)
	p.Save()
	p.ClipRect(clip.x0, clip.y0, clip.w(), clip.h())
	x.drawLine(st, f, s, tx, top+asc, tw, squeeze)
	p.Restore()
}

// textTop is the top of a text block of height h in inner, by vertical
// alignment.
func (x *sheetExport) textTop(st *model.Style, inner rect, h float64) float64 {
	switch st.VAlign {
	case model.VAlignTop:
		return inner.y0 + pxToPt
	case model.VAlignCenter, model.VAlignJustify, model.VAlignDistributed:
		return inner.y0 + (inner.h()-h)/2
	default:
		return inner.y1 - h - pxToPt
	}
}

// drawLine draws one line of text at baseline base, w points wide and
// squeezed horizontally by squeeze, with its underline and strikethrough.
func (x *sheetExport) drawLine(st *model.Style, f *pdf.Font, s string, tx, base, w, squeeze float64) {
	size := float64(st.Font.SizePt)
	x.p.DrawText(pdf.Run{Font: f, Size: size, X: tx, Y: base, Text: s, Color: st.Font.Color, HScale: squeeze})
	t := max(size/16, 0.5)
	if st.Font.Underline {
		x.p.FillRect(tx, base+size*0.1, w, t, st.Font.Color)
	}
	if st.Font.Strike {
		x.p.FillRect(tx, base-size*0.3, w, t, st.Font.Color)
	}
}

// overflow widens a text cell over empty neighbors on the page until the
// text fits, as Excel lets text spill into blank cells, and hides the
// gridlines it passes over.
func (x *sheetExport) overflow(at *Pos, rc rect, align model.HAlign, need float64) rect {
	sh := x.sh
	free := func(c int) bool {
		if c < x.band.C1 || c > x.band.C2 {
			return false
		}
		if _, ok := sh.MergeAt(at.Row, c); ok {
			return false
		}
		return sh.Get(at.Row, c).Text == ""
	}
	l, r := at.Col, at.Col
	span := rc
	for span.w() < need {
		grew := false
		if align != model.HAlignRight && free(r+1) {
			r++
			span.x1 = x.colX(r + 1)
			grew = true
		}
		if span.w() >= need {
			break
		}
		if align != model.HAlignLeft && free(l-1) {
			l--
			span.x0 = x.colX(l)
			grew = true
		}
		if !grew {
			break
		}
	}
	if x.o.Gridlines {
		for c := l; c < r; c++ {
			bg := x.style(sh.Get(at.Row, c)).Fill
			if bg.A == 0 {
				bg = colors.White
			}
			cx := x.colX(c + 1)
			x.p.Line(cx, rc.y0+0.2, cx, rc.y1-0.2, 0.6, bg)
		}
	}
	return span
}

// edge draws one side of a cell border, centered on the gridline. A
// transparent color draws nothing, as on screen.
func (x *sheetExport) edge(side int, rc rect, e model.Edge) {
	if e.Color.A == 0 {
		return
	}
	lw := 0.5
	var dash []float64
	switch e.Style {
	case model.BorderMedium:
		lw = 1
	case model.BorderThick:
		lw = 1.5
	case model.BorderHair:
		lw = 0.25
	case model.BorderDashed:
		dash = []float64{2.25, 0.75}
	case model.BorderDotted:
		dash = []float64{0.75, 0.75}
	case model.BorderMediumDashed:
		lw, dash = 1, []float64{4.5, 1.5}
	case model.BorderDashDot:
		dash = []float64{3, 1.5, 0.75, 1.5}
	case model.BorderDashDotDot:
		dash = []float64{3, 1.5, 0.75, 1.5, 0.75, 1.5}
	case model.BorderMediumDashDot, model.BorderSlantDashDot:
		lw, dash = 1, []float64{4.5, 1.5, 1.5, 1.5}
	case model.BorderMediumDashDotDot:
		lw, dash = 1, []float64{4.5, 1.5, 1.5, 1.5, 1.5, 1.5}
	}
	c := e.Color
	var x0, y0, x1, y1 float64
	switch side {
	case model.Left:
		x0, x1, y0, y1 = rc.x0, rc.x0, rc.y0, rc.y1
	case model.Right:
		x0, x1, y0, y1 = rc.x1, rc.x1, rc.y0, rc.y1
	case model.Top:
		x0, x1, y0, y1 = rc.x0, rc.x1, rc.y0, rc.y0
	case model.Bottom:
		x0, x1, y0, y1 = rc.x0, rc.x1, rc.y1, rc.y1
	}
	vertical := x0 == x1
	if e.Style == model.BorderDouble {
		for _, d := range []float64{-0.75, 0.75} {
			if vertical {
				x.p.Line(x0+d, y0-0.75, x1+d, y1+0.75, 0.4, c)
			} else {
				x.p.Line(x0-0.75, y0+d, x1+0.75, y1+d, 0.4, c)
			}
		}
		return
	}
	// Extend solid lines by half their width so corners join.
	if dash == nil {
		if vertical {
			y0, y1 = y0-lw/2, y1+lw/2
		} else {
			x0, x1 = x0-lw/2, x1+lw/2
		}
	}
	x.p.Line(x0, y0, x1, y1, lw, c, dash...)
}

// wrapText breaks s into lines no wider than width (points), at spaces
// and line breaks; a word wider than the line is broken by characters.
func wrapText(f *pdf.Font, size float64, s string, width float64) []string {
	var out []string
	for para := range strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line, lw := "", 0.0
		for _, word := range strings.SplitAfter(para, " ") {
			ww := f.Measure(word, size)
			trimmed := f.Measure(strings.TrimRight(word, " "), size)
			if line != "" && lw+trimmed > width {
				out = append(out, strings.TrimRight(line, " "))
				line, lw = "", 0
			}
			// Break a word that is too long on its own.
			for line == "" && trimmed > width && len([]rune(word)) > 1 {
				rs := []rune(word)
				n, w := 0, 0.0
				for n < len(rs) {
					a := f.Advance(rs[n], size)
					if n > 0 && w+a > width {
						break
					}
					w += a
					n++
				}
				out = append(out, string(rs[:n]))
				word = string(rs[n:])
				ww = f.Measure(word, size)
				trimmed = f.Measure(strings.TrimRight(word, " "), size)
			}
			line += word
			lw += ww
		}
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}
