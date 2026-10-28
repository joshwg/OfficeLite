package financial

import (
	"math"
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

// Expected values from Microsoft's documentation examples (shown there to
// 2-10 digits, hence the tolerances) plus the edge cases.
func TestLoans(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	approx := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 5e-9 * max(1, abs(v))} }
	ftest.Run(t, r, nil, []ftest.Case{
		{"=PV(0.08/12,12*20,500,,0)", approx(-59777.145851188)},
		{"=PV(0,10,-100)", 1000},
		{"=PV(0,10,-100,-50)", 1050},
		{"=PV(0.1,5,-100,0,1)", approx(416.986544634929)},
		{"=PV(0.1,5,-100,0,7)", approx(416.986544634929)}, // any non-zero type is 1
		{"=PV(0.1,5,\"x\")", f.ErrValue},
		{"=FV(0.1,5,-100)-PV(0.1,5,-100)*1.1^5", approx(0)}, // PV is FV discounted

		{"=NPER(0.12/12,-100,-1000,10000,1)", approx(59.6738657)},
		{"=NPER(0.12/12,-100,-1000,10000)", approx(60.0821229)},
		{"=NPER(0.12/12,-100,-1000)", approx(-9.57859404)},
		{"=NPER(0,-100,1000)", 10},
		{"=NPER(0,0,100)", f.ErrNum},
		{"=NPER(0.1,-10,1000)", f.ErrNum}, // the payment never covers the interest
		{"=NPER(-1,-10,1000)", f.ErrNum},

		{"=RATE(4*12,-200,8000)", approx(0.00770147248820203)},
		{"=RATE(4*12,-200,8000)*12", approx(0.0924176698584244)},
		{"=RATE(10,0,-100,200)", approx(math.Pow(2, 0.1) - 1)},
		{"=RATE(10,-100,1000)", approx(0)},
		{"=PV(RATE(10,-120,1000,0,1),10,-120,0,1)", approx(1000)},
		{"=PV(RATE(10,-120,1000,0,1,0.05),10,-120,0,1)", approx(1000)},
		{"=RATE(0,-100,1000)", f.ErrNum},
		{"=RATE(10,100,1000)", f.ErrNum}, // no rate makes these flows balance
		{"=RATE(\"x\",-100,1000)", f.ErrValue},

		{"=IPMT(0.1/12,1,3*12,8000)", approx(-66.6666666666667)},
		{"=IPMT(0.1,3,3,8000)", approx(-292.447129909365)},
		{"=IPMT(0.1,3.9,3,8000)", approx(-292.447129909365)}, // per is truncated
		{"=IPMT(0.1,1,3,8000,0,1)", 0},
		{"=IPMT(0,2,3,8000)", 0},
		{"=IPMT(0.1,0.5,3,8000)", f.ErrNum},
		{"=IPMT(0.1,4,3,8000)", f.ErrNum},
		{"=PPMT(0.1/12,1,2*12,2000)", approx(-75.6231860083663)},
		{"=PPMT(0.08,10,10,200000)", approx(-27598.0534624214)},
		{"=PPMT(0.1,1,3,8000,0,1)", approx(-2924.47129909366)},
		{"=PPMT(0.1,2,3,8000)+IPMT(0.1,2,3,8000)-PMT(0.1,3,8000)", approx(0)},
		{"=PPMT(0.1,0,3,8000)", f.ErrNum},

		{"=CUMIPMT(0.09/12,30*12,125000,13,24,0)", approx(-11135.2321307508)},
		{"=CUMIPMT(0.09/12,30*12,125000,1,1,0)", -937.5},
		{"=CUMIPMT(0.09/12,360,125000,13.9,24.9,0)", approx(-11135.2321307508)}, // periods truncated
		{"=CUMIPMT(0.09/12,360,125000,1,2,1)", approx(-930.012816839844)},
		{"=CUMPRINC(0.09/12,30*12,125000,13,24,0)", approx(-934.107123420897)},
		{"=CUMPRINC(0.09/12,30*12,125000,1,1,0)", approx(-68.2782711809783)},
		{"=CUMPRINC(0.09/12,360,125000,1,360,0)", approx(-125000)},
		{"=CUMIPMT(0,360,125000,1,2,0)", f.ErrNum},
		{"=CUMIPMT(0.01,360,-125000,1,2,0)", f.ErrNum},
		{"=CUMIPMT(0.01,360,125000,0,2,0)", f.ErrNum},
		{"=CUMIPMT(0.01,360,125000,3,2,0)", f.ErrNum},
		{"=CUMIPMT(0.01,360,125000,1,361,0)", f.ErrNum},
		{"=CUMIPMT(0.01,360,125000,1,2,2)", f.ErrNum}, // type must be 0 or 1

		{"=ISPMT(0.1/12,1,36,8000)", approx(-64.8148148148148)},
		{"=ISPMT(0.1,1,3,8000)", approx(-533.333333333333)},
		{"=ISPMT(0.1,0,3,8000)", -800},
		{"=ISPMT(0.1,1,0,8000)", f.ErrDiv0},
	})
}

