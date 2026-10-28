package render

import (
	"math"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"

	"officelite/excel/model"
)

// FitColWidth is the width in pixels (at 100% zoom) column c needs to show
// every cell's text in full, as Excel's AutoFit measures it: the widest
// line plus the cell padding. Merged cells don't count. ok is false when
// the column has no text.
func (v *View) FitColWidth(gtx layout.Context, c int) (px float64, ok bool) {
	g := gtx
	g.Ops = new(op.Ops) // measured, never drawn
	sh := v.Sheet
	widest := 0.0
	for r := range sh.UsedRows() {
		cell := sh.Get(r, c)
		if cell.Text == "" {
			continue
		}
		if _, merged := sh.MergeAt(r, c); merged {
			continue
		}
		st := v.style(cell)
		w := 0.0
		for line := range strings.SplitSeq(strings.ReplaceAll(cell.Text, "\r\n", "\n"), "\n") {
			_, dims := v.shapeText(g, line, st, 1<<20, false, text.Start)
			w = max(w, float64(dims.Size.X)*v.excelWidthFactor(g, st.Font))
		}
		w = w/v.scale + float64(st.Indent)*3*v.DigitWidth
		widest = max(widest, w)
		ok = true
	}
	// The text sits inside 2 pixels of padding each side and the gridline.
	return math.Ceil(widest) + 5, ok
}

// FitRowHeight is the height in pixels row r needs for its tallest text,
// wrapped text at its column's width, scaled so that one line in the
// sheet's default font is the default row height. ok is false when the
// row has no text.
func (v *View) FitRowHeight(gtx layout.Context, r int) (px float64, ok bool) {
	g := gtx
	g.Ops = new(op.Ops)
	sh := v.Sheet
	_, one := v.shapeText(g, "0", &v.Styles[0], 1<<20, false, text.Start)
	if one.Size.Y == 0 {
		return 0, false
	}
	tallest := 0
	for c := range sh.RowLen(r) {
		cell := sh.Get(r, c)
		if cell.Text == "" {
			continue
		}
		if _, merged := sh.MergeAt(r, c); merged {
			continue
		}
		st := v.style(cell)
		s, wrap, maxW := cell.Text, st.Wrap, 1<<20
		if wrap {
			maxW = max(v.px(float64(sh.ColWidthPx(c, v.DigitWidth)))-v.px(5), 1)
		} else {
			s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " ")
		}
		_, dims := v.shapeText(g, s, st, maxW, wrap, text.Start)
		tallest = max(tallest, dims.Size.Y)
		ok = true
	}
	return math.Round(float64(tallest) / float64(one.Size.Y) * float64(model.DefaultRowHeightPx)), ok
}
