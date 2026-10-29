package render

import (
	"image/color"
	"math"
	"strings"

	"officelite/excel/model"
)

// Charts with axes: columns and bars, lines, areas and scatter, alone or
// combined (a combo chart's groups may use secondary axes).

// plotAxes returns the category (X) and value (Y) axes of a group,
// creating their layout state on first use.
func (l *chartLayout) plotAxes(p *model.ChartPlot, first bool) (cat, val *axisInfo) {
	get := func(id int, isCat bool) *axisInfo {
		if a, ok := l.axes[id]; ok {
			return a
		}
		ax := l.c.Axis(id)
		if ax == nil {
			ax = &model.ChartAxis{ID: id, Kind: model.AxisVal, Crosses: "autoZero", TickLblPos: "nextTo"}
			if isCat && p.Type != model.ChartScatter {
				ax.Kind = model.AxisCat
				ax.Pos = "b"
			} else if isCat {
				ax.Pos = "b"
			} else {
				ax.Pos, ax.MajorGrid = "l", true
			}
		}
		a := &axisInfo{ax: ax, cat: ax.Kind != model.AxisVal, shown: !ax.Delete && ax.TickLblPos != "none", second: !first}
		l.axes[id] = a
		return a
	}
	ids := p.AxisIDs
	if len(ids) < 2 {
		ids = []int{-1, -2}
		if !first {
			ids = []int{-3, -4}
		}
	}
	cat, val = get(ids[0], true), get(ids[1], false)
	cat.cross, val.cross = val, cat
	vertCat := p.Type == model.ChartBar && p.Horizontal
	cat.vertical, val.vertical = vertCat, !vertCat
	return cat, val
}

// valueRange is the data range a group puts on its value axis (and, for
// scatter, on its X axis).
func (l *chartLayout) valueRange(p *model.ChartPlot) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	var ss []*series
	for _, sd := range l.ser {
		if sd.plot == p {
			ss = append(ss, sd)
		}
	}
	zeroBlanks := l.c.DispBlanks == model.BlanksZero
	switch {
	case p.Grouping == model.GroupPercent && p.Type != model.ChartScatter:
		lo, hi = 0, 1
		for _, sd := range ss {
			for i, v := range sd.vals {
				if sd.ok[i] && v < 0 {
					lo = -1
				}
			}
		}
		return lo, hi, true
	case p.Grouping == model.GroupStacked && p.Type != model.ChartScatter:
		for i := 0; i < l.nCat; i++ {
			pos, neg := 0.0, 0.0
			for _, sd := range ss {
				if i < len(sd.vals) && sd.ok[i] {
					if v := sd.vals[i]; v >= 0 {
						pos += v
					} else {
						neg += v
					}
				}
			}
			lo, hi = min(lo, neg, pos), max(hi, pos, neg)
		}
	default:
		for _, sd := range ss {
			for i, v := range sd.vals {
				if sd.ok[i] {
					lo, hi = min(lo, v), max(hi, v)
				} else if zeroBlanks {
					lo, hi = min(lo, 0), max(hi, 0)
				}
			}
		}
	}
	return lo, hi, lo <= hi
}

func (l *chartLayout) xRange(p *model.ChartPlot) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, sd := range l.ser {
		if sd.plot != p {
			continue
		}
		for i, x := range sd.xs {
			if sd.xok[i] && i < len(sd.ok) && sd.ok[i] {
				lo, hi = min(lo, x), max(hi, x)
			}
		}
	}
	return lo, hi, lo <= hi
}

// scaleAxis picks a value axis' scale for data lo..hi along length px.
func (l *chartLayout) scaleAxis(a *axisInfo, lo, hi float64, length float32, percent bool) {
	ax := a.ax
	o := scaleOpts{Min: ax.Min, Max: ax.Max, HasMin: ax.HasMin, HasMax: ax.HasMax, Major: ax.MajorUnit, Minor: ax.MinorUnit,
		HasUnit: ax.MajorUnit > 0, LogBase: ax.LogBase}
	if percent {
		if !o.HasMin {
			o.Min, o.HasMin = lo, true
		}
		if !o.HasMax {
			o.Max, o.HasMax = hi, true
		}
		if !o.HasUnit {
			o.Major, o.HasUnit = 0.1, true
		}
	}
	// No more intervals than the labels have room for.
	_, lh := l.measure(a.font, "0")
	per := lh * 1.15
	if !a.vertical {
		w1, _ := l.measure(a.font, l.formatNum(hi, a.fmt))
		w2, _ := l.measure(a.font, l.formatNum(lo, a.fmt))
		per = max(w1, w2) + 12
	}
	o.MaxSteps = int(min(max(float64(length/per), 2), 10))
	a.scale = niceScale(lo, hi, o)
	a.scale.Reversed = ax.Reversed
	a.scale.Percent = percent
	a.labels = a.labels[:0]
	for _, v := range a.scale.ticks() {
		a.labels = append(a.labels, l.formatNum(v, a.fmt))
	}
}