func TestReturns(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	approx := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 5e-9 * max(1, abs(v))} }
	ftest.Run(t, r, func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{-70000}, {12000}, {15000}, {18000}, {21000}, {26000}})
		ctx.SetRange("B1", [][]any{{-120000}, {39000}, {30000}, {21000}, {37000}, {46000}})
		ctx.SetRange("C1", [][]any{{-10000}, {2750}, {4250}, {3250}, {2750}})
		ctx.SetRange("D1", [][]any{{39448}, {39508}, {39751}, {39859}, {39904}}) // 1/1/2008 ... 4/1/2009
		ctx.SetRange("E1", [][]any{{-100}, {"x"}, {true}, {nil}, {110}})
		ctx.Set("F1", "=1/0")
	}, []ftest.Case{
		{"=IRR(A1:A5)", approx(-0.021244848)},
		{"=IRR(A1:A6)", approx(0.086630948)},
		{"=IRR(A1:A3,-10%)", approx(-0.443506941)},
		{"=IRR({-100,110})", approx(0.1)},
		{"=IRR(E1:E5)", approx(0.1)}, // text, logical values and blanks are skipped
		{"=IRR({1,2,3})", f.ErrNum},  // no sign change
		{"=IRR(A1:A5,-0.99)", f.ErrNum},
		{"=IRR(A1:F1)", f.ErrDiv0},
		{"=IRR(A1:A6,\"x\")", f.ErrValue},

		{"=MIRR(B1:B6,10%,12%)", approx(0.126094130)},
		{"=MIRR(B1:B4,10%,12%)", approx(-0.048044655)},
		{"=MIRR(B1:B6,10%,14%)", approx(0.134759111)},
		{"=MIRR({1,2},0.1,0.1)", f.ErrDiv0},
		{"=MIRR({-1,2},-1,0.1)", f.ErrDiv0},

		{"=XNPV(0.09,C1:C5,D1:D5)", approx(2086.647602)},
		{"=XNPV(0.09,{-100,110},{39448.7,39813.2})", approx(-100 + 110/1.09)}, // dates truncated
		{"=XNPV(0.09,C1:C5,{39448,39508,39400,39859,39904})", f.ErrNum},       // date before the first
		{"=XNPV(0.09,C1:C5,D1:D4)", f.ErrNum},
		{"=XNPV(0.09,E1:E5,D1:D5)", f.ErrValue},
		{"=XNPV(-1,C1:C5,D1:D5)", f.ErrNum},
		{"=XIRR(C1:C5,D1:D5,0.1)", ftest.Approx{V: 0.373362535, Tol: 2e-9}},
		{"=XIRR(C1:C5,D1:D5)", ftest.Approx{V: 0.373362535, Tol: 2e-9}},
		{"=XIRR({-100,110},{39448,39813})", approx(0.1)},
		{"=XIRR({100,110},{39448,39813})", f.ErrNum},
		{"=XIRR(C1:C5,D1:D4)", f.ErrNum},
		{"=XIRR(C1:C5,F1:F5)", f.ErrDiv0},

		{"=FVSCHEDULE(1,{0.09,0.11,0.1})", approx(1.33089)},
		{"=FVSCHEDULE(100,E4:E4)", 100}, // blank cells are 0%
		{"=FVSCHEDULE(100,E2:E3)", f.ErrValue},
		{"=FVSCHEDULE(100,0.1)", approx(110)},

		{"=EFFECT(0.0525,4)", approx(0.0535426674)},
		{"=EFFECT(0.0525,4.9)", approx(0.0535426674)},
		{"=EFFECT(0,4)", f.ErrNum},
		{"=EFFECT(0.05,0.9)", f.ErrNum},
		{"=NOMINAL(0.053543,4)", approx(0.0525003199)},
		{"=NOMINAL(EFFECT(0.07,12),12)", approx(0.07)},
		{"=NOMINAL(-0.01,4)", f.ErrNum},
		{"=RRI(96,10000,11000)", approx(0.000993307376)},
		{"=RRI(0,1,2)", f.ErrNum},
		{"=RRI(2,0,2)", f.ErrNum},
		{"=PDURATION(2.5%,2000,2200)", approx(3.859866163)},
		{"=PDURATION(0.025/12,1000,1200)", approx(87.60547642)},
		{"=PDURATION(0,1,2)", f.ErrNum},
		{"=PDURATION(0.1,-1,2)", f.ErrNum},
	})
}

