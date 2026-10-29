package render

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Drawing charts on the grid: each chart over the cells at its anchor,
// in every pane that shows part of it (clipped to the pane), so it
// scrolls, zooms and freezes with the cells. A chart sheet's chart fills
// the view.

var (
	chartSelLine   = colors.RGB(0xA0A0A0)
	chartHandleOut = colors.RGB(0x7F7F7F)
	chartSelElem   = colors.RGB(0x3C9AE8)
)

// chartBox is a chart's rectangle on the sheet, in unzoomed pixels.
func (v *View) chartBox(c *model.Chart) (x0, y0, x1, y1 float64) {
	return ChartBox(c, v.Geo)
}

// ChartBox is a chart's rectangle on a sheet laid out by g, in unzoomed
// pixels.
func ChartBox(c *model.Chart, g *Geometry) (x0, y0, x1, y1 float64) {
	a := &c.Anchor
	at := func(p model.CellOffset) (float64, float64) {
		col, row := g.Cols.Clamp(p.Col), g.Rows.Clamp(p.Row)
		x := g.Cols.Start(col) + min(float64(p.DX)/model.EMUPerPx, g.Cols.Size(col))
		y := g.Rows.Start(row) + min(float64(p.DY)/model.EMUPerPx, g.Rows.Size(row))
		return x, y
	}
	switch a.Kind {
	case model.AnchorAbsolute:
		x0, y0 = float64(a.PosX)/model.EMUPerPx, float64(a.PosY)/model.EMUPerPx
		return x0, y0, x0 + float64(a.ExtX)/model.EMUPerPx, y0 + float64(a.ExtY)/model.EMUPerPx
	case model.AnchorOneCell:
		x0, y0 = at(a.From)
		return x0, y0, x0 + float64(a.ExtX)/model.EMUPerPx, y0 + float64(a.ExtY)/model.EMUPerPx
	}
	x0, y0 = at(a.From)
	x1, y1 = at(a.To)
	return x0, y0, max(x1, x0), max(y1, y0)
}

// chartEnv returns the environment for laying out the view's charts.
func (v *View) chartEnv(gtx layout.Context) *chartEnv {
	if v.chartFmt == nil {
		date1904 := v.Book != nil && v.Book.Date1904
		v.chartFmt = numfmt.New(date1904)
	}
	nf := v.chartFmt
	return &chartEnv{
		wb:   v.Book,
		home: v.Sheet.Name,
		measure: func(f chartFont, s string) (float32, float32) {
			return measureChartText(gtx, v.Shaper, f, s)
		},
		format: func(x float64, id int, code string) string { return nf.Format(x, id, code) },
	}
}

// measureChartText measures a line of text at the font's unzoomed size.
func measureChartText(gtx layout.Context, sh *text.Shaper, f chartFont, s string) (float32, float32) {
	px := f.px()
	if sh == nil {
		return float32(len([]rune(s))) * px * 0.5, px * 1.22
	}
	sh.LayoutString(text.Parameters{
		Font:     fontFor(model.Font{Family: f.Family, Bold: f.Bold, Italic: f.Italic}),
		PxPerEm:  fixed.Int26_6(px * 64),
		MaxWidth: 1 << 24,
		MaxLines: 1,
		Locale:   gtx.Locale,
	}, s)
	var w fixed.Int26_6
	var asc, desc fixed.Int26_6
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		w += g.Advance
		asc, desc = max(asc, g.Ascent), max(desc, g.Descent)
	}
	h := float32(asc+desc) / 64
	if h <= 0 {
		h = px * 1.22
	}
	return float32(w) / 64, h
}

