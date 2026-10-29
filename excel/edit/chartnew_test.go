package edit

import (
	"testing"

	"officelite/excel/model"
)

// salesBook has, on Data, months down A2:A5 with North and South in B
// and C (headers in row 1), and a block without labels in E1:G2.
func salesBook(t *testing.T) *Book {
	b := newBook()
	rows := [][]string{
		{"Month", "North", "South"},
		{"Jan", "10", "20"},
		{"Feb", "15", "25"},
		{"Mar", "12", "22"},
		{"Apr", "18", "30"},
	}
	for r, row := range rows {
		for c, v := range row {
			b.enter(t, 1, model.CellName(r, c), v)
		}
	}
	for c, v := range []string{"1", "2", "3"} {
		b.enter(t, 1, model.CellName(0, 4+c), v)
		b.enter(t, 1, model.CellName(1, 4+c), v+"0")
	}
	return b
}

func seriesRefs(c *model.Chart) [][3]string {
	var out [][3]string
	for _, sr := range c.AllSeries() {
		s := c.Plots[sr.Plot].Series[sr.Series]
		out = append(out, [3]string{s.Name.Ref, s.Cat.Ref, s.Val.Ref})
	}
	return out
}

func TestNewChartLayout(t *testing.T) {
	b := salesBook(t)
	// The table's region from one cell: headers row and column, series in
	// columns (5 rows ≥ 3 columns).
	c, src, err := NewChart(b.WB, 1, model.Range{R1: 2, C1: 1, R2: 2, C2: 1}, ColumnClustered)
	if err != nil {
		t.Fatal(err)
	}
	if src.Range != rng("A1", "C5") || !src.ByCols || !src.HeaderRow || !src.HeaderCol {
		t.Fatalf("source %+v", src)
	}
	want := [][3]string{
		{"Data!$B$1", "Data!$A$2:$A$5", "Data!$B$2:$B$5"},
		{"Data!$C$1", "Data!$A$2:$A$5", "Data!$C$2:$C$5"},
	}
	if got := seriesRefs(c); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("series %v", got)
	}
	if c.Title == nil || c.Legend == nil || c.Legend.Pos != "b" || len(c.Axes) != 2 || c.Plots[0].Type != model.ChartBar {
		t.Errorf("chart %+v", c)
	}
	// Wider than tall: series in rows.
	c, src, _ = NewChart(b.WB, 1, rng("A1", "C2"), LinePlain)
	if src.ByCols || !src.HeaderRow || !src.HeaderCol {
		t.Fatalf("source %+v", src)
	}
	if got := seriesRefs(c); len(got) != 1 || got[0] != [3]string{"Data!$A$2", "Data!$B$1:$C$1", "Data!$B$2:$C$2"} {
		t.Errorf("by rows: %v", got)
	}
	// Numbers only: no labels; three columns and two rows, so series in
	// rows named automatically.
	c, src, _ = NewChart(b.WB, 1, rng("E1", "G2"), ColumnClustered)
	if src.HeaderRow || src.HeaderCol || src.ByCols {
		t.Fatalf("numbers: %+v", src)
	}
	if got := seriesRefs(c); len(got) != 2 || got[1] != [3]string{"", "", "Data!$E$2:$G$2"} {
		t.Errorf("numbers: %v", got)
	}
	// A blank top-left cell makes the first row and column labels.
	b.Clear(1, rng("A1", "A1"))
	_, src, _ = NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	if !src.HeaderRow || !src.HeaderCol {
		t.Errorf("blank corner: %+v", src)
	}
	// Scatter: the first column of numbers is X.
	c, _, _ = NewChart(b.WB, 1, rng("B2", "C5"), ScatterMarkers)
	if got := seriesRefs(c); len(got) != 1 || got[0][1] != "Data!$B$2:$B$5" || got[0][2] != "Data!$C$2:$C$5" {
		t.Errorf("scatter: %v", got)
	}
	if _, _, err := NewChart(b.WB, 0, model.Range{R1: 3, C1: 3, R2: 3, C2: 3}, PieChart); err == nil {
		t.Error("a chart from an empty sheet")
	}
}

