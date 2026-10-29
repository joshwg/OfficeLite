// Package render draws a worksheet grid with Gio, the way Excel shows it.
package render

import (
	"math"
	"sort"

	"gioui.org/text"
	"golang.org/x/image/math/fixed"

	"officelite/excel/model"
)

// Axis maps row (or column) indexes to unscaled pixel offsets.
type Axis struct {
	off []float64 // off[i] = start of i; off[n] = total size
}

// NewAxis builds offsets for n entries of size def, with overrides.
func NewAxis(n int, def float32, sizes map[int]float32) *Axis {
	off := make([]float64, n+1)
	for i := 0; i < n; i++ {
		s, ok := sizes[i]
		if !ok {
			s = def
		}
		off[i+1] = off[i] + float64(s)
	}
	return &Axis{off: off}
}

// Len is the number of entries; Start, End and Size give entry i's offsets
// and size (i clamped to the axis); Span is the size of entries i through j;
// Hidden reports a zero-size entry; Clamp clamps an index to the axis;
// Total is the whole axis's size; Between is the offset from i's start to j's.
func (a *Axis) Len() int                 { return len(a.off) - 1 }
func (a *Axis) Start(i int) float64      { return a.off[min(max(i, 0), a.Len())] }
func (a *Axis) End(i int) float64        { return a.Start(i + 1) }
func (a *Axis) Size(i int) float64       { return a.End(i) - a.Start(i) }
func (a *Axis) Span(i, j int) float64    { return a.End(j) - a.Start(i) }
func (a *Axis) Hidden(i int) bool        { return a.Size(i) == 0 }
func (a *Axis) Clamp(i int) int          { return min(max(i, 0), a.Len()-1) }
func (a *Axis) Total() float64           { return a.off[a.Len()] }
func (a *Axis) Between(i, j int) float64 { return a.Start(j) - a.Start(i) }

// Index returns the entry containing offset pos, clamped to the axis.
func (a *Axis) Index(pos float64) int {
	i := sort.Search(a.Len(), func(i int) bool { return a.off[i+1] > pos })
	return a.Clamp(i)
}

// Geometry is the row and column layout of one sheet.
type Geometry struct {
	Rows, Cols *Axis
}

// NewGeometry lays out sh for a default font whose maximum digit width is
// mdw pixels (see DigitWidth).
func NewGeometry(sh *model.Sheet, mdw float64) *Geometry {
	widths := make(map[int]float32, len(sh.ColWidth))
	for c, w := range sh.ColWidth {
		widths[c] = model.ColWidthToPx(float64(w), mdw)
	}
	heights := sh.RowHeight
	if len(sh.Filtered) > 0 {
		// Rows hidden by the AutoFilter take no space.
		heights = make(map[int]float32, len(sh.RowHeight)+len(sh.Filtered))
		for r, h := range sh.RowHeight {
			heights[r] = h
		}
		for r := range sh.Filtered {
			heights[r] = 0
		}
	}
	return &Geometry{
		Rows: NewAxis(model.MaxRows, sh.DefaultRowHeight, heights),
		Cols: NewAxis(model.MaxCols, sh.DefaultColWidthPx(mdw), widths),
	}
}

// DigitWidth measures Excel's "maximum digit width" of f: the widest digit
// at the font's exact pixel size (96 DPI), rounded to whole pixels as
// Excel's hinted metrics are.
func DigitWidth(shaper *text.Shaper, f model.Font) float64 {
	adv := digitAdvance(shaper, f)
	if adv <= 0 {
		return model.DefaultDigitWidth
	}
	return max(math.Round(adv), 1)
}

// digitAdvance is the unhinted width of the widest digit at the font's
// exact pixel size. The shaper rounds sizes to whole pixels, so the digits
// are measured at a large size and scaled down.
func digitAdvance(shaper *text.Shaper, f model.Font) float64 {
	const em = 512 // Gio allows up to 1023
	shaper.LayoutString(text.Parameters{
		Font:     fontFor(f),
		PxPerEm:  fixed.I(em),
		MaxWidth: 1 << 30,
		MaxLines: 1,
	}, "0123456789")
	var widest fixed.Int26_6
	for {
		g, ok := shaper.NextGlyph()
		if !ok {
			break
		}
		widest = max(widest, g.Advance)
	}
	if widest <= 0 || f.SizePt <= 0 {
		return 0
	}
	return float64(widest) / 64 / em * float64(f.SizePt) * 96 / 72
}