func TestDepreciation(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	approx := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 5e-9 * max(1, abs(v))} }
	ftest.Run(t, r, nil, []ftest.Case{
		{"=SLN(30000,7500,10)", 2250},
		{"=SLN(1,0,0)", f.ErrDiv0},
		{"=SYD(30000,7500,10,1)", approx(4090.90909090909)},
		{"=SYD(30000,7500,10,10)", approx(409.090909090909)},
		{"=SYD(30000,7500,10,11)", f.ErrNum},
		{"=SYD(30000,7500,0,1)", f.ErrNum},

		{"=DB(1000000,100000,6,1,7)", approx(186083.333333333)},
		{"=DB(1000000,100000,6,2,7)", approx(259639.416666667)},
		{"=DB(1000000,100000,6,3,7)", approx(176814.44275)},
		{"=DB(1000000,100000,6,4,7)", approx(120410.63551275)},
		{"=DB(1000000,100000,6,5,7)", approx(81999.6427841827)},
		{"=DB(1000000,100000,6,6,7)", approx(55841.7567360285)},
		{"=DB(1000000,100000,6,7,7)", approx(15845.0984738481)},
		{"=DB(1000000,100000,6,7.9,7.9)", approx(15845.0984738481)}, // period and month truncated
		{"=DB(1000000,100000,6,1)", 319000},                         // rate 0.319 (rounded)
		{"=DB(1000000,100000,6,8,7)", f.ErrNum},
		{"=DB(1000000,100000,6,1,13)", f.ErrNum},
		{"=DB(1000000,100000,6,1,0)", f.ErrNum},
		{"=DB(0,0,6,1)", f.ErrNum},

		{"=DDB(2400,300,10*365,1)", approx(1.31506849315065)},
		{"=DDB(2400,300,10*12,1,2)", 40},
		{"=DDB(2400,300,10,1,2)", 480},
		{"=DDB(2400,300,10,2,1.5)", approx(306)},
		{"=DDB(2400,300,10,10)", approx(22.1225472)},
		{"=DDB(2400,300,1,1,3)", 2100}, // factor/life >= 1: all in the first period
		{"=DDB(2400,300,10,11)", f.ErrNum},
		{"=DDB(2400,300,10,1,0)", f.ErrNum},
		{"=DDB(2400,-1,10,1)", f.ErrNum},

		{"=VDB(2400,300,10*365,0,1)", approx(1.31506849315065)},
		{"=VDB(2400,300,10*12,0,1)", 40},
		{"=VDB(2400,300,10,0,1)", 480},
		{"=VDB(2400,300,10*12,6,18)", approx(396.306053264751)},
		{"=VDB(2400,300,10*12,6,18,1.5)", approx(311.808936658234)},
		{"=VDB(2400,300,10,0,0.875,1.5)", approx(315)},
		{"=VDB(2400,300,10,0,10)", 2100}, // switches to straight-line and ends at salvage
		{"=VDB(2400,300,10,1,3,2,TRUE)", approx(691.2)},
		{"=VDB(2400,300,10,1.5,3.5,2,TRUE)", approx(622.08)},
		{"=VDB(2400,300,10,2,2)", 0},
		{"=VDB(2400,300,10,11,12)", f.ErrNum},
		{"=VDB(2400,300,10,3,2)", f.ErrNum},
		{"=VDB(2400,300,10,0,1,0)", f.ErrNum},
	})
}

func TestSecurities(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	approx := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 5e-9 * max(1, abs(v))} }
	ftest.Run(t, r, nil, []ftest.Case{
		{"=DOLLARDE(1.02,16)", approx(1.125)},
		{"=DOLLARDE(1.1,32)", approx(1.3125)},
		{"=DOLLARDE(-1.02,16)", approx(-1.125)},
		{"=DOLLARDE(1.5,10)", approx(1.5)},
		{"=DOLLARDE(1.02,16.9)", approx(1.125)},
		{"=DOLLARDE(1.02,0.5)", f.ErrDiv0},
		{"=DOLLARDE(1.02,-1)", f.ErrNum},
		{"=DOLLARFR(1.125,16)", approx(1.02)},
		{"=DOLLARFR(1.125,32)", approx(1.04)},
		{"=DOLLARFR(1.125,0)", f.ErrDiv0},

		// Settlement 3/31/2008 (39538), maturity 6/1/2008 (39600).
		{"=TBILLEQ(39538,39600,9.14%)", approx(0.094151494)},
		{"=TBILLEQ(39538,39800,9%)", approx(0.0962405547997487)}, // over 182 days
		{"=TBILLPRICE(39538,39600,9%)", approx(98.45)},
		{"=TBILLPRICE(39538.9,39600.9,9%)", approx(98.45)},
		{"=TBILLYIELD(39538,39600,98.45)", approx(0.0914169629253426)},
		{"=TBILLPRICE(39538,39903,9%)", approx(100 * (1 - 0.09*365/360))}, // exactly a year
		{"=TBILLPRICE(39538,39904,9%)", f.ErrNum},
		{"=TBILLPRICE(39600,39538,9%)", f.ErrNum},
		{"=TBILLPRICE(39538,39600,0)", f.ErrNum},
		{"=TBILLYIELD(39538,39600,0)", f.ErrNum},
		{"=TBILLEQ(39538,39600,\"x\")", f.ErrValue},
	})
}

func abs(x float64) float64 { return math.Abs(x) }