func (l *chartLayout) axesPlot(inner rect32) {
	// The groups and their axes; the first group's are the primary ones.
	type group struct {
		p        *model.ChartPlot
		cat, val *axisInfo
	}
	var groups []group
	for pi := range l.c.Plots {
		p := &l.c.Plots[pi]
		switch p.Type {
		case model.ChartPie, model.ChartDoughnut, model.ChartOther:
			continue
		}
		cat, val := l.plotAxes(p, len(groups) == 0)
		groups = append(groups, group{p, cat, val})
	}
	var all []*axisInfo
	seen := map[*axisInfo]bool{}
	for _, g := range groups {
		for _, a := range []*axisInfo{g.cat, g.val} {
			if !seen[a] {
				seen[a] = true
				all = append(all, a)
				a.font = l.resolveFont(a.ax.Font, l.font)
			}
		}
	}

	// Number formats: percentages for 100% stacked groups, the axis' own,
	// or the first series' (source linked).
	for _, g := range groups {
		for _, sd := range l.ser {
			if sd.plot != g.p {
				continue
			}
			g.val.fmt = l.axisFormat(g.val.ax, sd.fmt, g.p.Grouping == model.GroupPercent)
			if g.p.Type == model.ChartScatter {
				g.cat.fmt = l.axisFormat(g.cat.ax, numFormat{}, false)
			}
			break
		}
	}

	ranges := func() map[*axisInfo][3]float64 {
		r := map[*axisInfo][3]float64{}
		for _, g := range groups {
			lo, hi, ok := l.valueRange(g.p)
			if ok {
				prev, had := r[g.val]
				if had {
					lo, hi = min(lo, prev[0]), max(hi, prev[1])
				}
				pct := 0.0
				if g.p.Grouping == model.GroupPercent {
					pct = 1
				}
				r[g.val] = [3]float64{lo, hi, pct}
			}
			if g.p.Type == model.ChartScatter {
				if lo, hi, ok := l.xRange(g.p); ok {
					prev, had := r[g.cat]
					if had {
						lo, hi = min(lo, prev[0]), max(hi, prev[1])
					}
					r[g.cat] = [3]float64{lo, hi, 0}
				}
			}
		}
		return r
	}()
	scaleAll := func(plot rect32) {
		for a, r := range ranges {
			length := plot.h()
			if !a.vertical {
				length = plot.w()
			}
			l.scaleAxis(a, r[0], r[1], length, r[2] == 1)
		}
		for _, a := range all {
			if _, ok := ranges[a]; !ok && !a.cat {
				l.scaleAxis(a, 0, 1, 200, false)
			}
		}
	}

	// Category axes: one slot per category.
	for _, a := range all {
		if a.cat {
			a.labels = l.catLbl
			a.between = true
			if v := a.cross; v != nil {
				switch v.ax.CrossBetween {
				case "midCat":
					a.between = false
				case "":
					for _, g := range groups {
						if g.cat == a && g.p.Type == model.ChartArea {
							a.between = false
						}
					}
				}
			}
		}
	}

	// Two passes: the label sizes depend on the scales, which depend on
	// the plot's size.
	plot := inner
	scaleAll(plot)
	for pass := 0; pass < 2; pass++ {
		plot = inner
		for _, a := range all {
			l.reserve(a, &plot, inner)
		}
		if plot.w() < 10 || plot.h() < 10 {
			return
		}
		scaleAll(plot)
	}
	l.plot = plot

	// The plot area, then gridlines, then the series by kind.
	l.elem = ChartElem{Kind: ElemPlotArea}
	l.fillRect(plot, shapeFill(l.c.PlotArea, color.NRGBA{}))
	if lc, lw := shapeLine(l.c.PlotArea, color.NRGBA{}, 1); lc.A > 0 {
		l.stroke([]chartPt{{plot.x0, plot.y0}, {plot.x1, plot.y0}, {plot.x1, plot.y1}, {plot.x0, plot.y1}}, true, lc, lw, nil)
	}
	for _, a := range all {
		l.elem = ChartElem{Kind: ElemGridlines, Axis: l.axisIndex(a)}
		l.gridlines(a)
	}
	for _, kind := range []model.ChartType{model.ChartArea, model.ChartBar, model.ChartLine, model.ChartScatter} {
		for _, g := range groups {
			if g.p.Type != kind {
				continue
			}
			switch kind {
			case model.ChartArea:
				l.areas(g.p, g.cat, g.val)
			case model.ChartBar:
				l.bars(g.p, g.cat, g.val)
			default:
				l.lines(g.p, g.cat, g.val)
			}
		}
	}
	for _, a := range all {
		l.elem = ChartElem{Kind: ElemAxis, Axis: l.axisIndex(a)}
		l.axisLine(a)
	}
	for _, a := range all {
		l.elem = ChartElem{Kind: ElemAxis, Axis: l.axisIndex(a)}
		l.axisLabels(a)
	}
	for _, g := range groups {
		l.dataLabels(g.p, g.cat, g.val)
	}
}

// axisIndex is an axis' index in the chart's Axes (-1 for one made up
// for a group without).
func (l *chartLayout) axisIndex(a *axisInfo) int {
	for i := range l.c.Axes {
		if &l.c.Axes[i] == a.ax {
			return i
		}
	}
	return -1
}

// axisFormat is the number format of a value axis' labels.
func (l *chartLayout) axisFormat(ax *model.ChartAxis, src numFormat, percent bool) numFormat {
	switch {
	case percent && (ax.SourceLinked || ax.NumFmt == "" || strings.EqualFold(ax.NumFmt, "General")):
		return numFormat{id: 9}
	case !ax.SourceLinked && ax.NumFmt != "":
		return numFormat{code: ax.NumFmt}
	case src.code == "" && src.id == 0 && ax.NumFmt != "":
		return numFormat{code: ax.NumFmt}
	}
	return src
}

