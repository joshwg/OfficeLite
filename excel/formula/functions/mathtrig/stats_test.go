package mathtrig

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

const stdevData = "{1345,1301,1368,1322,1310,1370,1318,1350,1303,1299}"

func TestStats(t *testing.T) {
	ftest.Run(t, registry(), aggData, []ftest.Case{
		{"=AVERAGE(A1:A5)", 11},
		{"=AVERAGE(A1:A8)", 11},
		{"=AVERAGE(A:A,5)", 10},
		{"=AVERAGE(A6:A8)", f.ErrDiv0},
		{"=AVERAGE(\"x\")", f.ErrValue},
		{"=AVERAGE(1,)", 0.5},
		{"=AVERAGE(B1:B3)", f.ErrDiv0},
		{"=AVERAGEA(A1:A8)", 8}, // (55 + 0 + 1) / 7
		{"=AVERAGEA(\"x\")", f.ErrValue},
		{"=AVERAGEA(C1)", 0}, // "" counts as text
		{"=MIN(A1:A8)", 2},
		{"=MAX(A1:A8)", 27},
		{"=MAX(A:A)", 27},
		{"=MAX(A6:A8)", 0},
		{"=MAX(-5,\"-2\")", -2},
		{"=MAX(B1:B3)", f.ErrDiv0},
		{"=MAXA(A6:A8)", 1},
		{"=MINA(A6:A7)", 0},
		{"=MINA(A1:A8)", 0},
		{"=MEDIAN(A1:A5)", 9},
		{"=MEDIAN(1,2,3,4)", 2.5},
		{"=MEDIAN({1,2,3,4,5,6})", 3.5},
		{"=MEDIAN(A6:A8)", f.ErrNum},
		{"=MODE({5.6,4,4,3,2,4})", 4},
		{"=MODE.SNGL(5.6,4,4,3,2,4)", 4},
		{"=MODE(1,2,3)", f.ErrNA},
		{"=MODE(1,2,2,1)", 1}, // the first value with the top count
		{"=MODE.MULT({1,2,3,4,3,2,1,2,3,5,6,1})", ftest.Arr{{1}, {2}, {3}}},
		{"=MODE.MULT({1,2})", f.ErrNA},
		{"=STDEV.S(" + stdevData + ")", near(27.46391572)},
		{"=STDEV(" + stdevData + ")", near(27.46391572)},
		{"=STDEV.P(" + stdevData + ")", near(26.05455814)},
		{"=STDEVP(" + stdevData + ")", near(26.05455814)},
		{"=VAR.S(" + stdevData + ")", near(754.2666667)},
		{"=VAR(" + stdevData + ")", near(754.2666667)},
		{"=VAR.P(" + stdevData + ")", near(678.84)},
		{"=VARP(" + stdevData + ")", near(678.84)},
		{"=STDEV.S(1)", f.ErrDiv0},
		{"=STDEV.P(1)", 0},
		{"=VAR.P(A6:A8)", f.ErrDiv0},
		{"=VARA(A6:A7)", 0.5},
		{"=STDEVPA(A6:A7)", 0.5},
		{"=LARGE({3,5,3,5,4;4,2,4,6,7},3)", 5},
		{"=LARGE({3,5,3,5,4;4,2,4,6,7},7)", 4},
		{"=LARGE(A1:A8,1)", 27},
		{"=LARGE(A1:A8,{1,2})", ftest.Arr{{27, 10}}},
		{"=LARGE(A1:A8,1.5)", 10}, // a fractional k is rounded up
		{"=LARGE(A1:A8,0)", f.ErrNum},
		{"=LARGE(A1:A8,6)", f.ErrNum},
		{"=LARGE(A1:A8,\"x\")", f.ErrValue},
		{"=SMALL({3,4,5,2,3,4,5,6,4,7},4)", 4},
		{"=SMALL({1,4,8,3,7,12,54,8,23},2)", 3},
		{"=SMALL(A:A,1)", 2},
		{"=SMALL(B1:B3,1)", f.ErrDiv0},
		{"=PERCENTILE.INC({1,3,2,4},0.3)", 1.9},
		{"=PERCENTILE({1,3,2,4},0.3)", 1.9},
		{"=PERCENTILE.INC({1,3,2,4},1)", 4},
		{"=PERCENTILE.INC({1,3,2,4},0)", 1},
		{"=PERCENTILE.INC({1,3,2,4},1.1)", f.ErrNum},
		{"=PERCENTILE.INC(A6:A8,0.5)", f.ErrNum},
		{"=PERCENTILE.EXC({1,2,3,6,6,6,7,8,9},0.25)", 2.5},
		{"=PERCENTILE.EXC({1,2,3,6,6,6,7,8,9},0)", f.ErrNum},
		{"=PERCENTILE.EXC({1,2,3,6,6,6,7,8,9},0.01)", f.ErrNum},
		{"=PERCENTILE.EXC({1,2,3,6,6,6,7,8,9},2)", f.ErrNum},
		{"=PERCENTILE.EXC({1,2,3,6,6,6,7,8,9},0.9)", 9},
		{"=QUARTILE.INC({1,2,4,7,8,9,10,12},1)", 3.5},
		{"=QUARTILE({1,2,4,7,8,9,10,12},1)", 3.5},
		{"=QUARTILE.INC({1,2,4,7,8,9,10,12},4)", 12},
		{"=QUARTILE.INC({1,2,4,7,8,9,10,12},0)", 1},
		{"=QUARTILE.INC({1,2,4,7,8,9,10,12},5)", f.ErrNum},
		{"=QUARTILE.EXC({6,7,15,36,39,40,41,42,43,47,49},1)", 15},
		{"=QUARTILE.EXC({6,7,15,36,39,40,41,42,43,47,49},3)", 43},
		{"=QUARTILE.EXC({6,7,15,36,39,40,41,42,43,47,49},0)", f.ErrNum},
		{"=QUARTILE.EXC({6,7,15,36,39,40,41,42,43,47,49},4)", f.ErrNum},
	})
}