func TestAddMoveChartUndo(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	c.Anchor = model.Anchor{From: model.CellOffset{Row: 1, Col: 4}, To: model.CellOffset{Row: 15, Col: 11}}
	i, _ := b.AddChart(1, c)
	sh := b.WB.Sheets[1]
	if i != 0 || len(sh.Charts) != 1 || sh.Charts[0].Name != "Chart 1" {
		t.Fatalf("added %d: %d charts", i, len(sh.Charts))
	}
	a := sh.Charts[0].Anchor
	a.From.Row, a.To.Row = 3, 17
	b.MoveChart(1, 0, a)
	if sh.Charts[0].Anchor.From.Row != 3 {
		t.Fatalf("moved: %+v", sh.Charts[0].Anchor)
	}
	b.Undo()
	if sh.Charts[0].Anchor.From.Row != 1 {
		t.Errorf("undo move: %+v", sh.Charts[0].Anchor)
	}
	b.Undo()
	if len(sh.Charts) != 0 {
		t.Errorf("undo add: %d", len(sh.Charts))
	}
	b.Redo()
	b.Redo()
	if len(sh.Charts) != 1 || sh.Charts[0].Anchor.From.Row != 3 {
		t.Errorf("redo: %d", len(sh.Charts))
	}
}

// A moved chart from a file is still written as it came; an edited one
// isn't.
func TestChartEditDropsRaw(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	c.Raw = &model.ChartRaw{XML: []byte("<c:chartSpace/>")}
	b.AddChart(1, c)
	sh := b.WB.Sheets[1]
	a := sh.Charts[0].Anchor
	a.From.Col = 7
	b.MoveChart(1, 0, a)
	if sh.Charts[0].Raw == nil {
		t.Error("moving dropped the part as read")
	}
	e := sh.Charts[0].Clone()
	SetLegend(e, "r")
	b.SetChart(1, 0, e)
	if sh.Charts[0].Raw != nil || sh.Charts[0].Legend.Pos != "r" {
		t.Error("an edited chart kept its part as read")
	}
	b.Undo()
	if sh.Charts[0].Raw == nil || sh.Charts[0].Legend.Pos != "b" {
		t.Error("undo didn't bring the chart back")
	}
}

func TestSwitchRowColumn(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	if err := SwitchRowColumn(b.WB, c, "Data"); err != nil {
		t.Fatal(err)
	}
	got := seriesRefs(c)
	if len(got) != 4 || got[0] != [3]string{"Data!$A$2", "Data!$B$1:$C$1", "Data!$B$2:$C$2"} {
		t.Fatalf("switched: %v", got)
	}
	if c.Plots[0].Type != model.ChartBar || len(c.Plots) != 1 {
		t.Errorf("kind lost: %+v", c.Plots)
	}
	SwitchRowColumn(b.WB, c, "Data")
	if got := seriesRefs(c); len(got) != 2 || got[1][2] != "Data!$C$2:$C$5" {
		t.Errorf("back: %v", got)
	}
	// Series picked one by one can't be switched.
	c.Plots[0].Series[1].Val.Ref = "Data!$F$1:$F$2"
	if err := SwitchRowColumn(b.WB, c, "Data"); err == nil {
		t.Error("switched scattered series")
	}
}

