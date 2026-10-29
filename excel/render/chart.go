package render

import (
	"image/color"
	"math"
	"slices"
	"strings"

	"officelite/common/colors"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Charts are laid out into a list of primitives (filled polygons, stroked
// lines, text) in the chart's own pixels (96 DPI, unzoomed, origin at its
// top-left corner); the grid paints them with Gio (chartpaint.go) and the
// PDF export with PDF paths (pdf.go). The look follows Excel 2013+'s
// default chart style: theme accent colours, grey #595959 text at 9pt
// (14pt titles), light grey #D9D9D9 gridlines and borders.

type primKind uint8

const (
	primFill   primKind = iota // a filled polygon
	primStroke                 // a polyline (closed: a polygon's outline)
	primText                   // one line of text
)

type chartPt struct{ X, Y float32 }

// chartFont is a resolved text format.
type chartFont struct {
	Family string
	SizePt float32
	Bold   bool
	Italic bool
	Color  color.NRGBA
}

// px is the font's size in 96-DPI pixels.
func (f chartFont) px() float32 { return f.SizePt * 96 / 72 }

// prim is one primitive.
type prim struct {
	kind   primKind
	pts    []chartPt
	closed bool
	color  color.NRGBA
	width  float32   // stroke width, px
	dash   []float32 // on/off lengths, px; nil for solid
	// Text: drawn with its anchor point at x, y; ha and va align the
	// text's box to it (-1 left/top, 0 centre, 1 right/bottom), and rot
	// turns it (degrees clockwise) about that point.
	text string
	font chartFont
	x, y float32
	ha   int8
	va   int8
	rot  float32
	// elem is the part of the chart the primitive draws (what a click on
	// it selects); box a text's bounds.
	elem ChartElem
	box  rect32
}

// ChartElemKind is a kind of part of a chart that can be selected.
type ChartElemKind uint8

const (
	ElemNone ChartElemKind = iota
	ElemChartArea
	ElemPlotArea
	ElemTitle
	ElemLegend
	ElemSeries    // Plot and Series say which
	ElemLabels    // a series' data labels
	ElemAxis      // Axis is its index in Chart.Axes
	ElemAxisTitle // likewise
	ElemGridlines // likewise
)

// ChartElem is a part of a chart.
type ChartElem struct {
	Kind         ChartElemKind
	Plot, Series int
	Axis         int
}

var elemNames = map[ChartElemKind]string{ElemChartArea: "Chart Area", ElemPlotArea: "Plot Area", ElemTitle: "Chart Title",
	ElemLegend: "Legend", ElemSeries: "Series", ElemLabels: "Data Labels", ElemAxis: "Axis", ElemAxisTitle: "Axis Title",
	ElemGridlines: "Gridlines"}

// Name is the element's name as Excel's Chart Elements box shows it.
func (e ChartElem) Name(c *model.Chart, wb *model.Workbook, home string) string {
	switch e.Kind {
	case ElemSeries, ElemLabels:
		if e.Plot < len(c.Plots) && e.Series < len(c.Plots[e.Plot].Series) {
			n := 0
			for pi := 0; pi < e.Plot; pi++ {
				n += len(c.Plots[pi].Series)
			}
			name := "Series \"" + wb.SeriesName(&c.Plots[e.Plot].Series[e.Series], home, n+e.Series) + "\""
			if e.Kind == ElemLabels {
				name += " Data Labels"
			}
			return name
		}
	case ElemAxis, ElemAxisTitle, ElemGridlines:
		if e.Axis >= 0 && e.Axis < len(c.Axes) {
			ax := &c.Axes[e.Axis]
			dir := "Vertical"
			if ax.Pos == "b" || ax.Pos == "t" {
				dir = "Horizontal"
			}
			kind := "(Value)"
			if ax.Kind != model.AxisVal {
				kind = "(Category)"
			}
			pre := "Vertical"
			if ax.Pos == "r" || ax.Pos == "t" {
				pre = "Secondary Vertical"
				if dir == "Horizontal" {
					pre = "Secondary Horizontal"
				}
			} else {
				pre = dir
			}
			switch e.Kind {
			case ElemAxisTitle:
				return pre + " " + kind + " Axis Title"
			case ElemGridlines:
				return pre + " " + kind + " Axis Major Gridlines"
			}
			return pre + " " + kind + " Axis"
		}
	}
	return elemNames[e.Kind]
}

// chartMeasure measures one line of text: its width and line height,
// in 96-DPI pixels.
type chartMeasure func(f chartFont, s string) (w, h float32)

// chartEnv is what laying out a chart needs from its surroundings.
type chartEnv struct {
	wb      *model.Workbook // nil: draw from the cached values
	home    string          // the chart's sheet, for unqualified references
	measure chartMeasure
	// format renders a number in a number format (a built-in id or a
	// code); nil formats as General.
	format func(v float64, id int, code string) string
}

// Excel 2013+'s default chart look.
var (
	chartText      = colors.RGB(0x595959)
	chartLabelText = colors.RGB(0x404040)
	chartGrid      = colors.RGB(0xD9D9D9)
	chartBorder    = colors.RGB(0xD9D9D9)
)

const (
	chartPad     = 9 // the chart area's inner margin, px
	defTextPt    = 9
	defTitlePt   = 14
	defLineW     = 3 // a line series' default width, px (2.25pt)
	defMarker    = 5 // points
	gridW        = 1
	legendKeyBox = 7 // a legend key's square, px
)

// numFormat is a number format: a built-in id or a code.
type numFormat struct {
	id   int
	code string
}

// series is a series ready to draw.
type series struct {
	plot  *model.ChartPlot
	s     *model.ChartSeries
	index int // its place in the chart, for the automatic colours
	name  string
	vals  []float64
	ok    []bool // false: blank (or text)
	xs    []float64
	xok   []bool
	cats  []string
	fmt   numFormat
	fill  color.NRGBA
	line  color.NRGBA
	// pi and si are its place in Chart.Plots.
	pi, si int
}

// elem is the series (or its labels) as a chart element.
func (sd *series) elem(k ChartElemKind) ChartElem {
	return ChartElem{Kind: k, Plot: sd.pi, Series: sd.si}
}

// chartLayout lays out one chart.
type chartLayout struct {
	c     *model.Chart
	env   *chartEnv
	theme *model.Theme
	out   []prim
	w, h  float32
	font  chartFont // the chart's default text
	ser   []*series
	// The plot area (inside the axes) and the axes in use.
	plot   rect32
	axes   map[int]*axisInfo
	nCat   int
	catLbl []string
	catFmt numFormat
	elem   ChartElem // what the primitives drawn now belong to
}

type rect32 struct{ x0, y0, x1, y1 float32 }

func (r rect32) w() float32 { return r.x1 - r.x0 }
func (r rect32) h() float32 { return r.y1 - r.y0 }

// axisInfo is an axis as laid out.
type axisInfo struct {
	ax       *model.ChartAxis
	cat      bool // a category (or date) axis: slots, not values
	vertical bool // runs up the plot (a column chart's value axis)
	scale    axisScale
	between  bool // categories between tick marks
	fmt      numFormat
	labels   []string // category labels, or the ticks' texts
	font     chartFont
	labelRot float32 // degrees
	skip     int     // show every skip-th category label
	lines    int     // wrapped lines of category labels
	size     float32 // room the labels take beside the plot
	shown    bool
	cross    *axisInfo // the axis it crosses
	second   bool      // a secondary axis (right or top)
}

// layoutChart lays out c in a w × h pixel box.
func layoutChart(c *model.Chart, env *chartEnv, w, h float32) []prim {
	th := model.DefaultTheme()
	if env.wb != nil && env.wb.Theme != nil {
		th = env.wb.Theme
	}
	l := &chartLayout{c: c, env: env, theme: th, w: w, h: h, axes: map[int]*axisInfo{}}
	l.font = l.resolveFont(c.Font, chartFont{Family: th.MinorFont, SizePt: defTextPt, Color: chartText})
	if w < 4 || h < 4 {
		return nil
	}
	l.collect()
	l.layout()
	return l.out
}

// --- helpers ---

func (l *chartLayout) resolveFont(t model.TextProps, base chartFont) chartFont {
	f := base
	if t.SizePt > 0 {
		f.SizePt = t.SizePt
	}
	if t.Bold != nil {
		f.Bold = *t.Bold
	}
	if t.Italic {
		f.Italic = true
	}
	if t.Color.Kind == model.FillSolid {
		f.Color = t.Color.Color
	}
	if t.Font != "" {
		f.Family = t.Font
	}
	return f
}

func (l *chartLayout) fillRect(r rect32, c color.NRGBA) {
	if c.A == 0 || r.w() <= 0 || r.h() <= 0 {
		return
	}
	l.out = append(l.out, prim{kind: primFill, color: c, pts: []chartPt{{r.x0, r.y0}, {r.x1, r.y0}, {r.x1, r.y1}, {r.x0, r.y1}}, elem: l.elem})
}

func (l *chartLayout) poly(pts []chartPt, c color.NRGBA) {
	if c.A == 0 || len(pts) < 3 {
		return
	}
	l.out = append(l.out, prim{kind: primFill, color: c, pts: pts, elem: l.elem})
}

func (l *chartLayout) stroke(pts []chartPt, closed bool, c color.NRGBA, w float32, dash []float32) {
	if c.A == 0 || len(pts) < 2 || w <= 0 {
		return
	}
	l.out = append(l.out, prim{kind: primStroke, pts: pts, closed: closed, color: c, width: w, dash: dash, elem: l.elem})
}

func (l *chartLayout) line(x0, y0, x1, y1 float32, c color.NRGBA, w float32) {
	l.stroke([]chartPt{{x0, y0}, {x1, y1}}, false, c, w, nil)
}

func (l *chartLayout) text(s string, f chartFont, x, y float32, ha, va int8, rot float32) {
	if s == "" || f.Color.A == 0 {
		return
	}
	// The text's bounds, for picking it out with the pointer.
	w, h := l.measure(f, s)
	bh, bv := ha, va
	if rot == 90 || rot == -90 {
		w, h = h, w
		bh, bv = va, ha
	} else if rot != 0 {
		w, h = max(w, h), max(w, h)
		bh, bv = 0, 0
	}
	x0 := x - w*float32(bh+1)/2
	y0 := y - h*float32(bv+1)/2
	l.out = append(l.out, prim{kind: primText, text: s, font: f, x: x, y: y, ha: ha, va: va, rot: rot, elem: l.elem,
		box: rect32{x0, y0, x0 + w, y0 + h}})
}

func (l *chartLayout) measure(f chartFont, s string) (float32, float32) {
	if l.env.measure == nil {
		return float32(len([]rune(s))) * f.px() * 0.5, f.px() * 1.2
	}
	return l.env.measure(f, s)
}

// textBlock measures text of several lines.
func (l *chartLayout) textBlock(f chartFont, s string) (w, h float32) {
	for _, line := range strings.Split(s, "\n") {
		lw, lh := l.measure(f, line)
		w, h = max(w, lw), h+lh
	}
	return w, h
}

// multiText draws lines of text centred on x, the first one's top at y.
func (l *chartLayout) multiText(s string, f chartFont, x, y float32) {
	for _, line := range strings.Split(s, "\n") {
		_, lh := l.measure(f, line)
		l.text(line, f, x, y, 0, -1, 0)
		y += lh
	}
}

// shapeFill is a shape's fill colour, def when automatic.
func shapeFill(sp model.ShapeProps, def color.NRGBA) color.NRGBA {
	switch sp.Fill.Kind {
	case model.FillNone:
		return color.NRGBA{}
	case model.FillSolid:
		return sp.Fill.Color
	}
	return def
}

// shapeLine is a shape's outline colour and width (px), def when
// automatic.
func shapeLine(sp model.ShapeProps, def color.NRGBA, defW float32) (color.NRGBA, float32) {
	w := defW
	if sp.Width > 0 {
		w = max(sp.Width*96/72, 0.5)
	}
	switch sp.Line.Kind {
	case model.FillNone:
		return color.NRGBA{}, 0
	case model.FillSolid:
		return sp.Line.Color, w
	}
	return def, w
}

// dashOf turns a preset dash into on/off lengths for a line of width w.
func dashOf(name string, w float32) []float32 {
	w = max(w, 1)
	switch name {
	case "dash", "sysDash":
		return []float32{4 * w, 3 * w}
	case "dot", "sysDot":
		return []float32{w, w}
	case "dashDot", "sysDashDot":
		return []float32{4 * w, 3 * w, w, 3 * w}
	case "lgDash":
		return []float32{8 * w, 3 * w}
	case "lgDashDot", "lgDashDotDot", "sysDashDotDot":
		return []float32{8 * w, 3 * w, w, 3 * w}
	}
	return nil
}

func (l *chartLayout) formatNum(v float64, f numFormat) string {
	if l.env.format != nil {
		return l.env.format(v, f.id, f.code)
	}
	return generalNum(v)
}

// generalNum is Excel's General format for chart labels.
func generalNum(v float64) string { return numfmt.General(v) }

// --- the data ---

// collect reads the series' current data.
func (l *chartLayout) collect() {
	c := l.c
	n := 0
	for pi := range c.Plots {
		p := &c.Plots[pi]
		order := make([]int, len(p.Series))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(a, b int) int { return p.Series[a].Order - p.Series[b].Order })
		for _, si := range order {
			s := &p.Series[si]
			sd := &series{plot: p, s: s, index: s.Idx, pi: pi, si: si}
			sd.name = l.seriesName(s, n)
			vals, f := l.values(&s.Val)
			sd.fmt = f
			for _, v := range vals {
				ok := v.Kind == model.ValueNumber
				if v.Kind == model.ValueText {
					ok = true // text plots as 0, as in Excel
				}
				sd.vals = append(sd.vals, v.Num)
				sd.ok = append(sd.ok, ok)
			}
			cats, cf := l.values(&s.Cat)
			if p.Type == model.ChartScatter {
				numeric := len(cats) > 0
				for _, v := range cats {
					if v.Kind == model.ValueText {
						numeric = false
					}
				}
				for i := range sd.vals {
					x, ok := float64(i+1), true
					if numeric && i < len(cats) {
						x, ok = cats[i].Num, cats[i].Kind == model.ValueNumber
					}
					sd.xs, sd.xok = append(sd.xs, x), append(sd.xok, ok)
				}
			}
			for _, v := range cats {
				t := v.Text
				if v.Kind == model.ValueNumber && (t == "" || s.Cat.Ref == "" || l.env.wb == nil) {
					t = l.formatNum(v.Num, cf)
				}
				sd.cats = append(sd.cats, t)
			}
			if l.catLbl == nil && len(sd.cats) > 0 && p.Type != model.ChartScatter {
				l.catLbl, l.catFmt = sd.cats, cf
			}
			l.ser = append(l.ser, sd)
			n++
		}
	}
	for _, sd := range l.ser {
		if sd.plot.Type != model.ChartScatter {
			l.nCat = max(l.nCat, len(sd.vals))
		}
	}
	l.nCat = max(l.nCat, len(l.catLbl))
	for len(l.catLbl) < l.nCat {
		l.catLbl = append(l.catLbl, generalNum(float64(len(l.catLbl)+1)))
	}
	l.colors()
}

// values returns data's current values and their number format.
func (l *chartLayout) values(d *model.ChartData) ([]model.ChartValue, numFormat) {
	f := numFormat{code: d.Format}
	if d.Ref != "" && l.env.wb != nil {
		if vals, _, st, ok := l.env.wb.ChartValues(d.Ref, l.env.home, l.c.PlotVisOnly); ok {
			if l.c.PivotSource != "" && len(d.Cache) > 0 && len(vals) > len(d.Cache) {
				// A PivotChart shows the report's items, not its totals
				// below them, which its references take in.
				vals = vals[:len(d.Cache)]
			}
			if int(st) < len(l.env.wb.Styles) {
				s := l.env.wb.Styles[st]
				f = numFormat{s.NumFmtID, s.NumFmtCode}
			}
			return vals, f
		}
	}
	return d.Cache, f
}

func (l *chartLayout) seriesName(s *model.ChartSeries, n int) string {
	return l.env.wb.SeriesName(s, l.env.home, n) // a nil workbook reads the cache
}

// autoMarkers is the marker cycle of Excel's default style.
var autoMarkers = []string{"diamond", "square", "triangle", "x", "star", "circle", "plus"}

// colors resolves each series' fill and line colour.
func (l *chartLayout) colors() {
	for _, sd := range l.ser {
		auto := l.theme.SeriesColor(sd.index)
		if l.c.Style == 1 || l.c.Style == 9 || l.c.Style == 17 || l.c.Style == 25 || l.c.Style == 33 || l.c.Style == 41 {
			// The greyscale styles.
			auto = model.LumMod(colors.RGB(0x7F7F7F), 100000-(sd.index%6)*12000, (sd.index%6)*5000)
		}
		sd.fill = shapeFill(sd.s.Shape, auto)
		lc, _ := shapeLine(sd.s.Shape, auto, defLineW)
		switch sd.plot.Type {
		case model.ChartLine, model.ChartScatter:
			sd.line = lc
			if sd.s.Shape.Line.Kind == model.FillAuto && sd.s.Shape.Fill.Kind == model.FillSolid {
				sd.line = sd.s.Shape.Fill.Color
			}
		default:
			if sd.s.Shape.Line.Kind == model.FillSolid {
				sd.line = sd.s.Shape.Line.Color
			}
		}
	}
}

// pointFill is the fill of point i of a series (a pie slice, a bar of a
// varied-colour series).
func (l *chartLayout) pointFill(sd *series, i int) color.NRGBA {
	for _, p := range sd.s.Points {
		if p.Idx == i && p.Shape.Fill.Kind != model.FillAuto {
			return shapeFill(p.Shape, sd.fill)
		}
	}
	if sd.plot.VaryColors && (len(l.ser) == 1 || sd.plot.Type == model.ChartPie || sd.plot.Type == model.ChartDoughnut) {
		return l.theme.SeriesColor(i)
	}
	return sd.fill
}

// --- layout ---

func (l *chartLayout) layout() {
	c := l.c
	// The chart area.
	area := rect32{0, 0, l.w, l.h}
	l.elem = ChartElem{Kind: ElemChartArea}
	bg := shapeFill(c.ChartArea, colors.White)
	l.fillRect(area, bg)

	inner := rect32{chartPad, chartPad, l.w - chartPad, l.h - chartPad}

	// The title.
	if t, f, ok := l.titleText(); ok {
		_, th := l.textBlock(f, t)
		l.elem = ChartElem{Kind: ElemTitle}
		l.multiText(t, f, l.w/2, inner.y0)
		l.elem = ChartElem{Kind: ElemChartArea}
		if c.Title == nil || !c.Title.Overlay {
			inner.y0 += th + 4
		}
	}
	// The legend.
	if c.Legend != nil {
		l.elem = ChartElem{Kind: ElemLegend}
		inner = l.legend(inner)
		l.elem = ChartElem{Kind: ElemChartArea}
	}
	if inner.w() < 8 || inner.h() < 8 {
		l.border(area)
		return
	}
	var pie, other []*series
	for _, sd := range l.ser {
		switch sd.plot.Type {
		case model.ChartPie, model.ChartDoughnut:
			pie = append(pie, sd)
		case model.ChartOther:
		default:
			other = append(other, sd)
		}
	}
	switch {
	case len(other) > 0:
		l.axesPlot(inner)
	case len(pie) > 0:
		l.piePlot(inner, pie)
	default:
		l.fillRect(inner, shapeFill(c.PlotArea, color.NRGBA{}))
		if len(c.Plots) > 0 && c.Plots[0].Type == model.ChartOther {
			f := l.font
			l.text("("+strings.TrimSuffix(c.Plots[0].OtherKind, "Chart")+" charts aren't drawn yet)", f, l.w/2, (inner.y0+inner.y1)/2, 0, 0, 0)
		}
	}
	l.elem = ChartElem{Kind: ElemChartArea}
	l.border(area)
}

// border draws the chart area's outline (over everything).
func (l *chartLayout) border(area rect32) {
	lc, lw := shapeLine(l.c.ChartArea, chartBorder, 1)
	if lc.A == 0 {
		return
	}
	h := lw / 2
	l.stroke([]chartPt{{area.x0 + h, area.y0 + h}, {area.x1 - h, area.y0 + h}, {area.x1 - h, area.y1 - h}, {area.x0 + h, area.y1 - h}}, true, lc, lw, nil)
}

// titleText is the chart title's text, if it shows one: its own, the
// cell it's linked to, or (automatic) the single series' name.
func (l *chartLayout) titleText() (string, chartFont, bool) {
	c := l.c
	base := l.font
	base.SizePt = defTitlePt
	if c.Font.SizePt > 0 {
		base.SizePt = c.Font.SizePt * 1.2
	}
	if c.Title != nil {
		f := l.resolveFont(c.Title.Font, base)
		switch {
		case c.Title.Text != "":
			return c.Title.Text, f, true
		case c.Title.Ref.Ref != "" || len(c.Title.Ref.Cache) > 0:
			vals, _ := l.values(&c.Title.Ref)
			var parts []string
			for _, v := range vals {
				parts = append(parts, v.Text)
			}
			return strings.Join(parts, " "), f, true
		case len(l.ser) == 1:
			return l.ser[0].name, f, true
		}
		return "Chart Title", f, true
	}
	if !c.AutoTitleDeleted && len(l.ser) == 1 {
		return l.ser[0].name, base, true
	}
	return "", base, false
}

// legendEntry is one line of the legend.
type legendEntry struct {
	text string
	sd   *series
	pt   int // the point (category) for a varied-colour series, else -1
}

func (l *chartLayout) legendEntries() []legendEntry {
	var out []legendEntry
	for _, sd := range l.ser {
		if sd.plot.Type == model.ChartOther {
			continue
		}
		if sd.plot.VaryColors && (sd.plot.Type == model.ChartPie || sd.plot.Type == model.ChartDoughnut || len(l.ser) == 1) {
			if len(out) > 0 && out[len(out)-1].pt >= 0 {
				continue // one set of categories
			}
			for i := 0; i < len(sd.vals); i++ {
				t := generalNum(float64(i + 1))
				if i < len(sd.cats) {
					t = sd.cats[i]
				}
				out = append(out, legendEntry{t, sd, i})
			}
			continue
		}
		out = append(out, legendEntry{sd.name, sd, -1})
	}
	if lg := l.c.Legend; lg != nil && len(lg.Deleted) > 0 {
		kept := out[:0]
		for i, e := range out {
			if !slices.Contains(lg.Deleted, i) {
				kept = append(kept, e)
			}
		}
		out = kept
	}
	return out
}

// legend draws the legend and returns what is left of inner.
func (l *chartLayout) legend(inner rect32) rect32 {
	lg := l.c.Legend
	f := l.resolveFont(lg.Font, l.font)
	entries := l.legendEntries()
	if len(entries) == 0 {
		return inner
	}
	_, lh := l.measure(f, "Ag")
	// A line's key is a short stretch of line; others a small square.
	keyOf := func(e legendEntry) float32 {
		if t := e.sd.plot.Type; (t == model.ChartLine || t == model.ChartScatter) && e.pt < 0 {
			return 22
		}
		return legendKeyBox
	}
	const gap, entryGap = 4, 12
	widths := make([]float32, len(entries))
	maxW := float32(0)
	for i, e := range entries {
		tw, _ := l.measure(f, e.text)
		widths[i] = keyOf(e) + gap + tw
		maxW = max(maxW, widths[i])
	}
	pos := lg.Pos
	vertical := pos == "r" || pos == "l" || pos == "tr"
	var box rect32
	var rows [][]int
	if vertical {
		w := min(maxW, inner.w()/2)
		h := min(float32(len(entries))*lh, inner.h())
		y0 := inner.y0 + (inner.h()-h)/2
		switch pos {
		case "l":
			box = rect32{inner.x0, y0, inner.x0 + w, y0 + h}
		case "tr":
			box = rect32{inner.x1 - w, inner.y0, inner.x1, inner.y0 + h}
		default:
			box = rect32{inner.x1 - w, y0, inner.x1, y0 + h}
		}
		for i := range entries {
			rows = append(rows, []int{i})
		}
	} else {
		// Rows of entries, as many to a row as fit.
		var row []int
		rw := float32(0)
		for i := range entries {
			if len(row) > 0 && rw+entryGap+widths[i] > inner.w() {
				rows = append(rows, row)
				row, rw = nil, 0
			}
			if len(row) > 0 {
				rw += entryGap
			}
			row = append(row, i)
			rw += widths[i]
		}
		rows = append(rows, row)
		h := min(float32(len(rows))*lh, inner.h()/2)
		if pos == "t" {
			box = rect32{inner.x0, inner.y0, inner.x1, inner.y0 + h}
		} else {
			box = rect32{inner.x0, inner.y1 - h, inner.x1, inner.y1}
		}
	}
	l.fillRect(box, shapeFill(lg.Shape, color.NRGBA{}))
	for ri, row := range rows {
		y := box.y0 + float32(ri)*lh
		if y+lh > box.y1+1 {
			break
		}
		var x float32
		if vertical {
			x = box.x0
		} else {
			rw := float32(0)
			for k, i := range row {
				if k > 0 {
					rw += entryGap
				}
				rw += widths[i]
			}
			x = box.x0 + (box.w()-rw)/2
		}
		for _, i := range row {
			e := entries[i]
			l.legendKey(e, x, y+lh/2, keyOf(e))
			l.text(e.text, f, x+keyOf(e)+gap, y+lh/2, -1, 0, 0)
			x += widths[i] + entryGap
		}
	}
	if lg.Overlay {
		return inner
	}
	const sep = 8
	switch {
	case pos == "r":
		inner.x1 = box.x0 - sep
	case pos == "l":
		inner.x0 = box.x1 + sep
	case pos == "t":
		inner.y0 = box.y1 + sep
	case pos == "tr":
		inner.x1 = box.x0 - sep
	default:
		inner.y1 = box.y0 - sep
	}
	return inner
}

// legendKey draws an entry's key centred on y.
func (l *chartLayout) legendKey(e legendEntry, x, y, w float32) {
	sd := e.sd
	t := sd.plot.Type
	if (t == model.ChartLine || t == model.ChartScatter) && e.pt < 0 {
		if l.drawsLine(sd) {
			l.line(x, y, x+w, y, sd.line, l.lineWidth(sd))
		}
		if sym, size := l.markerOf(sd); sym != "none" {
			l.marker(sd, sym, min(size, 7), x+w/2, y)
		}
		return
	}
	fill := sd.fill
	if e.pt >= 0 {
		fill = l.pointFill(sd, e.pt)
	}
	h := float32(legendKeyBox) / 2
	l.fillRect(rect32{x, y - h, x + legendKeyBox, y + h}, fill)
	if sd.line.A > 0 {
		l.stroke([]chartPt{{x, y - h}, {x + legendKeyBox, y - h}, {x + legendKeyBox, y + h}, {x, y + h}}, true, sd.line, 1, nil)
	}
}

// --- pies and doughnuts ---

func (l *chartLayout) piePlot(inner rect32, pie []*series) {
	l.elem = ChartElem{Kind: ElemPlotArea}
	l.fillRect(inner, shapeFill(l.c.PlotArea, color.NRGBA{}))
	p := pie[0].plot
	cx, cy := (inner.x0+inner.x1)/2, (inner.y0+inner.y1)/2
	r := min(inner.w(), inner.h()) / 2
	labels := l.anyLabels(pie)
	if labels && r > 40 {
		r *= 0.85 // room for labels outside
	}
	rings := pie[:1]
	hole := float32(0)
	if p.Type == model.ChartDoughnut {
		rings = pie
		hole = r * float32(min(max(p.HoleSize, 10), 90)) / 100
	}
	ringW := (r - hole) / float32(len(rings))
	for ri, sd := range rings {
		r0 := hole + float32(ri)*ringW
		r1 := r0 + ringW
		total := 0.0
		for i, v := range sd.vals {
			if sd.ok[i] && v > 0 {
				total += v
			}
		}
		if total <= 0 {
			continue
		}
		ang := float64(p.FirstSlice)
		for i, v := range sd.vals {
			if !sd.ok[i] || v <= 0 {
				continue
			}
			sweep := v / total * 360
			fill := l.pointFill(sd, i)
			// Explosion pulls the slice out along its middle.
			ex := float32(sd.s.Explosion)
			for _, dp := range sd.s.Points {
				if dp.Idx == i && dp.Explosion > 0 {
					ex = float32(dp.Explosion)
				}
			}
			mid := (ang + sweep/2) * math.Pi / 180
			ox, oy := float32(0), float32(0)
			if ex > 0 && p.Type == model.ChartPie {
				d := r * ex / 100
				ox, oy = d*float32(math.Sin(mid)), -d*float32(math.Cos(mid))
			}
			pts := slicePoly(cx+ox, cy+oy, r0, r1, ang, sweep)
			l.elem = sd.elem(ElemSeries)
			l.poly(pts, fill)
			lc, lw := shapeLine(sd.s.Shape, colors.White, 1.3)
			for _, dp := range sd.s.Points {
				if dp.Idx == i && dp.Shape.Line.Kind != model.FillAuto {
					lc, lw = shapeLine(dp.Shape, lc, lw)
				}
			}
			l.stroke(pts, true, lc, lw, nil)
			if lb := l.labelsOf(sd); lb.Any() {
				txt := l.labelText(sd, lb, i, v/total)
				f := l.resolveFont(lb.Font, l.labelFont())
				rr := (r0 + r1) / 2
				if p.Type == model.ChartPie {
					switch lb.Pos {
					case "outEnd", "bestFit", "":
						rr = r * 1.12
					case "inEnd":
						rr = r * 0.75
					case "ctr":
						rr = r * 0.5
					}
				}
				lx, ly := cx+ox+rr*float32(math.Sin(mid)), cy+oy-rr*float32(math.Cos(mid))
				ha := int8(0)
				if rr > r {
					if math.Sin(mid) > 0.2 {
						ha = -1
					} else if math.Sin(mid) < -0.2 {
						ha = 1
					}
				}
				l.elem = sd.elem(ElemLabels)
				l.labelBlock(txt, f, lx, ly, ha)
			}
			ang += sweep
		}
	}
}

// slicePoly is a pie slice (r0 0) or a ring segment between angles a and
// a+sweep (degrees clockwise from 12 o'clock).
func slicePoly(cx, cy, r0, r1 float32, a, sweep float64) []chartPt {
	n := max(int(math.Ceil(sweep/2)), 2)
	pt := func(r float32, deg float64) chartPt {
		rad := deg * math.Pi / 180
		return chartPt{cx + r*float32(math.Sin(rad)), cy - r*float32(math.Cos(rad))}
	}
	var pts []chartPt
	for k := 0; k <= n; k++ {
		pts = append(pts, pt(r1, a+sweep*float64(k)/float64(n)))
	}
	if r0 <= 0 {
		if sweep < 359.99 {
			pts = append(pts, chartPt{cx, cy})
		}
		return pts
	}
	for k := n; k >= 0; k-- {
		pts = append(pts, pt(r0, a+sweep*float64(k)/float64(n)))
	}
	return pts
}

// --- data labels ---

func (l *chartLayout) labelsOf(sd *series) *model.DataLabels {
	if sd.s.Labels != nil {
		return sd.s.Labels
	}
	return sd.plot.Labels
}

func (l *chartLayout) anyLabels(ss []*series) bool {
	for _, sd := range ss {
		if l.labelsOf(sd).Any() {
			return true
		}
	}
	return false
}

func (l *chartLayout) labelFont() chartFont {
	f := l.font
	f.Color = chartLabelText
	if l.c.Font.Color.Kind == model.FillSolid {
		f.Color = l.c.Font.Color.Color
	}
	return f
}

// labelText is a point's data label: what lb shows, joined by its
// separator.
func (l *chartLayout) labelText(sd *series, lb *model.DataLabels, i int, pct float64) string {
	var parts []string
	if lb.ShowSer {
		parts = append(parts, sd.name)
	}
	if lb.ShowCat {
		switch {
		case sd.plot.Type == model.ChartScatter && i < len(sd.xs):
			parts = append(parts, generalNum(sd.xs[i]))
		case i < len(sd.cats):
			parts = append(parts, sd.cats[i])
		case i < len(l.catLbl):
			parts = append(parts, l.catLbl[i])
		}
	}
	if lb.ShowVal && i < len(sd.vals) {
		f := sd.fmt
		if lb.NumFmt != "" && !strings.EqualFold(lb.NumFmt, "General") {
			f = numFormat{code: lb.NumFmt}
		}
		parts = append(parts, l.formatNum(sd.vals[i], f))
	}
	if lb.ShowPercent && pct >= 0 {
		parts = append(parts, l.formatNum(pct, numFormat{id: 9}))
	}
	sep := lb.Separator
	if sep == "" {
		sep = ", "
	}
	return strings.Join(parts, sep)
}

// labelBlock draws a label of one or more lines centred on y.
func (l *chartLayout) labelBlock(s string, f chartFont, x, y float32, ha int8) {
	lines := strings.Split(s, "\n")
	_, lh := l.measure(f, "Ag")
	y -= lh * float32(len(lines)) / 2
	for _, line := range lines {
		l.text(line, f, x, y+lh/2, ha, 0, 0)
		y += lh
	}
}
