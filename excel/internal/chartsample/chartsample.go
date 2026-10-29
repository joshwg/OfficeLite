// Package chartsample builds a workbook with a chart of each kind
// ExcelLite draws (Samples/Charts.xlsx, and the chart tests' data).
package chartsample

import (
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Workbook builds the sample workbook.
func Workbook() *model.Workbook {
	wb := &model.Workbook{Name: "Charts.xlsx", Styles: []model.Style{model.DefaultStyle()}}
	pct := model.DefaultStyle()
	pct.NumFmtID = 9
	money := model.DefaultStyle()
	money.NumFmtCode = `"$"#,##0`
	bold := model.DefaultStyle()
	bold.Font.Bold = true
	wb.Styles = append(wb.Styles, pct, money, bold)
	const sPct, sMoney, sBold = 1, 2, 3

	sh := model.NewSheet("Data")
	sh.ColWidth[0] = 11
	text := func(r, c int, s string, st uint32) {
		sh.Set(r, c, model.Cell{Kind: model.KindText, Text: s, Style: st})
	}
	numStyled := func(r, c int, v float64, st uint32) {
		t := numfmt.General(v)
		switch st {
		case sPct:
			t = numfmt.General(v*100) + "%"
		case sMoney:
			t = "$" + numfmt.General(v)
		}
		sh.Set(r, c, model.Cell{Kind: model.KindNumber, Num: v, Text: t, Style: st})
	}
	// A1:F7: months by region, a margin and a net figure with losses.
	for c, h := range []string{"Month", "North", "South", "West", "Margin", "Net"} {
		text(0, c, h, sBold)
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"}
	north := []float64{120, 135, 160, 150, 182, 205}
	south := []float64{90, 110, 95, 130, 125, 150}
	west := []float64{60, 72, 88, 80, 97, 110}
	margin := []float64{0.12, 0.15, 0.11, 0.18, 0.21, 0.19}
	net := []float64{25, -14, 8, -30, 42, 18}
	for i, m := range months {
		text(i+1, 0, m, 0)
		numStyled(i+1, 1, north[i], sMoney)
		numStyled(i+1, 2, south[i], sMoney)
		numStyled(i+1, 3, west[i], sMoney)
		numStyled(i+1, 4, margin[i], sPct)
		numStyled(i+1, 5, net[i], 0)
	}
	// A10:B14: market share; A17:C25: measurements (X, Y) and a gappy
	// series.
	text(9, 0, "Product", sBold)
	text(9, 1, "Share", sBold)
	for i, p := range []string{"Laptops", "Phones", "Tablets", "Monitors", "Other"} {
		text(10+i, 0, p, 0)
		numStyled(10+i, 1, []float64{38, 27, 15, 12, 8}[i], 0)
	}
	text(16, 0, "Hours", sBold)
	text(16, 1, "Score", sBold)
	text(16, 2, "Trend", sBold)
	xs := []float64{1, 2, 3.5, 4, 5.5, 6, 7.5, 8}
	ys := []float64{52, 58, 61, 70, 74, 79, 85, 91}
	for i := range xs {
		numStyled(17+i, 0, xs[i], 0)
		numStyled(17+i, 1, ys[i], 0)
		if i != 3 { // a blank: a gap in the line
			numStyled(17+i, 2, ys[i]-10+float64(i%3)*4, 0)
		}
	}
	wb.Sheets = append(wb.Sheets, sh)

	ref := func(col string, r1, r2 int) string {
		return "Data!$" + col + "$" + itoa(r1) + ":$" + col + "$" + itoa(r2)
	}
	name := func(col string) model.ChartData { return model.ChartData{Ref: "Data!$" + col + "$1", Text: true} }
	monthCat := model.ChartData{Ref: ref("A", 2, 7), Text: true}
	ser := func(i int, col string) model.ChartSeries {
		return model.ChartSeries{Idx: i, Order: i, Name: name(col), Cat: monthCat, Val: model.ChartData{Ref: ref(col, 2, 7)}}
	}
	regions := func() []model.ChartSeries { return []model.ChartSeries{ser(0, "B"), ser(1, "C"), ser(2, "D")} }
	axes := func(barH bool) []model.ChartAxis {
		cat := model.ChartAxis{ID: 1001, Kind: model.AxisCat, Pos: "b", CrossAx: 1002, Crosses: "autoZero", TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none"}
		val := model.ChartAxis{ID: 1002, Kind: model.AxisVal, Pos: "l", CrossAx: 1001, Crosses: "autoZero", CrossBetween: "between",
			MajorGrid: true, TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none", SourceLinked: true}
		if barH {
			cat.Pos, val.Pos = "l", "b"
		}
		return []model.ChartAxis{cat, val}
	}
	title := func(s string) *model.ChartTitle { return &model.ChartTitle{Text: s} }
	legend := func(pos string) *model.ChartLegend { return &model.ChartLegend{Pos: pos} }
	// Charts are placed in a grid right of the data: 8 columns × 16 rows
	// each.
	n := 0
	place := func(c *model.Chart) {
		col, row := 7+(n%2)*8, (n/2)*16
		c.Anchor = model.Anchor{Kind: model.AnchorTwoCell,
			From: model.CellOffset{Row: row, Col: col}, To: model.CellOffset{Row: row + 15, Col: col + 7}}
		n++
		c.Name = "Chart " + itoa(n)
		c.Style, c.PlotVisOnly = 2, true
		sh.Charts = append(sh.Charts, c)
	}
	bar := func(g model.Grouping, horizontal bool, ss []model.ChartSeries) model.ChartPlot {
		p := model.ChartPlot{Type: model.ChartBar, Grouping: g, Horizontal: horizontal, GapWidth: 219, Overlap: -27, Series: ss, AxisIDs: []int{1001, 1002}}
		if g != model.GroupClustered {
			p.GapWidth, p.Overlap = 150, 100
		}
		return p
	}

	place(&model.Chart{Title: title("Clustered Column"), Legend: legend("b"),
		Plots: []model.ChartPlot{bar(model.GroupClustered, false, regions())}, Axes: axes(false)})
	place(&model.Chart{Title: title("Stacked Column"), Legend: legend("b"),
		Plots: []model.ChartPlot{bar(model.GroupStacked, false, regions())}, Axes: axes(false)})
	pc := &model.Chart{Title: title("100% Stacked Column"), Legend: legend("b"),
		Plots: []model.ChartPlot{bar(model.GroupPercent, false, regions())}, Axes: axes(false)}
	pc.Axes[1].SourceLinked = false
	pc.Axes[1].NumFmt = "0%"
	place(pc)
	place(&model.Chart{Title: title("Clustered Bar"), Legend: legend("r"),
		Plots: []model.ChartPlot{bar(model.GroupClustered, true, regions())}, Axes: axes(true)})

	line := regions()
	for i := range line {
		line[i].Marker = model.Marker{Symbol: "circle", Size: 5}
	}
	place(&model.Chart{Title: title("Line with Markers"), Legend: legend("b"),
		Plots: []model.ChartPlot{{Type: model.ChartLine, Series: line, Marker: true, AxisIDs: []int{1001, 1002}}}, Axes: axes(false)})

	labels := &model.DataLabels{ShowCat: true, ShowPercent: true, Pos: "bestFit", ShowLeader: true}
	place(&model.Chart{Title: title("Market Share"), Legend: legend("r"),
		Plots: []model.ChartPlot{{Type: model.ChartPie, VaryColors: true, Labels: labels, Series: []model.ChartSeries{{
			Name: model.ChartData{Ref: "Data!$B$10", Text: true},
			Cat:  model.ChartData{Ref: ref("A", 11, 15), Text: true}, Val: model.ChartData{Ref: ref("B", 11, 15)}}}}}})
	place(&model.Chart{Title: title("Doughnut"), Legend: legend("b"),
		Plots: []model.ChartPlot{{Type: model.ChartDoughnut, VaryColors: true, HoleSize: 50, Series: []model.ChartSeries{{
			Name: model.ChartData{Ref: "Data!$B$10", Text: true},
			Cat:  model.ChartData{Ref: ref("A", 11, 15), Text: true}, Val: model.ChartData{Ref: ref("B", 11, 15)}}}}}})

	area := model.ChartPlot{Type: model.ChartArea, Grouping: model.GroupStacked, Series: regions(), AxisIDs: []int{1001, 1002}}
	ac := &model.Chart{Title: title("Stacked Area"), Legend: legend("b"), Plots: []model.ChartPlot{area}, Axes: axes(false)}
	ac.Axes[1].CrossBetween = "midCat"
	place(ac)

	scatterAxes := func() []model.ChartAxis {
		x := model.ChartAxis{ID: 2001, Kind: model.AxisVal, Pos: "b", CrossAx: 2002, Crosses: "autoZero", CrossBetween: "midCat",
			MajorGrid: true, TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none", SourceLinked: true,
			Title: &model.ChartTitle{Text: "Hours"}}
		y := model.ChartAxis{ID: 2002, Kind: model.AxisVal, Pos: "l", CrossAx: 2001, Crosses: "autoZero", CrossBetween: "midCat",
			MajorGrid: true, TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none", SourceLinked: true,
			Title: &model.ChartTitle{Text: "Score", Font: model.TextProps{Rotation: -90, HasRot: true}}}
		return []model.ChartAxis{x, y}
	}
	noLine := model.ShapeProps{Line: model.Paint{Kind: model.FillNone}}
	place(&model.Chart{Title: title("Scatter (Markers)"),
		Plots: []model.ChartPlot{{Type: model.ChartScatter, ScatterStyle: "lineMarker", AxisIDs: []int{2001, 2002}, Series: []model.ChartSeries{{
			Name: model.ChartData{Ref: "Data!$B$17", Text: true}, Shape: noLine, Marker: model.Marker{Symbol: "circle", Size: 7},
			Cat: model.ChartData{Ref: ref("A", 18, 25)}, Val: model.ChartData{Ref: ref("B", 18, 25)}}}}},
		Axes: scatterAxes()})
	place(&model.Chart{Title: title("Scatter (Smooth Lines, a Gap)"), Legend: legend("b"),
		Plots: []model.ChartPlot{{Type: model.ChartScatter, ScatterStyle: "smoothMarker", AxisIDs: []int{2001, 2002}, Series: []model.ChartSeries{
			{Idx: 0, Order: 0, Name: model.ChartData{Ref: "Data!$B$17", Text: true}, Smooth: true, Marker: model.Marker{Symbol: "none"},
				Cat: model.ChartData{Ref: ref("A", 18, 25)}, Val: model.ChartData{Ref: ref("B", 18, 25)}},
			{Idx: 1, Order: 1, Name: model.ChartData{Ref: "Data!$C$17", Text: true}, Smooth: true, Marker: model.Marker{Symbol: "none"},
				Cat: model.ChartData{Ref: ref("A", 18, 25)}, Val: model.ChartData{Ref: ref("C", 18, 25)}}}}},
		Axes: scatterAxes()})

	// Combo: columns on the primary axes, the margin as a line on a
	// secondary value axis at the right.
	combo := &model.Chart{Title: title("Combo: Sales and Margin"), Legend: legend("b"),
		Plots: []model.ChartPlot{
			bar(model.GroupClustered, false, []model.ChartSeries{ser(0, "B"), ser(1, "C")}),
			{Type: model.ChartLine, Marker: true, AxisIDs: []int{1003, 1004}, Series: []model.ChartSeries{func() model.ChartSeries {
				s := ser(2, "E")
				s.Marker = model.Marker{Symbol: "circle", Size: 5}
				return s
			}()}},
		},
		Axes: append(axes(false),
			model.ChartAxis{ID: 1003, Kind: model.AxisCat, Pos: "b", Delete: true, CrossAx: 1004, Crosses: "autoZero", TickLblPos: "nextTo", MajorTick: "none", MinorTick: "none"},
			model.ChartAxis{ID: 1004, Kind: model.AxisVal, Pos: "r", CrossAx: 1003, Crosses: "max", CrossBetween: "between", TickLblPos: "nextTo",
				MajorTick: "none", MinorTick: "none", NumFmt: "0%", SourceLinked: false}),
	}
	place(combo)

	negs := model.ChartSeries{Name: name("F"), Cat: monthCat, Val: model.ChartData{Ref: ref("F", 2, 7)},
		Labels: &model.DataLabels{ShowVal: true, Pos: "outEnd"}}
	place(&model.Chart{Title: title("Net (Negative Values, Labels)"),
		Plots: []model.ChartPlot{bar(model.GroupClustered, false, []model.ChartSeries{negs})}, Axes: axes(false)})

	// A chart sheet.
	cs := model.NewSheet("Chart Sheet")
	cs.ChartSheet = &model.Chart{Name: "Chart 1", Title: title("Sales by Region (Chart Sheet)"), Legend: legend("r"), Style: 2, PlotVisOnly: true,
		Plots: []model.ChartPlot{bar(model.GroupClustered, false, regions())}, Axes: axes(false)}
	wb.Sheets = append(wb.Sheets, cs)
	return wb
}

func itoa(n int) string { return numfmt.General(float64(n)) }