func TestMatrix(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{1, 2, 1}, {3, 4, -1}, {0, 2, 0}})
		ctx.SetRange("A5", [][]any{{1, "x"}, {nil, 2}})
	}, []ftest.Case{
		{"=MMULT({1,3;7,2},{2,0;0,2})", ftest.Arr{{2, 6}, {14, 4}}},
		{"=MMULT({1,2},{3;4})", 11},
		{"=MMULT({1;2},{3,4})", ftest.Arr{{3, 4}, {6, 8}}},
		{"=MMULT(A1:C3,MUNIT(3))", ftest.Arr{{1, 2, 1}, {3, 4, -1}, {0, 2, 0}}},
		{"=MMULT({1,2},{3,4})", f.ErrValue},
		{"=MMULT({1,\"a\"},{1;2})", f.ErrValue},
		{"=MMULT(A5:B6,{1;1})", f.ErrValue},
		{"=MMULT({1,#N/A},{1;1})", f.ErrNA},
		{"=MDETERM({1,3,8,5;1,3,6,1;1,1,1,0;7,3,10,2})", 88},
		{"=MDETERM({3,6;1,1})", -3},
		{"=MDETERM({3,6,1;1,1,0;3,10,2})", 1},
		{"=MDETERM(A1:C3)", 8},
		{"=MDETERM({1,2;2,4})", 0},
		{"=MDETERM(5)", 5},
		{"=MDETERM({1,2,3})", f.ErrValue},
		{"=MDETERM(A5:B6)", f.ErrValue},
		{"=MINVERSE({4,-1;2,0})", ftest.Arr{{0, 0.5}, {-1, 2}}},
		{"=MINVERSE(A1:C3)", ftest.Arr{{0.25, 0.25, -0.75}, {0, 0, 0.5}, {0.75, -0.25, -0.25}}},
		{"=MINVERSE({1,2;2,4})", f.ErrNum},
		{"=MINVERSE({0})", f.ErrNum},
		{"=MINVERSE({2})", 0.5},
		{"=MINVERSE({1,2})", f.ErrValue},
		{"=MMULT(MINVERSE({2,1;1,3}),{2,1;1,3})", ftest.Arr{{1, near(0)}, {near(0), 1}}},
		{"=MUNIT(3)", ftest.Arr{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}},
		{"=MUNIT(1)", 1},
		{"=MUNIT(0)", f.ErrValue},
		{"=MUNIT(\"x\")", f.ErrValue},
	})
}
