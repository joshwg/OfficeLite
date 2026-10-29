package render

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

var (
	gridLineColor  = colors.RGB(0xE1E1E1)
	headerBG       = colors.RGB(0xF5F5F5)
	headerSelBG    = colors.RGB(0xF6E2C6)
	headerLine     = colors.RGB(0xD4D4D4)
	headerText     = colors.RGB(0x444444)
	accent         = gui.ExcelAccent
	selectionFill  = color.NRGBA{R: 0xA8, G: 0x5C, B: 0x00, A: 0x1E}
	freezeLine     = colors.RGB(0xA6A6A6)
	headerFontSize = float32(11)
)

// overflowScan is how many columns beyond the viewport are checked for
// text that overflows into view.
const overflowScan = 32

// Pos is a zero-based cell position.
type Pos struct{ Row, Col int }

// Selection is the active cell plus the selected range containing it.
// Extra holds the other ranges of a multiple selection (Ctrl+click),
// in the order they were added; Range is the last one.
type Selection struct {
	Active Pos
	Range  model.Range
	Extra  []model.Range
}

// Ranges is every selected range, Range last.
func (s *Selection) Ranges() []model.Range {
	if len(s.Extra) == 0 {
		return []model.Range{s.Range}
	}
	return append(append([]model.Range(nil), s.Extra...), s.Range)
}

// Multiple reports whether more than one range is selected.
func (s *Selection) Multiple() bool { return len(s.Extra) > 0 }

// Contains reports whether any selected range contains row, col.
func (s *Selection) Contains(row, col int) bool {
	if s.Range.Contains(row, col) {
		return true
	}
	for _, r := range s.Extra {
		if r.Contains(row, col) {
			return true
		}
	}
	return false
}

// coversCol and coversRow report whether a selected range spans column
// c (row r), for the highlighted headers.
func (s *Selection) coversCol(c int) bool {
	if c >= s.Range.C1 && c <= s.Range.C2 {
		return true
	}
	for _, r := range s.Extra {
		if c >= r.C1 && c <= r.C2 {
			return true
		}
	}
	return false
}

func (s *Selection) coversRow(row int) bool {
	if row >= s.Range.R1 && row <= s.Range.R2 {
		return true
	}
	for _, r := range s.Extra {
		if row >= r.R1 && row <= r.R2 {
			return true
		}
	}
	return false
}

// Area identifies the part of the grid a point falls in.
type Area int

const (
	AreaNone Area = iota
	AreaCell
	AreaColHeader
	AreaRowHeader
	AreaCorner
	AreaOutline // the outline's gutters (outline.go)
)

type Hit struct {
	Area     Area
	Row, Col int
}

// View draws one sheet. The caller owns scrolling and selection state and
// sets the fields before each Draw.
type View struct {
	Sheet  *model.Sheet
	Styles []model.Style
	Geo    *Geometry
	Shaper *text.Shaper

	TopRow, LeftCol int // first visible row/column of the scrolling pane
	Zoom            float32
	DigitWidth      float64 // of the default font; see DigitWidth
	Sel             Selection
	// Book is the workbook, which charts read their data from (nil: the
	// values cached in the file); SelChart the selected chart, if any.
	Book     *model.Workbook
	SelChart *model.Chart
	// SelElem is the selected part of the selected chart (ElemNone or
	// ElemChartArea for the chart as a whole).
	SelElem ChartElem

	// Layout from the last Draw, used by HitTest and the scroll helpers.
	scale            float64
	size             image.Point
	hdrW, hdrH       int // where the cells start: the gutters plus the headers
	gutW, gutH       int // the outline's gutters (outline.go)
	outlineBtns      []OutlineButton
	frozenX, frozenY int
	fr, fc           int
	pxPerDp          float32
	widthFactor      map[model.Font]float64  // see excelWidthFactor; per zoom
	chartCache       map[*model.Chart][]prim // charts laid out in this Draw
	chartFmt         *numfmt.Formatter       // formats charts' numbers
}

// pane is one of up to four regions (frozen/scrolling rows × columns).
type pane struct {
	r0, r1, c0, c1 int     // index ranges, end exclusive
	sx, sy         float64 // sheet offset shown at the pane's origin
	ox, oy         int     // screen origin
	clip           image.Rectangle
}

func (v *View) px(x float64) int { return int(math.Round(x * v.scale)) }