// drawCharts draws the sheet's charts in pane p.
func (v *View) drawCharts(gtx layout.Context, p *pane) {
	if len(v.Sheet.Charts) == 0 {
		return
	}
	st := clip.Rect(p.clip).Push(gtx.Ops)
	defer st.Pop()
	for _, c := range v.Sheet.Charts {
		rc := v.chartScreenRect(p, c)
		if !rc.Overlaps(p.clip) || rc.Dx() <= 1 || rc.Dy() <= 1 {
			continue
		}
		v.paintChart(gtx, c, rc)
	}
	for _, c := range v.Sheet.Charts {
		if c == v.SelChart {
			v.drawChartFrame(gtx, v.chartScreenRect(p, c))
			v.drawElemSelection(gtx, c)
		}
	}
}

// chartScreenRect is a chart's rectangle in pane p (screen pixels).
func (v *View) chartScreenRect(p *pane, c *model.Chart) image.Rectangle {
	x0, y0, x1, y1 := v.chartBox(c)
	return image.Rect(p.ox+v.px(x0-p.sx), p.oy+v.px(y0-p.sy), p.ox+v.px(x1-p.sx), p.oy+v.px(y1-p.sy))
}

// paintChart lays a chart out for its unzoomed size and paints it scaled
// into rc.
func (v *View) paintChart(gtx layout.Context, c *model.Chart, rc image.Rectangle) {
	if v.chartCache == nil {
		v.chartCache = map[*model.Chart][]prim{}
	}
	prims, ok := v.chartCache[c]
	w, h := float32(float64(rc.Dx())/v.scale), float32(float64(rc.Dy())/v.scale)
	if !ok {
		prims = layoutChart(c, v.chartEnv(gtx), w, h)
		v.chartCache[c] = prims
	}
	cl := clip.Rect(rc).Push(gtx.Ops)
	paintPrims(gtx, v.Shaper, prims, float32(rc.Min.X), float32(rc.Min.Y), float32(v.scale))
	cl.Pop()
}

// PaintChart lays a chart of sheet home out for rc's size at scale (rc's
// pixels per unzoomed pixel) and paints it into rc, outside a grid (the
// chart tools' previews, excelshot).
func PaintChart(gtx layout.Context, sh *text.Shaper, wb *model.Workbook, home string, c *model.Chart, rc image.Rectangle, scale float32) {
	v := &View{Sheet: &model.Sheet{Name: home}, Book: wb, Shaper: sh, scale: float64(scale)}
	defer func() {
		if v.chartFmt != nil {
			v.chartFmt.Close()
		}
	}()
	v.paintChart(gtx, c, rc)
}

// paintPrims paints laid-out primitives at ox, oy, scaled by s.
func paintPrims(gtx layout.Context, sh *text.Shaper, prims []prim, ox, oy, s float32) {
	at := func(p chartPt) f32.Point { return f32.Pt(ox+p.X*s, oy+p.Y*s) }
	for i := range prims {
		pr := &prims[i]
		switch pr.kind {
		case primFill:
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(at(pr.pts[0]))
			for _, q := range pr.pts[1:] {
				path.LineTo(at(q))
			}
			path.Close()
			paint.FillShape(gtx.Ops, pr.color, clip.Outline{Path: path.End()}.Op())
		case primStroke:
			pts := pr.pts
			if pr.closed {
				pts = append(pts[:len(pts):len(pts)], pts[0])
			}
			w := max(pr.width*s, 1)
			for _, seg := range dashed(pts, pr.dash, s) {
				var path clip.Path
				path.Begin(gtx.Ops)
				path.MoveTo(at(seg[0]))
				for _, q := range seg[1:] {
					path.LineTo(at(q))
				}
				paint.FillShape(gtx.Ops, pr.color, clip.Stroke{Path: path.End(), Width: w}.Op())
			}
		case primText:
			paintChartText(gtx, sh, pr, at(chartPt{pr.x, pr.y}), s)
		}
	}
}

