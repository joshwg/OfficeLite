// Tests for Compute: layouts, subtotals, Show Values As, sorting, filters and empty reports.

package pivot

import (
	"strconv"
	"strings"
	"testing"

	"officelite/excel/model"
)

// sales is a small source table in A1:D9.
var sales = [][]string{
	{"Region", "Product", "Sales", "Qty"},
	{"East", "Pen", "10", "1"},
	{"West", "Pen", "20", "2"},
	{"East", "Ink", "30", "3"},
	{"East", "Pen", "5", "1"},
	{"West", "Pad", "7", "4"},
	{"north", "Ink", "8", "2"},
	{"", "Pad", "2", ""},
	{"West", "ink", "1", "1"},
}

// sheetOf builds a sheet from rows of text, storing numeric strings as numbers
// and skipping empty ones.
func sheetOf(rows [][]string) *model.Sheet {
	sh := model.NewSheet("Data")
	for r, row := range rows {
		for c, v := range row {
			if v == "" {
				continue
			}
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				sh.Set(r, c, model.Cell{Kind: model.KindNumber, Num: n, Text: v})
			} else {
				sh.Set(r, c, model.Cell{Kind: model.KindText, Text: v})
			}
		}
	}
	return sh
}

// source reads rows as pivot source data, failing the test on error.
func source(t *testing.T, rows [][]string) *Source {
	t.Helper()
	sh := sheetOf(rows)
	src, err := ReadSource(sh, 0, model.Range{R2: len(rows) - 1, C2: len(rows[0]) - 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// newPT returns an empty report at row 2 with one field per source column.
func newPT(src *Source) *model.PivotTable {
	pt := &model.PivotTable{Name: "PivotTable1", Row: 2}
	for _, f := range src.Fields {
		pt.Fields = append(pt.Fields, model.PivotField{Name: f.Name})
	}
	return pt
}

// sum returns a Sum value field over source field f.
func sum(src *Source, f int) model.PivotValue {
	return model.PivotValue{Field: f, Func: model.FuncSum, Name: ValueName(model.FuncSum, src.Fields[f].Name)}
}

// grid renders a result's area as lines of tab-separated cell texts,
// indents as leading spaces and trailing tabs trimmed.
func grid(res *Result) string {
	a := res.Out.Area
	rows := make([][]string, a.R2-a.R1+1)
	for i := range rows {
		rows[i] = make([]string, a.C2-a.C1+1)
	}
	for _, c := range res.Cells {
		s := c.Text
		if c.Indent > 0 {
			s = strings.Repeat(" ", c.Indent) + s
		}
		rows[c.Row-a.R1][c.Col-a.C1] = s
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.TrimRight(strings.Join(r, "\t"), "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

// check fails the test when res's grid differs from want.
func check(t *testing.T, name string, res *Result, want string) {
	t.Helper()
	if got := grid(res); got != want {
		t.Errorf("%s:\n%s\nwant:\n%s", name, strings.ReplaceAll(got, "\t", " | "), strings.ReplaceAll(want, "\t", " | "))
	}
}

// TestCompactRowsAndColumns checks a compact report with a row and a column field.
func TestCompactRowsAndColumns(t *testing.T) {
	src := source(t, sales)
	pt := newPT(src)
	pt.Rows = []int{0}
	pt.Cols = []int{1}
	pt.Values = []model.PivotValue{sum(src, 2)}
	res := Compute(pt, src, nil)
	check(t, "rows+cols", res, ""+
		"Sum of Sales\tColumn Labels\n"+
		"Row Labels\tInk\tPad\tPen\tGrand Total\n"+
		"East\t30\t\t15\t45\n"+
		"north\t8\t\t\t8\n"+
		"West\t1\t7\t20\t28\n"+
		"(blank)\t\t2\t\t2\n"+
		"Grand Total\t39\t9\t35\t83\n")
	o := res.Out
	if o.Body != (model.Range{R1: 2, C1: 0, R2: 8, C2: 4}) || o.FirstHeaderRow != 1 || o.FirstDataRow != 2 || o.FirstDataCol != 1 {
		t.Errorf("out %+v", o)
	}
	if len(o.Buttons) != 2 || o.Buttons[0].Row != 2 || o.Buttons[0].Col != 1 || o.Buttons[1].Row != 3 || o.Buttons[1].Col != 0 {
		t.Errorf("buttons %+v", o.Buttons)
	}
	// "ink" and "Ink" are one item, labeled as first seen.
	if n := len(src.Fields[1].Items); n != 3 {
		t.Errorf("%d products", n)
	}
}

// TestTwoValuesAndSubtotals checks subtotals across compact, tabular and outline layouts.
func TestTwoValuesAndSubtotals(t *testing.T) {
	src := source(t, sales)
	pt := newPT(src)
	pt.Rows = []int{0, 1}
	pt.Cols = []int{model.DataField}
	pt.Values = []model.PivotValue{sum(src, 2), {Field: 3, Func: model.FuncCount, Name: "Count of Qty"}}
	pt.Fields[0].Hidden = []string{"", "s:north"}
	check(t, "compact, subtotals at top", Compute(pt, src, nil), ""+
		"Row Labels\tSum of Sales\tCount of Qty\n"+
		"East\t45\t3\n"+
		" Ink\t30\t1\n"+
		" Pen\t15\t2\n"+
		"West\t28\t3\n"+
		" Ink\t1\t1\n"+
		" Pad\t7\t1\n"+
		" Pen\t20\t1\n"+
		"Grand Total\t73\t6\n")

	pt.Layout = model.LayoutTabular
	pt.Values = pt.Values[:1]
	pt.Cols = nil
	check(t, "tabular", Compute(pt, src, nil), ""+
		"Region\tProduct\tSum of Sales\n"+
		"East\tInk\t30\n"+
		"\tPen\t15\n"+
		"East Total\t\t45\n"+
		"West\tInk\t1\n"+
		"\tPad\t7\n"+
		"\tPen\t20\n"+
		"West Total\t\t28\n"+
		"Grand Total\t\t73\n")

	pt.RepeatLabels = true
	pt.Fields[0].NoSubtotal = true
	pt.NoColGrand = true
	check(t, "tabular, repeated labels, no subtotals or grand total", Compute(pt, src, nil), ""+
		"Region\tProduct\tSum of Sales\n"+
		"East\tInk\t30\n"+
		"East\tPen\t15\n"+
		"West\tInk\t1\n"+
		"West\tPad\t7\n"+
		"West\tPen\t20\n")

	pt.Layout = model.LayoutOutline
	pt.RepeatLabels = false
	pt.Fields[0].NoSubtotal = false
	pt.SubtotalsBottom = true
	check(t, "outline, subtotals at the bottom", Compute(pt, src, nil), ""+
		"Region\tProduct\tSum of Sales\n"+
		"East\n"+
		"\tInk\t30\n"+
		"\tPen\t15\n"+
		"East Total\t\t45\n"+
		"West\n"+
		"\tInk\t1\n"+
		"\tPad\t7\n"+
		"\tPen\t20\n"+
		"West Total\t\t28\n")
}

// TestShowValuesAsAndSort checks % of total and % of column with item sorting.
func TestShowValuesAsAndSort(t *testing.T) {
	src := source(t, sales)
	pt := newPT(src)
	pt.Rows = []int{1}
	v := sum(src, 2)
	v.ShowAs = model.ShowPercentOfTotal
	pt.Values = []model.PivotValue{v}
	pt.Fields[1].Sort = model.SortDescending
	res := Compute(pt, src, nil)
	check(t, "% of total, descending", res, ""+
		"Row Labels\tSum of Sales\n"+
		"Pen\t0.421686747\n"+
		"Pad\t0.108433735\n"+
		"Ink\t0.469879518\n"+
		"Grand Total\t1\n")
	for _, c := range res.Cells {
		if c.Kind == model.KindNumber && c.NumFmtID != 10 {
			t.Errorf("%v: format %d", c, c.NumFmtID)
		}
	}

	// Columns: % of column total.
	pt.Cols = []int{0}
	pt.Fields[1].Sort = model.SortAscending
	pt.Fields[0].Hidden = []string{"", "s:north"}
	pt.Values[0].ShowAs = model.ShowPercentOfCol
	res = Compute(pt, src, nil)
	if got := grid(res); !strings.Contains(got, "Ink\t0.666666667\t0.035714286\t0.424657534") {
		t.Errorf("%% of column:\n%s", got)
	}
}

// TestFiltersAndFunctions checks report filters and the Average and StdDev functions.
func TestFiltersAndFunctions(t *testing.T) {
	src := source(t, sales)
	pt := newPT(src)
	pt.Row = 0
	pt.Pages = []int{0}
	pt.Rows = []int{1}
	pt.Values = []model.PivotValue{{Field: 2, Func: model.FuncAverage, Name: "Average of Sales"}}
	pt.Fields[0].Hidden = []string{"s:east", "s:north", ""}
	res := Compute(pt, src, nil)
	check(t, "page filter on West", res, ""+
		"Region\tWest\n"+
		"\n"+
		"Row Labels\tAverage of Sales\n"+
		"Ink\t1\n"+
		"Pad\t7\n"+
		"Pen\t20\n"+
		"Grand Total\t9.333333333\n")
	if res.Row != 2 || res.Out.Area.R1 != 0 {
		t.Errorf("body at row %d, area %+v", res.Row, res.Out.Area)
	}
	pt.Fields[0].Hidden = []string{"s:north"}
	pt.Values[0].Func = model.FuncStdDev
	if got := grid(Compute(pt, src, nil)); !strings.HasPrefix(got, "Region\t(Multiple Items)\n") || !strings.Contains(got, "Ink\t20.50609665\n") {
		t.Errorf("multiple items:\n%s", got)
	}
}

// TestNoAxisAndEmpty checks the placeholder and reports with values but no row field.
func TestNoAxisAndEmpty(t *testing.T) {
	src := source(t, sales)
	pt := newPT(src)
	check(t, "empty", Compute(pt, src, nil), "\nPivotTable1\n\nTo build a report, choose\nfields from the PivotTable\nField List\n\n\n\n\n\n\n\n\n\n\n\n\n")
	pt.Values = []model.PivotValue{sum(src, 2)}
	check(t, "one value", Compute(pt, src, nil), "Sum of Sales\n83\n")
	pt.Values = append(pt.Values, sum(src, 3))
	pt.Cols = []int{model.DataField}
	check(t, "two values", Compute(pt, src, nil), "Sum of Sales\tSum of Qty\n83\t14\n")
	pt.Cols = []int{0, model.DataField}
	pt.Fields[0].Hidden = []string{"", "s:north"}
	check(t, "column field with values inside", Compute(pt, src, nil), ""+
		"\tColumn Labels\n"+
		"\tEast\t\tWest\t\tTotal Sum of Sales\tTotal Sum of Qty\n"+
		"\tSum of Sales\tSum of Qty\tSum of Sales\tSum of Qty\n"+
		"\t45\t5\t28\t7\t73\t12\n")
}

// TestComputeUnsyncedFields checks that a report whose source gained
// columns computes instead of panicking.
func TestComputeUnsyncedFields(t *testing.T) {
	src := source(t, [][]string{{"a", "b", "c"}, {"x", "1", "p"}, {"y", "2", "q"}})
	pt := newPT(src)
	pt.Fields = pt.Fields[:1]
	pt.Rows = []int{2} // a column the report has no field for
	if Compute(pt, src, nil) == nil {
		t.Error("Compute returned nil")
	}
	if len(pt.Fields) != 1 {
		t.Errorf("Compute changed the report's fields: %d", len(pt.Fields))
	}
}
