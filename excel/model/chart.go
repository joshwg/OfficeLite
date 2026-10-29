package model

import (
	"image/color"
	"slices"
)

// Chart is a chart: one on a sheet's drawing, placed by its Anchor, or
// the one chart of a chart sheet (Sheet.ChartSheet). It holds what the
// chart part (DrawingML's c:chartSpace) says, with colours resolved
// against the workbook's theme; colours left automatic are picked when the
// chart is drawn (see Theme.SeriesColor). The data are references into
// the workbook (series values are drawn from the cells as they are now),
// with the values the file cached as a fallback.
type Chart struct {
	Name   string // "Chart 1", the drawing object's name
	Anchor Anchor

	// Plots are the chart groups (c:barChart, c:lineChart ...): one for an
	// ordinary chart, several for a combo chart, each on its pair of axes.
	Plots []ChartPlot
	Axes  []ChartAxis

	Title *ChartTitle // nil: no title of its own
	// AutoTitleDeleted turns off the title Excel shows by itself for a
	// chart with one series (the series name).
	AutoTitleDeleted bool
	Legend           *ChartLegend // nil: no legend

	ChartArea ShapeProps // the chart's background and border
	PlotArea  ShapeProps
	Font      TextProps // the chart's default text (c:chartSpace/c:txPr)

	Style          int // c:style, 1..48 (2 is Excel's default)
	RoundedCorners bool
	DispBlanks     BlanksAs
	// PlotVisOnly leaves hidden rows and columns out of the data, as
	// Excel does unless "Show data in hidden rows and columns" is on.
	PlotVisOnly bool
	// PivotSource names the PivotTable a PivotChart shows
	// ([Book.xlsx]Sheet!PivotTable1); its data are the report's.
	PivotSource string

	// Raw is the chart part as read, written back as it came (references
	// and cached values brought up to date) while the chart isn't
	// edited; nil for a chart made or changed here.
	Raw *ChartRaw
}

// AnchorKind is how a drawing object is placed on its sheet.
type AnchorKind uint8

const (
	// AnchorTwoCell: From and To cells; moves and sizes with the cells.
	AnchorTwoCell AnchorKind = iota
	// AnchorOneCell: the From cell and a fixed size (Ext); moves with the
	// cells but keeps its size.
	AnchorOneCell
	// AnchorAbsolute: a fixed position and size on the sheet.
	AnchorAbsolute
)

// EMUPerPx is the number of English Metric Units in a 96-DPI pixel.
const EMUPerPx = 9525

// CellOffset is a point on a sheet: a cell and an offset into it (EMU).
type CellOffset struct {
	Row, Col int
	DX, DY   int64
}

// Anchor places a chart on its sheet.
type Anchor struct {
	Kind     AnchorKind
	From, To CellOffset
	// Pos and Ext are the position (AnchorAbsolute) and size (AnchorOneCell
	// and AnchorAbsolute) in EMU.
	PosX, PosY int64
	ExtX, ExtY int64
	// EditAs is a two-cell anchor's behaviour when cells move: "" or
	// "twoCell" (move and size with cells), "oneCell" (move, don't size)
	// or "absolute" (don't move or size).
	EditAs string
}

// ChartType is the kind of a chart group.
type ChartType uint8

const (
	ChartBar      ChartType = iota // c:barChart: columns, or bars when Horizontal
	ChartLine                      // c:lineChart
	ChartPie                       // c:pieChart
	ChartDoughnut                  // c:doughnutChart
	ChartArea                      // c:areaChart
	ChartScatter                   // c:scatterChart
	// ChartOther is a group of a type that isn't drawn (radar, bubble,
	// stock, surface, 3-D ...); OtherKind names it.
	ChartOther
)

// Grouping is how a group's series combine.
type Grouping uint8

const (
	GroupClustered Grouping = iota // bars side by side; lines and areas "standard"
	GroupStacked
	GroupPercent // stacked to 100%
)

// ChartPlot is one chart group: its series and how they are drawn.
type ChartPlot struct {
	Type       ChartType
	OtherKind  string // the element's name for ChartOther, e.g. "radarChart"
	Horizontal bool   // bars rather than columns
	Grouping   Grouping
	VaryColors bool // each point its own colour (pies)
	GapWidth   int  // bars: the gap between categories, % of a bar's width
	Overlap    int  // bars: -100..100 % of a bar's width
	HoleSize   int  // doughnut: % of the diameter
	FirstSlice int  // pie, doughnut: the first slice's angle, degrees from 12 o'clock
	// ScatterStyle is the scatter chart's c:scatterStyle (lineMarker,
	// line, marker, smooth, smoothMarker); the series say what is drawn.
	ScatterStyle string
	// Marker is a line chart's c:marker: markers are drawn where the
	// series doesn't say otherwise.
	Marker bool
	Labels *DataLabels // the group's data labels
	Series []ChartSeries
	// AxisIDs are the group's axes: category (X) and value (Y). Pies have
	// none.
	AxisIDs []int
}

