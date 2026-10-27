package mathtrig

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/model"
)

// aggData: A1:A8 = 10, 7, 9, 27, 2, "3" (text), TRUE, blank; B1:B3 = 1,
// #DIV/0!, 3; C1 = "" (formula result); Sheet2!A1:A2 = 1, 2.
func aggData(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{{10}, {7}, {9}, {27}, {2}, {"3"}, {true}, {nil}})
	ctx.SetRange("B1", [][]any{{1}, {"=1/0"}, {3}})
	ctx.Set("C1", `=""`)
	ctx.SetRange("Sheet2!A1", [][]any{{1}, {2}})
}

func TestSumCount(t *testing.T) {
	ftest.Run(t, registry(), aggData, []ftest.Case{
		{"=SUM(A1:A8)", 55},
		{"=SUM(A:A)", 55},
		{"=SUM(1:1)", 11},
		{"=SUM(A1:A8,\"3\",TRUE)", 59}, // direct text and booleans are coerced
		{"=SUM(\"3\",TRUE)", 4},
		{"=SUM(\"abc\")", f.ErrValue},
		{"=SUM(A6)", 0}, // text in a cell is ignored
		{"=SUM({1,\"2\",TRUE})", 1},
		{"=SUM(1,)", 1},
		{"=SUM(B1:B3)", f.ErrDiv0},
		{"=SUM(1,#N/A)", f.ErrNA},
		{"=SUM(A1:A3*2)", 52},
		{"=SUM(Sheet2!A:A,A1)", 13},
		{"=SUM(A8)", 0},
		{"=PRODUCT(5,15,30)", 2250},
		{"=PRODUCT(A1:A3)", 630},
		{"=PRODUCT(A6:A8)", 0},
		{"=SUMSQ(3,4)", 25},
		{"=SUMSQ(A1:A2,TRUE)", 150},
		{"=COUNT(A1:A8)", 5},
		{"=COUNT(A:A)", 5},
		{"=COUNT(A1:A8,\"3\",TRUE,\"x\",#N/A)", 7},
		{"=COUNT(,)", 2},
		{"=COUNT(B1:B3)", 2},
		{"=COUNT({1,\"2\",TRUE})", 1},
		{"=COUNTA(A1:A8)", 7},
		{"=COUNTA(A:A)", 7},
		{"=COUNTA(B1:B3)", 3},
		{"=COUNTA(C1)", 1},
		{"=COUNTA(C2)", 0},
		{"=COUNTA(1,\"x\",#N/A)", 3},
		{"=COUNTBLANK(A1:A8)", 1},
		{"=COUNTBLANK(A1:A10)", 3},
		{"=COUNTBLANK(C1:C2)", 2},
		{"=COUNTBLANK(A:A)", float64(model.MaxRows - 7)},
		{"=COUNTBLANK(B1:B3)", 0},
		{"=SUMPRODUCT({3,4;8,6;1,9},{2,7;6,7;5,3})", 156},
		{"=SUMPRODUCT({1,2,3})", 6},
		{"=SUMPRODUCT(({1,2,3}>1)*{4,5,6})", 11},
		{"=SUMPRODUCT({1,\"a\",TRUE},{1,1,1})", 1},
		{"=SUMPRODUCT({1,2},{1,2,3})", f.ErrValue},
		{"=SUMPRODUCT({1,#N/A})", f.ErrNA},
		{"=SUMPRODUCT(A1:A3,A2:A4)", 10*7 + 7*9 + 9*27},
		{"=SUMPRODUCT(A:A,A:A)", 100 + 49 + 81 + 729 + 4},
		{"=SUMPRODUCT(A:A,Sheet2!A:A)", 10 + 14},
		{"=SUMPRODUCT(A1:A2,A1:B1)", f.ErrValue},
		{"=SUMX2MY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", -55},
		{"=SUMX2PY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", 521},
		{"=SUMXMY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", 79},
		{"=SUMX2MY2({1,2},{1})", f.ErrNA},
		{"=SUMXMY2({1,\"a\",3},{2,2,2})", 2},
		{"=SUMXMY2({1,#N/A},{2,2})", f.ErrNA},
		{"=SUMX2PY2(A1:A3,B1:B1)", f.ErrNA},
		{"=PERCENTOF(10,40)", 0.25},
		{"=PERCENTOF({1,2},{1,2,3,4})", 0.3},
		{"=PERCENTOF(A1,A1:A8)", 10.0 / 55},
		{"=PERCENTOF(1,0)", f.ErrDiv0},
		{"=PERCENTOF(B1:B3,1)", f.ErrDiv0},
	})
}

// subtotalData: A1:A3 = 1, 2, 3, A4 = SUBTOTAL(9,A1:A3), A5 = 10;
// B1:B4 = 1, #DIV/0!, 3, AGGREGATE(9,6,B1:B3).
func subtotalData(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{{1}, {2}, {3}, {"=SUBTOTAL(9,A1:A3)"}, {10}})
	ctx.SetRange("B1", [][]any{{1}, {"=1/0"}, {3}, {"=AGGREGATE(9,6,B1:B3)"}})
	ctx.SetRange("C1", [][]any{{"=SUM(A1:A3)"}, {"=sum(subtotal(9,A1),1)"}})
}

