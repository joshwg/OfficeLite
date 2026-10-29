// Making and changing charts from the menus and ribbons: the Insert
// menu's chart entries and the Charts group of the Data ribbon (ExcelLite
// has no Insert tab), the contextual Chart Design and Format tabs that
// show while a chart is selected (or a chart sheet is active), and the
// chart's right-click menu.

package ui

import (
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// The contextual chart tabs (pivot.go numbers the PivotTable ones).
const (
	tabChartDesign = 200 + iota
	tabChartFormat
)

// chartsGroup is the Data ribbon's Charts group's widget state.
type chartsGroup struct {
	colDD, barDD, lineDD gui.DropDown
	pieDD, areaDD, xyDD  gui.DropDown
	combo, all           widget.Clickable
	tipsSet              bool
}

// chartDesignRibbon is the Chart Design ribbon's widget state.
type chartDesignRibbon struct {
	ribbon                   gui.Ribbon
	elemDD, colorsDD, styles gui.DropDown
	switchRC, data, kind     widget.Clickable
	move                     widget.Clickable
	tipsSet                  bool
}

// chartFormatRibbon is the Format ribbon's widget state.
type chartFormatRibbon struct {
	ribbon          gui.Ribbon
	elemDD          gui.DropDown
	formatSel       widget.Clickable
	reset           widget.Clickable
	fill, line      widget.Clickable
	fillCP, lineCP  gui.ColorPicker
	lastFill, lastL gui.ColorChoice
	tipsSet         bool
}

// kindsOf lists the kinds of a family, for its drop-down.
func kindsOf(family string) (kinds []edit.ChartKind, labels []string) {
	for _, k := range edit.ChartKinds {
		if k.Family == family {
			kinds, labels = append(kinds, k.Kind), append(labels, k.Name)
		}
	}
	return kinds, labels
}

// kindIcon is a kind's icon.
func kindIcon(k edit.ChartKind) *gui.Icon {
	switch k {
	case edit.ColumnStacked, edit.ColumnPercent:
		return gui.IconChartStacked
	case edit.BarClustered, edit.BarStacked, edit.BarPercent:
		return gui.IconChartBar
	case edit.LinePlain, edit.LineStacked:
		return gui.IconChartLine
	case edit.LineMarkers:
		return gui.IconChartMarkers
	case edit.PieChart:
		return gui.IconChartPie
	case edit.DoughnutChart:
		return gui.IconChartDoughnut
	case edit.AreaStandard, edit.AreaStacked, edit.AreaPercent:
		return gui.IconChartArea
	case edit.ScatterMarkers, edit.ScatterLines:
		return gui.IconChartScatter
	case edit.ComboColumnLine:
		return gui.IconChartCombo
	}
	return gui.IconChartColumn
}

// setTips sets the tooltips of the Charts group's buttons.
func (g *chartsGroup) setTips() {
	tip := gui.SetTip
	tip(&g.colDD, "Insert Column Chart", "Alt+F1", "Use this chart type to visually compare values across a few categories.")
	tip(&g.barDD, "Insert Bar Chart", "", "Compare values across categories when the chart shows long labels or durations.")
	tip(&g.lineDD, "Insert Line Chart", "", "Use this chart type to show trends over time (years, months and days) or categories.")
	tip(&g.pieDD, "Insert Pie or Doughnut Chart", "", "Use this chart type to show proportions of a whole.")
	tip(&g.areaDD, "Insert Area Chart", "", "Show trends over time and how the parts add up to the whole.")
	tip(&g.xyDD, "Insert Scatter (X, Y) Chart", "", "Use this chart type to show the relationship between sets of values.")
	tip(&g.combo, "Insert Combo Chart", "", "Use this chart type to highlight different types of information: columns and a line on a secondary axis.")
	tip(&g.all, "All Charts", "", "Pick from all the chart types in the Insert Chart dialog. F11 puts the default chart on a sheet of its own.")
}

// chartsRibbonGroup is the Charts group of the Data ribbon.
func (a *App) chartsRibbonGroup(gtx layout.Context, k *gui.RibbonKit) gui.RibbonGroup {
	g := &a.ch.charts
	if !g.tipsSet {
		g.setTips()
		g.tipsSet = true
	}
	k.Click(gtx, &g.combo, func() { a.insertChart(edit.ComboColumnLine) })
	k.Click(gtx, &g.all, func() { a.openChartTypeDialog(true) })
	on := !a.onChartSheet()
	drop := func(dd *gui.DropDown, family, label string, ic *gui.Icon) layout.Widget {
		kinds, labels := kindsOf(family)
		if family == "Pie" {
			label = "Pie"
		}
		return gui.Enabled(on, k.Drop(dd, ic, label, gui.MenuItems(labels), 280, func(i int) { a.insertChart(kinds[i]) }))
	}
	row, stack := gui.RibbonRow, gui.RibbonStack
	return gui.RibbonGroup{Caption: "Charts", Content: gui.RibbonSide(2,
		gui.Enabled(on, k.BigBtn(&g.all, gui.IconChangeType, "All Charts", false)),
		stack(
			row(drop(&g.colDD, "Column", "Column", gui.IconChartColumn), drop(&g.lineDD, "Line", "Line", gui.IconChartLine),
				drop(&g.pieDD, "Pie", "Pie", gui.IconChartPie)),
			row(drop(&g.barDD, "Bar", "Bar", gui.IconChartBar), drop(&g.areaDD, "Area", "Area", gui.IconChartArea),
				drop(&g.xyDD, "X Y (Scatter)", "Scatter", gui.IconChartScatter)),
		),
		gui.Enabled(on, k.BigBtn(&g.combo, gui.IconChartCombo, "Combo", false)),
	)}
}

// chartMenus are the contextual tabs, shown while a chart is selected or
// a chart sheet is active.
func (a *App) chartMenus() []gui.Menu {
	if c, _ := a.curChart(); c == nil {
		if a.tab >= tabChartDesign {
			a.tab = tabHome
		}
		return nil
	}
	return []gui.Menu{
		{Label: "Chart Design", Tab: true, Selected: a.tab == tabChartDesign},
		{Label: "Format", Tab: true, Selected: a.tab == tabChartFormat},
	}
}

// chartMenuChosen switches to a chart tab (i is 0 or 1).
func (a *App) chartMenuChosen(i int) {
	if i == 0 {
		a.tab = tabChartDesign
	} else {
		a.tab = tabChartFormat
	}
}

// Add Chart Element's items: headers, then the choices under them.
var elemItems = []gui.DropItem{
	{Label: "Axes", Header: true}, {Label: "Primary Horizontal"}, {Label: "Primary Vertical"},
	{Label: "Axis Titles", Header: true}, {Label: "Primary Horizontal Title"}, {Label: "Primary Vertical Title"},
	{Label: "Chart Title", Header: true}, {Label: "None"}, {Label: "Above Chart"},
	{Label: "Data Labels", Header: true}, {Label: "None"}, {Label: "Center"}, {Label: "Inside End"}, {Label: "Outside End"}, {Label: "Best Fit"},
	{Label: "Gridlines", Header: true}, {Label: "Primary Major Horizontal"}, {Label: "Primary Major Vertical"},
	{Label: "Primary Minor Horizontal"}, {Label: "Primary Minor Vertical"},
	{Label: "Legend", Header: true}, {Label: "None"}, {Label: "Right"}, {Label: "Top"}, {Label: "Left"}, {Label: "Bottom"},
}

// elementItems are Add Chart Element's items, those in effect checked.
func elementItems(c *model.Chart) []gui.DropItem {
	items := make([]gui.DropItem, len(elemItems))
	copy(items, elemItems)
	lpos, labels := edit.LabelsPos(c)
	legend := ""
	if c.Legend != nil {
		legend = c.Legend.Pos
	}
	titled := c.Title != nil || !c.AutoTitleDeleted && len(c.AllSeries()) == 1
	on := []bool{
		false, edit.AxisShown(c, true), edit.AxisShown(c, false),
		false, edit.AxisTitleShown(c, true), edit.AxisTitleShown(c, false),
		false, !titled, titled,
		false, !labels, labels && lpos == "ctr", labels && lpos == "inEnd", labels && (lpos == "outEnd" || lpos == "t"), labels && lpos == "bestFit",
		false, edit.Gridlines(c, true, true), edit.Gridlines(c, false, true), edit.Gridlines(c, true, false), edit.Gridlines(c, false, false),
		false, legend == "", legend == "r", legend == "t", legend == "l", legend == "b",
	}
	for i := range items {
		if !items[i].Header {
			items[i].Label = "    " + items[i].Label
			if on[i] {
				items[i].Label = "✓  " + items[i].Label[4:]
			}
		}
	}
	return items
}

// addElement runs Add Chart Element item i.
func (a *App) addElement(i int) {
	a.changeChart(func(c *model.Chart) error {
		switch i {
		case 1, 2:
			edit.SetAxisShown(c, i == 1, !edit.AxisShown(c, i == 1))
		case 4, 5:
			edit.SetAxisTitle(c, i == 4, !edit.AxisTitleShown(c, i == 4))
		case 7, 8:
			edit.SetChartTitle(c, i == 8)
		case 10:
			edit.SetDataLabels(c, "", false)
		case 11, 12, 13, 14:
			edit.SetDataLabels(c, []string{"ctr", "inEnd", "outEnd", "bestFit"}[i-11], true)
		case 16, 17, 18, 19:
			h, major := i == 16 || i == 18, i <= 17
			edit.SetGridlines(c, h, major, !edit.Gridlines(c, h, major))
		case 21, 22, 23, 24, 25:
			edit.SetLegend(c, []string{"", "r", "t", "l", "b"}[i-21])
		}
		return nil
	})
}

// paletteItems are Change Colors' choices, each with its first color.
func (a *App) paletteItems() []gui.DropItem {
	items := []gui.DropItem{{Label: "Colorful", Header: true}}
	for i, p := range edit.Palettes {
		if i == 4 {
			items = append(items, gui.DropItem{Label: "Monochromatic", Header: true})
		}
		items = append(items, gui.DropItem{Label: p.Name, Swatch: p.Color(a.wb.Theme, 0, 3)})
	}
	return items
}

// paletteChosen applies Change Colors item i.
func (a *App) paletteChosen(i int) {
	p := i - 1
	if i > 4 {
		p--
	}
	home := a.wb.Sheets[a.active].Name
	a.changeChart(func(c *model.Chart) error {
		edit.ApplyPalette(a.wb, c, home, p)
		return nil
	})
}

// setTips sets the tooltips of the Chart Design ribbon's buttons.
func (r *chartDesignRibbon) setTips() {
	tip := gui.SetTip
	tip(&r.elemDD, "Add Chart\nElement", "", "Add elements to the chart: axes, titles, data labels, gridlines and a legend.")
	tip(&r.colorsDD, "Change\nColors", "", "Choose a different color palette for the chart.")
	tip(&r.styles, "Chart Styles", "", "Change the overall visual style of the chart.")
	tip(&r.switchRC, "Switch Row/Column", "", "Swap the data over the axis: data in rows become columns, and the other way round.")
	tip(&r.data, "Select Data", "", "Change the data range included in the chart.")
	tip(&r.kind, "Change Chart Type", "", "Change to a different type of chart.")
	tip(&r.move, "Move Chart", "", "Move this chart to another sheet or tab in the workbook.")
}

// layoutChartRibbon draws the chart tab showing.
func (a *App) layoutChartRibbon(gtx layout.Context) layout.Dimensions {
	c, _ := a.curChart()
	if c == nil {
		a.tab = tabHome
		return layout.Dimensions{}
	}
	if a.tab == tabChartFormat {
		return a.layoutChartFormat(gtx, c)
	}
	r := &a.ch.design
	if !r.tipsSet {
		r.setTips()
		r.tipsSet = true
	}
	th := a.ui
	k := &gui.RibbonKit{Th: th}
	home := a.wb.Sheets[a.active].Name
	k.Click(gtx, &r.switchRC, func() {
		a.changeChart(func(c *model.Chart) error { return edit.SwitchRowColumn(a.wb, c, home) })
	})
	k.Click(gtx, &r.data, a.openSelectData)
	k.Click(gtx, &r.kind, func() { a.openChartTypeDialog(false) })
	k.Click(gtx, &r.move, a.openMoveChart)
	ok := chartEditable(c)
	big := func(b *widget.Clickable, ic *gui.Icon, label string) layout.Widget {
		return gui.Enabled(ok, k.BigBtn(b, ic, label, false))
	}
	groups := []gui.RibbonGroup{
		{Caption: "Chart Layouts", Content: gui.Enabled(ok, k.BigDrop(&r.elemDD, gui.IconChartElement, "Add Chart\nElement", elementItems(c), 230, a.addElement))},
		{Caption: "Chart Styles", Content: gui.RibbonSide(2,
			gui.Enabled(ok, k.BigDrop(&r.colorsDD, gui.IconChartColors, "Change\nColors", a.paletteItems(), 230, a.paletteChosen)),
			gui.Enabled(ok, k.BigDrop(&r.styles, gui.IconChartStyles, "Chart\nStyles", gui.MenuItems(edit.ChartStyles), 150, func(i int) {
				a.changeChart(func(c *model.Chart) error {
					edit.ApplyChartStyle(c, i)
					return nil
				})
			})),
		)},
		{Caption: "Data", Content: gui.RibbonSide(2, big(&r.switchRC, gui.IconSwitchRowCol, "Switch Row/\nColumn"), big(&r.data, gui.IconSelectData, "Select Data"))},
		{Caption: "Type", Content: k.BigBtn(&r.kind, gui.IconChangeType, "Change\nChart Type", false)},
		{Caption: "Location", Content: k.BigBtn(&r.move, gui.IconMove, "Move Chart", false)},
	}
	dims := r.ribbon.Layout(gtx, th, groups...)
	if k.Did && !a.modalOpen() && !a.ch.title.active {
		a.grid.focusPending = true
	}
	return dims
}

// chartElems lists the parts of chart c that can be selected, for the
// Format tab's Chart Elements box.
func (a *App) chartElems(c *model.Chart) []render.ChartElem {
	out := []render.ChartElem{{Kind: render.ElemChartArea}}
	if c.Title != nil || !c.AutoTitleDeleted && len(c.AllSeries()) == 1 {
		out = append(out, render.ChartElem{Kind: render.ElemTitle})
	}
	if c.Legend != nil {
		out = append(out, render.ChartElem{Kind: render.ElemLegend})
	}
	out = append(out, render.ChartElem{Kind: render.ElemPlotArea})
	for i := range c.Axes {
		if !c.Axes[i].Delete {
			out = append(out, render.ChartElem{Kind: render.ElemAxis, Axis: i})
		}
		if c.Axes[i].MajorGrid {
			out = append(out, render.ChartElem{Kind: render.ElemGridlines, Axis: i})
		}
		if c.Axes[i].Title != nil {
			out = append(out, render.ChartElem{Kind: render.ElemAxisTitle, Axis: i})
		}
	}
	for _, sr := range c.AllSeries() {
		out = append(out, render.ChartElem{Kind: render.ElemSeries, Plot: sr.Plot, Series: sr.Series})
	}
	return out
}

// setTips sets the tooltips of the Format ribbon's buttons.
func (r *chartFormatRibbon) setTips() {
	tip := gui.SetTip
	tip(&r.elemDD, "Chart Elements", "", "Select a part of the chart to format.")
	tip(&r.formatSel, "Format Selection", "", "Set the fill and outline of the selected part of the chart.")
	tip(&r.reset, "Reset to Match Style", "", "Clear the custom formatting of the selected part so it matches the chart's style.")
	tip(&r.fill, "Shape Fill", "", "Fill the selected part with a solid color.")
	tip(&r.fillCP, "Shape Fill", "", "Choose a fill color for the selected part.")
	tip(&r.line, "Shape Outline", "", "Outline the selected part in a color.")
	tip(&r.lineCP, "Shape Outline", "", "Choose an outline color for the selected part.")
}

// layoutChartFormat draws the Format ribbon.
func (a *App) layoutChartFormat(gtx layout.Context, c *model.Chart) layout.Dimensions {
	r := &a.ch.format
	if !r.tipsSet {
		r.setTips()
		r.tipsSet = true
		r.lastFill = gui.ColorChoice{Color: model.DefaultTheme().Colors[model.ThemeAccent1]}
		r.lastL = r.lastFill
	}
	th := a.ui
	k := &gui.RibbonKit{Th: th}
	v := &a.state().view
	home := a.wb.Sheets[a.active].Name
	elems := a.chartElems(c)
	names := make([]string, len(elems))
	cur := render.ChartElem{Kind: render.ElemChartArea}
	if v.SelChart == c && v.SelElem.Kind > render.ElemNone {
		cur = v.SelElem
	}
	for i, e := range elems {
		names[i] = e.Name(c, a.wb, home)
	}
	k.Click(gtx, &r.formatSel, func() { a.openFormatElem() })
	k.Click(gtx, &r.reset, func() { a.colorElem(cur, true, gui.ColorChoice{}, true) })
	k.Click(gtx, &r.fill, func() { a.colorElem(cur, true, r.lastFill, false) })
	k.Click(gtx, &r.line, func() { a.colorElem(cur, false, r.lastL, false) })
	ok := chartEditable(c)
	row, stack := gui.RibbonRow, gui.RibbonStack
	small := func(b *widget.Clickable, ic *gui.Icon, label string) layout.Widget {
		return gui.Enabled(ok, k.Small(b, ic, label))
	}
	paint := func(ch gui.ColorChoice) color.NRGBA {
		if ch.None {
			return color.NRGBA{}
		}
		return ch.Color
	}
	groups := []gui.RibbonGroup{
		{Caption: "Current Selection", Content: stack(
			row(k.Combo(&r.elemDD, 190, cur.Name(c, a.wb, home), dropItems(names), 230, func(i int) {
				if v.SelChart != c {
					_, idx := a.curChart()
					a.selectChart(idx, elems[i])
				}
				v.SelElem = elems[i]
			})),
			row(small(&r.formatSel, gui.IconValueSettings, "Format Selection"), small(&r.reset, gui.IconClearAll, "Reset to Match Style")),
		)},
		{Caption: "Shape Styles", Content: stack(
			row(gui.Enabled(ok, k.ColorSplit(&r.fill, &r.fillCP, gui.IconFill, paint(r.lastFill), "No Fill", func(ch gui.ColorChoice) {
				r.lastFill = ch
				a.colorElem(cur, true, ch, false)
			})), gui.Gap(4), gui.Enabled(ok, a.ui.Label(12, "Shape Fill", chromeText, font.Normal).Layout)),
			row(gui.Enabled(ok, k.ColorSplit(&r.line, &r.lineCP, gui.IconShapeOutline, paint(r.lastL), "No Outline", func(ch gui.ColorChoice) {
				r.lastL = ch
				a.colorElem(cur, false, ch, false)
			})), gui.Gap(4), gui.Enabled(ok, a.ui.Label(12, "Shape Outline", chromeText, font.Normal).Layout)),
		)},
	}
	dims := r.ribbon.Layout(gtx, th, groups...)
	if k.Did && !a.modalOpen() && !a.ch.title.active {
		a.grid.focusPending = true
	}
	return dims
}

// choicePaint is a picked color as a chart's paint.
func choicePaint(ch gui.ColorChoice) model.Paint {
	if ch.None {
		return model.Paint{Kind: model.FillNone}
	}
	return model.Paint{Kind: model.FillSolid, Color: ch.Color}
}

// colorElem sets the fill (or outline) of part e of the current chart;
// reset puts back the style's for both.
func (a *App) colorElem(e render.ChartElem, fill bool, ch gui.ColorChoice, reset bool) {
	a.changeChart(func(c *model.Chart) error {
		p := choicePaint(ch)
		set := func(sp *model.ShapeProps) {
			switch {
			case reset:
				*sp = model.ShapeProps{}
			case fill:
				sp.Fill = p
			default:
				sp.Line = p
			}
		}
		switch e.Kind {
		case render.ElemPlotArea:
			set(&c.PlotArea)
		case render.ElemTitle:
			edit.SetChartTitle(c, true)
			set(&c.Title.Shape)
		case render.ElemLegend:
			if c.Legend != nil {
				set(&c.Legend.Shape)
			}
		case render.ElemSeries, render.ElemLabels:
			if e.Plot >= len(c.Plots) || e.Series >= len(c.Plots[e.Plot].Series) {
				return nil
			}
			pl := &c.Plots[e.Plot]
			s := &pl.Series[e.Series]
			if e.Kind == render.ElemLabels {
				if s.Labels == nil && pl.Labels != nil {
					l := *pl.Labels
					s.Labels = &l
				}
				if s.Labels != nil {
					set(&s.Labels.Shape)
				}
				return nil
			}
			line := pl.Type == model.ChartLine || pl.Type == model.ChartScatter
			switch {
			case reset:
				s.Shape, s.Points, s.Marker.Shape = model.ShapeProps{}, nil, model.ShapeProps{}
			case line:
				// A line's color is its outline; its markers follow.
				s.Shape.Line, s.Shape.Fill = p, p
				s.Marker.Shape.Fill, s.Marker.Shape.Line = p, p
			case fill:
				s.Shape.Fill = p
				s.Points = nil
				if pl.Type == model.ChartPie || pl.Type == model.ChartDoughnut {
					pl.VaryColors = false // one color for the slices
				}
			default:
				s.Shape.Line = p
			}
		case render.ElemAxis, render.ElemAxisTitle, render.ElemGridlines:
			if e.Axis < 0 || e.Axis >= len(c.Axes) {
				return nil
			}
			ax := &c.Axes[e.Axis]
			switch {
			case e.Kind == render.ElemGridlines && reset:
				ax.MajorGridLine = model.ShapeProps{}
			case e.Kind == render.ElemGridlines:
				ax.MajorGridLine.Line = p
			case e.Kind == render.ElemAxisTitle && ax.Title != nil:
				set(&ax.Title.Shape)
			case reset:
				ax.Line = model.ShapeProps{}
			case !fill:
				ax.Line.Line = p
			}
		default:
			set(&c.ChartArea)
		}
		return nil
	})
}

// --- the menus ---

// chartContextMenu opens the menu of a right-clicked chart, which (and
// the part under the pointer) it selects first.
func (a *App) chartContextMenu(v *render.View, c *model.Chart, pos f32.Point) {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	a.finishTitleEdit(true)
	a.grid.focusPending = true
	if c == nil {
		c = v.Sheet.ChartSheet
	}
	if c == nil {
		return
	}
	v.SelChart, v.SelElem = c, v.ChartElementAt(c, pos.X, pos.Y)
	openCtx(&a.cm.grid, &a.cm.gridActs, pos.Round(), a.chartMenu())
}

// chartMenu is the right-click menu of the selected chart.
func (a *App) chartMenu() []ctxItem {
	c, idx := a.selChart()
	if c == nil {
		return nil
	}
	v := &a.state().view
	sheet := idx < 0
	ok := chartEditable(c)
	noClip := a.ch.clip == nil
	items := []ctxItem{
		{"Cut", "Ctrl+X", "Move the chart to the Clipboard.", sheet, func() { a.copyChart(a.gtx, true) }},
		{"Copy", "Ctrl+C", "Copy the chart to the Clipboard.", false, func() { a.copyChart(a.gtx, false) }},
		{"Paste", "Ctrl+V", "Paste the copied chart.", sheet || noClip, func() { a.pasteChart(a.ch.clipText) }},
		ctxSep,
	}
	if v.SelElem.Kind == render.ElemTitle {
		items = append(items, ctxItem{"Edit Text", "F2", "Change the chart title.", !ok, func() { a.startTitleEdit("", false) }})
	}
	items = append(items,
		ctxItem{"Change Chart Type…", "", "Change to a different type of chart.", false, func() { a.openChartTypeDialog(false) }},
		ctxItem{"Select Data…", "", "Change the data range included in the chart.", !ok, a.openSelectData},
		ctxItem{"Move Chart…", "", "Move this chart to another sheet or tab in the workbook.", false, a.openMoveChart},
		ctxSep,
		ctxItem{"Format Chart Area…", "", "Set the fill and border of the chart area.", !ok, func() {
			v.SelElem = render.ChartElem{Kind: render.ElemChartArea}
			a.openFormatElem()
		}},
	)
	del := "Delete"
	if v.SelElem.Kind > render.ElemPlotArea {
		del = "Delete " + v.SelElem.Name(c, a.wb, v.Sheet.Name)
	}
	items = append(items, ctxSep, ctxItem{del, "Delete", "Delete the chart, or the selected part of it.", sheet && v.SelElem.Kind <= render.ElemPlotArea,
		func() { a.deleteSelectedChart() }})
	return items
}

// Insert menu's chart items (after insPivot).
const (
	insChart = insPivot + 2 + iota
	insColumnChart
	insBarChart
	insLineChart
	insPieChart
	insDoughnutChart
	insAreaChart
	insScatterChart
	insComboChart
	insChartSheet
)

// chartInsertItems are the Insert menu's chart entries.
func (a *App) chartInsertItems() []gui.MenuItem {
	off := a.ed.active || a.onChartSheet()
	return []gui.MenuItem{
		{Separator: true},
		{Label: "Chart…", Disabled: off, Tip: "Pick a chart type for the selected data in the Insert Chart dialog."},
		{Label: "Column Chart", Shortcut: "Alt+F1", Disabled: off, Tip: "Insert a clustered column chart of the selected data beside it."},
		{Label: "Bar Chart", Disabled: off, Tip: "Insert a clustered bar chart of the selected data."},
		{Label: "Line Chart", Disabled: off, Tip: "Insert a line chart with markers of the selected data."},
		{Label: "Pie Chart", Disabled: off, Tip: "Insert a pie chart of the selected data."},
		{Label: "Doughnut Chart", Disabled: off, Tip: "Insert a doughnut chart of the selected data."},
		{Label: "Area Chart", Disabled: off, Tip: "Insert a stacked area chart of the selected data."},
		{Label: "Scatter Chart", Disabled: off, Tip: "Insert an X Y (scatter) chart of the selected data."},
		{Label: "Combo Chart", Disabled: off, Tip: "Insert clustered columns with the last series as a line on a secondary axis."},
		{Label: "Chart Sheet", Shortcut: "F11", Disabled: off, Tip: "Put a chart of the selected data on a sheet of its own."},
	}
}

// chartInsertChosen runs an Insert menu chart item; false when item
// isn't one.
func (a *App) chartInsertChosen(item int) bool {
	kinds := map[int]edit.ChartKind{insColumnChart: edit.ColumnClustered, insBarChart: edit.BarClustered, insLineChart: edit.LineMarkers,
		insPieChart: edit.PieChart, insDoughnutChart: edit.DoughnutChart, insAreaChart: edit.AreaStacked,
		insScatterChart: edit.ScatterMarkers, insComboChart: edit.ComboColumnLine}
	switch k, ok := kinds[item]; {
	case ok:
		a.insertChart(k)
	case item == insChart:
		a.openChartTypeDialog(true)
	case item == insChartSheet:
		a.insertChartSheet()
	default:
		return false
	}
	return true
}