func (v *View) layoutGeometry(gtx layout.Context) {
	if v.scale != float64(gtx.Metric.PxPerDp*v.Zoom) {
		v.widthFactor = nil // depends on the drawn font size
	}
	v.pxPerDp = gtx.Metric.PxPerDp
	v.scale = float64(gtx.Metric.PxPerDp * v.Zoom)
	v.size = gtx.Constraints.Max
	v.fr = min(v.Sheet.FreezeRows, model.MaxRows-1)
	v.fc = min(v.Sheet.FreezeCols, model.MaxCols-1)
	v.TopRow = min(max(v.TopRow, v.fr), model.MaxRows-1)
	v.LeftCol = min(max(v.LeftCol, v.fc), model.MaxCols-1)

	// Row header wide enough for the largest visible row number.
	last := v.Geo.Rows.Index(v.Geo.Rows.Start(v.TopRow)+float64(v.size.Y)/v.scale) + 1
	digits := len(strconv.Itoa(last))
	v.gutW, v.gutH = v.gutterSize(true), v.gutterSize(false)
	v.hdrW = v.gutW + v.px(float64(max(26, digits*8+12)))
	v.hdrH = v.gutH + v.px(20)
	v.frozenX = v.hdrW + v.px(v.Geo.Cols.Start(v.fc))
	v.frozenY = v.hdrH + v.px(v.Geo.Rows.Start(v.fr))
}

// panes returns the visible panes in drawing order.
func (v *View) panes() []pane {
	type span struct {
		i0, i1 int
		s      float64
		o      int
		lo, hi int
	}
	axisSpans := func(ax *Axis, frozen, first, hdr, frozenEdge, limit int) []span {
		var out []span
		if frozen > 0 {
			out = append(out, span{0, frozen, 0, hdr, hdr, frozenEdge})
		}
		s := ax.Start(first)
		end := ax.Index(s+float64(limit-frozenEdge)/v.scale) + 1
		out = append(out, span{first, min(end+1, ax.Len()), s, frozenEdge, frozenEdge, limit})
		return out
	}
	rows := axisSpans(v.Geo.Rows, v.fr, v.TopRow, v.hdrH, v.frozenY, v.size.Y)
	cols := axisSpans(v.Geo.Cols, v.fc, v.LeftCol, v.hdrW, v.frozenX, v.size.X)
	var out []pane
	for _, r := range rows {
		for _, c := range cols {
			out = append(out, pane{
				r0: r.i0, r1: r.i1, c0: c.i0, c1: c.i1,
				sx: c.s, sy: r.s, ox: c.o, oy: r.o,
				clip: image.Rect(c.lo, r.lo, c.hi, r.hi),
			})
		}
	}
	return out
}

func (v *View) cellRect(p *pane, r, c int) image.Rectangle {
	g := v.Geo
	return image.Rect(
		p.ox+v.px(g.Cols.Start(c)-p.sx), p.oy+v.px(g.Rows.Start(r)-p.sy),
		p.ox+v.px(g.Cols.End(c)-p.sx), p.oy+v.px(g.Rows.End(r)-p.sy))
}

func (v *View) rangeRect(p *pane, m model.Range) image.Rectangle {
	return v.cellRect(p, m.R1, m.C1).Union(v.cellRect(p, m.R2, m.C2))
}

func (v *View) style(c *model.Cell) *model.Style {
	if int(c.Style) < len(v.Styles) {
		return &v.Styles[c.Style]
	}
	return &v.Styles[0]
}

// Draw paints the grid, headers and selection into gtx.Constraints.Max.
func (v *View) Draw(gtx layout.Context) layout.Dimensions {
	v.layoutGeometry(gtx)
	clear(v.chartCache)
	if v.Sheet.IsChartSheet() {
		return v.drawChartSheet(gtx)
	}
	gui.FillRect(gtx.Ops, image.Rectangle{Max: v.size}, colors.White)
	panes := v.panes()
	for _, p := range panes {
		st := clip.Rect(p.clip).Push(gtx.Ops)
		v.drawPane(gtx, &p)
		st.Pop()
	}
	// The selection outline straddles gridlines, so let it spill a few
	// pixels over a pane's top/left edge (onto the frozen area or headers).
	for _, p := range panes {
		c := p.clip
		c.Min = c.Min.Sub(image.Pt(v.px(2)+1, v.px(2)+1))
		st := clip.Rect(c).Push(gtx.Ops)
		v.drawSelection(gtx, &p)
		st.Pop()
	}
	// Charts float over the cells and the selection.
	for _, p := range panes {
		v.drawCharts(gtx, &p)
	}
	v.drawHeaders(gtx)
	v.drawOutline(gtx)
	if v.fr > 0 {
		gui.HLine(gtx.Ops, 0, v.size.X, v.frozenY-1, 1, freezeLine)
	}
	if v.fc > 0 {
		gui.VLine(gtx.Ops, v.frozenX-1, 0, v.size.Y, 1, freezeLine)
	}
	return layout.Dimensions{Size: v.size}
}

