// chartedit.go holds chart editing: adding one, changing one (a chart
// changed here is saved from the model, its part as read dropped), moving
// and sizing one, chart sheets; and the changes the Chart Design tab makes
// to a chart (elements, colors, styles), which work on a copy the caller
// then sets with SetChart.

package edit

import (
	"fmt"
	"image/color"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/model"
)

// chartAt returns chart idx of sheet, or the chart sheet's chart for
// idx < 0.
func (b *Book) chartAt(sheet, idx int) *model.Chart {
	if sheet < 0 || sheet >= len(b.WB.Sheets) {
		return nil
	}
	sh := b.WB.Sheets[sheet]
	if idx < 0 {
		return sh.ChartSheet
	}
	if idx < len(sh.Charts) {
		return sh.Charts[idx]
	}
	return nil
}

// chartWhere is the cell at c's top-left anchor, for Undo to select.
func chartWhere(sheet int, c *model.Chart) Where {
	a := c.Anchor.From
	return Where{sheet, model.Range{R1: a.Row, C1: a.Col, R2: a.Row, C2: a.Col}}
}

// NextChartName is the first free "Chart n" name of the workbook.
func NextChartName(wb *model.Workbook) string {
	used := map[string]bool{}
	for _, sh := range wb.Sheets {
		for _, c := range sh.Charts {
			used[strings.ToLower(c.Name)] = true
		}
	}
	for i := 1; ; i++ {
		if n := fmt.Sprintf("Chart %d", i); !used[strings.ToLower(n)] {
			return n
		}
	}
}

// NextChartSheetName is the first free "Chartn" sheet name.
func NextChartSheetName(wb *model.Workbook) string {
	for i := 1; ; i++ {
		if n := fmt.Sprintf("Chart%d", i); wb.SheetByName(n) == nil {
			return n
		}
	}
}

// AddChart adds c to sheet's charts (named Chart n when it has no name)
// and returns its index.
func (b *Book) AddChart(sheet int, c *model.Chart) (int, calc.Result) {
	sh := b.WB.Sheets[sheet]
	c = c.Clone()
	if c.Name == "" {
		c.Name = NextChartName(b.WB)
	}
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.charts = append(ch.after.charts, c)
	res := b.applySheet(chartWhere(sheet, c), ch, nil)
	return len(sh.Charts) - 1, res
}

// SetChart replaces chart idx of sheet (the chart sheet's chart for idx
// < 0) with c, as an edit: it is saved from the model from now on.
func (b *Book) SetChart(sheet, idx int, c *model.Chart) calc.Result {
	c = c.Clone()
	c.Raw = nil
	return b.replaceChart(sheet, idx, c)
}

// MoveChart moves or sizes chart idx of sheet: only its anchor changes,
// so a chart read from a file is still saved as it came.
func (b *Book) MoveChart(sheet, idx int, a model.Anchor) calc.Result {
	old := b.chartAt(sheet, idx)
	if old == nil || idx < 0 || old.Anchor == a {
		return calc.Result{}
	}
	c := old.Clone()
	c.Anchor = a
	return b.replaceChart(sheet, idx, c)
}

// replaceChart puts c in place of chart idx of sheet (the chart sheet's
// chart for idx < 0) as an undoable sheet change; nothing happens when
// there is no such chart.
func (b *Book) replaceChart(sheet, idx int, c *model.Chart) calc.Result {
	if b.chartAt(sheet, idx) == nil {
		return calc.Result{}
	}
	sh := b.WB.Sheets[sheet]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	where := Where{Sheet: sheet}
	if idx < 0 {
		ch.after.chart = c
	} else {
		ch.after.charts[idx] = c
		where = chartWhere(sheet, c)
	}
	return b.applySheet(where, ch, nil)
}

// InsertChartSheet adds a chart sheet showing c before sheet index at.
func (b *Book) InsertChartSheet(at int, name string, c *model.Chart) error {
	if err := ValidSheetName(name); err != nil {
		return err
	}
	if b.sheetIndex(name) >= 0 {
		return fmt.Errorf("That name is already taken. Try a different one.")
	}
	c = c.Clone()
	c.Anchor = model.Anchor{}
	_, err := b.structural(&structural{kind: insertSheet, sheet: at, name: name, chart: c}, Where{Sheet: at})
	return err
}

