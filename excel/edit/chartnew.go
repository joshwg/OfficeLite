// chartnew.go makes charts as Excel's Insert > Charts does: the kinds
// offered, the series a range of cells gives (DetectSource), and a chart's
// kind changed with its series kept (SetChartKind, SetComboKinds).

package edit

import (
	"errors"
	"slices"
	"strconv"

	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// ChartKind is a kind of chart Insert and Change Chart Type offer.
type ChartKind int

const (
	ColumnClustered ChartKind = iota
	ColumnStacked
	ColumnPercent
	BarClustered
	BarStacked
	BarPercent
	LinePlain
	LineMarkers
	LineStacked
	PieChart
	DoughnutChart
	AreaStandard
	AreaStacked
	AreaPercent
	ScatterMarkers
	ScatterLines
	ComboColumnLine // clustered columns and a line on the secondary axis
)

// ChartKindInfo names a kind, with the family it's listed under.
type ChartKindInfo struct {
	Kind   ChartKind
	Name   string
	Family string
}

// ChartKinds lists the kinds in Excel's order.
var ChartKinds = []ChartKindInfo{
	{ColumnClustered, "Clustered Column", "Column"},
	{ColumnStacked, "Stacked Column", "Column"},
	{ColumnPercent, "100% Stacked Column", "Column"},
	{LinePlain, "Line", "Line"},
	{LineMarkers, "Line with Markers", "Line"},
	{LineStacked, "Stacked Line", "Line"},
	{PieChart, "Pie", "Pie"},
	{DoughnutChart, "Doughnut", "Pie"},
	{BarClustered, "Clustered Bar", "Bar"},
	{BarStacked, "Stacked Bar", "Bar"},
	{BarPercent, "100% Stacked Bar", "Bar"},
	{AreaStandard, "Area", "Area"},
	{AreaStacked, "Stacked Area", "Area"},
	{AreaPercent, "100% Stacked Area", "Area"},
	{ScatterMarkers, "Scatter", "X Y (Scatter)"},
	{ScatterLines, "Scatter with Straight Lines and Markers", "X Y (Scatter)"},
	{ComboColumnLine, "Clustered Column - Line on Secondary Axis", "Combo"},
}

// ComboKinds are the kinds a series of a combo chart can take.
var ComboKinds = []ChartKind{ColumnClustered, ColumnStacked, LinePlain, LineMarkers, AreaStandard, AreaStacked}

// String is the kind's name in ChartKinds, or "Chart" for an unlisted one.
func (k ChartKind) String() string {
	for _, i := range ChartKinds {
		if i.Kind == k {
			return i.Name
		}
	}
	return "Chart"
}

// ErrNoChartData is the refusal to make a chart from nothing.
var ErrNoChartData = errors.New("Select a range with data for the chart first")

// ChartSource is a block of cells a chart's series are read from, as
// Excel lays it out: series in columns (ByCols) or rows; the first row
// and column labels (series names or categories) when HeaderRow and
// HeaderCol.
type ChartSource struct {
	Sheet                string
	Range                model.Range
	ByCols               bool
	HeaderRow, HeaderCol bool
}

// DetectSource lays out range r of sheet sh as Excel does for a new
// chart: a blank top-left cell makes the first row and column labels; a
// first row without numbers over numbers is the series names, a first
// column of text or dates beside numbers the categories; series run down
// the columns when there are at least as many rows as columns.
func DetectSource(wb *model.Workbook, sh *model.Sheet, r model.Range) ChartSource {
	src := ChartSource{Sheet: sh.Name, Range: r}
	rows, cols := r.R2-r.R1+1, r.C2-r.C1+1
	src.ByCols = rows >= cols
	num := func(row, col int) bool { return sh.Get(row, col).Kind == model.KindNumber }
	label := func(row, col int) bool { c := sh.Get(row, col); return c.Kind != model.KindNumber }
	date := func(row, col int) bool {
		c := sh.Get(row, col)
		if c.Kind != model.KindNumber || int(c.Style) >= len(wb.Styles) {
			return false
		}
		st := wb.Styles[c.Style]
		return numfmt.IsDate(st.NumFmtID, st.NumFmtCode)
	}
	all := func(r1, c1, r2, c2 int, f func(int, int) bool) bool {
		for row := r1; row <= r2; row++ {
			for col := c1; col <= c2; col++ {
				if !f(row, col) {
					return false
				}
			}
		}
		return true
	}
	anyNum := func(r1, c1, r2, c2 int) bool {
		return !all(r1, c1, r2, c2, func(row, col int) bool { return !num(row, col) })
	}
	if rows > 1 && cols > 1 && sh.Get(r.R1, r.C1).IsBlank() {
		src.HeaderRow, src.HeaderCol = true, true
		return src
	}
	if rows > 1 {
		src.HeaderRow = all(r.R1, r.C1, r.R1, r.C2, label) && anyNum(r.R1+1, r.C1, r.R2, r.C2)
	}
	if cols > 1 {
		top := r.R1
		if src.HeaderRow {
			top++
		}
		src.HeaderCol = top <= r.R2 && all(top, r.C1, r.R2, r.C1, func(row, col int) bool { return label(row, col) || date(row, col) }) &&
			anyNum(top, r.C1+1, r.R2, r.C2)
		if src.HeaderCol && !src.HeaderRow && rows > 1 && all(r.R1, r.C1+1, r.R1, r.C2, label) {
			src.HeaderRow = true // a label over the categories and over each series
		}
	}
	return src
}

// absRef is Sheet!$A$1:$B$2 (or Sheet!$A$1 for one cell).
func absRef(sheet string, r1, c1, r2, c2 int) string {
	cell := func(r, c int) string { return "$" + model.ColName(c) + "$" + strconv.Itoa(r+1) }
	s := formula.QuoteSheet(sheet) + "!" + cell(r1, c1)
	if r1 != r2 || c1 != c2 {
		s += ":" + cell(r2, c2)
	}
	return s
}

// SourceRef is the source's range as a reference.
func (s ChartSource) SourceRef() string {
	return absRef(s.Sheet, s.Range.R1, s.Range.C1, s.Range.R2, s.Range.C2)
}

// Series are the series the source gives; for a scatter chart the first
// series of an unlabeled block is the X values of the others.
func (s ChartSource) Series(wb *model.Workbook, scatter bool) []model.ChartSeries {
	r := s.Range
	d := r // the data, without the labels
	if s.HeaderRow {
		d.R1++
	}
	if s.HeaderCol {
		d.C1++
	}
	if d.R1 > d.R2 || d.C1 > d.C2 {
		return nil
	}
	type line struct{ r1, c1, r2, c2 int }
	var names []line // where each series' name is, if anywhere
	var vals []line
	var cat *line
	if s.ByCols {
		for c := d.C1; c <= d.C2; c++ {
			vals = append(vals, line{d.R1, c, d.R2, c})
			if s.HeaderRow {
				names = append(names, line{r.R1, c, r.R1, c})
			}
		}
		if s.HeaderCol {
			cat = &line{d.R1, r.C1, d.R2, r.C1}
		}
	} else {
		for row := d.R1; row <= d.R2; row++ {
			vals = append(vals, line{row, d.C1, row, d.C2})
			if s.HeaderCol {
				names = append(names, line{row, r.C1, row, r.C1})
			}
		}
		if s.HeaderRow {
			cat = &line{r.R1, d.C1, r.R1, d.C2}
		}
	}
	if scatter && cat == nil && len(vals) > 1 {
		x := vals[0]
		cat, vals = &x, vals[1:]
		if len(names) > 0 {
			names = names[1:]
		}
	}
	sh := wb.SheetByName(s.Sheet)
	text := func(l line) bool {
		for row := l.r1; row <= l.r2; row++ {
			for col := l.c1; col <= l.c2; col++ {
				if sh != nil && sh.Get(row, col).Kind == model.KindText {
					return true
				}
			}
		}
		return false
	}
	out := make([]model.ChartSeries, len(vals))
	for i, v := range vals {
		ser := &out[i]
		ser.Idx, ser.Order = i, i
		ser.Val = model.ChartData{Ref: absRef(s.Sheet, v.r1, v.c1, v.r2, v.c2)}
		if i < len(names) {
			n := names[i]
			ser.Name = model.ChartData{Ref: absRef(s.Sheet, n.r1, n.c1, n.r2, n.c2), Text: true}
		}
		if cat != nil {
			ser.Cat = model.ChartData{Ref: absRef(s.Sheet, cat.r1, cat.c1, cat.r2, cat.c2), Text: text(*cat)}
		}
	}
	return out
}

// dateCats reports whether a series' categories are dates.
func dateCats(wb *model.Workbook, s *model.ChartSeries, home string) bool {
	rs, ok := model.ParseRefList(s.Cat.Ref, home)
	if !ok || len(rs) != 1 || s.Cat.Text {
		return false
	}
	sh := wb.SheetByName(rs[0].Sheet)
	if sh == nil {
		return false
	}
	r := rs[0].Range
	any := false
	for row := r.R1; row <= min(r.R2, sh.UsedRows()-1); row++ {
		for col := r.C1; col <= min(r.C2, sh.UsedCols()-1); col++ {
			c := sh.Get(row, col)
			if c.IsBlank() {
				continue
			}
			if c.Kind != model.KindNumber || int(c.Style) >= len(wb.Styles) {
				return false
			}
			st := wb.Styles[c.Style]
			if !numfmt.IsDate(st.NumFmtID, st.NumFmtCode) {
				return false
			}
			any = true
		}
	}
	return any
}

// ChartDataSource is the block of cells a chart's series come from, if
// they are laid out as one (what Select Data shows as the chart data
// range); ok is false for series picked one by one.
func ChartDataSource(wb *model.Workbook, c *model.Chart, home string) (ChartSource, bool) {
	all := c.AllSeries()
	if len(all) == 0 {
		return ChartSource{}, false
	}
	var src ChartSource
	first := true
	add := func(ref string) bool {
		rs, ok := model.ParseRefList(ref, home)
		if !ok || len(rs) != 1 {
			return false
		}
		if first {
			src.Sheet, src.Range, first = rs[0].Sheet, rs[0].Range, false
			return true
		}
		if rs[0].Sheet != src.Sheet {
			return false
		}
		g := rs[0].Range
		src.Range = model.Range{R1: min(src.Range.R1, g.R1), C1: min(src.Range.C1, g.C1), R2: max(src.Range.R2, g.R2), C2: max(src.Range.C2, g.C2)}
		return true
	}
	scatter := false
	for _, sr := range all {
		p := &c.Plots[sr.Plot]
		s := &p.Series[sr.Series]
		scatter = scatter || p.Type == model.ChartScatter
		for _, d := range []*model.ChartData{&s.Name, &s.Cat, &s.Val} {
			if d.Ref != "" && !add(d.Ref) {
				return ChartSource{}, false
			}
		}
		if s.Val.Ref == "" {
			return ChartSource{}, false
		}
	}
	s0 := &c.Plots[all[0].Plot].Series[all[0].Series]
	rs, _ := model.ParseRefList(s0.Val.Ref, home)
	v := rs[0].Range
	src.ByCols = v.C1 == v.C2 && (v.R1 != v.R2 || len(all) == 1 && src.Range.C1 == src.Range.C2)
	if s0.Name.Ref != "" {
		if src.ByCols {
			src.HeaderRow = true
		} else {
			src.HeaderCol = true
		}
	}
	if s0.Cat.Ref != "" {
		cr, _ := model.ParseRefList(s0.Cat.Ref, home)
		if src.ByCols {
			src.HeaderCol = cr[0].Range.C1 < v.C1 || !scatter
		} else {
			src.HeaderRow = cr[0].Range.R1 < v.R1 || !scatter
		}
	}
	// The layout must give back the very series the chart has.
	got := src.Series(wb, scatter)
	if len(got) != len(all) {
		return ChartSource{}, false
	}
	for i, sr := range all {
		s := &c.Plots[sr.Plot].Series[sr.Series]
		g := &got[i]
		if !sameRef(g.Name.Ref, s.Name.Ref, home) || !sameRef(g.Cat.Ref, s.Cat.Ref, home) || !sameRef(g.Val.Ref, s.Val.Ref, home) {
			return ChartSource{}, false
		}
	}
	return src, true
}

// sameRef reports whether references a and b (relative to sheet home)
// name the same ranges; an empty one equals only an empty one.
func sameRef(a, b, home string) bool {
	if a == "" || b == "" {
		return a == b
	}
	ra, ok1 := model.ParseRefList(a, home)
	rb, ok2 := model.ParseRefList(b, home)
	return ok1 && ok2 && slices.Equal(ra, rb)
}

// NewChart makes a chart of kind from range r of sheet (from the data
// around the active cell when r is one cell, as Excel does): its series
// laid out by DetectSource, a title, a legend when there are several
// series (or slices), and no anchor yet.
func NewChart(wb *model.Workbook, sheet int, r model.Range, kind ChartKind) (*model.Chart, ChartSource, error) {
	sh := wb.Sheets[sheet]
	if r.R1 == r.R2 && r.C1 == r.C2 {
		r = CurrentRegion(sh, r.R1, r.C1)
	}
	r.R2 = min(r.R2, sh.UsedRows()-1)
	r.C2 = min(r.C2, sh.UsedCols()-1)
	if r.R2 < r.R1 || r.C2 < r.C1 {
		return nil, ChartSource{}, ErrNoChartData
	}
	src := DetectSource(wb, sh, r)
	ser := src.Series(wb, kind == ScatterMarkers || kind == ScatterLines)
	if len(ser) == 0 {
		return nil, src, ErrNoChartData
	}
	c := &model.Chart{Style: 2, PlotVisOnly: true, Title: &model.ChartTitle{}}
	c.Plots = []model.ChartPlot{{Series: ser}}
	SetChartKind(wb, c, sh.Name, kind)
	if len(ser) > 1 || kind == PieChart || kind == DoughnutChart {
		c.Legend = &model.ChartLegend{Pos: "b"}
	}
	return c, src, nil
}

// Axis ids of the charts made here: the primary pair and the secondary.
const (
	axCat1 = 500000001 + iota
	axVal1
	axCat2
	axVal2
)

// KindOf is the kind a chart is (the closest one listed), for Change
// Chart Type.
func KindOf(c *model.Chart) ChartKind {
	var plots []*model.ChartPlot
	for i := range c.Plots {
		if c.Plots[i].Type != model.ChartOther {
			plots = append(plots, &c.Plots[i])
		}
	}
	if len(plots) > 1 {
		return ComboColumnLine
	}
	if len(plots) == 0 {
		return ColumnClustered
	}
	return plotKind(plots[0])
}

// plotKind is the listed kind closest to chart group p.
func plotKind(p *model.ChartPlot) ChartKind {
	g := int(p.Grouping)
	switch p.Type {
	case model.ChartBar:
		if p.Horizontal {
			return BarClustered + ChartKind(g)
		}
		return ColumnClustered + ChartKind(g)
	case model.ChartLine:
		switch {
		case p.Grouping != model.GroupClustered:
			return LineStacked
		case p.Marker:
			return LineMarkers
		}
		return LinePlain
	case model.ChartPie:
		return PieChart
	case model.ChartDoughnut:
		return DoughnutChart
	case model.ChartArea:
		return AreaStandard + ChartKind(g)
	case model.ChartScatter:
		if p.ScatterStyle == "marker" || len(p.Series) > 0 && p.Series[0].Shape.Line.Kind == model.FillNone {
			return ScatterMarkers
		}
		return ScatterLines
	}
	return ColumnClustered
}

// SeriesKinds are each series' kind and whether it's on the secondary
// axis, in the order of Chart.AllSeries.
func SeriesKinds(c *model.Chart) (kinds []ChartKind, secondary []bool) {
	primary := -1
	for pi := range c.Plots {
		p := &c.Plots[pi]
		if primary < 0 && p.Type != model.ChartOther {
			primary = pi
		}
		sec := primary >= 0 && pi != primary && len(p.AxisIDs) == 2 && len(c.Plots[primary].AxisIDs) == 2 &&
			p.AxisIDs[1] != c.Plots[primary].AxisIDs[1]
		for range p.Series {
			kinds = append(kinds, plotKind(p))
			secondary = append(secondary, sec)
		}
	}
	return kinds, secondary
}

// SetChartKind makes c a chart of kind, keeping its series (in order),
// titles, legend and the primary axes' titles and gridlines. home is
// the chart's sheet. A combo chart puts the last series in a line on
// the secondary axis.
func SetChartKind(wb *model.Workbook, c *model.Chart, home string, kind ChartKind) {
	n := len(c.AllSeries())
	kinds, sec := make([]ChartKind, n), make([]bool, n)
	for i := range kinds {
		kinds[i] = kind
	}
	if kind == ComboColumnLine {
		for i := range kinds {
			kinds[i] = ColumnClustered
		}
		if n > 1 {
			kinds[n-1], sec[n-1] = LinePlain, true
		}
	}
	SetComboKinds(wb, c, home, kinds, sec)
}

// SetComboKinds gives each series of c (in the order of AllSeries) its
// kind and axis: a chart group for each kind and axis, in the order they
// first appear. A pie, doughnut or scatter kind takes all the series.
func SetComboKinds(wb *model.Workbook, c *model.Chart, home string, kinds []ChartKind, secondary []bool) {
	var all []model.ChartSeries
	oldKinds, _ := SeriesKinds(c)
	for pi := range c.Plots {
		if c.Plots[pi].Type == model.ChartOther {
			continue
		}
		all = append(all, c.Plots[pi].Series...)
	}
	var oldLabels *model.DataLabels
	for pi := range c.Plots {
		if c.Plots[pi].Labels != nil {
			oldLabels = c.Plots[pi].Labels
			break
		}
	}
	// The primary axes' settings are kept: titles, gridlines, hidden.
	var keepCat, keepVal *model.ChartAxis
	for pi := range c.Plots {
		if ids := c.Plots[pi].AxisIDs; len(ids) == 2 && c.Plots[pi].Type != model.ChartOther {
			keepCat, keepVal = c.Axis(ids[0]), c.Axis(ids[1])
			break
		}
	}
	for len(kinds) < len(all) {
		kinds = append(kinds, ColumnClustered)
	}
	for len(secondary) < len(all) {
		secondary = append(secondary, false)
	}
	whole := func(k ChartKind) bool {
		return k == PieChart || k == DoughnutChart || k == ScatterMarkers || k == ScatterLines
	}
	for _, k := range kinds[:len(all)] {
		if whole(k) {
			for i := range kinds {
				kinds[i], secondary[i] = k, false
			}
			break
		}
	}
	family := func(k ChartKind) int {
		switch {
		case k <= BarPercent:
			return 0
		case k <= LineStacked:
			return 1
		case k <= DoughnutChart:
			return 2
		case k <= AreaPercent:
			return 3
		}
		return 4
	}
	type key struct {
		kind ChartKind
		sec  bool
	}
	var order []key
	groups := map[key][]model.ChartSeries{}
	for i, s := range all {
		k := key{kinds[i], secondary[i]}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		if i < len(oldKinds) && family(oldKinds[i]) != family(kinds[i]) {
			// What belonged to the old kind: markers, slices' own colors.
			s.Marker, s.Smooth, s.Explosion, s.Points = model.Marker{}, false, 0, nil
			s.Labels = nil
		}
		s.Idx, s.Order = i, i
		groups[k] = append(groups[k], s)
	}
	// The secondary groups go after the primary ones.
	slices.SortStableFunc(order, func(a, b key) int {
		switch {
		case a.sec == b.sec:
			return 0
		case b.sec:
			return -1
		}
		return 1
	})
	c.Plots = nil
	hasSec, horizontal, scatter, pie := false, false, false, false
	for _, k := range order {
		p := newPlot(k.kind)
		p.Series = groups[k]
		if oldLabels != nil {
			l := *oldLabels
			p.Labels = &l
		}
		switch {
		case k.kind == PieChart || k.kind == DoughnutChart:
			pie = true
		case k.sec:
			p.AxisIDs, hasSec = []int{axCat2, axVal2}, true
		default:
			p.AxisIDs = []int{axCat1, axVal1}
		}
		horizontal = horizontal || p.Horizontal && !k.sec
		scatter = scatter || p.Type == model.ChartScatter
		c.Plots = append(c.Plots, p)
	}
	if scatter {
		fixScatterLines(c, kinds[0])
	}
	if pie {
		c.Axes = nil
		return
	}
	cat := model.ChartAxis{ID: axCat1, Kind: model.AxisCat, Pos: "b", CrossAx: axVal1, Crosses: "autoZero",
		TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none"}
	val := model.ChartAxis{ID: axVal1, Kind: model.AxisVal, Pos: "l", CrossAx: axCat1, Crosses: "autoZero",
		CrossBetween: "between", MajorGrid: true, TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none", SourceLinked: true}
	switch {
	case scatter:
		cat.Kind, cat.CrossBetween, cat.SourceLinked = model.AxisVal, "midCat", true
		val.CrossBetween = "midCat"
	case len(all) > 0 && dateCats(wb, &all[0], home):
		cat.Kind = model.AxisDate
	}
	for _, p := range c.Plots {
		if p.Type == model.ChartArea && !p.Horizontal {
			val.CrossBetween = "midCat"
		}
	}
	if horizontal {
		cat.Pos, val.Pos = "l", "b"
	}
	keep := func(to *model.ChartAxis, from *model.ChartAxis) {
		if from == nil {
			return
		}
		to.Title, to.Delete, to.Font = from.Title, from.Delete, from.Font
		to.MajorGrid, to.MinorGrid = from.MajorGrid, from.MinorGrid
		to.MajorGridLine, to.MinorGridLine, to.Line = from.MajorGridLine, from.MinorGridLine, from.Line
		if from.Kind == to.Kind {
			to.NumFmt, to.SourceLinked = from.NumFmt, from.SourceLinked
		}
	}
	keep(&cat, keepCat)
	keep(&val, keepVal)
	c.Axes = []model.ChartAxis{cat, val}
	if hasSec {
		c.Axes = append(c.Axes,
			model.ChartAxis{ID: axCat2, Kind: cat.Kind, Pos: "b", Delete: true, CrossAx: axVal2, Crosses: "autoZero",
				TickLblPos: "nextTo", MajorTick: "out", MinorTick: "none"},
			model.ChartAxis{ID: axVal2, Kind: model.AxisVal, Pos: "r", CrossAx: axCat2, Crosses: "max",
				CrossBetween: "between", TickLblPos: "nextTo", MajorTick: "out", MinorTick: "none", SourceLinked: true})
	}
	for _, p := range c.Plots {
		if p.Type == model.ChartBar && p.Grouping == model.GroupPercent || p.Type == model.ChartArea && p.Grouping == model.GroupPercent {
			for i := range c.Axes {
				if c.Axes[i].ID == p.AxisIDs[1] {
					c.Axes[i].NumFmt, c.Axes[i].SourceLinked = "0%", false
				}
			}
		}
	}
}

// newPlot is an empty chart group of kind with Excel's defaults.
func newPlot(kind ChartKind) model.ChartPlot {
	var p model.ChartPlot
	stacking := func(base ChartKind) model.Grouping { return model.Grouping(kind - base) }
	switch {
	case kind <= ColumnPercent:
		p = model.ChartPlot{Type: model.ChartBar, Grouping: stacking(ColumnClustered)}
	case kind <= BarPercent:
		p = model.ChartPlot{Type: model.ChartBar, Horizontal: true, Grouping: stacking(BarClustered)}
	case kind == LinePlain:
		p = model.ChartPlot{Type: model.ChartLine}
	case kind == LineMarkers:
		p = model.ChartPlot{Type: model.ChartLine, Marker: true}
	case kind == LineStacked:
		p = model.ChartPlot{Type: model.ChartLine, Grouping: model.GroupStacked}
	case kind == PieChart:
		p = model.ChartPlot{Type: model.ChartPie, VaryColors: true}
	case kind == DoughnutChart:
		p = model.ChartPlot{Type: model.ChartDoughnut, VaryColors: true, HoleSize: 75}
	case kind <= AreaPercent:
		p = model.ChartPlot{Type: model.ChartArea, Grouping: stacking(AreaStandard)}
	case kind == ScatterMarkers:
		p = model.ChartPlot{Type: model.ChartScatter, ScatterStyle: "lineMarker"}
	default:
		p = model.ChartPlot{Type: model.ChartScatter, ScatterStyle: "lineMarker"}
	}
	if p.Type == model.ChartBar {
		p.GapWidth, p.Overlap = 219, -27
		if p.Grouping != model.GroupClustered {
			p.GapWidth, p.Overlap = 150, 100
		}
	}
	return p
}

// fixScatterLines sets what a scatter kind says about each series' line:
// none for markers only.
func fixScatterLines(c *model.Chart, kind ChartKind) {
	for pi := range c.Plots {
		p := &c.Plots[pi]
		if p.Type != model.ChartScatter {
			continue
		}
		for si := range p.Series {
			s := &p.Series[si]
			if kind == ScatterMarkers {
				s.Shape.Line = model.Paint{Kind: model.FillNone}
			} else if s.Shape.Line.Kind == model.FillNone {
				s.Shape.Line = model.Paint{}
			}
		}
	}
}