// reserve takes the room an axis' labels and title need from plot.
func (l *chartLayout) reserve(a *axisInfo, plot *rect32, inner rect32) {
	if !a.shown && (a.ax.Title == nil || a.ax.Delete) {
		return
	}
	_, lh := l.measure(a.font, "Ag")
	room := float32(0)
	if a.shown {
		if a.vertical {
			for _, s := range a.labels {
				w, _ := l.measure(a.font, s)
				room = max(room, w)
			}
			room += 6
		} else if a.cat {
			room = l.catLabelRoom(a, plot.w()) + 4
		} else {
			room = lh + 4
		}
	}
	if t, f, ok := l.axisTitle(a); ok {
		_, th := l.textBlock(f, t)
		room += th + 4
	}
	if a.cat && !a.vertical && a.shown && a.labelRot != 0 && len(a.labels) > 0 && l.side(a) == "b" {
		// Turned labels end at their category: the first reaches left
		// of the plot by its width along the slant.
		w0, _ := l.measure(a.font, a.labels[0])
		slot := plot.w() / float32(len(a.labels))
		over := w0*float32(math.Cos(float64(a.labelRot)*math.Pi/180)) - slot/2
		plot.x0 = max(plot.x0, min(inner.x0+over, inner.x0+inner.w()/3))
	}
	if a.cat && !a.vertical && !a.between && a.shown && a.labelRot == 0 && len(a.labels) > 0 {
		// Labels on the plot's edges (areas): room for their halves.
		w0, _ := l.measure(a.font, a.labels[0])
		w1, _ := l.measure(a.font, a.labels[len(a.labels)-1])
		plot.x0 = max(plot.x0, inner.x0+w0/2)
		plot.x1 = min(plot.x1, inner.x1-w1/2)
	}
	switch l.side(a) {
	case "l":
		plot.x0 = max(plot.x0, inner.x0+room)
	case "r":
		plot.x1 = min(plot.x1, inner.x1-room)
	case "t":
		plot.y0 = max(plot.y0, inner.y0+room)
	default:
		plot.y1 = min(plot.y1, inner.y1-room)
	}
}

// side is where an axis' labels go: l, r, t or b.
func (l *chartLayout) side(a *axisInfo) string {
	pos := a.ax.Pos
	if a.vertical {
		if pos == "r" || pos != "l" && a.second {
			return "r"
		}
		if v := a.cross; v != nil && v.ax.Crosses == "max" && a.ax.Pos != "l" {
			return "r"
		}
		return "l"
	}
	if pos == "t" || (a.cross != nil && a.cross.ax.Crosses == "max" && pos != "b") {
		return "t"
	}
	return "b"
}

func (l *chartLayout) axisTitle(a *axisInfo) (string, chartFont, bool) {
	t := a.ax.Title
	if t == nil || a.ax.Delete {
		return "", chartFont{}, false
	}
	base := l.font
	base.SizePt = 10
	if l.c.Font.SizePt > 0 {
		base.SizePt = l.c.Font.SizePt
	}
	base.Bold = false
	f := l.resolveFont(t.Font, base)
	switch {
	case t.Text != "":
		return t.Text, f, true
	case t.Ref.Ref != "" || len(t.Ref.Cache) > 0:
		vals, _ := l.values(&t.Ref)
		var parts []string
		for _, v := range vals {
			parts = append(parts, v.Text)
		}
		return strings.Join(parts, " "), f, true
	}
	return "Axis Title", f, true
}

// catLabelRoom decides how category labels fit along a horizontal axis
// of width w (straight, wrapped onto two lines, turned 45° or thinned
// out) and returns their height.
func (l *chartLayout) catLabelRoom(a *axisInfo, w float32) float32 {
	n := max(len(a.labels), 1)
	slot := w / float32(n)
	_, lh := l.measure(a.font, "Ag")
	widest := float32(0)
	for _, s := range a.labels {
		tw, _ := l.measure(a.font, s)
		widest = max(widest, tw)
	}
	a.labelRot, a.lines, a.skip = 0, 1, max(a.ax.LblSkip, 1)
	if a.ax.Font.HasRot {
		a.labelRot = float32(a.ax.Font.Rotation)
	}
	switch {
	case a.labelRot != 0:
	case widest <= slot-4:
		return lh
	default:
		// Two lines, split at a space, when each half fits.
		fits := true
		for _, s := range a.labels {
			l1, l2 := splitHalf(s)
			w1, _ := l.measure(a.font, l1)
			w2, _ := l.measure(a.font, l2)
			if max(w1, w2) > slot-4 {
				fits = false
				break
			}
		}
		if fits {
			a.lines = 2
			return 2 * lh
		}
		a.labelRot = -45
	}
	rad := math.Abs(float64(a.labelRot)) * math.Pi / 180
	// Turned labels need a line height of room each along the axis.
	if per := lh / float32(math.Sin(rad)+1e-6); a.ax.LblSkip == 0 && per > slot {
		a.skip = int(math.Ceil(float64(per / slot)))
	}
	h := widest*float32(math.Sin(rad)) + lh*float32(math.Cos(rad))
	return min(h, l.h/2)
}

// splitHalf splits s at the space nearest its middle.
func splitHalf(s string) (string, string) {
	best, bi := len(s)+1, -1
	for i, r := range s {
		if r == ' ' {
			if d := int(math.Abs(float64(i - len(s)/2))); d < best {
				best, bi = d, i
			}
		}
	}
	if bi < 0 {
		return s, ""
	}
	return s[:bi], s[bi+1:]
}

