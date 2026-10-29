// Drawing and hit-testing the row and column outline gutters.

package render

import (
	"image"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/model"
)

// The outline's gutters: left of the row headers for grouped rows, above
// the column headers for grouped columns. Each holds a lane per level:
// the level buttons (1, 2, 3...) in the header band, and for each group a
// bracket line along its detail with a - button (+ when collapsed) at
// its summary row. The gutters grow with the number of levels; the
// headers and cells start after them (hdrW, hdrH), so hit-testing,
// panes and scrolling all count from there.

var (
	gutterBG      = colors.RGB(0xFFFFFF)
	outlineLine   = colors.RGB(0x6E6E6E)
	outlineBorder = colors.RGB(0x9C9C9C)
	outlineText   = colors.RGB(0x444444)
)

// Outline metrics, unzoomed pixels.
const (
	outlineLane  = 16 // a level's lane
	outlinePad   = 4  // around the lanes
	outlineBox   = 11 // a +/- button's side
	outlineLevel = 12 // a level button's side
)

// OutlineButton is a button of the outline: a level button (Level > 0)
// or a group's +/- button.
type OutlineButton struct {
	Rows  bool
	Level int // the level button's number; 0 for a group's button
	Group model.OutlineGroup
	Rect  image.Rectangle // on screen, from the last Draw
}

// gutterSize is the width (rows) or height (columns) of the outline's
// gutter: none without an outline.
func (v *View) gutterSize(rows bool) int {
	l := v.Sheet.MaxLevel(rows)
	if l == 0 {
		return 0
	}
	return v.px(float64(outlinePad + outlineLane*(l+1)))
}

// lane is the middle of level lane i (0 for level 1) across the gutter.
func (v *View) lane(i int) int {
	return v.px(float64(outlinePad)/2 + float64(outlineLane*i) + float64(outlineLane)/2)
}

// drawOutline paints the outline gutters and their buttons, recording the
// buttons for hit-testing.
func (v *View) drawOutline(gtx layout.Context) {
	v.outlineBtns = v.outlineBtns[:0]
	ops := gtx.Ops
	if v.gutW > 0 {
		gui.FillRect(ops, image.Rect(0, 0, v.gutW, v.size.Y), gutterBG)
		gui.VLine(ops, v.gutW-1, 0, v.size.Y, 1, headerLine)
	}
	if v.gutH > 0 {
		gui.FillRect(ops, image.Rect(0, 0, v.size.X, v.gutH), gutterBG)
		gui.HLine(ops, 0, v.size.X, v.gutH-1, 1, headerLine)
	}
	for _, rows := range []bool{true, false} {
		if v.Sheet.MaxLevel(rows) > 0 {
			v.drawGutter(gtx, rows)
		}
	}
}

