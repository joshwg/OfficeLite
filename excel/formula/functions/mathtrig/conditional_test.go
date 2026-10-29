// Tests for the criteria functions (SUMIF(S), COUNTIF(S), AVERAGEIF(S),
// MINIFS, MAXIFS).

package mathtrig

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/model"
)

// condData lays out Microsoft's documentation examples:
//
//	Sheet1!A1:C7  Category / Food / Sales (SUMIF example 2)
//	Sheet1!E1:F5  Property value / Commission (SUMIF, AVERAGEIF)
//	Sheet1!H1:I5  Fruit / Count (COUNTIF)
//	Sheet1!K1:L6  Grade / Weight (MINIFS, MAXIFS)
//	Sheet1!M1     #DIV/0!
//	Sheet2!A1:C9  Quantity / Product / Salesperson (SUMIFS)
func condData(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{
		{"Category", "Food", "Sales"},
		{"Vegetables", "Tomatoes", 2300},
		{"Vegetables", "Celery", 5500},
		{"Fruits", "Oranges", 800},
		{nil, "Butter", 400},
		{"Vegetables", "Carrots", 4200},
		{"Fruits", "Apples", 1200},
	})
	ctx.SetRange("E1", [][]any{
		{"Property", "Commission"},
		{100000, 7000},
		{200000, 14000},
		{300000, 21000},
		{400000, 28000},
	})
	ctx.SetRange("H1", [][]any{
		{"Fruit", "Count"},
		{"apples", 32},
		{"oranges", 54},
		{"peaches", 75},
		{"apples", 86},
	})
	ctx.SetRange("K1", [][]any{{89, 1}, {93, 2}, {96, 2}, {85, 3}, {91, 1}, {88, 1}})
	ctx.Set("M1", "=1/0")
	ctx.SetRange("Sheet2!A1", [][]any{
		{"Quantity", "Product", "Salesperson"},
		{5, "Apples", "Tom"},
		{4, "Apples", "Sarah"},
		{15, "Artichokes", "Tom"},
		{3, "Artichokes", "Sarah"},
		{22, "Bananas", "Tom"},
		{12, "Bananas", "Sarah"},
		{10, "Carrots", "Tom"},
		{33, "Carrots", "Sarah"},
	})
}

// TestConditional checks the criteria functions on Microsoft's examples,
// whole-column ranges, array criteria and shape mismatches.
func TestConditional(t *testing.T) {
	const rows = float64(model.MaxRows)
	ftest.Run(t, registry(), condData, []ftest.Case{
		{"=SUMIF(E2:E5,\">160000\",F2:F5)", 63000},
		{"=SUMIF(E2:E5,\">160000\")", 900000},
		{"=SUMIF(E2:E5,300000,F2:F5)", 21000},
		{"=SUMIF(E2:E5,\">\"&E3,F2:F5)", 49000},
		{"=SUMIF(E2:E5,\">160000\",F2)", 63000}, // shape taken from the range
		{"=SUMIF(A:A,\"Fruits\",C:C)", 2000},
		{"=SUMIF(A:A,\"Vegetables\",C:C)", 12000},
		{"=SUMIF(B:B,\"*es\",C:C)", 4300},
		{"=SUMIF(A:A,\"\",C:C)", 400},
		{"=SUMIF(A2:A7,\"fruits\",C2)", 2000},
		{"=SUMIF(A2:A7,{\"Fruits\",\"Vegetables\"},C2:C7)", ftest.Arr{{2000, 12000}}},
		{"=SUMIF(C2:C7,\">1000\")", 13200},
		{"=SUMIF(K1:K6,\"<90\",M1:M6)", f.ErrDiv0}, // M1 is #DIV/0!
		{"=SUMIF(K1:K6,\">90\",M1:M6)", 0},
		{"=SUMIFS(Sheet2!A2:A9,Sheet2!B2:B9,\"=A*\",Sheet2!C2:C9,\"Tom\")", 20},
		{"=SUMIFS(Sheet2!A2:A9,Sheet2!B2:B9,\"<>Bananas\",Sheet2!C2:C9,\"Tom\")", 30},
		{"=SUMIFS(Sheet2!A:A,Sheet2!B:B,{\"Apples\",\"Carrots\"})", ftest.Arr{{9, 43}}},
		{"=SUMIFS(Sheet2!A:A,Sheet2!C:C,\"Sarah\",Sheet2!A:A,\">3\")", 49},
		{"=SUMIFS(Sheet2!A2:A9,Sheet2!B2:B8,\"x\")", f.ErrValue},
		{"=SUMIFS(Sheet2!A2:A9,Sheet2!B2:B9)", f.ErrValue},
		{"=SUMIFS(C:C,A:A,\"Fruits\",B:B,\"A*\")", 1200},
		{"=COUNTIF(H2:H5,\"apples\")", 2},
		{"=COUNTIF(H2:H5,H4)", 1},
		{"=COUNTIF(H2:H5,H2)+COUNTIF(H2:H5,H3)", 3},
		{"=COUNTIF(I2:I5,\">55\")", 2},
		{"=COUNTIF(I2:I5,\"<>\"&I4)", 3},
		{"=COUNTIF(I2:I5,\">=32\")-COUNTIF(I2:I5,\">85\")", 3},
		{"=COUNTIF(H2:H5,\"*\")", 4},
		{"=COUNTIF(H2:H5,\"?????es\")", 2},
		{"=COUNTIF(H2:H5,{\"apples\",\"peaches\"})", ftest.Arr{{2, 1}}},
		{"=COUNTIF(H:H,\"\")", rows - 5},
		{"=COUNTIF(H:H,\"<>\")", 5},
		{"=COUNTIF(M1:M2,#DIV/0!)", 1},
		{"=COUNTIFS(Sheet2!B:B,\"Apples\",Sheet2!C:C,\"Tom\")", 1},
		{"=COUNTIFS(Sheet2!A2:A9,\">4\",Sheet2!A2:A9,\"<20\")", 4},
		{"=COUNTIFS(Sheet2!A:A,\"\",Sheet2!B:B,\"\")", rows - 9},
		{"=COUNTIFS(Sheet2!A2:A9,\">4\",Sheet2!B2:B5,\"x\")", f.ErrValue},
		{"=COUNTIFS(Sheet2!A2:A9,\">4\",Sheet2!B2:B9)", f.ErrValue},
		{"=AVERAGEIF(F2:F5,\"<23000\")", 14000},
		{"=AVERAGEIF(E2:E5,\"<250000\")", 150000},
		{"=AVERAGEIF(E2:E5,\"<95000\")", f.ErrDiv0},
		{"=AVERAGEIF(E2:E5,\">250000\",F2:F5)", 24500},
		{"=AVERAGEIF(A:A,\"Vegetables\",C:C)", 4000},
		{"=AVERAGEIFS(Sheet2!A:A,Sheet2!C:C,\"Tom\")", 13},
		{"=AVERAGEIFS(Sheet2!A:A,Sheet2!C:C,\"Nobody\")", f.ErrDiv0},
		{"=MINIFS(K1:K6,L1:L6,1)", 88},
		{"=MAXIFS(K1:K6,L1:L6,1)", 91},
		{"=MAXIFS(K1:K6,L1:L6,\">1\")", 96},
		{"=MINIFS(K1:K6,L1:L6,9)", 0},
		{"=MAXIFS(K:K,L:L,2,K:K,\"<95\")", 93},
		{"=MAXIFS(K1:K6,L1:L5,1)", f.ErrValue},
	})
}