func (v *View) drawPane(gtx layout.Context, p *pane) {
	sh, g := v.Sheet, v.Geo
	ops := gtx.Ops

	// Merges touching the pane.
	var merges []model.Range
	for _, m := range sh.Merges {
		if m.R2 >= p.r0 && m.R1 < p.r1 && m.C2 >= p.c0 && m.C1 < p.c1 {
			merges = append(merges, m)
		}
	}
	mergeAt := func(r, c int) (model.Range, bool) {
		for _, m := range merges {
			if m.Contains(r, c) {
				return m, true
			}
		}
		return model.Range{}, false
	}

	// 1. Gridlines: each cell owns the line on its right and bottom edge.
	if sh.ShowGridLines {
		for c := p.c0; c < p.c1; c++ {
			if !g.Cols.Hidden(c) {
				x := v.cellRect(p, p.r0, c).Max.X - 1
				gui.VLine(ops, x, p.clip.Min.Y, p.clip.Max.Y, 1, gridLineColor)
			}
		}
		for r := p.r0; r < p.r1; r++ {
			if !g.Rows.Hidden(r) {
				y := v.cellRect(p, r, p.c0).Max.Y - 1
				gui.HLine(ops, p.clip.Min.X, p.clip.Max.X, y, 1, gridLineColor)
			}
		}
	}

	// 2. Fills. A filled cell also hides the gridlines around it.
	for r := p.r0; r < p.r1; r++ {
		if g.Rows.Hidden(r) {
			continue
		}
		for c := p.c0; c < p.c1; c++ {
			cell := sh.Get(r, c)
			if cell.Style == 0 || g.Cols.Hidden(c) {
				continue
			}
			if _, ok := mergeAt(r, c); ok {
				continue
			}
			if f := v.style(cell).Fill; f.A > 0 {
				rc := v.cellRect(p, r, c)
				rc.Min = rc.Min.Sub(image.Pt(1, 1))
				gui.FillRect(ops, rc, f)
			}
		}
	}

	// 3. Merged areas are painted as one cell, hiding interior gridlines.
	for _, m := range merges {
		rc := v.rangeRect(p, m)
		bg := v.style(sh.Get(m.R1, m.C1)).Fill
		if bg.A > 0 {
			rc.Min = rc.Min.Sub(image.Pt(1, 1))
		} else {
			bg = colors.White
			rc.Max = rc.Max.Sub(image.Pt(1, 1))
		}
		gui.FillRect(ops, rc, bg)
	}

	// 4. Text, scanning a little beyond the pane for overflowing text.
	lo := max(p.c0-overflowScan, 0)
	if p.c0 >= v.fc {
		lo = max(lo, v.fc)
	}
	hi := min(p.c1+overflowScan, g.Cols.Len())
	if p.c0 < v.fc {
		hi = p.c1
	}
	// Merged text is centered etc. across the whole merge, which may start
	// outside this pane (e.g. in a frozen column).
	for _, m := range merges {
		if cell := sh.Get(m.R1, m.C1); cell.Text != "" {
			v.drawCellText(gtx, p, cell, v.rangeRect(p, m), nil)
		}
	}
	for r := p.r0; r < p.r1; r++ {
		if g.Rows.Hidden(r) {
			continue
		}
		for c := lo; c < hi; c++ {
			cell := sh.Get(r, c)
			if cell.Text == "" || g.Cols.Hidden(c) {
				continue
			}
			if _, ok := sh.MergeAt(r, c); ok {
				continue
			}
			v.drawCellText(gtx, p, cell, v.cellRect(p, r, c), &Pos{r, c})
		}
	}

	// 5. Borders.
	for r := p.r0; r < p.r1; r++ {
		if g.Rows.Hidden(r) {
			continue
		}
		for c := p.c0; c < p.c1; c++ {
			cell := sh.Get(r, c)
			if cell.Style == 0 || g.Cols.Hidden(c) {
				continue
			}
			st := v.style(cell)
			for side, e := range st.Border {
				if e.Style != model.BorderNone {
					v.drawEdge(ops, side, v.cellRect(p, r, c), e)
				}
			}
		}
	}

	// 6. The AutoFilter's drop-down buttons (filter.go).
	v.drawFilterButtons(ops, p)
	v.drawPivotButtons(ops, p) // and the PivotTables' (pivot.go)

}