// dashed splits a polyline into its dashes (in unscaled units).
func dashed(pts []chartPt, dash []float32, s float32) [][]chartPt {
	if len(dash) == 0 {
		return [][]chartPt{pts}
	}
	var out [][]chartPt
	var cur []chartPt
	k, left, on := 0, dash[0], true
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		seg := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
		pos := float32(0)
		for pos < seg {
			step := min(left, seg-pos)
			t0, t1 := pos/seg, (pos+step)/seg
			p0 := chartPt{a.X + (b.X-a.X)*t0, a.Y + (b.Y-a.Y)*t0}
			p1 := chartPt{a.X + (b.X-a.X)*t1, a.Y + (b.Y-a.Y)*t1}
			if on {
				if len(cur) == 0 {
					cur = append(cur, p0)
				}
				cur = append(cur, p1)
			}
			pos += step
			left -= step
			if left <= 1e-4 {
				if on && len(cur) > 1 {
					out = append(out, cur)
				}
				cur = nil
				on = !on
				k = (k + 1) % len(dash)
				left = dash[k]
			}
		}
	}
	if on && len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// paintChartText draws a text primitive anchored at pt.
func paintChartText(gtx layout.Context, sh *text.Shaper, pr *prim, pt f32.Point, s float32) {
	if sh == nil {
		return
	}
	f := pr.font
	p := text.Parameters{
		Font:    fontFor(model.Font{Family: f.Family, Bold: f.Bold, Italic: f.Italic}),
		PxPerEm: fixed.I(max(gtx.Sp(unit.Sp(f.px()*s)), 1)),
	}
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
	m := op.Record(gtx.Ops)
	dims := gui.TextLabel{Outlines: pr.rot != 0}.Layout(g, sh, p, pr.text, f.Color)
	call := m.Stop()
	w, h := float32(dims.Size.X), float32(dims.Size.Y)
	dx := map[int8]float32{-1: 0, 0: -w / 2, 1: -w}[pr.ha]
	dy := map[int8]float32{-1: 0, 0: -h / 2, 1: -h}[pr.va]
	if pr.rot == 0 {
		off := op.Offset(image.Pt(int(math.Round(float64(pt.X+dx))), int(math.Round(float64(pt.Y+dy))))).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		return
	}
	rad := float32(pr.rot) * math.Pi / 180
	tr := f32.AffineId().Offset(f32.Pt(dx, dy)).Rotate(f32.Pt(0, 0), rad).Offset(pt)
	t := op.Affine(tr).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
}

// drawChartFrame draws a selected chart's frame and sizing handles.
func (v *View) drawChartFrame(gtx layout.Context, rc image.Rectangle) {
	ops := gtx.Ops
	gui.HLine(ops, rc.Min.X, rc.Max.X, rc.Min.Y, 1, chartSelLine)
	gui.HLine(ops, rc.Min.X, rc.Max.X, rc.Max.Y-1, 1, chartSelLine)
	gui.VLine(ops, rc.Min.X, rc.Min.Y, rc.Max.Y, 1, chartSelLine)
	gui.VLine(ops, rc.Max.X-1, rc.Min.Y, rc.Max.Y, 1, chartSelLine)
	for _, h := range ChartHandles(rc) {
		r := max(v.px(4), 3)
		circle := func(rad int, c color.NRGBA) {
			paint.FillShape(ops, c, clip.Ellipse{Min: h.Sub(image.Pt(rad, rad)), Max: h.Add(image.Pt(rad+1, rad+1))}.Op(ops))
		}
		circle(r, chartHandleOut)
		circle(r-1, colors.White)
	}
}

// ChartHandles are the eight sizing handles of a chart frame rc: the
// corners and the middles of the sides, clockwise from the top-left.
func ChartHandles(rc image.Rectangle) [8]image.Point {
	mx, my := (rc.Min.X+rc.Max.X)/2, (rc.Min.Y+rc.Max.Y)/2
	x0, y0, x1, y1 := rc.Min.X, rc.Min.Y, rc.Max.X-1, rc.Max.Y-1
	return [8]image.Point{{x0, y0}, {mx, y0}, {x1, y0}, {x1, my}, {x1, y1}, {mx, y1}, {x0, y1}, {x0, my}}
}