// ChartSeries is one series of a group.
type ChartSeries struct {
	Idx, Order int
	Name       ChartData // c:tx: a reference or a literal (Literal)
	// Cat and Val are the categories and values; for a scatter chart the
	// X and Y values.
	Cat, Val ChartData
	Shape    ShapeProps
	Marker   Marker
	Smooth   bool
	// InvertNeg fills negative bars white (with the series' outline).
	InvertNeg bool
	Explosion int // pie: % of the radius the slices are pulled out
	Labels    *DataLabels
	Points    []DataPoint // points formatted on their own (c:dPt)
}

// DataPoint is a point's own format.
type DataPoint struct {
	Idx       int
	Shape     ShapeProps
	Explosion int
}

// ChartData is a series' name, categories or values: a reference (formula
// text without '=', e.g. Sheet1!$B$2:$B$10) with the values Excel cached,
// or literal values.
type ChartData struct {
	Ref     string
	Literal string // a literal series name (c:tx/c:v)
	// Cache is the values saved with the chart; for literal data, the
	// values themselves.
	Cache  []ChartValue
	Format string // the cache's number format code
	// Text marks string data (c:strRef, c:strLit, multi-level labels).
	Text bool
	// Levels are a multi-level category axis' labels, inner level first
	// (c:multiLvlStrRef). Cache holds the inner level.
	Levels [][]ChartValue
}

// IsZero reports whether d holds nothing.
func (d *ChartData) IsZero() bool {
	return d.Ref == "" && d.Literal == "" && len(d.Cache) == 0
}

// ValueKind is the kind of a ChartValue.
type ValueKind uint8

const (
	ValueBlank ValueKind = iota
	ValueNumber
	ValueText
)

// ChartValue is one data point's value.
type ChartValue struct {
	Kind ValueKind
	Num  float64
	Text string // the number as formatted, for a number
}

// BlanksAs is how empty cells are plotted (c:dispBlanksAs).
type BlanksAs uint8

const (
	BlanksGap  BlanksAs = iota // leave a gap
	BlanksZero                 // plot as zero
	BlanksSpan                 // connect the line over them
)

// Marker is a line or scatter series' marker.
type Marker struct {
	// Symbol is c:symbol: none, circle, square, diamond, triangle, x,
	// star, dot, dash, plus, auto; "" when the series doesn't say.
	Symbol string
	Size   int // 2..72 points; 0 for the default (5)
	Shape  ShapeProps
}

// DataLabels say what is written beside the points.
type DataLabels struct {
	Delete                        bool // no labels (overrides the group's)
	ShowVal, ShowPercent, ShowCat bool
	ShowSer, ShowKey, ShowLeader  bool
	Pos                           string // c:dLblPos: ctr, inEnd, inBase, outEnd, l, r, t, b, bestFit
	NumFmt                        string
	Separator                     string
	Font                          TextProps
	Shape                         ShapeProps
}

// Any reports whether the labels show something.
func (d *DataLabels) Any() bool {
	return d != nil && !d.Delete && (d.ShowVal || d.ShowPercent || d.ShowCat || d.ShowSer)
}

// ChartTitle is the chart's or an axis' title.
type ChartTitle struct {
	Text    string    // rich text, paragraphs separated by '\n'; "" for auto
	Ref     ChartData // a title linked to a cell (c:tx/c:strRef)
	Overlay bool      // drawn over the plot area instead of taking room
	Font    TextProps // the title's text format
	Shape   ShapeProps
}

// ChartLegend is the legend.
type ChartLegend struct {
	Pos     string // r, l, t, b, tr
	Overlay bool
	// Deleted are the entries removed from the legend (c:legendEntry
	// with c:delete), by index.
	Deleted []int
	Font    TextProps
	Shape   ShapeProps
}

// AxisKind is the kind of an axis.
type AxisKind uint8

const (
	AxisCat  AxisKind = iota // c:catAx: categories
	AxisVal                  // c:valAx: values (both axes of a scatter chart)
	AxisDate                 // c:dateAx: categories that are dates
	AxisSer                  // c:serAx: the series axis of a 3-D chart
)