// drawCellText draws a cell's text inside rc. at is the cell's position
// when it is a plain (unmerged) cell, which allows overflow into empty
// neighbors.
func (v *View) drawCellText(gtx layout.Context, p *pane, cell *model.Cell, rc image.Rectangle, at *Pos) {
	st := v.style(cell)
	pad := v.px(2)
	inner := image.Rect(rc.Min.X+pad, rc.Min.Y, rc.Max.X-1-pad, rc.Max.Y-1)
	if inner.Dx() <= 0 || inner.Dy() <= 0 {
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
	indent := v.px(float64(st.Indent) * 3 * v.DigitWidth)
	wrap := st.Wrap || align == model.HAlignJustify || align == model.HAlignDistributed

	if wrap {
		ta := map[model.HAlign]text.Alignment{model.HAlignCenter: text.Middle, model.HAlignRight: text.End}[align]
		call, dims := v.shapeText(gtx, cell.Text, st, inner.Dx()-indent, true, ta)
		x := inner.Min.X
		if align == model.HAlignLeft {
			x += indent
		}
		v.placeText(gtx, call, dims, st, image.Pt(x, v.textY(st, inner, dims.Size.Y)), inner, false)
		return
	}

	s := strings.ReplaceAll(strings.ReplaceAll(cell.Text, "\r\n", " "), "\n", " ")
	call, dims := v.shapeText(gtx, s, st, 1<<20, false, text.Start)
	tw := dims.Size.X

	clipR := inner
	squeeze := float32(1) // horizontal scale for numbers Excel would fit
	if tw > inner.Dx()-indent {
		switch {
		case at == nil:
			// Merged: clip to the merge.
		case cell.Kind == model.KindNumber:
			// Numbers may use the cell padding before they count as too
			// wide, and are measured at the font's exact size: Gio rounds
			// sizes to whole pixels (11pt = 14.67px is drawn at 15px) and
			// does not hint, so text measures wider than in Excel.
			if float64(tw)*v.excelWidthFactor(gtx, st.Font) <= float64(inner.Dx()-indent+2*pad) {
				clipR = image.Rect(rc.Min.X, inner.Min.Y, rc.Max.X-1, inner.Max.Y)
				if avail := clipR.Dx() - indent; tw > avail {
					squeeze = float32(avail) / float32(tw)
					tw = avail
				}
				break
			}
			// Numbers never overflow; Excel shows ### instead.
			_, hdims := v.shapeText(gtx, "#", st, 1<<20, false, text.Start)
			n := max(inner.Dx()/max(hdims.Size.X, 1), 1)
			call, dims = v.shapeText(gtx, strings.Repeat("#", n), st, 1<<20, false, text.Start)
			tw = dims.Size.X
		default:
			clipR = v.overflowSpan(gtx, p, at, rc, align, tw+2*pad+indent)
			clipR = image.Rect(clipR.Min.X+pad, inner.Min.Y, clipR.Max.X-1-pad, inner.Max.Y)
		}
	}

	var x int
	switch align {
	case model.HAlignRight:
		x = inner.Max.X - tw - indent // overflow extends to the left
	case model.HAlignCenter, model.HAlignCenterContinuous:
		x = rc.Min.X + (rc.Dx()-tw)/2
	default:
		x = inner.Min.X + indent
	}
	if squeeze < 1 {
		x = max(x, clipR.Min.X)
		if align == model.HAlignRight {
			x = clipR.Max.X - tw - indent
		}
		// Compress the text into the cell around its left edge, drawn as
		// outlines, which scale cleanly.
		call, _ = v.shape(gtx, s, st, 1<<20, false, gui.TextLabel{Outlines: true})
		y := v.textY(st, inner, dims.Size.Y)
		t := op.Affine(f32.AffineId().Scale(f32.Pt(float32(x), float32(y)), f32.Pt(squeeze, 1))).Push(gtx.Ops)
		v.placeText(gtx, call, dims, st, image.Pt(x, y), clipR, true)
		t.Pop()
		return
	}
	v.placeText(gtx, call, dims, st, image.Pt(x, v.textY(st, inner, dims.Size.Y)), clipR, true)
}

// excelWidthFactor converts a drawn text width to the width Excel would
// measure. Gio draws at a whole-pixel size (11pt = 14.67px is drawn at
// 15px) without hinting, while Excel's hinted digits are narrower (Calibri
// 11: 7px instead of 7.43px), so "1/15/2026" must not turn into ###
// where Excel shows it.
func (v *View) excelWidthFactor(gtx layout.Context, f model.Font) float64 {
	if v.widthFactor == nil {
		v.widthFactor = map[model.Font]float64{}
	}
	if k, ok := v.widthFactor[f]; ok {
		return k
	}
	exact := float64(f.SizePt) * 96 / 72 * float64(v.Zoom) * float64(gtx.Metric.PxPerSp)
	drawn := float64(gtx.Sp(unit.Sp(f.SizePt * 96 / 72 * v.Zoom)))
	k := exact / max(drawn, 1)
	if adv := digitAdvance(v.Shaper, f); adv > 0 {
		k *= math.Round(adv) / adv
	}
	v.widthFactor[f] = k
	return k
}

func (v *View) textY(st *model.Style, inner image.Rectangle, h int) int {
	switch st.VAlign {
	case model.VAlignTop:
		return inner.Min.Y + v.px(1)
	case model.VAlignCenter, model.VAlignJustify, model.VAlignDistributed:
		return inner.Min.Y + (inner.Dy()-h)/2
	default:
		return inner.Max.Y - h - v.px(1)
	}
}

// overflowSpan widens a text cell over empty neighbors until the text fits,
// as Excel lets text spill into blank cells. It also erases the gridlines
// the text passes over.
func (v *View) overflowSpan(gtx layout.Context, p *pane, at *Pos, rc image.Rectangle, align model.HAlign, need int) image.Rectangle {
	sh, g := v.Sheet, v.Geo
	free := func(c int) bool {
		if c < 0 || c >= g.Cols.Len() {
			return false
		}
		if _, ok := sh.MergeAt(at.Row, c); ok {
			return false
		}
		return sh.Get(at.Row, c).Text == ""
	}
	l, r := at.Col, at.Col
	span := rc
	for span.Dx() < need {
		grew := false
		if align != model.HAlignRight && free(r+1) && r+1 < at.Col+overflowScan {
			r++
			span.Max.X = v.cellRect(p, at.Row, r).Max.X
			grew = true
		}
		if span.Dx() >= need {
			break
		}
		if align != model.HAlignLeft && free(l-1) && l-1 > at.Col-overflowScan {
			l--
			span.Min.X = v.cellRect(p, at.Row, l).Min.X
			grew = true
		}
		if !grew {
			break
		}
	}
	// Hide the gridlines between the spanned cells.
	if v.Sheet.ShowGridLines {
		for c := l; c < r; c++ {
			cr := v.cellRect(p, at.Row, c)
			bg := v.style(sh.Get(at.Row, c)).Fill
			if bg.A == 0 {
				bg = colors.White
			}
			gui.VLine(gtx.Ops, cr.Max.X-1, cr.Min.Y, cr.Max.Y-1, 1, bg)
		}
	}
	return span
}

func fontFor(mf model.Font) font.Font {
	f := font.Font{Typeface: gui.Typeface(mf.Family)}
	if mf.Bold {
		f.Weight = font.Bold
	}
	if mf.Italic {
		f.Style = font.Italic
	}
	return f
}

func (v *View) shapeText(gtx layout.Context, s string, st *model.Style, maxW int, wrap bool, align text.Alignment) (op.CallOp, layout.Dimensions) {
	return v.shape(gtx, s, st, maxW, wrap, gui.TextLabel{Alignment: align})
}

func (v *View) shape(gtx layout.Context, s string, st *model.Style, maxW int, wrap bool, l gui.TextLabel) (op.CallOp, layout.Dimensions) {
	p := text.Parameters{
		Font:    fontFor(st.Font),
		PxPerEm: fixed.I(gtx.Sp(unit.Sp(st.Font.SizePt * 96 / 72 * v.Zoom))),
	}
	m := op.Record(gtx.Ops)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(max(maxW, 1), 1<<20)}
	if wrap {
		g.Constraints.Min.X = g.Constraints.Max.X
	}
	dims := l.Layout(g, v.Shaper, p, s, st.Font.Color)
	return m.Stop(), dims
}