// ChartToSheet moves chart idx of sheet to a new chart sheet named name
// after it (Move Chart > New sheet), as one undo step; it returns the
// new sheet's index.
func (b *Book) ChartToSheet(sheet, idx int, name string) (int, error) {
	c := b.chartAt(sheet, idx)
	if c == nil {
		return 0, fmt.Errorf("no chart")
	}
	c = c.Clone()
	b.BeginGroup()
	defer b.EndGroup()
	if err := b.InsertChartSheet(sheet+1, name, c); err != nil {
		return 0, err
	}
	if idx >= 0 {
		b.DeleteChart(sheet, idx)
	} else if _, err := b.DeleteSheet(sheet); err != nil {
		return 0, err
	} else {
		return sheet, nil
	}
	return sheet + 1, nil
}

// ChartToObject moves chart idx of sheet (a chart sheet's for idx < 0,
// which is then deleted) onto sheet to at anchor a (Move Chart > Object
// in), as one undo step; it returns where it went: the sheet (its index
// after the move) and the chart's index there.
func (b *Book) ChartToObject(sheet, idx, to int, a model.Anchor) (int, int, error) {
	c := b.chartAt(sheet, idx)
	if c == nil || to < 0 || to >= len(b.WB.Sheets) || b.WB.Sheets[to].IsChartSheet() {
		return 0, 0, fmt.Errorf("no chart")
	}
	if sheet == to && idx >= 0 {
		return to, idx, nil
	}
	c = c.Clone()
	c.Anchor = a
	if c.Name == "" || idx < 0 {
		c.Name = NextChartName(b.WB)
	}
	if idx < 0 {
		if err := b.canDeleteSheet(sheet); err != nil {
			return 0, 0, err // checked first: nothing is added if the sheet must stay
		}
	}
	b.BeginGroup()
	defer b.EndGroup()
	i, _ := b.AddChart(to, c)
	if idx >= 0 {
		b.DeleteChart(sheet, idx)
		return to, i, nil
	}
	if _, err := b.DeleteSheet(sheet); err != nil {
		return 0, 0, err
	}
	if to > sheet {
		to--
	}
	return to, i, nil
}

// --- the Chart Design tab's changes (on a chart the caller then sets) ---

// SetChartTitle shows or hides the chart title.
func SetChartTitle(c *model.Chart, on bool) {
	if on {
		if c.Title == nil {
			c.Title = &model.ChartTitle{}
		}
		c.AutoTitleDeleted = false
		return
	}
	c.Title, c.AutoTitleDeleted = nil, true
}

// SetChartTitleText gives the chart its own title text ("" for the
// automatic title).
func SetChartTitleText(c *model.Chart, text string) {
	SetChartTitle(c, true)
	c.Title.Text = text
	c.Title.Ref = model.ChartData{}
}

// SetLegend places the legend: r, t, l, b; "" for none.
func SetLegend(c *model.Chart, pos string) {
	if pos == "" {
		c.Legend = nil
		return
	}
	if c.Legend == nil {
		c.Legend = &model.ChartLegend{}
	}
	c.Legend.Pos, c.Legend.Overlay = pos, false
}

// SetDataLabels shows values beside the points at pos (ctr, inEnd,
// outEnd, bestFit; each kind takes the positions it can), or none for
// "". Labels set on single series give way.
func SetDataLabels(c *model.Chart, pos string, on bool) {
	for pi := range c.Plots {
		p := &c.Plots[pi]
		for si := range p.Series {
			p.Series[si].Labels = nil
		}
		if !on {
			p.Labels = nil
			continue
		}
		lb := &model.DataLabels{ShowVal: true, Pos: labelPos(p, pos)}
		if p.Type == model.ChartPie || p.Type == model.ChartDoughnut {
			lb.ShowLeader = true
		}
		p.Labels = lb
	}
}

// labelPos is the position closest to pos a chart group takes.
func labelPos(p *model.ChartPlot, pos string) string {
	switch p.Type {
	case model.ChartBar:
		if pos == "bestFit" || pos == "outEnd" && p.Grouping != model.GroupClustered {
			return "inEnd"
		}
		return pos
	case model.ChartLine, model.ChartScatter:
		switch pos {
		case "ctr":
			return "ctr"
		case "inEnd":
			return "b"
		}
		return "t"
	case model.ChartPie:
		return pos
	}
	return "" // areas and doughnuts: the default
}