// drawChartSheet draws a chart sheet: its chart filling the view.
func (v *View) drawChartSheet(gtx layout.Context) layout.Dimensions {
	c := v.Sheet.ChartSheet
	gui.FillRect(gtx.Ops, image.Rectangle{Max: v.size}, colors.RGB(0xF3F3F3))
	rc := v.chartSheetRect()
	v.paintChart(gtx, c, rc)
	if v.SelChart == c {
		v.drawChartFrame(gtx, rc)
		v.drawElemSelection(gtx, c)
	}
	return layout.Dimensions{Size: v.size}
}

// chartSheetRect is where a chart sheet's chart is drawn.
func (v *View) chartSheetRect() image.Rectangle {
	m := v.px(12)
	return image.Rect(m, m, v.size.X-m, v.size.Y-m)
}

// ChartAt returns the chart under a point (grid coordinates, from the
// last Draw), the topmost if several overlap.
func (v *View) ChartAt(x, y float32) *model.Chart {
	if v.scale == 0 {
		return nil
	}
	pt := image.Pt(int(x), int(y))
	if c := v.Sheet.ChartSheet; c != nil {
		if pt.In(v.chartSheetRect()) {
			return c
		}
		return nil
	}
	for i := len(v.Sheet.Charts) - 1; i >= 0; i-- {
		c := v.Sheet.Charts[i]
		for _, p := range v.panes() {
			if !pt.In(p.clip) {
				continue
			}
			if pt.In(v.chartScreenRect(&p, c)) {
				return c
			}
		}
	}
	return nil
}

