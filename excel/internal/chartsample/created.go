package chartsample

import (
	"officelite/excel/edit"
	"officelite/excel/model"
)

// Created builds Samples/Charts-Created.xlsx: charts made the way the
// Insert menu and the Chart Design tab make them (through edit), each
// kind, with titles, legends, labels, colours and styles changed, a
// switched row/column, a combo with per-series kinds and a chart moved
// to a chart sheet, for checking in Excel that they open without repair
// and look as ExcelLite draws them.
func Created() *model.Workbook {
	src := Workbook()
	wb := &model.Workbook{Name: "Charts-Created.xlsx", Styles: src.Styles, Sheets: []*model.Sheet{src.Sheets[0]}}
	wb.Sheets[0].Charts = nil
	b := edit.New(wb)
	defer b.Close()
	n := 0
	// Each chart 8 columns × 15 rows, two to a row, right of the data.
	add := func(r model.Range, kind edit.ChartKind, change func(c *model.Chart)) {
		c, _, err := edit.NewChart(wb, 0, r, kind)
		if err != nil {
			panic(err)
		}
		col, row := 7+(n%2)*8, (n/2)*16
		c.Anchor = model.Anchor{Kind: model.AnchorTwoCell,
			From: model.CellOffset{Row: row, Col: col}, To: model.CellOffset{Row: row + 15, Col: col + 7}}
		if change != nil {
			change(c)
		}
		b.AddChart(0, c)
		n++
	}
	title := func(s string) func(c *model.Chart) { return func(c *model.Chart) { edit.SetChartTitleText(c, s) } }
	sales := model.Range{R1: 0, C1: 0, R2: 6, C2: 3} // A1:D7, months by region
	share := model.Range{R1: 9, C1: 0, R2: 14, C2: 1}
	xy := model.Range{R1: 16, C1: 0, R2: 24, C2: 1}
	both := func(fs ...func(c *model.Chart)) func(c *model.Chart) {
		return func(c *model.Chart) {
			for _, f := range fs {
				f(c)
			}
		}
	}

	add(sales, edit.ColumnClustered, nil) // "Chart Title", legend at the bottom
	add(sales, edit.ColumnStacked, both(title("Stacked Column, Legend Right"), func(c *model.Chart) { edit.SetLegend(c, "r") }))
	add(sales, edit.ColumnPercent, both(title("100% Stacked, Labels Centre"), func(c *model.Chart) { edit.SetDataLabels(c, "ctr", true) }))
	add(sales, edit.BarClustered, both(title("Clustered Bar, Axis Titles"), func(c *model.Chart) {
		edit.SetAxisTitle(c, true, true)
		edit.SetAxisTitle(c, false, true)
	}))
	add(sales, edit.LineMarkers, both(title("Line with Markers, Minor Gridlines"), func(c *model.Chart) {
		edit.SetGridlines(c, true, false, true)
		edit.SetLegend(c, "t")
	}))
	add(sales, edit.LinePlain, both(title("Line, Switched Row/Column"), func(c *model.Chart) {
		if err := edit.SwitchRowColumn(wb, c, "Data"); err != nil {
			panic(err)
		}
	}))
	add(share, edit.PieChart, both(func(c *model.Chart) { edit.SetDataLabels(c, "bestFit", true) }, func(c *model.Chart) {
		edit.ApplyPalette(wb, c, "Data", 3)
	})) // the series name as the title
	add(share, edit.DoughnutChart, both(title("Doughnut, No Legend"), func(c *model.Chart) { edit.SetLegend(c, "") }))
	add(sales, edit.AreaStacked, both(title("Stacked Area, Monochromatic"), func(c *model.Chart) { edit.ApplyPalette(wb, c, "Data", 4) }))
	add(xy, edit.ScatterMarkers, both(title("Scatter, No Gridlines"), func(c *model.Chart) {
		edit.SetGridlines(c, true, true, false)
		edit.SetAxisTitle(c, true, true)
	}))
	add(sales, edit.ComboColumnLine, title("Combo: Column and Line on Secondary Axis"))
	add(sales, edit.ColumnClustered, both(title("Combo by Series: Area, Column, Line"), func(c *model.Chart) {
		edit.SetComboKinds(wb, c, "Data", []edit.ChartKind{edit.AreaStandard, edit.ColumnClustered, edit.LineMarkers}, []bool{false, false, true})
	}))
	add(sales, edit.ColumnClustered, both(title("Chart Style 4 (Dark)"), func(c *model.Chart) { edit.ApplyChartStyle(c, 3) }))
	add(sales, edit.BarStacked, both(func(c *model.Chart) { edit.SetChartTitle(c, false) }, func(c *model.Chart) { edit.ApplyChartStyle(c, 4) }))

	// A chart moved to a chart sheet of its own.
	add(sales, edit.ColumnClustered, title("Moved to a Chart Sheet"))
	if _, err := b.ChartToSheet(0, len(wb.Sheets[0].Charts)-1, "Sales Chart"); err != nil {
		panic(err)
	}
	return wb
}