// placeText replays shaped text at pt, clipped to clipR, adding underline
// and strikethrough (which Gio does not draw itself).
func (v *View) placeText(gtx layout.Context, call op.CallOp, dims layout.Dimensions, st *model.Style, pt image.Point, clipR image.Rectangle, decorate bool) {
	cs := clip.Rect(clipR).Push(gtx.Ops)
	defer cs.Pop()
	off := op.Offset(pt).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	if !decorate || (!st.Font.Underline && !st.Font.Strike) {
		return
	}
	baseline := pt.Y + dims.Size.Y - dims.Baseline
	t := max(1, v.px(float64(st.Font.SizePt)/14))
	if st.Font.Underline {
		gui.HLine(gtx.Ops, pt.X, pt.X+dims.Size.X, baseline+t, t, st.Font.Color)
	}
	if st.Font.Strike {
		gui.HLine(gtx.Ops, pt.X, pt.X+dims.Size.X, baseline-v.px(float64(st.Font.SizePt)*0.33*96/72), t, st.Font.Color)
	}
}

func (v *View) drawEdge(ops *op.Ops, side int, rc image.Rectangle, e model.Edge) {
	unitPx := max(1, int(math.Round(float64(v.pxPerDp))))
	t := unitPx
	on, off := 0, 0 // solid
	switch e.Style {
	case model.BorderMedium:
		t = 2 * unitPx
	case model.BorderThick:
		t = 3 * unitPx
	case model.BorderDouble:
		t = 3 * unitPx
	case model.BorderDashed:
		on, off = 3*unitPx, unitPx
	case model.BorderDotted, model.BorderHair:
		on, off = unitPx, unitPx
	case model.BorderMediumDashed:
		t, on, off = 2*unitPx, 6*unitPx, 2*unitPx
	case model.BorderDashDot, model.BorderDashDotDot:
		on, off = 4*unitPx, 2*unitPx
	case model.BorderMediumDashDot, model.BorderMediumDashDotDot, model.BorderSlantDashDot:
		t, on, off = 2*unitPx, 6*unitPx, 2*unitPx
	}
	// The line sits on the gridline (the pixel just inside the right/bottom
	// edge, or just outside the left/top edge), centered for thick lines.
	var x0, y0, x1, y1 int
	switch side {
	case model.Left:
		x0, x1 = rc.Min.X-1, rc.Min.X-1
		y0, y1 = rc.Min.Y-1, rc.Max.Y
	case model.Right:
		x0, x1 = rc.Max.X-1, rc.Max.X-1
		y0, y1 = rc.Min.Y-1, rc.Max.Y
	case model.Top:
		y0, y1 = rc.Min.Y-1, rc.Min.Y-1
		x0, x1 = rc.Min.X-1, rc.Max.X
	case model.Bottom:
		y0, y1 = rc.Max.Y-1, rc.Max.Y-1
		x0, x1 = rc.Min.X-1, rc.Max.X
	}
	shift := (t - 1) / 2
	if x0 == x1 {
		x0, x1 = x0-shift, x1-shift
	} else {
		y0, y1 = y0-shift, y1-shift
	}
	if e.Style == model.BorderDouble {
		// Two hairlines with a gap.
		if x0 == x1 {
			gui.VLine(ops, x0, y0, y1, unitPx, e.Color)
			gui.VLine(ops, x0+2*unitPx, y0, y1, unitPx, e.Color)
		} else {
			gui.HLine(ops, x0, x1, y0, unitPx, e.Color)
			gui.HLine(ops, x0, x1, y0+2*unitPx, unitPx, e.Color)
		}
		return
	}
	if on == 0 {
		if x0 == x1 {
			gui.VLine(ops, x0, y0, y1, t, e.Color)
		} else {
			gui.HLine(ops, x0, x1, y0, t, e.Color)
		}
		return
	}
	gui.DashedLine(ops, x0, y0, x1, y1, t, on, off, e.Color)
}