// --- positions ---

// catPos is the centre of category i along a category axis.
func (l *chartLayout) catPos(a *axisInfo, i int, n int) float32 {
	n = max(n, 1)
	var f float32
	if a.between || n == 1 {
		f = (float32(i) + 0.5) / float32(n)
	} else {
		f = float32(i) / float32(n-1)
	}
	if a.ax.Reversed {
		f = 1 - f
	}
	if a.vertical {
		return l.plot.y1 - f*l.plot.h()
	}
	return l.plot.x0 + f*l.plot.w()
}

// slotWidth is a category's width.
func (l *chartLayout) slotWidth(a *axisInfo, n int) float32 {
	length := l.plot.w()
	if a.vertical {
		length = l.plot.h()
	}
	return length / float32(max(n, 1))
}

// valPos is a value's position along a value axis.
func (l *chartLayout) valPos(a *axisInfo, v float64) float32 {
	f := float32(a.scale.frac(v))
	if a.vertical {
		return l.plot.y1 - f*l.plot.h()
	}
	return l.plot.x0 + f*l.plot.w()
}

// crossValue is the value where the category axis crosses the value
// axis v: the bars' base.
func (l *chartLayout) crossValue(v *axisInfo) float64 {
	s := &v.scale
	c := v.cross
	crosses := "autoZero"
	if c != nil {
		crosses = c.ax.Crosses
		if c.ax.HasCrossesAt {
			return min(max(c.ax.CrossesAt, s.Min), s.Max)
		}
	}
	switch crosses {
	case "min":
		return s.Min
	case "max":
		return s.Max
	}
	if s.Log > 1 {
		return s.Min
	}
	return min(max(0, s.Min), s.Max)
}

// point maps a category position and a value position to x, y.
func point(cat *axisInfo, cp, vp float32) chartPt {
	if cat.vertical {
		return chartPt{vp, cp}
	}
	return chartPt{cp, vp}
}

// --- gridlines and axes ---

func (l *chartLayout) gridlines(a *axisInfo) {
	p := l.plot
	draw := func(pos float32, c color.NRGBA, w float32, dash []float32) {
		if a.vertical {
			l.stroke([]chartPt{{p.x0, pos}, {p.x1, pos}}, false, c, w, dash)
		} else {
			l.stroke([]chartPt{{pos, p.y0}, {pos, p.y1}}, false, c, w, dash)
		}
	}
	if a.ax.MinorGrid && !a.cat {
		c, w := shapeLine(a.ax.MinorGridLine, colorsMix(chartGrid), gridW)
		for _, v := range a.scale.minorTicks() {
			draw(l.valPos(a, v), c, w, dashOf(a.ax.MinorGridLine.Dash, w))
		}
	}
	if !a.ax.MajorGrid {
		return
	}
	c, w := shapeLine(a.ax.MajorGridLine, chartGrid, gridW)
	dash := dashOf(a.ax.MajorGridLine.Dash, w)
	if a.cat {
		n := max(len(a.labels), 1)
		for i := 0; i <= n; i++ {
			f := float32(i) / float32(n)
			if a.vertical {
				draw(p.y1-f*p.h(), c, w, dash)
			} else {
				draw(p.x0+f*p.w(), c, w, dash)
			}
		}
		return
	}
	for _, v := range a.scale.ticks() {
		draw(l.valPos(a, v), c, w, dash)
	}
}

// colorsMix is the minor gridlines' default: lighter than the major ones.
func colorsMix(c color.NRGBA) color.NRGBA {
	return color.NRGBA{R: c.R + (255-c.R)/2, G: c.G + (255-c.G)/2, B: c.B + (255-c.B)/2, A: c.A}
}

// axisAt is where an axis' line lies across the plot.
func (l *chartLayout) axisAt(a *axisInfo) float32 {
	p := l.plot
	o := a.cross
	if o == nil {
		if a.vertical {
			return p.x0
		}
		return p.y1
	}
	if !o.cat {
		return l.valPos(o, l.crossValue(o))
	}
	// A value axis crosses a category axis at its start or end.
	end := a.ax.Crosses == "max"
	if o.ax.Reversed {
		end = !end
	}
	if a.vertical {
		if end {
			return p.x1
		}
		return p.x0
	}
	if end {
		return p.y0
	}
	return p.y1
}

func (l *chartLayout) axisLine(a *axisInfo) {
	if a.ax.Delete {
		return
	}
	def := color.NRGBA{}
	if a.cat {
		def = chartBorder
	}
	c, w := shapeLine(a.ax.Line, def, gridW)
	at := l.axisAt(a)
	p := l.plot
	if c.A > 0 {
		if a.vertical {
			l.stroke([]chartPt{{at, p.y0}, {at, p.y1}}, false, c, w, dashOf(a.ax.Line.Dash, w))
		} else {
			l.stroke([]chartPt{{p.x0, at}, {p.x1, at}}, false, c, w, dashOf(a.ax.Line.Dash, w))
		}
	}
	// Major tick marks.
	if c.A == 0 || a.ax.MajorTick == "none" || a.ax.MajorTick == "" {
		return
	}
	var t0, t1 float32 = 0, 4
	switch a.ax.MajorTick {
	case "in":
		t0, t1 = -4, 0
	case "cross":
		t0, t1 = -4, 4
	}
	sign := float32(1)
	if s := l.side(a); s == "l" || s == "t" {
		sign = -1
	}
	var pos []float32
	if a.cat {
		n := max(len(a.labels), 1)
		for i := 0; i <= n; i++ {
			f := float32(i) / float32(n)
			if !a.between && n > 1 {
				if i == n {
					break
				}
				f = float32(i) / float32(n-1)
			}
			if a.vertical {
				pos = append(pos, p.y1-f*p.h())
			} else {
				pos = append(pos, p.x0+f*p.w())
			}
		}
	} else {
		for _, v := range a.scale.ticks() {
			pos = append(pos, l.valPos(a, v))
		}
	}
	for _, q := range pos {
		if a.vertical {
			l.line(at+sign*t0, q, at+sign*t1, q, c, w)
		} else {
			l.line(q, at+sign*t0, q, at+sign*t1, c, w)
		}
	}
}

