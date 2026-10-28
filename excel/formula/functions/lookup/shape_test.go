package lookup

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/formula/functions/mathtrig"
	"officelite/excel/model"
)

func shapeSetup(ctx *f.TestContext) {
	// TAKE / DROP documentation data.
	ctx.SetRange("A1", [][]any{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}})
	// CHOOSECOLS / CHOOSEROWS documentation data.
	ctx.SetRange("A5", [][]any{
		{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}, {13, 14, 15, 16}, {17, 18, 19, 20}, {21, 22, 23, 24},
	})
	// TOCOL documentation data: names with a blank and an error.
	ctx.SetRange("F1", [][]any{
		{"Ben", "Peter", "Mary", "Sam"},
		{"John", "Hillary", "Jenny", "James"},
		{"Agnes", "Harry", nil, f.ErrNA},
	})
	// A block with blank margins for TRIMRANGE.
	ctx.SetRange("L3", [][]any{{nil, 5, nil}, {6, nil, 7}})
	ctx.Set("L7", "x")
	ctx.Set("N30", 1) // extends the used area below the blank cells
}

func TestShape(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	mathtrig.Register(r)
	ftest.Run(t, r, shapeSetup, []ftest.Case{
		// CHOOSECOLS (Microsoft's examples).
		{"=CHOOSECOLS(A5:D10,1,3)", ftest.Arr{{1, 3}, {5, 7}, {9, 11}, {13, 15}, {17, 19}, {21, 23}}},
		{"=CHOOSECOLS(A5:D10,1,1)", ftest.Arr{{1, 1}, {5, 5}, {9, 9}, {13, 13}, {17, 17}, {21, 21}}},
		{"=CHOOSECOLS(A5:D10,3,-1)", ftest.Arr{{3, 4}, {7, 8}, {11, 12}, {15, 16}, {19, 20}, {23, 24}}},
		{"=CHOOSECOLS(A5:D10,{2,4})", ftest.Arr{{2, 4}, {6, 8}, {10, 12}, {14, 16}, {18, 20}, {22, 24}}},
		{"=CHOOSECOLS(A5:D10,0)", f.ErrValue},
		{"=CHOOSECOLS(A5:D10,5)", f.ErrValue},
		{"=CHOOSECOLS(A5:D10,-5)", f.ErrValue},
		{"=CHOOSECOLS(A5:D10,#DIV/0!)", f.ErrDiv0},
		{"=CHOOSECOLS({1,2,3},2)", 2},

		// CHOOSEROWS (Microsoft's examples).
		{"=CHOOSEROWS(A5:D10,1,3,5,1)", ftest.Arr{{1, 2, 3, 4}, {9, 10, 11, 12}, {17, 18, 19, 20}, {1, 2, 3, 4}}},
		{"=CHOOSEROWS(A5:D10,{1,3},5,1)", ftest.Arr{{1, 2, 3, 4}, {9, 10, 11, 12}, {17, 18, 19, 20}, {1, 2, 3, 4}}},
		{"=CHOOSEROWS(A5:D10,-1,-2)", ftest.Arr{{21, 22, 23, 24}, {17, 18, 19, 20}}},
		{"=CHOOSEROWS(A5:D10,7)", f.ErrValue},
		{"=CHOOSEROWS(A5:D10,0)", f.ErrValue},
		{"=SUM(CHOOSEROWS(A5:D10,SEQUENCE(2)))", 36},

		// TAKE (Microsoft's examples).
		{"=TAKE(A1:C3,2)", ftest.Arr{{1, 2, 3}, {4, 5, 6}}},
		{"=TAKE(A1:C3,,2)", ftest.Arr{{1, 2}, {4, 5}, {7, 8}}},
		{"=TAKE(A1:C3,-2)", ftest.Arr{{4, 5, 6}, {7, 8, 9}}},
		{"=TAKE(A1:C3,2,2)", ftest.Arr{{1, 2}, {4, 5}}},
		{"=TAKE({1,2,3;4,5,6},1,-1)", 3},
		{"=TAKE(A1:C3,10,10)", ftest.Arr{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}},
		{"=TAKE(A1:C3,0)", f.ErrCalc},
		{"=TAKE(A1:C3,1,0)", f.ErrCalc},
		{"=TAKE(A1:C3,\"x\")", f.ErrValue},
		{"=SUM(TAKE(A:A,2))", 5},
		{"=ROWS(TAKE(A:A,-1))", 1},
		{"=ROW(TAKE(A:A,-1))", float64(model.MaxRows)}, // a reference: the sheet's last row

		// DROP (Microsoft's examples).
		{"=DROP(A1:C3,2)", ftest.Arr{{7, 8, 9}}},
		{"=DROP(A1:C3,,2)", ftest.Arr{{3}, {6}, {9}}},
		{"=DROP(A1:C3,-2)", ftest.Arr{{1, 2, 3}}},
		{"=DROP(A1:C3,2,2)", 9},
		{"=DROP(A1:C3,0)", ftest.Arr{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}},
		{"=DROP(A1:C3,3)", f.ErrCalc},
		{"=DROP(A1:C3,-5)", f.ErrCalc},
		{"=DROP({1,2;3,4},1,-1)", 3},

		// EXPAND (Microsoft's examples).
		{"=EXPAND({1,2;3,4},3,3)", ftest.Arr{{1, 2, f.ErrNA}, {3, 4, f.ErrNA}, {f.ErrNA, f.ErrNA, f.ErrNA}}},
		{"=EXPAND({1,2;3,4},3,3,\"-\")", ftest.Arr{{1, 2, "-"}, {3, 4, "-"}, {"-", "-", "-"}}},
		{"=EXPAND({1,2;3,4},,3,0)", ftest.Arr{{1, 2, 0}, {3, 4, 0}}},
		{"=EXPAND(5,2)", ftest.Arr{{5}, {f.ErrNA}}},
		{"=EXPAND({1,2;3,4},1,2)", f.ErrValue},
		{"=EXPAND({1,2;3,4},2,1)", f.ErrValue},
		{"=EXPAND({1,2;3,4},2000000)", f.ErrNum},

		// HSTACK / VSTACK (Microsoft's examples).
		{"=HSTACK({\"A\",\"B\",\"C\"},{\"AA\",\"BB\",\"CC\"})", ftest.Arr{{"A", "B", "C", "AA", "BB", "CC"}}},
		{"=VSTACK({\"A\",\"B\",\"C\"},{\"AA\",\"BB\",\"CC\"})", ftest.Arr{{"A", "B", "C"}, {"AA", "BB", "CC"}}},
		{"=HSTACK({1;2;3},{4;5})", ftest.Arr{{1, 4}, {2, 5}, {3, f.ErrNA}}},
		{"=VSTACK({1,2},{3;4})", ftest.Arr{{1, 2}, {3, f.ErrNA}, {4, f.ErrNA}}},
		{"=VSTACK(A1:C1,\"x\",#DIV/0!)", ftest.Arr{{1, 2, 3}, {"x", f.ErrNA, f.ErrNA}, {f.ErrDiv0, f.ErrNA, f.ErrNA}}},
		{"=HSTACK(1)", 1},
		{"=SUM(VSTACK(A1:C1,A3:C3))", 30},

		// TOCOL / TOROW (Microsoft's examples).
		{"=TOCOL(F1:I3)", ftest.Arr{{"Ben"}, {"Peter"}, {"Mary"}, {"Sam"}, {"John"}, {"Hillary"},
			{"Jenny"}, {"James"}, {"Agnes"}, {"Harry"}, {nil}, {f.ErrNA}}},
		{"=TOCOL(F1:I3,1)", ftest.Arr{{"Ben"}, {"Peter"}, {"Mary"}, {"Sam"}, {"John"}, {"Hillary"},
			{"Jenny"}, {"James"}, {"Agnes"}, {"Harry"}, {f.ErrNA}}},
		{"=TOCOL(F1:I3,2)", ftest.Arr{{"Ben"}, {"Peter"}, {"Mary"}, {"Sam"}, {"John"}, {"Hillary"},
			{"Jenny"}, {"James"}, {"Agnes"}, {"Harry"}, {nil}}},
		{"=TOCOL(F1:I3,3,TRUE)", ftest.Arr{{"Ben"}, {"John"}, {"Agnes"}, {"Peter"}, {"Hillary"},
			{"Harry"}, {"Mary"}, {"Jenny"}, {"Sam"}, {"James"}}},
		{"=TOROW({1,2;3,4})", ftest.Arr{{1, 2, 3, 4}}},
		{"=TOROW({1,2;3,4},0,TRUE)", ftest.Arr{{1, 3, 2, 4}}},
		{"=TOROW(H3:I3,3)", f.ErrCalc},
		{"=TOROW({1,2},4)", f.ErrValue},
		{"=TOCOL(7)", 7},

		// WRAPROWS / WRAPCOLS (Microsoft's examples).
		{"=WRAPROWS({\"A\",\"B\",\"C\",\"D\",\"E\",\"F\",\"G\"},3)",
			ftest.Arr{{"A", "B", "C"}, {"D", "E", "F"}, {"G", f.ErrNA, f.ErrNA}}},
		{"=WRAPROWS({\"A\",\"B\",\"C\",\"D\",\"E\",\"F\",\"G\"},3,\"x\")",
			ftest.Arr{{"A", "B", "C"}, {"D", "E", "F"}, {"G", "x", "x"}}},
		{"=WRAPCOLS({\"A\",\"B\",\"C\",\"D\",\"E\",\"F\",\"G\"},3)",
			ftest.Arr{{"A", "D", "G"}, {"B", "E", f.ErrNA}, {"C", "F", f.ErrNA}}},
		{"=WRAPCOLS({\"A\";\"B\";\"C\";\"D\"},2,0)", ftest.Arr{{"A", "C"}, {"B", "D"}}},
		{"=WRAPROWS(SEQUENCE(1,4),4)", ftest.Arr{{1, 2, 3, 4}}},
		{"=WRAPROWS({1,2;3,4},2)", f.ErrValue},
		{"=WRAPROWS({1,2,3},0)", f.ErrNum},

		// TRIMRANGE: L3:N4 holds values with a blank row above (L2) and
		// the text in L7 below a blank row.
		{"=ROWS(TRIMRANGE(L1:N10))", 5},
		{"=ROW(TRIMRANGE(L1:N10))", ftest.Arr{{3}, {4}, {5}, {6}, {7}}},
		{"=TRIMRANGE(L1:N4)", ftest.Arr{{nil, 5, nil}, {6, nil, 7}}},
		{"=TRIMRANGE(L1:P4)", ftest.Arr{{nil, 5, nil}, {6, nil, 7}}},
		{"=ROWS(TRIMRANGE(L1:N4,1))", 2},
		{"=ROWS(TRIMRANGE(L3:N10,2))", 5},
		{"=ROWS(TRIMRANGE(L1:N10,0))", 10},
		{"=COLUMNS(TRIMRANGE(K1:P4,3,0))", 6},
		{"=COLUMNS(TRIMRANGE(K1:P4,0,1))", 5},
		{"=COLUMNS(TRIMRANGE(K1:P4,0,2))", 4},
		{"=ROWS(TRIMRANGE(A:A))", 10},
		{"=SUM(TRIMRANGE(A:A))", 78},
		{"=ROWS(TRIMRANGE(M:M))", 1},
		{"=TRIMRANGE(Q1:Q5)", f.ErrRef},
		{"=TRIMRANGE(L1:N4,4)", f.ErrValue},
		{"=TRIMRANGE({1,2})", ftest.Arr{{1, 2}}},
	})
}