func (v *View) drawSelection(gtx layout.Context, p *pane) {
	if v.Sel.Multiple() {
		v.drawMultiSelection(gtx, p)
		return
	}
	ops := gtx.Ops
	rng := v.Sel.Range
	if rng.R1 >= p.r1 || rng.R2 < p.r0 || rng.C1 >= p.c1 || rng.C2 < p.c0 {
		return
	}
	outer := v.rangeRect(p, rng)
	active := v.cellRect(p, v.Sel.Active.Row, v.Sel.Active.Col)
	if m, ok := v.Sheet.MergeAt(v.Sel.Active.Row, v.Sel.Active.Col); ok {
		active = v.rangeRect(p, m)
	}

	// Shade the range around the active cell.
	if outer != active {
		for _, r := range []image.Rectangle{
			{Min: outer.Min, Max: image.Pt(outer.Max.X, active.Min.Y)},
			{Min: image.Pt(outer.Min.X, active.Max.Y), Max: outer.Max},
			{Min: image.Pt(outer.Min.X, active.Min.Y), Max: image.Pt(active.Min.X, active.Max.Y)},
			{Min: image.Pt(active.Max.X, active.Min.Y), Max: image.Pt(outer.Max.X, active.Max.Y)},
		} {
			gui.FillRect(ops, r.Intersect(outer), selectionFill)
		}
	}

	// Outline straddling the gridlines.
	t := max(2, int(math.Round(float64(2*v.pxPerDp))))
	o := image.Rect(outer.Min.X-t/2-1, outer.Min.Y-t/2-1, outer.Max.X+t/2, outer.Max.Y+t/2)
	gui.HLine(ops, o.Min.X, o.Max.X, o.Min.Y, t, accent)
	gui.HLine(ops, o.Min.X, o.Max.X, o.Max.Y-t, t, accent)
	gui.VLine(ops, o.Min.X, o.Min.Y, o.Max.Y, t, accent)
	gui.VLine(ops, o.Max.X-t, o.Min.Y, o.Max.Y, t, accent)

	// Fill handle.
	h := v.px(6)
	hp := image.Pt(o.Max.X-t/2, o.Max.Y-t/2)
	gui.FillRect(ops, image.Rect(hp.X-h/2-1, hp.Y-h/2-1, hp.X+h/2+1, hp.Y+h/2+1), colors.White)
	gui.FillRect(ops, image.Rect(hp.X-h/2, hp.Y-h/2, hp.X+h/2, hp.Y+h/2), accent)
}

// drawMultiSelection shades every range of a multiple selection, as
// Excel does: no outline or fill handle, the active cell left white with
// a thin border.
func (v *View) drawMultiSelection(gtx layout.Context, p *pane) {
	ops := gtx.Ops
	active := v.cellRect(p, v.Sel.Active.Row, v.Sel.Active.Col)
	if m, ok := v.Sheet.MergeAt(v.Sel.Active.Row, v.Sel.Active.Col); ok {
		active = v.rangeRect(p, m)
	}
	for _, rng := range v.Sel.Ranges() {
		if rng.R1 >= p.r1 || rng.R2 < p.r0 || rng.C1 >= p.c1 || rng.C2 < p.c0 {
			continue
		}
		outer := v.rangeRect(p, clampRange(rng, p))
		for _, r := range []image.Rectangle{
			{Min: outer.Min, Max: image.Pt(outer.Max.X, active.Min.Y)},
			{Min: image.Pt(outer.Min.X, active.Max.Y), Max: outer.Max},
			{Min: image.Pt(outer.Min.X, active.Min.Y), Max: image.Pt(active.Min.X, active.Max.Y)},
			{Min: image.Pt(active.Max.X, active.Min.Y), Max: image.Pt(outer.Max.X, active.Max.Y)},
		} {
			if r.Dx() > 0 && r.Dy() > 0 {
				gui.FillRect(ops, r.Intersect(outer), selectionFill)
			}
		}
	}
	if v.Sel.Active.Row >= p.r0 && v.Sel.Active.Row < p.r1 && v.Sel.Active.Col >= p.c0 && v.Sel.Active.Col < p.c1 {
		t := max(1, int(math.Round(float64(v.pxPerDp))))
		o := active
		gui.HLine(ops, o.Min.X-1, o.Max.X, o.Min.Y-1, t, accent)
		gui.HLine(ops, o.Min.X-1, o.Max.X, o.Max.Y-t, t, accent)
		gui.VLine(ops, o.Min.X-1, o.Min.Y-1, o.Max.Y, t, accent)
		gui.VLine(ops, o.Max.X-t, o.Min.Y-1, o.Max.Y, t, accent)
	}
}

