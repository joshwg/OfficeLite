package financial

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// Expected values are Excel's results, most from Microsoft's documentation.
func TestFinancial(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	approx := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 1e-6} }
	ftest.Run(t, r, func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{8000}, {9200}, {10000}, {12000}, {14500}})
		// Text, logical values and blank cells in a range are skipped.
		ctx.SetRange("B1", [][]any{{-10000}, {"x"}, {3000}, {true}, {nil}, {4200}, {"3"}, {6800}})
		ctx.Set("C1", "=1/0")
	}, []ftest.Case{
		{"=FV(0.06/12,10,-200,-500,1)", approx(2581.403374)},
		{"=FV(0.12/12,12,-1000)", approx(12682.503013)},
		{"=FV(0.11/12,35,-2000,,1)", approx(82846.246372)},
		{"=FV(0.06/12,10,-200,-500,2)", approx(2581.403374)}, // any non-zero type is 1
		{"=FV(0,10,-100,-1000)", 2000},
		{"=FV(0,10,-100,-1000,1)", 2000},
		{"=FV(\"6%\",1,0,-100)", approx(106)},
		{"=FV(\"x\",1,0,-100)", f.ErrValue},
		{"=FV(0.1,1,#N/A)", f.ErrNA},
		{"=FV({0,0.1},1,0,-100)", ftest.Arr{{100, approx(110)}}},

		{"=PMT(0.08/12,10,10000)", approx(-1037.032089)},
		{"=PMT(0.08/12,10,10000,0,1)", approx(-1030.164327)},
		{"=PMT(0.06/12,18*12,0,50000)", approx(-129.081161)},
		{"=PMT(0,10,1000)", -100},
		{"=PMT(0,10,1000,500)", -150},
		{"=PMT(0.1,0,1000)", f.ErrNum},
		{"=PMT(0.1,10,\"x\")", f.ErrValue},

		{"=NPV(0.1,-10000,3000,4200,6800)", approx(1188.443412)},
		{"=NPV(0.08,A1:A5)", approx(41922.061555)},
		{"=NPV(0.08,A1:A5)+-40000", approx(1922.061555)},
		{"=NPV(0.08,A1:A5,-9000)", approx(41922.061555 - 9000/1.08/1.08/1.08/1.08/1.08/1.08)},
		{"=NPV(0.1,B1:B8)", approx(1188.443412)},
		{"=NPV(0.1,B1,B2,B3,B5,B6,B8)", approx(1188.443412)}, // single-cell refs follow range rules
		{"=NPV(0.1,{-10000,3000,4200,6800})", approx(1188.443412)},
		{"=NPV(0.1,\"110\")", approx(100)}, // direct text is coerced
		{"=NPV(0.1,TRUE)", approx(1 / 1.1)},
		{"=NPV(0,1,2,3)", 6},
		{"=NPV(0.1,\"x\")", f.ErrValue},
		{"=NPV(0.1,A1:A5,C1)", f.ErrDiv0},
		{"=NPV(-1,100)", f.ErrDiv0},
		{"=NPV(#REF!,100)", f.ErrRef},
	})
}