func TestSubtotalAggregate(t *testing.T) {
	ftest.Run(t, registry(), subtotalData, []ftest.Case{
		{"=SUM(A1:A5)", 22},
		{"=SUBTOTAL(9,A1:A5)", 16}, // the nested SUBTOTAL in A4 is skipped
		{"=SUBTOTAL(109,A1:A5)", 16},
		{"=SUBTOTAL(9,A:A)", 16},
		{"=SUBTOTAL(9,A1:A2,A5)", 13},
		{"=SUBTOTAL(1,A1:A5)", 4},
		{"=SUBTOTAL(2,A1:A5)", 4},
		{"=SUBTOTAL(3,A1:A5)", 4},
		{"=SUBTOTAL(4,A1:A5)", 10},
		{"=SUBTOTAL(5,A1:A5)", 1},
		{"=SUBTOTAL(6,A1:A5)", 60},
		{"=SUBTOTAL(7,A1:A5)", near(4.082482905)},
		{"=SUBTOTAL(8,A1:A5)", near(3.535533906)},
		{"=SUBTOTAL(10,A1:A5)", near(16.66666667)},
		{"=SUBTOTAL(11,A1:A5)", 12.5},
		{"=SUBTOTAL(9,C1:C2)", 6 + 0}, // C2 contains SUBTOTAL, C1 does not
		{"=SUBTOTAL(9,B1:B3)", f.ErrDiv0},
		{"=SUBTOTAL(12,A1:A5)", f.ErrValue},
		{"=SUBTOTAL(0,A1:A5)", f.ErrValue},
		{"=SUBTOTAL({9,4},A1:A5)", ftest.Arr{{16, 10}}},
		{"=AGGREGATE(9,4,A1:A5)", 22},
		{"=AGGREGATE(9,0,A1:A5)", 16},
		{"=AGGREGATE(9,,A1:A5)", 16},
		{"=AGGREGATE(9,5,A1:A5)", 22},
		{"=AGGREGATE(9,6,B1:B4)", 8}, // option 6 counts the nested AGGREGATE
		{"=AGGREGATE(9,7,B1:B4)", 8},
		{"=AGGREGATE(9,3,B1:B4)", 4},
		{"=AGGREGATE(9,2,B1:B4)", 4},
		{"=AGGREGATE(9,4,B1:B3)", f.ErrDiv0},
		{"=AGGREGATE(9,1,B1:B3)", f.ErrDiv0},
		{"=AGGREGATE(1,6,B1:B3)", 2},
		{"=AGGREGATE(2,4,B1:B3)", 2},
		{"=AGGREGATE(3,6,B1:B3)", 2},
		{"=AGGREGATE(3,4,B1:B3)", 3},
		{"=AGGREGATE(4,6,B1:B3)", 3},
		{"=AGGREGATE(5,6,B1:B3)", 1},
		{"=AGGREGATE(6,6,B1:B3)", 3},
		{"=AGGREGATE(7,6,B1:B3)", near(1.414213562)},
		{"=AGGREGATE(8,6,B1:B3)", 1},
		{"=AGGREGATE(10,6,B1:B3)", 2},
		{"=AGGREGATE(11,6,B1:B3)", 1},
		{"=AGGREGATE(12,6,B1:B3)", 2},
		{"=AGGREGATE(12,4,A1:A3,A5)", 2.5},
		{"=AGGREGATE(13,6,{1,2,2,3})", 2},
		{"=AGGREGATE(13,6,{1,2,3})", f.ErrNA},
		{"=AGGREGATE(14,6,B1:B3,1)", 3},
		{"=AGGREGATE(14,6,B1:B3,{1,2})", ftest.Arr{{3, 1}}},
		{"=AGGREGATE(14,6,B1:B3,3)", f.ErrNum},
		{"=AGGREGATE(15,6,B1:B3*2,1)", 2},
		{"=AGGREGATE(15,4,B1:B3*2,1)", f.ErrDiv0},
		{"=AGGREGATE(16,6,{1,3,2,4},0.3)", 1.9},
		{"=AGGREGATE(17,6,{1,2,4,7,8,9,10,12},1)", 3.5},
		{"=AGGREGATE(18,6,{1,2,3,6,6,6,7,8,9},0.25)", 2.5},
		{"=AGGREGATE(19,0,{6,7,15,36,39,40,41,42,43,47,49},1)", 15},
		{"=AGGREGATE(14,0,A1:A3)", f.ErrValue},
		{"=AGGREGATE(20,0,A1)", f.ErrValue},
		{"=AGGREGATE(0,0,A1)", f.ErrValue},
		{"=AGGREGATE(9,8,A1)", f.ErrValue},
	})
}