// labelLine is where an axis' labels go across the plot: next to its
// line, or at the low or high edge.
func (l *chartLayout) labelLine(a *axisInfo) float32 {
	p := l.plot
	s := l.side(a)
	edge := map[string]float32{"l": p.x0, "r": p.x1, "t": p.y0, "b": p.y1}[s]
	switch a.ax.TickLblPos {
	case "low":
		return edge
	case "high":
		return map[string]float32{"l": p.x1, "r": p.x0, "t": p.y1, "b": p.y0}[s]
	}
	at := l.axisAt(a)
	// Next to an axis line inside the plot, but never off it.
	switch s {
	case "l":
		return min(max(at, p.x0), p.x1)
	case "r":
		return max(min(at, p.x1), p.x0)
	}
	return min(max(at, p.y0), p.y1)
}

func (l *chartLayout) axisLabels(a *axisInfo) {
	s := l.side(a)
	at := l.labelLine(a)
	const gap = 4
	if a.shown {
		if a.cat {
			n := len(a.labels)
			for i := 0; i < n; i += max(a.skip, 1) {
				cp := l.catPos(a, i, n)
				l.catLabel(a, a.labels[i], s, at, cp)
			}
		} else {
			for i, v := range a.scale.ticks() {
				if i >= len(a.labels) {
					break
				}
				vp := l.valPos(a, v)
				switch s {
				case "l":
					l.text(a.labels[i], a.font, at-gap, vp, 1, 0, 0)
				case "r":
					l.text(a.labels[i], a.font, at+gap, vp, -1, 0, 0)
				case "t":
					l.text(a.labels[i], a.font, vp, at-gap, 0, 1, 0)
				default:
					l.text(a.labels[i], a.font, vp, at+gap, 0, -1, 0)
				}
			}
		}
	}
	// The axis title, beyond the labels.
	t, f, ok := l.axisTitle(a)
	if !ok {
		return
	}
	l.elem = ChartElem{Kind: ElemAxisTitle, Axis: l.axisIndex(a)}
	p := l.plot
	_, th := l.textBlock(f, t)
	switch s {
	case "l":
		l.text(t, f, chartPad+th/2, (p.y0+p.y1)/2, 0, 0, -90)
	case "r":
		l.text(t, f, l.w-chartPad-th/2, (p.y0+p.y1)/2, 0, 0, -90)
	case "t":
		l.text(t, f, (p.x0+p.x1)/2, p.y0-th-gap, 0, 1, 0)
	default:
		y := l.h - chartPad - th
		if lg := l.c.Legend; lg != nil && !lg.Overlay && (lg.Pos == "b" || lg.Pos == "") {
			_, lh := l.measure(l.font, "Ag")
			y -= lh + 8
		}
		l.multiText(t, f, (p.x0+p.x1)/2, y)
	}
}

// catLabel draws one category label beside the axis.
func (l *chartLayout) catLabel(a *axisInfo, s, side string, at, cp float32) {
	const gap = 4
	switch side {
	case "l":
		l.text(s, a.font, at-gap, cp, 1, 0, 0)
	case "r":
		l.text(s, a.font, at+gap, cp, -1, 0, 0)
	case "t":
		l.text(s, a.font, cp, at-gap, 0, 1, 0)
	default:
		switch {
		case a.labelRot != 0:
			// Turned labels end at the tick, reading up to the right.
			l.text(s, a.font, cp, at+gap, 1, -1, a.labelRot)
		case a.lines == 2:
			l1, l2 := splitHalf(s)
			_, lh := l.measure(a.font, "Ag")
			l.text(l1, a.font, cp, at+gap, 0, -1, 0)
			l.text(l2, a.font, cp, at+gap+lh, 0, -1, 0)
		default:
			l.text(s, a.font, cp, at+gap, 0, -1, 0)
		}
	}
}

// --- series ---

func (l *chartLayout) groupSeries(p *model.ChartPlot) []*series {
	var out []*series
	for _, sd := range l.ser {
		if sd.plot == p {
			out = append(out, sd)
		}
	}
	return out
}