// ChartAxis is an axis.
type ChartAxis struct {
	ID       int
	Kind     AxisKind
	Pos      string // l, r, t, b
	Delete   bool   // not shown (the data still use it)
	Reversed bool   // c:orientation maxMin
	// Min, Max, MajorUnit and MinorUnit are fixed values when the Has
	// flags are set; otherwise Excel picks them from the data.
	Min, Max             float64
	HasMin, HasMax       bool
	MajorUnit, MinorUnit float64
	LogBase              float64 // 0: linear
	NumFmt               string  // the labels' format; "" for the data's
	SourceLinked         bool    // the labels use the data's format
	MajorGrid, MinorGrid bool
	MajorGridLine        ShapeProps
	MinorGridLine        ShapeProps
	Title                *ChartTitle
	CrossAx              int
	// Crosses is where the crossing axis crosses this one: autoZero, min,
	// max, or at CrossesAt when HasCrossesAt.
	Crosses      string
	CrossesAt    float64
	HasCrossesAt bool
	// CrossBetween (value axes): "between" puts the crossing category axis'
	// categories between tick marks (bars), "midCat" on them (lines,
	// areas).
	CrossBetween string
	TickLblPos   string // nextTo, low, high, none
	MajorTick    string // none, in, out, cross
	MinorTick    string
	LblSkip      int // show every nth category label; 0 for automatic
	TickSkip     int
	NoMultiLvl   bool
	Line         ShapeProps
	Font         TextProps
}

// FillKind is how a shape or line is painted.
type FillKind uint8

const (
	FillAuto  FillKind = iota // what the chart's style gives
	FillNone                  // nothing
	FillSolid                 // Color
)

// Paint is a fill or a line's colour.
type Paint struct {
	Kind  FillKind
	Color color.NRGBA
}

// ShapeProps is a shape's format (c:spPr): its fill and outline.
type ShapeProps struct {
	Fill  Paint
	Line  Paint
	Width float32 // the line's width in points; 0 for the default
	Dash  string  // a:prstDash: solid, dash, sysDash, sysDot, ...
}

// TextProps is a text format (c:txPr, a:defRPr).
type TextProps struct {
	SizePt   float32 // 0 for the default
	Bold     *bool   // nil for the default
	Italic   bool
	Color    Paint
	Font     string // "" for the theme's
	Rotation int    // degrees, from a:bodyPr rot; 0 also for the default
	HasRot   bool
}

// ChartRaw is a chart part as read from an .xlsx, kept to save the chart
// as it came while it isn't edited.
type ChartRaw struct {
	XML []byte
	// Refs are the chart's references (c:f) in document order, where each
	// one lives in the model (a series' name, categories, values; a
	// title): saving writes the model's current text in their place.
	Refs []RawRef
	// Rels are the chart part's relationships; Parts the parts they
	// point at (chart style and colours, a drawing of shapes on the
	// chart, pictures), by their name in the package as read.
	Rels  []RawRel
	Parts map[string]*RawPart
}

// RawRef is where a c:f of a raw chart part is in the model: plot and
// series, and which of the series' data (RefName, RefCat, RefVal); or
// the chart's title (Plot -1) or an axis' (Plot -2, Series the axis); or
// somewhere not modelled (Plot -3), written as read.
type RawRef struct {
	Plot, Series, Field int
	Text                string // as read
}

// Fields of a series a RawRef points at.
const (
	RefName = iota
	RefCat
	RefVal
)

// RawRel is a package relationship.
type RawRel struct {
	ID, Type, Target string
	External         bool
}

// RawPart is a package part kept as read: its content type, data and
// relationships (internal targets by their part name).
type RawPart struct {
	ContentType string
	Data        []byte
	Rels        []RawRel
}

// Theme is the workbook theme's colours and fonts, which charts use.
type Theme struct {
	// Colors are dk1, lt1, dk2, lt2, accent1..6, hlink, folHlink.
	Colors    [12]color.NRGBA
	MajorFont string
	MinorFont string
	// XML is the theme part as read, saved back unchanged.
	XML []byte
}

// Theme colour indexes.
const (
	ThemeDk1 = iota
	ThemeLt1
	ThemeDk2
	ThemeLt2
	ThemeAccent1
)