func TestChartKinds(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	for _, k := range ChartKinds {
		SetChartKind(b.WB, c, "Data", k.Kind)
		if got := KindOf(c); got != k.Kind {
			t.Errorf("%s: kind %s", k.Name, got)
		}
		if n := len(c.AllSeries()); n != 2 {
			t.Errorf("%s: %d series", k.Name, n)
		}
		pie := k.Kind == PieChart || k.Kind == DoughnutChart
		if pie != (len(c.Axes) == 0) {
			t.Errorf("%s: %d axes", k.Name, len(c.Axes))
		}
	}
	// Combo: columns and a line on the secondary axis.
	SetChartKind(b.WB, c, "Data", ColumnClustered)
	SetAxisTitle(c, false, true)
	SetChartKind(b.WB, c, "Data", ComboColumnLine)
	kinds, sec := SeriesKinds(c)
	if len(c.Plots) != 2 || kinds[0] != ColumnClustered || kinds[1] != LinePlain || sec[0] || !sec[1] || len(c.Axes) != 4 {
		t.Fatalf("combo: %v %v, %d plots, %d axes", kinds, sec, len(c.Plots), len(c.Axes))
	}
	// Per series: both lines, the second on the primary axis.
	SetComboKinds(b.WB, c, "Data", []ChartKind{LineMarkers, AreaStandard}, []bool{true, false})
	kinds, sec = SeriesKinds(c)
	if kinds[0] != AreaStandard || kinds[1] != LineMarkers || sec[0] || !sec[1] {
		t.Errorf("per series: %v %v", kinds, sec)
	}
	// The vertical axis' title survived the changes of kind.
	SetChartKind(b.WB, c, "Data", LinePlain)
	if !AxisTitleShown(c, false) {
		t.Error("axis title lost")
	}
}

func TestChartElements(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	SetChartTitle(c, false)
	if c.Title != nil || !c.AutoTitleDeleted {
		t.Error("title off")
	}
	SetChartTitleText(c, "Sales")
	if c.Title == nil || c.Title.Text != "Sales" || c.AutoTitleDeleted {
		t.Error("title text")
	}
	SetLegend(c, "")
	if c.Legend != nil {
		t.Error("legend off")
	}
	SetLegend(c, "t")
	if c.Legend.Pos != "t" {
		t.Error("legend top")
	}
	SetDataLabels(c, "outEnd", true)
	if pos, ok := LabelsPos(c); !ok || pos != "outEnd" {
		t.Errorf("labels %q %v", pos, ok)
	}
	SetChartKind(b.WB, c, "Data", ColumnStacked)
	if pos, _ := LabelsPos(c); pos != "outEnd" {
		t.Errorf("labels kept: %q", pos) // written as Excel allows (inEnd) in the file
	}
	SetDataLabels(c, "", false)
	if _, ok := LabelsPos(c); ok {
		t.Error("labels off")
	}
	SetAxisShown(c, true, false)
	if AxisShown(c, true) || !AxisShown(c, false) {
		t.Error("horizontal axis off")
	}
	SetGridlines(c, true, true, false)
	SetGridlines(c, false, false, true)
	if Gridlines(c, true, true) || !Gridlines(c, false, false) {
		t.Error("gridlines")
	}
	SetAxisTitle(c, true, true)
	if !AxisTitleShown(c, true) || c.Axes[0].Title.Text != "Axis Title" {
		t.Error("axis title")
	}
	ApplyPalette(b.WB, c, "Data", 4)
	f := c.Plots[0].Series[0].Shape.Fill
	if f.Kind != model.FillSolid {
		t.Error("palette")
	}
	ApplyPalette(b.WB, c, "Data", 0)
	if c.Plots[0].Series[0].Shape.Fill.Kind != model.FillAuto {
		t.Error("palette 1 is automatic")
	}
	for i := range ChartStyles {
		ApplyChartStyle(c, i)
	}
	ApplyChartStyle(c, 0)
	if c.ChartArea.Fill.Kind != model.FillAuto || !Gridlines(c, true, true) {
		t.Error("style 1 isn't the default look")
	}
	// A pie's slices take the palette.
	SetChartKind(b.WB, c, "Data", PieChart)
	ApplyPalette(b.WB, c, "Data", 1)
	if n := len(c.Plots[0].Series[0].Points); n != 4 {
		t.Errorf("pie slices coloured: %d", n)
	}
}