// stackedValues returns each series' bottom and top value per category
// for a stacked or 100% stacked group (blanks count as 0).
func (l *chartLayout) stackedValues(p *model.ChartPlot, ss []*series) (lo, hi [][]float64) {
	n := l.nCat
	lo, hi = make([][]float64, len(ss)), make([][]float64, len(ss))
	totals := make([]float64, n)
	if p.Grouping == model.GroupPercent {
		for _, sd := range ss {
			for i := 0; i < n && i < len(sd.vals); i++ {
				if sd.ok[i] {
					totals[i] += math.Abs(sd.vals[i])
				}
			}
		}
	}
	pos, neg := make([]float64, n), make([]float64, n)
	for k, sd := range ss {
		lo[k], hi[k] = make([]float64, n), make([]float64, n)
		for i := 0; i < n; i++ {
			v := 0.0
			if i < len(sd.vals) && sd.ok[i] {
				v = sd.vals[i]
			}
			if p.Grouping == model.GroupPercent && totals[i] > 0 {
				v /= totals[i]
			}
			// Areas and lines stack on the running sum; bars stack
			// positive and negative values apart.
			if p.Type == model.ChartBar && v < 0 {
				lo[k][i], hi[k][i] = neg[i], neg[i]+v
				neg[i] += v
				continue
			}
			lo[k][i], hi[k][i] = pos[i], pos[i]+v
			pos[i] += v
		}
	}
	return lo, hi
}

func (l *chartLayout) bars(p *model.ChartPlot, cat, val *axisInfo) {
	ss := l.groupSeries(p)
	if len(ss) == 0 {
		return
	}
	n := l.nCat
	slot := l.slotWidth(cat, n)
	stacked := p.Grouping != model.GroupClustered
	k := float32(len(ss))
	ov := float32(min(max(p.Overlap, -100), 100)) / 100
	if stacked {
		k, ov = 1, 1
	}
	gap := float32(min(max(p.GapWidth, 0), 500)) / 100
	bw := slot / (k - (k-1)*ov + gap)
	base := l.valPos(val, l.crossValue(val))
	var lo, hi [][]float64
	if stacked {
		lo, hi = l.stackedValues(p, ss)
	}
	for si, sd := range ss {
		l.elem = sd.elem(ElemSeries)
		lc, lw := shapeLine(sd.s.Shape, color.NRGBA{}, 1)
		for i := 0; i < n && i < len(sd.vals); i++ {
			if !sd.ok[i] && !stacked {
				continue
			}
			c0 := l.catPos(cat, i, n) - slot/2 + gap*bw/2
			if !stacked {
				c0 += float32(si) * (bw - ov*bw)
			}
			if cat.ax.Reversed != cat.vertical {
				// Series run the other way along a reversed (or a bar
				// chart's upward) category axis.
				c0 = l.catPos(cat, i, n) + slot/2 - gap*bw/2 - bw
				if !stacked {
					c0 -= float32(si) * (bw - ov*bw)
				}
			}
			v0, v1 := base, l.valPos(val, sd.vals[i])
			if stacked {
				v0, v1 = l.valPos(val, lo[si][i]), l.valPos(val, hi[si][i])
			}
			if math.Abs(float64(v1-v0)) < 0.01 {
				continue
			}
			var r rect32
			if cat.vertical {
				r = rect32{min(v0, v1), c0, max(v0, v1), c0 + bw}
			} else {
				r = rect32{c0, min(v0, v1), c0 + bw, max(v0, v1)}
			}
			r = clipRect(r, l.plot)
			fill := l.pointFill(sd, i)
			if sd.s.InvertNeg && sd.vals[i] < 0 {
				fill = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				if lc.A == 0 {
					lc, lw = sd.fill, 1
				}
			}
			l.fillRect(r, fill)
			if lc.A > 0 {
				l.stroke([]chartPt{{r.x0, r.y0}, {r.x1, r.y0}, {r.x1, r.y1}, {r.x0, r.y1}}, true, lc, lw, dashOf(sd.s.Shape.Dash, lw))
			}
		}
	}
}

func clipRect(r, c rect32) rect32 {
	return rect32{max(r.x0, c.x0), max(r.y0, c.y0), min(r.x1, c.x1), min(r.y1, c.y1)}
}

func (l *chartLayout) areas(p *model.ChartPlot, cat, val *axisInfo) {
	ss := l.groupSeries(p)
	n := l.nCat
	if len(ss) == 0 || n == 0 {
		return
	}
	stacked := p.Grouping != model.GroupClustered
	var lo, hi [][]float64
	if stacked {
		lo, hi = l.stackedValues(p, ss)
	}
	base := l.valPos(val, l.crossValue(val))
	for si, sd := range ss {
		l.elem = sd.elem(ElemSeries)
		var top, bottom []chartPt
		for i := 0; i < n; i++ {
			cp := l.catPos(cat, i, n)
			v := 0.0
			if i < len(sd.vals) && sd.ok[i] {
				v = sd.vals[i]
			}
			b := base
			if stacked {
				v = hi[si][i]
				b = l.valPos(val, lo[si][i])
			}
			top = append(top, point(cat, cp, l.valPos(val, v)))
			bottom = append(bottom, point(cat, cp, b))
		}
		if n == 1 {
			// One category: a band across the slot.
			w := l.slotWidth(cat, n) / 2
			shift := func(pt chartPt, d float32) chartPt {
				if cat.vertical {
					pt.Y += d
				} else {
					pt.X += d
				}
				return pt
			}
			top = []chartPt{shift(top[0], -w), shift(top[0], w)}
			bottom = []chartPt{shift(bottom[0], -w), shift(bottom[0], w)}
		}
		poly := append([]chartPt(nil), top...)
		for i := len(bottom) - 1; i >= 0; i-- {
			poly = append(poly, bottom[i])
		}
		l.poly(poly, sd.fill)
		if lc, lw := shapeLine(sd.s.Shape, color.NRGBA{}, 1); lc.A > 0 {
			l.stroke(poly, true, lc, lw, dashOf(sd.s.Shape.Dash, lw))
		}
	}
}

