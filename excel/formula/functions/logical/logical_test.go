package logical

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

func TestLogical(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{1, "x", true}, {2, nil, false}, {3, "y", 0}})
	}, []ftest.Case{
		{"=IF(1>0,\"yes\",\"no\")", "yes"},
		{"=IF(FALSE,1)", false},
		{"=IF(TRUE,,1)", 0.0},
		{"=IF(\"x\",1,2)", f.ErrValue},
		{"=IF(1/0>1,1,2)", f.ErrDiv0},
		{"=IF(TRUE,1,1/0)", 1.0}, // lazy: the false branch is not evaluated
		{"=IF({1,0,1},\"a\",\"b\")", ftest.Arr{{"a", "b", "a"}}},
		{"=IF(A1:A3>1,A1:A3*10,0)", ftest.Arr{{0.0}, {20.0}, {30.0}}},
		{"=IFS(A1>5,\"big\",A1>0,\"pos\")", "pos"},
		{"=IFS(FALSE,1)", f.ErrNA},
		{"=SWITCH(2,1,\"one\",2,\"two\",\"other\")", "two"},
		{"=SWITCH(9,1,\"one\",\"other\")", "other"},
		{"=SWITCH(9,1,\"one\")", f.ErrNA},
		{"=IFERROR(1/0,\"div\")", "div"},
		{"=IFERROR(5,\"div\")", 5.0},
		{"=IFERROR({1,#N/A},0)", ftest.Arr{{1.0, 0.0}}},
		{"=IFNA(#N/A,\"na\")", "na"},
		{"=IFNA(1/0,\"na\")", f.ErrDiv0},
		{"=AND(TRUE,1,A1)", true},
		{"=AND(A1:C1)", true}, // text in the range is ignored
		{"=AND(A2:C2)", false},
		{"=AND(\"x\")", f.ErrValue},
		{"=AND(B2)", f.ErrValue}, // no logical values
		{"=OR(FALSE,0,A2:C2)", true},
		{"=XOR(TRUE,TRUE,TRUE)", true},
		{"=XOR(TRUE,FALSE,TRUE)", false},
		{"=NOT(0)", true},
		{"=NOT({1,0})", ftest.Arr{{false, true}}},
		{"=TRUE()", true},
		{"=FALSE()", false},
		{"=LET(x,2,y,x*3,x+y)", 8.0},
		{"=LET(f,LAMBDA(n,n*n),f(4))", 16.0},
		{"=LAMBDA(a,b,a-b)(10,3)", 7.0},
		{"=LAMBDA(x,x)", f.ErrCalc},
		{"=MAP({1,2,3},LAMBDA(x,x*2))", ftest.Arr{{2.0, 4.0, 6.0}}},
		{"=MAP({1,2},{10,20},LAMBDA(a,b,a+b))", ftest.Arr{{11.0, 22.0}}},
		{"=REDUCE(0,{1,2,3,4},LAMBDA(acc,v,acc+v))", 10.0},
		{"=SCAN(0,{1,2,3},LAMBDA(acc,v,acc+v))", ftest.Arr{{1.0, 3.0, 6.0}}},
		{"=BYROW({1,2;3,4},LAMBDA(r,REDUCE(0,r,LAMBDA(a,v,a+v))))", ftest.Arr{{3.0}, {7.0}}},
		{"=BYCOL({1,2;3,4},LAMBDA(c,REDUCE(1,c,LAMBDA(a,v,a*v))))", ftest.Arr{{3.0, 8.0}}},
		{"=MAKEARRAY(2,3,LAMBDA(r,c,r*10+c))", ftest.Arr{{11.0, 12.0, 13.0}, {21.0, 22.0, 23.0}}},
		{"=BYROW({1,2;3,4},LAMBDA(r,r))", ftest.Arr{{f.ErrCalc}, {f.ErrCalc}}},
		// Functions named as values (eta-reduced lambdas).
		{"=BYROW({TRUE,FALSE;TRUE,TRUE},AND)", ftest.Arr{{false}, {true}}},
		{"=MAP({TRUE,FALSE},NOT)", ftest.Arr{{false, true}}},
		{"=REDUCE(FALSE,{FALSE,TRUE},OR)", true},
		{"=SCAN(FALSE,{TRUE,TRUE},XOR)", ftest.Arr{{true, false}}},
		{"=MAP({1,2},NOT,NOT)", ftest.Arr{{f.ErrValue, f.ErrValue}}}, // NOT takes one argument
		{"=MAP({1,2},IF)", ftest.Arr{{true, true}}},                  // IF is a lazy function value
		{"=MAP({1,0},{\"a\",\"a\"},{\"b\",\"b\"},IF)", ftest.Arr{{"a", "b"}}},
		{"=MAP({1,2},{\"a\",\"b\"},IFERROR)", ftest.Arr{{1.0, 2.0}}},
		{"=AND", f.ErrCalc},
	})
}