// DefaultTheme is Office's theme (2013-2022): what Excel uses for a
// workbook that has none.
func DefaultTheme() *Theme {
	return &Theme{
		Colors: [12]color.NRGBA{
			rgb(0x000000), rgb(0xFFFFFF), rgb(0x44546A), rgb(0xE7E6E6),
			rgb(0x4472C4), rgb(0xED7D31), rgb(0xA5A5A5), rgb(0xFFC000), rgb(0x5B9BD5), rgb(0x70AD47),
			rgb(0x0563C1), rgb(0x954F72),
		},
		MajorFont: "Calibri Light",
		MinorFont: "Calibri",
	}
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// seriesVariations are the luminance changes (lumMod, lumOff, per
// 100,000) Excel 2013+ applies to the accents for series 7 and up: the
// six accents, then the six darker, then lighter, and so on.
var seriesVariations = [][2]int{
	{100000, 0}, {60000, 0}, {80000, 20000}, {80000, 0}, {60000, 40000},
	{50000, 0}, {70000, 30000}, {70000, 0}, {50000, 50000},
}

// SeriesColor is the automatic colour of series (or pie slice) i.
func (t *Theme) SeriesColor(i int) color.NRGBA {
	if t == nil {
		t = DefaultTheme()
	}
	i = max(i, 0)
	c := t.Colors[ThemeAccent1+i%6]
	v := seriesVariations[(i/6)%len(seriesVariations)]
	return LumMod(c, v[0], v[1])
}

// LumMod applies DrawingML's lumMod and lumOff (per 100,000) to c: its
// HSL luminance becomes L*mod + off.
func LumMod(c color.NRGBA, mod, off int) color.NRGBA {
	if mod == 100000 && off == 0 {
		return c
	}
	h, s, l := toHSL(c)
	l = min(max(l*float64(mod)/100000+float64(off)/100000, 0), 1)
	out := fromHSL(h, s, l)
	out.A = c.A
	return out
}

func toHSL(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func fromHSL(h, s, l float64) color.NRGBA {
	if s == 0 {
		v := uint8(l*255 + 0.5)
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		switch {
		case t < 0:
			t++
		case t > 1:
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	f := func(v float64) uint8 { return uint8(min(max(v, 0), 1)*255 + 0.5) }
	return color.NRGBA{R: f(hue(h + 1.0/3)), G: f(hue(h)), B: f(hue(h - 1.0/3)), A: 255}
}

// AllSeries lists the chart's series with their plot index, in the order
// of the plots.
func (c *Chart) AllSeries() []SeriesRef {
	var out []SeriesRef
	for pi := range c.Plots {
		for si := range c.Plots[pi].Series {
			out = append(out, SeriesRef{pi, si})
		}
	}
	return out
}

// SeriesRef is a series' position in Chart.Plots.
type SeriesRef struct{ Plot, Series int }

// Axis returns the axis with id, or nil.
func (c *Chart) Axis(id int) *ChartAxis {
	for i := range c.Axes {
		if c.Axes[i].ID == id {
			return &c.Axes[i]
		}
	}
	return nil
}

// Refs calls fn with every reference of the chart (series names,
// categories, values, linked titles), which may rewrite it.
func (c *Chart) Refs(fn func(*string)) {
	for pi := range c.Plots {
		for si := range c.Plots[pi].Series {
			s := &c.Plots[pi].Series[si]
			for _, d := range []*ChartData{&s.Name, &s.Cat, &s.Val} {
				if d.Ref != "" {
					fn(&d.Ref)
				}
			}
		}
	}
	if c.Title != nil && c.Title.Ref.Ref != "" {
		fn(&c.Title.Ref.Ref)
	}
	for i := range c.Axes {
		if t := c.Axes[i].Title; t != nil && t.Ref.Ref != "" {
			fn(&t.Ref.Ref)
		}
	}
}

// RefAt returns the model's text of a raw reference.
func (c *Chart) RefAt(r RawRef) (string, bool) {
	switch {
	case r.Plot >= 0:
		if r.Plot >= len(c.Plots) || r.Series < 0 || r.Series >= len(c.Plots[r.Plot].Series) {
			return "", false
		}
		s := &c.Plots[r.Plot].Series[r.Series]
		switch r.Field {
		case RefName:
			return s.Name.Ref, true
		case RefCat:
			return s.Cat.Ref, true
		case RefVal:
			return s.Val.Ref, true
		}
	case r.Plot == -1:
		if c.Title != nil {
			return c.Title.Ref.Ref, true
		}
	case r.Plot == -2:
		if r.Series >= 0 && r.Series < len(c.Axes) && c.Axes[r.Series].Title != nil {
			return c.Axes[r.Series].Title.Ref.Ref, true
		}
	case r.Plot == -3:
		return r.Text, true
	}
	return "", false
}

// DataAt returns the model's data a raw reference points at, or nil.
func (c *Chart) DataAt(r RawRef) *ChartData {
	switch {
	case r.Plot >= 0:
		if r.Plot >= len(c.Plots) || r.Series < 0 || r.Series >= len(c.Plots[r.Plot].Series) {
			return nil
		}
		s := &c.Plots[r.Plot].Series[r.Series]
		return []*ChartData{&s.Name, &s.Cat, &s.Val}[min(max(r.Field, 0), 2)]
	case r.Plot == -1 && c.Title != nil:
		return &c.Title.Ref
	case r.Plot == -2 && r.Series >= 0 && r.Series < len(c.Axes) && c.Axes[r.Series].Title != nil:
		return &c.Axes[r.Series].Title.Ref
	}
	return nil
}

// Clone returns a deep copy of the chart (the raw part data, which is
// never changed, is shared).
func (c *Chart) Clone() *Chart {
	if c == nil {
		return nil
	}
	n := *c
	n.Plots = slices.Clone(c.Plots)
	for i := range n.Plots {
		p := &n.Plots[i]
		p.AxisIDs = slices.Clone(p.AxisIDs)
		p.Labels = p.Labels.clone()
		p.Series = slices.Clone(p.Series)
		for j := range p.Series {
			s := &p.Series[j]
			s.Name, s.Cat, s.Val = s.Name.clone(), s.Cat.clone(), s.Val.clone()
			s.Labels = s.Labels.clone()
			s.Points = slices.Clone(s.Points)
		}
	}
	n.Axes = slices.Clone(c.Axes)
	for i := range n.Axes {
		n.Axes[i].Title = n.Axes[i].Title.clone()
	}
	n.Title = c.Title.clone()
	if c.Legend != nil {
		l := *c.Legend
		l.Deleted = slices.Clone(l.Deleted)
		n.Legend = &l
	}
	return &n
}

func (d ChartData) clone() ChartData {
	d.Cache = slices.Clone(d.Cache)
	d.Levels = slices.Clone(d.Levels)
	return d
}

func (d *DataLabels) clone() *DataLabels {
	if d == nil {
		return nil
	}
	n := *d
	return &n
}

func (t *ChartTitle) clone() *ChartTitle {
	if t == nil {
		return nil
	}
	n := *t
	n.Ref = t.Ref.clone()
	return &n
}

// CloneCharts deep-copies a sheet's charts.
func CloneCharts(cs []*Chart) []*Chart {
	if cs == nil {
		return nil
	}
	out := make([]*Chart, len(cs))
	for i, c := range cs {
		out[i] = c.Clone()
	}
	return out
}

// IsChartSheet reports whether s is a chart sheet: one chart, no cells.
func (s *Sheet) IsChartSheet() bool { return s.ChartSheet != nil }

// shiftCharts moves the charts' anchors for inserted or deleted rows or
// columns, as Excel moves (and sizes) objects with the cells.
func (s *Sheet) shiftCharts(rows bool, at, n int) {
	for _, c := range s.Charts {
		c.Anchor.shift(rows, at, n)
	}
}

func (a *Anchor) shift(rows bool, at, n int) {
	if a.Kind == AnchorAbsolute || a.EditAs == "absolute" {
		return
	}
	get := func(p *CellOffset) *int {
		if rows {
			return &p.Row
		}
		return &p.Col
	}
	from, to := get(&a.From), get(&a.To)
	if a.Kind == AnchorOneCell || a.EditAs == "oneCell" {
		// Moves with its top-left cell, keeping its size; a deleted cell
		// leaves it at the first cell after the deletion.
		lo, _ := ShiftIndex(*from, at, n)
		*to += lo - *from
		*from = lo
		return
	}
	lo, hi, ok := ShiftSpan(*from, *to, at, n)
	if !ok {
		// Everything under it went: it shrinks to nothing at the cut.
		*from, *to = at, at
		if rows {
			a.From.DY, a.To.DY = 0, 0
		} else {
			a.From.DX, a.To.DX = 0, 0
		}
		return
	}
	if _, kept := ShiftIndex(*from, at, n); !kept {
		if rows {
			a.From.DY = 0
		} else {
			a.From.DX = 0
		}
	}
	if _, kept := ShiftIndex(*to, at, n); !kept {
		// Its end cell was deleted: it now ends at the top of the cell
		// after the deletion.
		hi++
		if rows {
			a.To.DY = 0
		} else {
			a.To.DX = 0
		}
	}
	*from, *to = lo, hi
}