// chartAxis finds a primary axis: the horizontal one (at the bottom or
// top) or the vertical one.
func chartAxis(c *model.Chart, horizontal bool) *model.ChartAxis {
	for pi := range c.Plots {
		ids := c.Plots[pi].AxisIDs
		if len(ids) != 2 || c.Plots[pi].Type == model.ChartOther {
			continue
		}
		for _, id := range ids {
			if ax := c.Axis(id); ax != nil && (ax.Pos == "b" || ax.Pos == "t") == horizontal {
				return ax
			}
		}
		return nil
	}
	return nil
}

// HasAxis reports whether c has the primary horizontal or vertical axis.
func HasAxis(c *model.Chart, horizontal bool) bool { return chartAxis(c, horizontal) != nil }

// AxisShown reports whether a primary axis is shown.
func AxisShown(c *model.Chart, horizontal bool) bool {
	ax := chartAxis(c, horizontal)
	return ax != nil && !ax.Delete
}

// SetAxisShown shows or hides a primary axis.
func SetAxisShown(c *model.Chart, horizontal, on bool) {
	if ax := chartAxis(c, horizontal); ax != nil {
		ax.Delete = !on
	}
}

// AxisTitleShown reports whether a primary axis has a title.
func AxisTitleShown(c *model.Chart, horizontal bool) bool {
	ax := chartAxis(c, horizontal)
	return ax != nil && ax.Title != nil
}

// SetAxisTitle gives a primary axis the title "Axis Title" (turned up
// the side for a vertical axis), or takes it away.
func SetAxisTitle(c *model.Chart, horizontal, on bool) {
	ax := chartAxis(c, horizontal)
	switch {
	case ax == nil:
	case !on:
		ax.Title = nil
	case ax.Title == nil:
		ax.Title = &model.ChartTitle{Text: "Axis Title"}
		if !horizontal {
			ax.Title.Font.Rotation, ax.Title.Font.HasRot = -90, true
		}
	}
}

// Gridlines reports whether a primary axis draws its major (or minor)
// gridlines: horizontal gridlines are the vertical axis'.
func Gridlines(c *model.Chart, horizontal, major bool) bool {
	ax := chartAxis(c, !horizontal)
	if ax == nil {
		return false
	}
	if major {
		return ax.MajorGrid
	}
	return ax.MinorGrid
}

// SetGridlines turns gridlines on or off.
func SetGridlines(c *model.Chart, horizontal, major, on bool) {
	if ax := chartAxis(c, !horizontal); ax != nil {
		if major {
			ax.MajorGrid = on
		} else {
			ax.MinorGrid = on
		}
	}
}

// LabelsPos is the data labels' position ("" for the default), and
// whether there are any.
func LabelsPos(c *model.Chart) (string, bool) {
	for _, p := range c.Plots {
		if p.Labels.Any() {
			return p.Labels.Pos, true
		}
	}
	return "", false
}

// Palette is one of Change Colors' color sets.
type Palette struct {
	Name string
	// Accents are the theme accents (1..6) used in turn; for a
	// monochromatic palette, the one accent shaded.
	Accents []int
	Mono    bool
}

// Palettes are the Change Colors choices: Excel's colorful palettes and
// a few monochromatic ones.
var Palettes = []Palette{
	{"Colorful Palette 1", []int{1, 2, 3, 4, 5, 6}, false},
	{"Colorful Palette 2", []int{2, 4, 6}, false},
	{"Colorful Palette 3", []int{3, 5, 1}, false},
	{"Colorful Palette 4", []int{6, 5, 4, 3, 2, 1}, false},
	{"Monochromatic Palette 1", []int{1}, true},
	{"Monochromatic Palette 2", []int{2}, true},
	{"Monochromatic Palette 4", []int{4}, true},
	{"Monochromatic Palette 6", []int{6}, true},
}