// drawsLine reports whether a line or scatter series has its line.
func (l *chartLayout) drawsLine(sd *series) bool {
	if sd.s.Shape.Line.Kind == model.FillNone {
		return false
	}
	if sd.plot.Type == model.ChartScatter && sd.s.Shape.Line.Kind == model.FillAuto {
		st := sd.plot.ScatterStyle
		return st != "marker" && st != ""
	}
	return sd.line.A > 0
}

func (l *chartLayout) lineWidth(sd *series) float32 {
	_, w := shapeLine(sd.s.Shape, sd.line, defLineW)
	return w
}

// markerOf is a series' marker symbol and size (px).
func (l *chartLayout) markerOf(sd *series) (string, float32) {
	sym := sd.s.Marker.Symbol
	if sym == "" || sym == "auto" {
		switch {
		case sd.plot.Type == model.ChartScatter && (sd.plot.ScatterStyle == "line" || sd.plot.ScatterStyle == "smooth"):
			sym = "none"
		case sd.plot.Type == model.ChartLine && !sd.plot.Marker && sd.s.Marker.Symbol == "":
			sym = "none"
		default:
			sym = autoMarkers[sd.index%len(autoMarkers)]
		}
	}
	size := sd.s.Marker.Size
	if size <= 0 {
		size = defMarker
	}
	return sym, float32(size) * 96 / 72
}

func (l *chartLayout) lines(p *model.ChartPlot, cat, val *axisInfo) {
	ss := l.groupSeries(p)
	n := l.nCat
	stacked := p.Grouping != model.GroupClustered && p.Type == model.ChartLine
	var hi [][]float64
	if stacked {
		_, hi = l.stackedValues(p, ss)
	}
	for si, sd := range ss {
		l.elem = sd.elem(ElemSeries)
		var segs [][]chartPt
		var cur []chartPt
		var marks []chartPt
		cnt := len(sd.vals)
		if p.Type == model.ChartLine {
			cnt = min(cnt, n)
		}
		for i := 0; i < cnt; i++ {
			ok := sd.ok[i]
			v := sd.vals[i]
			if stacked {
				v, ok = hi[si][i], true
			}
			if p.Type == model.ChartScatter && (i >= len(sd.xok) || !sd.xok[i]) {
				ok = false
			}
			if !ok {
				switch l.c.DispBlanks {
				case model.BlanksZero:
					v, ok = 0, true
				case model.BlanksSpan:
					continue
				}
			}
			if !ok {
				if len(cur) > 0 {
					segs = append(segs, cur)
					cur = nil
				}
				continue
			}
			var pt chartPt
			if p.Type == model.ChartScatter {
				pt = chartPt{l.valPos(cat, sd.xs[i]), l.valPos(val, v)}
				if cat.vertical {
					pt = chartPt{l.valPos(val, v), l.valPos(cat, sd.xs[i])}
				}
			} else {
				pt = point(cat, l.catPos(cat, i, n), l.valPos(val, v))
			}
			cur = append(cur, pt)
			marks = append(marks, pt)
		}
		if len(cur) > 0 {
			segs = append(segs, cur)
		}
		if l.drawsLine(sd) {
			w := l.lineWidth(sd)
			for _, seg := range segs {
				if sd.s.Smooth {
					seg = smoothLine(seg)
				}
				l.stroke(seg, false, sd.line, w, dashOf(sd.s.Shape.Dash, w))
			}
		}
		if sym, size := l.markerOf(sd); sym != "none" {
			for _, m := range marks {
				if m.X >= l.plot.x0-1 && m.X <= l.plot.x1+1 && m.Y >= l.plot.y0-1 && m.Y <= l.plot.y1+1 {
					l.marker(sd, sym, size, m.X, m.Y)
				}
			}
		}
	}
}