// clampRange limits r to a little beyond pane p's rows and columns, so
// that a whole-column range doesn't make a rectangle millions of pixels
// tall.
func clampRange(r model.Range, p *pane) model.Range {
	r.R1, r.R2 = max(r.R1, p.r0-1, 0), min(r.R2, p.r1)
	r.C1, r.C2 = max(r.C1, p.c0-1, 0), min(r.C2, p.c1)
	return r
}

func (v *View) drawHeaders(gtx layout.Context) {
	ops := gtx.Ops
	g := v.Geo
	hdrStyle := &model.Style{Font: model.Font{Family: "Segoe UI", SizePt: headerFontSize, Color: headerText}}
	selStyle := &model.Style{Font: model.Font{Family: "Segoe UI", SizePt: headerFontSize, Color: accent, Bold: true}}
	filtStyle := &model.Style{Font: model.Font{Family: "Segoe UI", SizePt: headerFontSize, Color: filterRowText}}

	gx, gy := v.gutW, v.gutH // the headers start after the outline's gutters
	gui.FillRect(ops, image.Rect(gx, gy, v.size.X, v.hdrH), headerBG)
	gui.FillRect(ops, image.Rect(gx, gy, v.hdrW, v.size.Y), headerBG)

	// Each column span (frozen, scrolling) appears in two panes when rows
	// are frozen too; draw its headers once.
	doneCols, doneRows := map[int]bool{}, map[int]bool{}
	for _, p := range v.panes() {
		if !doneCols[p.c0] {
			doneCols[p.c0] = true
			cs := clip.Rect(image.Rect(p.clip.Min.X, gy, p.clip.Max.X, v.hdrH)).Push(ops)
			for c := p.c0; c < p.c1; c++ {
				if g.Cols.Hidden(c) {
					continue
				}
				rc := v.cellRect(&p, p.r0, c)
				rc.Min.Y, rc.Max.Y = gy, v.hdrH
				on := v.Sel.coversCol(c)
				st := hdrStyle
				if on {
					st = selStyle
					gui.FillRect(ops, rc, headerSelBG)
				}
				v.headerLabel(gtx, model.ColName(c), st, rc)
				gui.VLine(ops, rc.Max.X-1, gy, v.hdrH, 1, headerLine)
				if on {
					gui.HLine(ops, rc.Min.X-1, rc.Max.X, v.hdrH-v.px(2), v.px(2), accent)
				}
			}
			cs.Pop()
		}
		if !doneRows[p.r0] {
			doneRows[p.r0] = true
			cs := clip.Rect(image.Rect(gx, p.clip.Min.Y, v.hdrW, p.clip.Max.Y)).Push(ops)
			for r := p.r0; r < p.r1; r++ {
				if g.Rows.Hidden(r) {
					continue
				}
				rc := v.cellRect(&p, r, p.c0)
				rc.Min.X, rc.Max.X = gx, v.hdrW
				on := v.Sel.coversRow(r)
				st := hdrStyle
				if v.filteredRow(r) {
					st = filtStyle // rows of a filtered range, as Excel numbers them
				}
				if on {
					st = selStyle
					gui.FillRect(ops, rc, headerSelBG)
				}
				v.headerLabel(gtx, strconv.Itoa(r+1), st, rc)
				gui.HLine(ops, gx, v.hdrW, rc.Max.Y-1, 1, headerLine)
				if on {
					gui.VLine(ops, v.hdrW-v.px(2), rc.Min.Y-1, rc.Max.Y, v.px(2), accent)
				}
			}
			cs.Pop()
		}
	}
	gui.HLine(ops, gx, v.size.X, v.hdrH-1, 1, headerLine)
	gui.VLine(ops, v.hdrW-1, gy, v.size.Y, 1, headerLine)
	// Select-all corner triangle.
	tri := v.px(10)
	for i := 0; i < tri; i++ {
		gui.HLine(ops, v.hdrW-4-i, v.hdrW-4, v.hdrH-4-tri+i, 1, colors.RGB(0xBFBFBF))
	}
}

func (v *View) headerLabel(gtx layout.Context, s string, st *model.Style, rc image.Rectangle) {
	call, dims := v.shapeText(gtx, s, st, 1<<20, false, text.Start)
	pt := image.Pt(rc.Min.X+(rc.Dx()-dims.Size.X)/2, rc.Min.Y+(rc.Dy()-dims.Size.Y)/2)
	v.placeText(gtx, call, dims, st, pt, rc, false)
}