// Color is the palette's color for item i of n.
func (p Palette) Color(th *model.Theme, i, n int) color.NRGBA {
	if th == nil {
		th = model.DefaultTheme()
	}
	if p.Mono {
		c := th.Colors[model.ThemeAccent1+p.Accents[0]-1]
		if n <= 1 {
			return c
		}
		// From a darker shade to a lighter one.
		t := float64(i) / float64(n-1)
		if t < 0.5 {
			return model.LumMod(c, int(100000-(0.5-t)*2*40000), 0)
		}
		k := (t - 0.5) * 2 * 0.6
		return model.LumMod(c, int((1-k)*100000), int(k*100000))
	}
	c := th.Colors[model.ThemeAccent1+p.Accents[i%len(p.Accents)]-1]
	// Past the first round, Excel's darker and lighter variations.
	round := i / len(p.Accents)
	v := [][2]int{{100000, 0}, {60000, 0}, {80000, 20000}, {80000, 0}, {60000, 40000}, {50000, 0}}[round%6]
	return model.LumMod(c, v[0], v[1])
}

// ApplyPalette colors c's series (a pie's slices) with Palettes[p]; the
// first palette puts back the automatic colors.
func ApplyPalette(wb *model.Workbook, c *model.Chart, home string, p int) {
	pal := Palettes[min(max(p, 0), len(Palettes)-1)]
	auto := p == 0
	all := c.AllSeries()
	for k, sr := range all {
		pl := &c.Plots[sr.Plot]
		s := &pl.Series[sr.Series]
		if pl.VaryColors && (pl.Type == model.ChartPie || pl.Type == model.ChartDoughnut) {
			n := len(wb.SeriesData(&s.Val, home, c.PlotVisOnly))
			s.Points = nil
			if !auto {
				for i := 0; i < n; i++ {
					s.Points = append(s.Points, model.DataPoint{Idx: i, Shape: model.ShapeProps{Fill: solid(pal.Color(wb.Theme, i, n))}})
				}
			}
			continue
		}
		var paint model.Paint
		if !auto {
			paint = solid(pal.Color(wb.Theme, k, len(all)))
		}
		switch pl.Type {
		case model.ChartLine, model.ChartScatter:
			if s.Shape.Line.Kind != model.FillNone {
				s.Shape.Line = paint
			}
			s.Shape.Fill = paint
			s.Marker.Shape.Fill, s.Marker.Shape.Line = paint, paint
		default:
			s.Shape.Fill = paint
		}
	}
}

// solid is a solid paint of color c.
func solid(c color.NRGBA) model.Paint { return model.Paint{Kind: model.FillSolid, Color: c} }

// ChartStyles name the Chart Styles gallery's choices.
var ChartStyles = []string{"Style 1", "Style 2", "Style 3", "Style 4", "Style 5", "Style 6"}