// smoothLine turns a polyline into a Catmull-Rom curve, as Excel smooths
// lines.
func smoothLine(pts []chartPt) []chartPt {
	if len(pts) < 3 {
		return pts
	}
	const steps = 10
	out := []chartPt{pts[0]}
	for i := 0; i < len(pts)-1; i++ {
		p0, p1, p2 := pts[max(i-1, 0)], pts[i], pts[i+1]
		p3 := pts[min(i+2, len(pts)-1)]
		for k := 1; k <= steps; k++ {
			t := float32(k) / steps
			t2, t3 := t*t, t*t*t
			f := func(a, b, c, d float32) float32 {
				return 0.5 * (2*b + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			out = append(out, chartPt{f(p0.X, p1.X, p2.X, p3.X), f(p0.Y, p1.Y, p2.Y, p3.Y)})
		}
	}
	return out
}

// marker draws a marker of size px centred on x, y.
func (l *chartLayout) marker(sd *series, sym string, size, x, y float32) {
	ms := sd.s.Marker.Shape
	fill := shapeFill(ms, sd.line)
	if sd.line.A == 0 {
		fill = shapeFill(ms, sd.fill)
	}
	lc, lw := shapeLine(ms, fill, 1)
	if sd.s.Marker.Shape.Line.Kind == model.FillAuto && sd.line.A > 0 {
		lc = sd.line
	}
	r := size / 2
	var pts []chartPt
	switch sym {
	case "square":
		pts = []chartPt{{x - r, y - r}, {x + r, y - r}, {x + r, y + r}, {x - r, y + r}}
	case "diamond":
		pts = []chartPt{{x, y - r}, {x + r, y}, {x, y + r}, {x - r, y}}
	case "triangle":
		pts = []chartPt{{x, y - r}, {x + r, y + r}, {x - r, y + r}}
	case "dash":
		pts = []chartPt{{x - r, y - r/4}, {x + r, y - r/4}, {x + r, y + r/4}, {x - r, y + r/4}}
	case "dot":
		pts = []chartPt{{x - r/3, y - r/3}, {x + r/3, y - r/3}, {x + r/3, y + r/3}, {x - r/3, y + r/3}}
	case "x", "star", "plus":
		if lc.A == 0 {
			lc = fill
		}
		if sym != "plus" {
			l.line(x-r, y-r, x+r, y+r, lc, max(lw, 1))
			l.line(x-r, y+r, x+r, y-r, lc, max(lw, 1))
		}
		if sym != "x" {
			l.line(x, y-r, x, y+r, lc, max(lw, 1))
			if sym == "plus" {
				l.line(x-r, y, x+r, y, lc, max(lw, 1))
			}
		}
		return
	default: // circle
		for k := 0; k < 16; k++ {
			a := float64(k) * math.Pi / 8
			pts = append(pts, chartPt{x + r*float32(math.Cos(a)), y + r*float32(math.Sin(a))})
		}
	}
	l.poly(pts, fill)
	if lc.A > 0 {
		l.stroke(pts, true, lc, lw, nil)
	}
}

// dataLabels writes the labels of a group's points.
func (l *chartLayout) dataLabels(p *model.ChartPlot, cat, val *axisInfo) {
	ss := l.groupSeries(p)
	n := l.nCat
	var lo, hi [][]float64
	stacked := p.Grouping != model.GroupClustered && p.Type != model.ChartScatter
	if stacked {
		lo, hi = l.stackedValues(p, ss)
	}
	slot := l.slotWidth(cat, n)
	k := float32(len(ss))
	ov := float32(min(max(p.Overlap, -100), 100)) / 100
	if stacked {
		k, ov = 1, 1
	}
	gap := float32(p.GapWidth) / 100
	bw := slot / (k - (k-1)*ov + gap)
	base := l.valPos(val, l.crossValue(val))
	for si, sd := range ss {
		lb := l.labelsOf(sd)
		if !lb.Any() {
			continue
		}
		l.elem = sd.elem(ElemLabels)
		f := l.resolveFont(lb.Font, l.labelFont())
		_, lh := l.measure(f, "Ag")
		pos := lb.Pos
		if pos == "" {
			switch {
			case p.Type == model.ChartBar && stacked, p.Type == model.ChartArea:
				pos = "ctr"
			case p.Type == model.ChartBar:
				pos = "outEnd"
			default:
				pos = "r"
			}
		}
		for i := 0; i < len(sd.vals); i++ {
			if !sd.ok[i] || (p.Type != model.ChartScatter && i >= n) {
				continue
			}
			pct := -1.0
			if p.Grouping == model.GroupPercent && stacked {
				pct = hi[si][i] - lo[si][i]
			}
			txt := l.labelText(sd, lb, i, pct)
			v := sd.vals[i]
			var cp, vp, vb float32
			switch {
			case p.Type == model.ChartScatter:
				if i >= len(sd.xs) || !sd.xok[i] {
					continue
				}
				cp, vp = l.valPos(cat, sd.xs[i]), l.valPos(val, v)
				vb = vp
			case stacked:
				cp = l.catPos(cat, i, n)
				vp, vb = l.valPos(val, hi[si][i]), l.valPos(val, lo[si][i])
			default:
				cp = l.catPos(cat, i, n)
				vp, vb = l.valPos(val, v), base
			}
			if p.Type == model.ChartBar && !stacked {
				off := -slot/2 + gap*bw/2 + float32(si)*(bw-ov*bw) + bw/2
				if cat.ax.Reversed != cat.vertical {
					off = -off
				}
				cp += off
			}
			// up is the direction values grow on screen.
			up := float32(-1)
			if !val.vertical {
				up = 1
			}
			if vp*up < vb*up {
				up = -up // a negative bar
			}
			var x, y float32
			ha := int8(0)
			if val.vertical {
				x, y = cp, vp
				switch pos {
				case "outEnd", "t":
					y += up * lh * 0.7
				case "inEnd":
					y -= up * lh * 0.7
				case "inBase":
					y = vb - up*lh*0.7
				case "ctr":
					y = (vp + vb) / 2
				case "b":
					y -= up * lh * 0.7
				case "r":
					x += 5
					ha = -1
				case "l":
					x -= 5
					ha = 1
				}
			} else {
				x, y = vp, cp
				switch pos {
				case "outEnd", "r":
					x += up * 5
					ha = -1
					if up < 0 {
						ha = 1
					}
				case "inEnd":
					x -= up * 5
					ha = 1
					if up < 0 {
						ha = -1
					}
				case "inBase":
					x, ha = vb+up*5, -1
				case "ctr":
					x = (vp + vb) / 2
				case "t":
					y -= lh * 0.7
				case "b":
					y += lh * 0.7
				}
			}
			l.labelBlock(txt, f, x, y, ha)
		}
	}
}
