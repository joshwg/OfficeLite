package lookup

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/formula/functions/logical"
	"officelite/excel/formula/functions/mathtrig"
)

func groupSetup(ctx *f.TestContext) {
	ctx.SetRange("T1", [][]any{
		{"Region", "Product", "Year", "Sales"},
		{"East", "A", 2023, 10},
		{"West", "B", 2023, 20},
		{"east", "B", 2024, 30},
		{"West", "A", 2024, 40},
		{"East", "A", 2024, 50},
	})
}

func TestGroupBy(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	mathtrig.Register(r)
	logical.Register(r)
	ftest.Run(t, r, groupSetup, []ftest.Case{
		// Functions named as values.
		{"=BYROW({1,2;3,4},SUM)", ftest.Arr{{3}, {7}}},
		{"=MAP({1,4,9},SQRT)", ftest.Arr{{1, 2, 3}}},

		// GROUPBY: keys sorted ascending (text ignoring case, the first
		// spelling kept), a grand total by default.
		{"=GROUPBY(T2:T6,W2:W6,SUM)", ftest.Arr{{"East", 90}, {"West", 60}, {"Total", 150}}},
		{"=GROUPBY(T1:T6,W1:W6,SUM)", ftest.Arr{{"Region", "Sales"}, {"East", 90}, {"West", 60}, {"Total", 150}}},
		{"=GROUPBY(T1:T6,W1:W6,SUM,1)", ftest.Arr{{"East", 90}, {"West", 60}, {"Total", 150}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,2)", ftest.Arr{{"Row Field 1", "Value 1"}, {"East", 90}, {"West", 60}, {"Total", 150}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,0)", ftest.Arr{{"East", 90}, {"West", 60}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,-1)", ftest.Arr{{"Total", 150}, {"East", 90}, {"West", 60}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,-1)", ftest.Arr{{"West", 60}, {"East", 90}, {"Total", 150}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,2)", ftest.Arr{{"West", 60}, {"East", 90}, {"Total", 150}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,{1,2})", f.ErrValue}, // several sort columns must be keys
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,3)", f.ErrValue},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,2)", f.ErrValue}, // subtotals need two key columns
		{"=GROUPBY(T2:T6,W2:W6,LAMBDA(x,MAX(x)))", ftest.Arr{{"East", 50}, {"West", 40}, {"Total", 50}}},
		{"=GROUPBY(T2:T6,W2:W6,PERCENTOF)", ftest.Arr{{"East", 0.6}, {"West", 0.4}, {"Total", 1}}},
		{"=GROUPBY(T2:T6,W2:W6,LAMBDA(s,a,SUM(s)/SUM(a)),,0)", ftest.Arr{{"East", 0.6}, {"West", 0.4}}},
		{"=GROUPBY(T2:T6,W2:W6,HSTACK(SUM,COUNT))",
			ftest.Arr{{"", "SUM", "COUNT"}, {"East", 90, 3}, {"West", 60, 2}, {"Total", 150, 5}}},
		{"=GROUPBY(T2:T6,W2:W6,VSTACK(SUM,COUNT),,0)", ftest.Arr{{"", "SUM", "COUNT"}, {"East", 90, 3}, {"West", 60, 2}}},
		{"=GROUPBY(T2:T6,W2:W6,VSTACK(HSTACK(SUM,MAX),{\"Sum\",\"Top\"}),,0)",
			ftest.Arr{{"", "Sum", "Top"}, {"East", 90, 50}, {"West", 60, 40}}},
		{"=GROUPBY(T2:T6,V2:W6,SUM,,0)", ftest.Arr{{"East", 6071, 90}, {"West", 4047, 60}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,,V2:V6=2024)", ftest.Arr{{"east", 80}, {"West", 40}, {"Total", 120}}},
		{"=GROUPBY(T1:T6,W1:W6,SUM,3,0,,V1:V6=2023)", ftest.Arr{{"Region", "Sales"}, {"East", 10}, {"West", 20}}},
		{"=GROUPBY(T2:T6,W2:W6,SUM,,,,V2:V6=1999)", f.ErrCalc},
		{"=GROUPBY(T2:T6,W2:W5,SUM)", f.ErrValue},
		{"=GROUPBY(T2:T6,W2:W6,1)", f.ErrValue},
		{"=GROUPBY(T2:T6,W2:W6,LAMBDA(x,x))", ftest.Arr{{"East", f.ErrCalc}, {"West", f.ErrCalc}, {"Total", f.ErrCalc}}},

		// Two key columns: subtotals by default.
		{"=GROUPBY(T2:U6,W2:W6,SUM)", ftest.Arr{
			{"East", "A", 60}, {"East", "B", 30}, {"East", "", 90},
			{"West", "A", 40}, {"West", "B", 20}, {"West", "", 60},
			{"Total", "", 150}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,1)", ftest.Arr{
			{"East", "A", 60}, {"East", "B", 30}, {"West", "A", 40}, {"West", "B", 20}, {"Total", "", 150}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,-2)", ftest.Arr{
			{"Total", "", 150}, {"East", "", 90}, {"East", "A", 60}, {"East", "B", 30},
			{"West", "", 60}, {"West", "A", 40}, {"West", "B", 20}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,0,{-1,-2})", ftest.Arr{
			{"West", "B", 20}, {"West", "A", 40}, {"East", "B", 30}, {"East", "A", 60}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,2,-3)", ftest.Arr{ // groups by their subtotals
			{"East", "A", 60}, {"East", "B", 30}, {"East", "", 90},
			{"West", "A", 40}, {"West", "B", 20}, {"West", "", 60},
			{"Total", "", 150}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,0,3)", ftest.Arr{
			{"West", "B", 20}, {"West", "A", 40}, {"East", "B", 30}, {"East", "A", 60}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,0,3,,1)", ftest.Arr{ // table: sorted as a flat list
			{"West", "B", 20}, {"East", "B", 30}, {"West", "A", 40}, {"East", "A", 60}}},
		{"=GROUPBY(T2:U6,W2:W6,SUM,,2,,,1)", f.ErrValue},

		// PIVOTBY.
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,SUM)", ftest.Arr{
			{"", 2023, 2024, "Total"},
			{"East", 10, 80, 90},
			{"West", 20, 40, 60},
			{"Total", 30, 120, 150}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,SUM,,0,,0)", ftest.Arr{{"", 2023, 2024}, {"East", 10, 80}, {"West", 20, 40}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,SUM,,-1,-1,0,-1)", ftest.Arr{
			{"", 2024, 2023}, {"Total", 120, 30}, {"West", 40, 20}, {"East", 80, 10}}},
		{"=PIVOTBY(T2:T6,U2:U6,W2:W6,SUM,,0,,0,,V2:V6=2023)", ftest.Arr{{"", "A", "B"}, {"East", 10, ""}, {"West", "", 20}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,PERCENTOF,,0,,0)", ftest.Arr{
			{"", 2023, 2024}, {"East", 10.0 / 30, 80.0 / 120}, {"West", 20.0 / 30, 40.0 / 120}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,PERCENTOF,,0,,0,,,1)", ftest.Arr{
			{"", 2023, 2024}, {"East", 10.0 / 90, 80.0 / 90}, {"West", 20.0 / 60, 40.0 / 60}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,PERCENTOF,,0,,0,,,2)", ftest.Arr{
			{"", 2023, 2024}, {"East", 10.0 / 150, 80.0 / 150}, {"West", 20.0 / 150, 40.0 / 150}}},
		{"=PIVOTBY(T1:T6,V1:V6,W1:W6,SUM,,0,,0)", ftest.Arr{
			{"Year", 2023, 2024}, {"Region", "Sales", "Sales"}, {"East", 10, 80}, {"West", 20, 40}}},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,HSTACK(SUM,COUNT),,0,,0)", ftest.Arr{
			{"", 2023, 2023, 2024, 2024}, {"", "SUM", "COUNT", "SUM", "COUNT"},
			{"East", 10, 1, 80, 2}, {"West", 20, 1, 40, 1}}},
		{"=PIVOTBY(T2:T6,U2:V6,W2:W6,SUM,,0)", ftest.Arr{
			{"", "A", "A", "A", "B", "B", "B", "Total"},
			{"", 2023, 2024, "", 2023, 2024, "", ""},
			{"East", 10, 50, 60, "", 30, 30, 90},
			{"West", "", 40, 40, 20, "", 20, 60}}},
		{"=PIVOTBY(T2:T6,V2:V5,W2:W6,SUM)", f.ErrValue},
		{"=PIVOTBY(T2:T6,V2:V6,W2:W6,SUM,,,,,,,5)", f.ErrValue},
	})
}