// ApplyChartStyle gives c the look of ChartStyles[style]: Style 1 is Excel's
// default; 2 adds data labels and drops the value axis and gridlines; 3
// a shaded background; 4 a dark one; 5 outlined, unfilled shapes; 6
// bold titles with dashed gridlines.
func ApplyChartStyle(c *model.Chart, style int) {
	c.ChartArea, c.PlotArea = model.ShapeProps{}, model.ShapeProps{}
	// The text colors (each part's own, as a file writes them) go back
	// to the style's.
	text := func(p model.Paint) {
		c.Font.Color = p
		if c.Title != nil {
			c.Title.Font.Color = p
		}
		if c.Legend != nil {
			c.Legend.Font.Color = p
		}
		for i := range c.Axes {
			c.Axes[i].Font.Color = p
			if t := c.Axes[i].Title; t != nil {
				t.Font.Color = p
			}
		}
	}
	text(model.Paint{})
	if c.Title != nil {
		c.Title.Font.Bold = nil
	}
	SetDataLabels(c, "", false)
	for i := range c.Axes {
		c.Axes[i].MajorGridLine, c.Axes[i].Line = model.ShapeProps{}, model.ShapeProps{}
	}
	SetAxisShown(c, true, true)
	SetAxisShown(c, false, true)
	SetGridlines(c, true, true, true)
	for pi := range c.Plots {
		for si := range c.Plots[pi].Series {
			s := &c.Plots[pi].Series[si]
			if c.Plots[pi].Type != model.ChartLine && c.Plots[pi].Type != model.ChartScatter {
				s.Shape.Line, s.Shape.Width = model.Paint{}, 0
				if s.Shape.Fill.Kind == model.FillNone {
					s.Shape.Fill = model.Paint{}
				}
			}
		}
	}
	grey := func(v uint32) model.Paint {
		return solid(color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255})
	}
	switch style {
	case 1:
		SetDataLabels(c, "outEnd", true)
		SetGridlines(c, true, true, false)
		SetGridlines(c, false, true, false)
		for _, h := range []bool{true, false} {
			if ax := chartAxis(c, h); ax != nil && ax.Kind == model.AxisVal && len(c.Plots) > 0 && c.Plots[0].Type != model.ChartScatter {
				ax.Delete = true // the labels give the values
			}
		}
	case 2:
		c.ChartArea.Fill = grey(0xF2F2F2)
		c.PlotArea.Fill = grey(0xFFFFFF)
	case 3:
		c.ChartArea.Fill, c.ChartArea.Line = grey(0x404040), grey(0x404040)
		text(grey(0xD9D9D9))
		for i := range c.Axes {
			c.Axes[i].MajorGridLine.Line = grey(0x595959)
		}
	case 4:
		for pi := range c.Plots {
			p := &c.Plots[pi]
			if p.Type == model.ChartLine || p.Type == model.ChartScatter {
				continue
			}
			for si := range p.Series {
				s := &p.Series[si]
				auto := s.Shape.Fill
				if auto.Kind != model.FillSolid {
					th := model.DefaultTheme()
					auto = solid(th.SeriesColor(s.Idx))
				}
				s.Shape.Line, s.Shape.Width = auto, 1.5
				s.Shape.Fill = solid(model.LumMod(auto.Color, 20000, 80000))
			}
		}
	case 5:
		if c.Title != nil {
			bold := true
			c.Title.Font.Bold = &bold
		}
		for i := range c.Axes {
			c.Axes[i].MajorGridLine.Dash = "dash"
		}
	}
}

// SwitchRowColumn lays the chart's data out the other way: the series
// become the categories and the categories the series. It fails when the
// series don't come from one block of cells.
func SwitchRowColumn(wb *model.Workbook, c *model.Chart, home string) error {
	src, ok := ChartDataSource(wb, c, home)
	if !ok {
		return fmt.Errorf("The data range is too complex to be displayed: switch rows and columns in Select Data with one range of cells")
	}
	src.ByCols = !src.ByCols
	SetChartSource(wb, c, home, src)
	return nil
}

// SetChartSource gives c the series src lays out, keeping its kind
// (and the combo layout of the last series).
func SetChartSource(wb *model.Workbook, c *model.Chart, home string, src ChartSource) {
	kind := KindOf(c)
	ser := src.Series(wb, kind == ScatterMarkers || kind == ScatterLines)
	keepFmt := map[int]model.ChartSeries{}
	for _, sr := range c.AllSeries() {
		s := c.Plots[sr.Plot].Series[sr.Series]
		keepFmt[s.Idx] = s
	}
	for i := range ser {
		if old, ok := keepFmt[i]; ok {
			ser[i].Shape, ser[i].Marker = old.Shape, old.Marker
		}
	}
	SetChartSeries(wb, c, home, ser)
}

// SetChartSeries replaces c's series, keeping its kind; a combo chart
// keeps each series' kind by position and puts new ones in the first
// group.
func SetChartSeries(wb *model.Workbook, c *model.Chart, home string, ser []model.ChartSeries) {
	kind := KindOf(c)
	kinds, sec := SeriesKinds(c)
	if len(c.Plots) == 0 {
		c.Plots = []model.ChartPlot{newPlot(kind)}
	}
	first := 0
	for i := range c.Plots {
		if c.Plots[i].Type != model.ChartOther {
			first = i
			break
		}
	}
	for i := range c.Plots {
		c.Plots[i].Series = nil
	}
	c.Plots[first].Series = ser
	if kind != ComboColumnLine {
		SetChartKind(wb, c, home, kind)
		return
	}
	for len(kinds) < len(ser) {
		k := ColumnClustered // a combo with no series yet has no kind to copy
		if len(kinds) > 0 {
			k = kinds[0]
		}
		kinds = append(kinds, k)
		sec = append(sec, false)
	}
	SetComboKinds(wb, c, home, kinds[:len(ser)], sec[:len(ser)])
}