// ChartRect returns a chart's on-screen rectangle (grid coordinates,
// from the last Draw), as far as it is visible.
func (v *View) ChartRect(c *model.Chart) (rc image.Rectangle, ok bool) {
	if v.scale == 0 {
		return rc, false
	}
	if c == v.Sheet.ChartSheet {
		return v.chartSheetRect(), true
	}
	for _, p := range v.panes() {
		part := v.chartScreenRect(&p, c).Intersect(p.clip)
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

// Scale is the view's pixels per unzoomed sheet pixel, from the last
// Draw.
func (v *View) Scale() float64 { return v.scale }

// chartScreen is a chart's whole rectangle on screen in the pane where
// it shows (the one holding point pt when it is in several; the
// scrolling pane when it is in none), and that pane's clip.
func (v *View) chartScreen(c *model.Chart, pt image.Point) (image.Rectangle, image.Rectangle) {
	if c == v.Sheet.ChartSheet {
		return v.chartSheetRect(), image.Rectangle{Max: v.size}
	}
	ps := v.panes()
	if len(ps) == 0 {
		return image.Rectangle{}, image.Rectangle{}
	}
	best := ps[len(ps)-1]
	for _, p := range ps {
		if pt.In(p.clip) && pt.In(v.chartScreenRect(&p, c)) {
			best = p
		}
	}
	return v.chartScreenRect(&best, c), best.clip
}

// SheetToScreen is a box in unzoomed sheet pixels on screen, placed by
// the pane showing chart c (for a chart's outline while it is dragged).
func (v *View) SheetToScreen(c *model.Chart, x0, y0, x1, y1 float64) image.Rectangle {
	rc, _ := v.chartScreen(c, image.Pt(-1, -1))
	cx0, cy0, _, _ := v.chartBox(c)
	ox, oy := float64(rc.Min.X)-cx0*v.scale, float64(rc.Min.Y)-cy0*v.scale
	return image.Rect(int(math.Round(ox+x0*v.scale)), int(math.Round(oy+y0*v.scale)),
		int(math.Round(ox+x1*v.scale)), int(math.Round(oy+y1*v.scale)))
}

// ChartAnchor is anchor a of a chart moved or sized to the box x0, y0,
// x1, y1 (unzoomed pixels) of a sheet laid out by g, of the same kind:
// cells and offsets, a cell and a size, or a position and a size.
func ChartAnchor(a model.Anchor, g *Geometry, x0, y0, x1, y1 float64) model.Anchor {
	emu := func(px float64) int64 { return int64(math.Round(px * model.EMUPerPx)) }
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = max(x1, x0+1), max(y1, y0+1)
	at := func(x, y float64) model.CellOffset {
		col := g.Cols.Clamp(g.Cols.Index(x))
		row := g.Rows.Clamp(g.Rows.Index(y))
		return model.CellOffset{Row: row, Col: col, DX: emu(max(x-g.Cols.Start(col), 0)), DY: emu(max(y-g.Rows.Start(row), 0))}
	}
	out := a
	switch a.Kind {
	case model.AnchorAbsolute:
		out.PosX, out.PosY = emu(x0), emu(y0)
		out.ExtX, out.ExtY = emu(x1-x0), emu(y1-y0)
	case model.AnchorOneCell:
		out.From = at(x0, y0)
		out.ExtX, out.ExtY = emu(x1-x0), emu(y1-y0)
	default:
		out.From, out.To = at(x0, y0), at(x1, y1)
		out.ExtX, out.ExtY = emu(x1-x0), emu(y1-y0)
	}
	return out
}

// ChartElementAt is the part of chart c under a point (grid
// coordinates, from the last Draw): ElemChartArea where nothing else is.
func (v *View) ChartElementAt(c *model.Chart, x, y float32) ChartElem {
	prims, ok := v.chartCache[c]
	if !ok || v.scale == 0 {
		return ChartElem{Kind: ElemChartArea}
	}
	rc, _ := v.chartScreen(c, image.Pt(int(x), int(y)))
	s := float32(v.scale)
	px, py := (x-float32(rc.Min.X))/s, (y-float32(rc.Min.Y))/s
	slop := 4 / s
	// Titles and the legend take the whole box they fill.
	for _, k := range []ChartElemKind{ElemTitle, ElemAxisTitle, ElemLegend} {
		for e, b := range elemBounds(prims) {
			if e.Kind == k && px >= b.x0-2 && px <= b.x1+2 && py >= b.y0-2 && py <= b.y1+2 {
				return e
			}
		}
	}
	for i := len(prims) - 1; i >= 0; i-- {
		pr := &prims[i]
		if pr.elem.Kind <= ElemChartArea {
			continue
		}
		switch pr.kind {
		case primFill:
			if inPoly(pr.pts, px, py) {
				return pr.elem
			}
		case primStroke:
			pts := pr.pts
			if pr.closed {
				pts = append(pts[:len(pts):len(pts)], pts[0])
			}
			for k := 0; k+1 < len(pts); k++ {
				if segDist(pts[k], pts[k+1], px, py) <= max(pr.width/2, slop) {
					return pr.elem
				}
			}
		case primText:
			b := pr.box
			if px >= b.x0 && px <= b.x1 && py >= b.y0 && py <= b.y1 {
				return pr.elem
			}
		}
	}
	return ChartElem{Kind: ElemChartArea}
}

// ChartElementRect is where element e of chart c is on screen (grid
// coordinates, from the last Draw), with the points of a series (the
// middles of its bars or slices, its line's vertices) for its handles.
func (v *View) ChartElementRect(c *model.Chart, e ChartElem) (image.Rectangle, []image.Point, bool) {
	prims, ok := v.chartCache[c]
	if !ok || v.scale == 0 {
		return image.Rectangle{}, nil, false
	}
	rc, _ := v.chartScreen(c, image.Pt(-1, -1))
	s := float32(v.scale)
	to := func(x, y float32) image.Point {
		return image.Pt(rc.Min.X+int(math.Round(float64(x*s))), rc.Min.Y+int(math.Round(float64(y*s))))
	}
	b, ok := elemBounds(prims)[e]
	if !ok {
		return image.Rectangle{}, nil, false
	}
	var pts []image.Point
	if e.Kind == ElemSeries {
		for i := range prims {
			pr := &prims[i]
			if pr.elem != e {
				continue
			}
			switch pr.kind {
			case primFill:
				pb := primBounds(pr)
				pts = append(pts, to((pb.x0+pb.x1)/2, (pb.y0+pb.y1)/2))
			case primStroke:
				for _, q := range pr.pts {
					pts = append(pts, to(q.X, q.Y))
				}
			}
			if len(pts) > 200 {
				break
			}
		}
	}
	return image.Rectangle{Min: to(b.x0, b.y0), Max: to(b.x1, b.y1)}, pts, true
}

// elemBounds are the bounds of each element's primitives.
func elemBounds(prims []prim) map[ChartElem]rect32 {
	out := map[ChartElem]rect32{}
	for i := range prims {
		pr := &prims[i]
		b := primBounds(pr)
		if o, ok := out[pr.elem]; ok {
			b = rect32{min(o.x0, b.x0), min(o.y0, b.y0), max(o.x1, b.x1), max(o.y1, b.y1)}
		}
		out[pr.elem] = b
	}
	return out
}

func primBounds(pr *prim) rect32 {
	if pr.kind == primText {
		return pr.box
	}
	b := rect32{pr.pts[0].X, pr.pts[0].Y, pr.pts[0].X, pr.pts[0].Y}
	for _, q := range pr.pts[1:] {
		b = rect32{min(b.x0, q.X), min(b.y0, q.Y), max(b.x1, q.X), max(b.y1, q.Y)}
	}
	h := pr.width / 2
	return rect32{b.x0 - h, b.y0 - h, b.x1 + h, b.y1 + h}
}

// inPoly reports whether x, y is inside a polygon (even-odd).
func inPoly(pts []chartPt, x, y float32) bool {
	in := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		a, b := pts[i], pts[j]
		if (a.Y > y) != (b.Y > y) && x < (b.X-a.X)*(y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// segDist is the distance from x, y to the segment a-b.
func segDist(a, b chartPt, x, y float32) float32 {
	dx, dy := b.X-a.X, b.Y-a.Y
	t := float32(0)
	if l := dx*dx + dy*dy; l > 0 {
		t = min(max(((x-a.X)*dx+(y-a.Y)*dy)/l, 0), 1)
	}
	ex, ey := a.X+t*dx-x, a.Y+t*dy-y
	return float32(math.Sqrt(float64(ex*ex + ey*ey)))
}

// drawElemSelection marks the selected element of a chart drawn at rc:
// a frame with corner handles, or a series' points.
func (v *View) drawElemSelection(gtx layout.Context, c *model.Chart) {
	e := v.SelElem
	if e.Kind <= ElemChartArea {
		return
	}
	r, pts, ok := v.ChartElementRect(c, e)
	if !ok {
		return
	}
	ops := gtx.Ops
	sq := func(p image.Point) {
		h := max(v.px(3), 2)
		gui.FillRect(ops, image.Rect(p.X-h-1, p.Y-h-1, p.X+h+2, p.Y+h+2), chartHandleOut)
		gui.FillRect(ops, image.Rect(p.X-h, p.Y-h, p.X+h+1, p.Y+h+1), colors.White)
	}
	if e.Kind == ElemSeries {
		for _, p := range pts {
			sq(p)
		}
		return
	}
	r = r.Inset(-2)
	gui.HLine(ops, r.Min.X, r.Max.X, r.Min.Y, 1, chartSelElem)
	gui.HLine(ops, r.Min.X, r.Max.X, r.Max.Y-1, 1, chartSelElem)
	gui.VLine(ops, r.Min.X, r.Min.Y, r.Max.Y, 1, chartSelElem)
	gui.VLine(ops, r.Max.X-1, r.Min.Y, r.Max.Y, 1, chartSelElem)
	for _, p := range []image.Point{r.Min, {r.Max.X - 1, r.Min.Y}, {r.Min.X, r.Max.Y - 1}, r.Max.Sub(image.Pt(1, 1))} {
		sq(p)
	}
}