func TestChartSheets(t *testing.T) {
	b := salesBook(t)
	c, _, _ := NewChart(b.WB, 1, rng("A1", "C5"), ColumnClustered)
	b.AddChart(1, c)
	// Move Chart > New sheet: one undo step.
	at, err := b.ChartToSheet(1, 0, "Chart1")
	if err != nil {
		t.Fatal(err)
	}
	if at != 2 || !b.WB.Sheets[2].IsChartSheet() || len(b.WB.Sheets[1].Charts) != 0 {
		t.Fatalf("to sheet: at %d, %d sheets", at, len(b.WB.Sheets))
	}
	b.Undo()
	if len(b.WB.Sheets) != 2 || len(b.WB.Sheets[1].Charts) != 1 {
		t.Fatalf("undo: %d sheets, %d charts", len(b.WB.Sheets), len(b.WB.Sheets[1].Charts))
	}
	b.Redo()
	// Edit the chart sheet's chart, undoably.
	e := b.WB.Sheets[2].ChartSheet.Clone()
	SetChartTitleText(e, "On its own")
	b.SetChart(2, -1, e)
	if b.WB.Sheets[2].ChartSheet.Title.Text != "On its own" {
		t.Fatal("chart sheet edit")
	}
	b.Undo()
	if b.WB.Sheets[2].ChartSheet.Title.Text != "" {
		t.Error("chart sheet undo")
	}
	// And back onto Sheet1 as an object; the chart sheet goes.
	a := model.Anchor{From: model.CellOffset{Row: 2, Col: 2}, To: model.CellOffset{Row: 12, Col: 9}}
	to, i, err := b.ChartToObject(2, -1, 0, a)
	if err != nil || to != 0 || i != 0 || len(b.WB.Sheets) != 2 || len(b.WB.Sheets[0].Charts) != 1 {
		t.Fatalf("to object: %v %d %d, %d sheets", err, to, i, len(b.WB.Sheets))
	}
	b.Undo()
	if len(b.WB.Sheets) != 3 || len(b.WB.Sheets[0].Charts) != 0 || !b.WB.Sheets[2].IsChartSheet() {
		t.Errorf("undo to object: %d sheets", len(b.WB.Sheets))
	}
	// Deleting a chart sheet is undoable.
	if _, err := b.DeleteSheet(2); err != nil {
		t.Fatal(err)
	}
	b.Undo()
	if len(b.WB.Sheets) != 3 || !b.WB.Sheets[2].IsChartSheet() {
		t.Error("chart sheet not back")
	}
	if NextChartSheetName(b.WB) != "Chart2" {
		t.Errorf("next sheet name %s", NextChartSheetName(b.WB))
	}
}

func TestChartDataSourceSelectData(t *testing.T) {
	b := salesBook(t)
	c, src, _ := NewChart(b.WB, 1, rng("A1", "C5"), LineMarkers)
	got, ok := ChartDataSource(b.WB, c, "Data")
	if !ok || got != src || got.SourceRef() != "Data!$A$1:$C$5" {
		t.Fatalf("source %+v %v", got, ok)
	}
	// Select Data: a narrower range, then a series edited by hand.
	SetChartSource(b.WB, c, "Data", DetectSource(b.WB, b.WB.Sheets[1], rng("A1", "B5")))
	if n := len(c.AllSeries()); n != 1 || KindOf(c) != LineMarkers {
		t.Fatalf("narrower: %d series, %s", n, KindOf(c))
	}
	ser := []model.ChartSeries{c.Plots[0].Series[0], {Name: model.ChartData{Literal: "Extra"}, Val: model.ChartData{Ref: "Data!$C$2:$C$5"}}}
	SetChartSeries(b.WB, c, "Data", ser)
	if n := len(c.AllSeries()); n != 2 || c.Plots[0].Series[1].Idx != 1 {
		t.Fatalf("added: %d", n)
	}
	if _, ok := ChartDataSource(b.WB, c, "Data"); ok {
		t.Error("a hand-made series still gives one range")
	}
}