// HdrW and HdrH are where the cells start on screen: the width of the row
// headers (and outline gutter) and the height of the column headers, from
// the last Draw. A point left of or above them is not on a cell.
func (v *View) HdrW() int { return v.hdrW }
func (v *View) HdrH() int { return v.hdrH }

// HitTest maps a point in the grid's coordinates (from the last Draw) to a
// cell or header.
func (v *View) HitTest(x, y float32) Hit {
	if v.scale == 0 {
		return Hit{}
	}
	g := v.Geo
	col := func() int {
		if int(x) < v.frozenX {
			return min(g.Cols.Index(float64(x-float32(v.hdrW))/v.scale), max(v.fc-1, 0))
		}
		return g.Cols.Index(g.Cols.Start(v.LeftCol) + float64(x-float32(v.frozenX))/v.scale)
	}
	row := func() int {
		if int(y) < v.frozenY {
			return min(g.Rows.Index(float64(y-float32(v.hdrH))/v.scale), max(v.fr-1, 0))
		}
		return g.Rows.Index(g.Rows.Start(v.TopRow) + float64(y-float32(v.frozenY))/v.scale)
	}
	switch {
	case int(x) < v.gutW || int(y) < v.gutH:
		return Hit{Area: AreaOutline}
	case int(x) < v.hdrW && int(y) < v.hdrH:
		return Hit{Area: AreaCorner}
	case int(y) < v.hdrH:
		return Hit{Area: AreaColHeader, Col: col()}
	case int(x) < v.hdrW:
		return Hit{Area: AreaRowHeader, Row: row()}
	default:
		return Hit{Area: AreaCell, Row: row(), Col: col()}
	}
}

// VisibleRows is how many rows of the scrolling pane fit on screen.
func (v *View) VisibleRows() int {
	if v.scale == 0 {
		return 1
	}
	g := v.Geo.Rows
	end := g.Index(g.Start(v.TopRow) + float64(v.size.Y-v.frozenY)/v.scale)
	return max(end-v.TopRow, 1)
}

// VisibleCols is how many columns of the scrolling pane fit on screen.
func (v *View) VisibleCols() int {
	if v.scale == 0 {
		return 1
	}
	g := v.Geo.Cols
	end := g.Index(g.Start(v.LeftCol) + float64(v.size.X-v.frozenX)/v.scale)
	return max(end-v.LeftCol, 1)
}

// EnsureVisible scrolls so that row, col is fully inside the scrolling pane.
func (v *View) EnsureVisible(row, col int) {
	if v.scale == 0 {
		return
	}
	rows, cols := v.Geo.Rows, v.Geo.Cols
	if row >= v.fr {
		avail := float64(v.size.Y-v.frozenY) / v.scale
		if row < v.TopRow {
			v.TopRow = row
		} else if rows.End(row)-rows.Start(v.TopRow) > avail {
			v.TopRow = min(rows.Index(rows.End(row)-avail)+1, row)
		}
	}
	if col >= v.fc {
		avail := float64(v.size.X-v.frozenX) / v.scale
		if col < v.LeftCol {
			v.LeftCol = col
		} else if cols.End(col)-cols.Start(v.LeftCol) > avail {
			v.LeftCol = min(cols.Index(cols.End(col)-avail)+1, col)
		}
	}
}

// CellRect returns the on-screen rectangle (grid coordinates, from the
// last Draw) of the cell at row, col, or of the merged area it belongs
// to, and the bounds of the pane showing it. ok is false when the cell is
// scrolled out of view.
func (v *View) CellRect(row, col int) (rc, pane image.Rectangle, ok bool) {
	if v.scale == 0 {
		return rc, pane, false
	}
	m, merged := v.Sheet.MergeAt(row, col)
	if !merged {
		m = model.Range{R1: row, C1: col, R2: row, C2: col}
	}
	for _, p := range v.panes() {
		if m.R1 < p.r1 && m.R2 >= p.r0 && m.C1 < p.c1 && m.C2 >= p.c0 {
			return v.rangeRect(&p, m), p.clip, true
		}
	}
	return rc, pane, false
}

// RangeRect returns the on-screen rectangle (grid coordinates, from the
// last Draw) of range r, as far as it is visible: its parts in the panes
// showing it, joined. ok is false when none of it is in view.
func (v *View) RangeRect(r model.Range) (rc image.Rectangle, ok bool) {
	if v.scale == 0 {
		return rc, false
	}
	for _, p := range v.panes() {
		if r.R1 >= p.r1 || r.R2 < p.r0 || r.C1 >= p.c1 || r.C2 < p.c0 {
			continue
		}
		part := v.rangeRect(&p, clampRange(r, &p)).Intersect(p.clip)
		if part.Empty() {
			continue
		}
		if !ok {
			rc, ok = part, true
		} else {
			rc = rc.Union(part)
		}
	}
	return rc, ok
}