// drawGutter draws one gutter. Positions are worked out along the axis
// (y for rows) and across the gutter (x for rows).
func (v *View) drawGutter(gtx layout.Context, rows bool) {
	ops := gtx.Ops
	sh := v.Sheet
	ax, first, frozen := v.Geo.Rows, v.TopRow, v.fr
	hdr0, hdr, frozenEdge, limit, gut := v.gutH, v.hdrH, v.frozenY, v.size.Y, v.gutW
	if !rows {
		ax, first, frozen = v.Geo.Cols, v.LeftCol, v.fc
		hdr0, hdr, frozenEdge, limit, gut = v.gutW, v.hdrW, v.frozenX, v.size.X, v.gutH
	}
	pt := func(along, across int) image.Point {
		if rows {
			return image.Pt(across, along)
		}
		return image.Pt(along, across)
	}
	box := func(along, across, side int) image.Rectangle {
		h := side / 2
		return image.Rectangle{Min: pt(along-h, across-h), Max: pt(along-h+side, across-h+side)}
	}
	line := func(a0, a1, c int) { // along the axis
		if rows {
			gui.VLine(ops, c, a0, a1, 1, outlineLine)
		} else {
			gui.HLine(ops, a0, a1, c, 1, outlineLine)
		}
	}
	tick := func(a, c0, c1 int) { // across the gutter
		if rows {
			gui.HLine(ops, c0, c1, a, 1, outlineLine)
		} else {
			gui.VLine(ops, a, c0, c1, 1, outlineLine)
		}
	}

	// The level buttons, in the header band.
	levels := sh.MaxLevel(rows)
	mid := (hdr0 + hdr) / 2
	label := &model.Style{Font: model.Font{Family: "Segoe UI", SizePt: 8, Color: outlineText}}
	for n := 1; n <= levels+1; n++ {
		b := box(mid, v.lane(n-1), min(v.px(outlineLevel), hdr-hdr0-2))
		gui.FillRect(ops, b, outlineBorder)
		gui.FillRect(ops, b.Inset(1), colors.RGB(0xF3F3F3))
		v.headerLabel(gtx, strconv.Itoa(n), label, b)
		v.outlineBtns = append(v.outlineBtns, OutlineButton{Rows: rows, Level: n, Rect: b})
	}

	// The brackets and +/- buttons, in the frozen and the scrolling band.
	type band struct {
		lo, hi int // screen extent along
		i0, i1 int // rows shown
		start  func(i int) int
	}
	var bands []band
	if frozen > 0 {
		bands = append(bands, band{hdr, frozenEdge, 0, frozen - 1, func(i int) int { return hdr + v.px(ax.Start(i)) }})
	}
	s := ax.Start(first)
	last := ax.Index(s + float64(limit-frozenEdge)/v.scale)
	bands = append(bands, band{frozenEdge, limit, first, last, func(i int) int { return frozenEdge + v.px(ax.Start(i)-s) }})
	groups := sh.Groups(rows)
	after := rows && !sh.Outline.SummaryAbove || !rows && !sh.Outline.SummaryLeft
	for _, b := range bands {
		if b.hi <= b.lo {
			continue
		}
		area := image.Rectangle{Min: pt(b.lo, 0), Max: pt(b.hi, gut-1)}
		cs := clip.Rect(area).Push(ops)
		for _, g := range groups {
			lo, hi := min(g.Lo, g.Summary), max(g.Hi, g.Summary)
			if hi < b.i0 || lo > b.i1 {
				continue
			}
			c := v.lane(g.Level - 1)
			if a0, a1 := b.start(g.Lo), b.start(g.Hi+1); !g.Collapsed && a1 > a0 {
				end := v.px(4)
				if after {
					line(a0+v.px(2), a1, c)
					tick(a0+v.px(2), c, c+end)
				} else {
					line(a0, a1-v.px(2), c)
					tick(a1-v.px(2), c, c+end)
				}
			}
			if !g.HasSummary() || ax.Hidden(g.Summary) {
				continue
			}
			s0, s1 := b.start(g.Summary), b.start(g.Summary+1)
			r := box((s0+s1)/2, c, min(v.px(outlineBox), s1-s0))
			v.drawOutlineBox(ops, r, g.Collapsed)
			if vis := r.Intersect(area); !vis.Empty() {
				v.outlineBtns = append(v.outlineBtns, OutlineButton{Rows: rows, Group: g, Rect: vis})
			}
		}
		cs.Pop()
	}
}

// drawOutlineBox draws a group's button: - when expanded, + collapsed.
func (v *View) drawOutlineBox(ops *op.Ops, r image.Rectangle, plus bool) {
	if r.Dx() < 5 || r.Dy() < 5 {
		return
	}
	gui.FillRect(ops, r, outlineBorder)
	gui.FillRect(ops, r.Inset(1), colors.White)
	m := r.Min.Add(r.Max).Div(2)
	arm := max((r.Dx()-4)/2-1, 1)
	gui.HLine(ops, m.X-arm, m.X+arm+1, m.Y, 1, outlineText)
	if plus {
		gui.VLine(ops, m.X, m.Y-arm, m.Y+arm+1, 1, outlineText)
	}
}

// OutlineButtonAt returns the outline button at x, y (grid coordinates,
// from the last Draw).
func (v *View) OutlineButtonAt(x, y float32) (OutlineButton, bool) {
	p := image.Pt(int(x), int(y))
	for _, b := range v.outlineBtns {
		if p.In(b.Rect) {
			return b, true
		}
	}
	return OutlineButton{}, false
}

// OutlineButtons lists the outline's buttons on screen at the last Draw.
func (v *View) OutlineButtons() []OutlineButton { return v.outlineBtns }
