// Tests for the basic math and trig functions, plus the shared test
// helpers (registry, near, between, arrayOf).

package mathtrig

import (
	"fmt"
	"math"
	"strings"
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// registry returns a fresh registry holding this package's functions.
func registry() *f.Registry {
	r := f.NewRegistry()
	Register(r)
	return r
}

// near matches a number within 1e-6 of v.
func near(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 1e-6} }

// between checks a number in [lo, hi], optionally an integer.
func between(lo, hi float64, integer bool) func(f.Value) error {
	return func(v f.Value) error {
		n, ok := v.(f.Number)
		x := float64(n)
		if !ok || x < lo || x > hi || (integer && x != math.Trunc(x)) {
			return fmt.Errorf("got %s, want a number in [%v, %v] (integer %v)", ftest.Show(v), lo, hi, integer)
		}
		return nil
	}
}

// arrayOf checks an array's shape and every element.
func arrayOf(rows, cols int, el func(f.Value) error) func(f.Value) error {
	return func(v f.Value) error {
		a, ok := v.(*f.Array)
		if !ok || a.Rows != rows || a.Cols != cols {
			return fmt.Errorf("got %s, want a %d×%d array", ftest.Show(v), rows, cols)
		}
		for _, x := range a.V {
			if err := el(x); err != nil {
				return err
			}
		}
		return nil
	}
}

// TestBasic checks the basic math functions against Excel's documented
// results and domain errors.
func TestBasic(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{24, 36, 12}, {nil, "x", true}})
	}, []ftest.Case{
		{"=ABS(-4)", 4},
		{"=ABS(\"-2\")", 2},
		{"=ABS(\"x\")", f.ErrValue},
		{"=SIGN(10)", 1},
		{"=SIGN(4-4)", 0},
		{"=SIGN(-0.00001)", -1},
		{"=EXP(1)", near(2.718281828)},
		{"=EXP(2)", near(7.389056099)},
		{"=EXP(1000)", f.ErrNum},
		{"=LN(86)", near(4.454347296)},
		{"=LN(EXP(3))", 3},
		{"=LN(0)", f.ErrNum},
		{"=LOG(10)", 1},
		{"=LOG(8,2)", 3},
		{"=LOG(86,2.7182818)", near(4.4543473)},
		{"=LOG(1,1)", f.ErrDiv0},
		{"=LOG(-1)", f.ErrNum},
		{"=LOG(10,0)", f.ErrNum},
		{"=LOG10(86)", near(1.934498451)},
		{"=LOG10(1E5)", 5},
		{"=LOG10(0)", f.ErrNum},
		{"=SQRT(16)", 4},
		{"=SQRT(-16)", f.ErrNum},
		{"=SQRT(ABS(-16))", 4},
		{"=SQRTPI(1)", near(1.772453851)},
		{"=SQRTPI(2)", near(2.506628275)},
		{"=SQRTPI(-1)", f.ErrNum},
		{"=POWER(5,2)", 25},
		{"=POWER(98.6,3.2)", ftest.Approx{V: 2401077.222, Tol: 1e-3}},
		{"=POWER(4,5/4)", near(5.656854249)},
		{"=POWER(0,0)", f.ErrNum},
		{"=POWER(0,-1)", f.ErrDiv0},
		{"=POWER(-8,1/3)", f.ErrNum},
		{"=POWER(-2,3)", -8},
		{"=PI()", math.Pi},
		{"=MOD(3,2)", 1},
		{"=MOD(-3,2)", 1},
		{"=MOD(3,-2)", -1},
		{"=MOD(-3,-2)", -1},
		{"=MOD(1,0)", f.ErrDiv0},
		{"=MOD(5.5,2)", 1.5},
		{"=QUOTIENT(5,2)", 2},
		{"=QUOTIENT(4.5,3.1)", 1},
		{"=QUOTIENT(-10,3)", -3},
		{"=QUOTIENT(1,0)", f.ErrDiv0},
		{"=FACT(5)", 120},
		{"=FACT(1.9)", 1},
		{"=FACT(0)", 1},
		{"=FACT(1)", 1},
		{"=FACT(-1)", f.ErrNum},
		{"=FACT(170)", near(7.257415615307994e306)},
		{"=FACT(171)", f.ErrNum},
		{"=FACTDOUBLE(6)", 48},
		{"=FACTDOUBLE(7)", 105},
		{"=FACTDOUBLE(0)", 1},
		{"=FACTDOUBLE(-1)", 1},
		{"=FACTDOUBLE(-2)", f.ErrNum},
		{"=COMBIN(8,2)", 28},
		{"=COMBIN(8.9,2.1)", 28},
		{"=COMBIN(10,0)", 1},
		{"=COMBIN(2,8)", f.ErrNum},
		{"=COMBIN(-1,0)", f.ErrNum},
		{"=COMBIN(60,30)", 118264581564861424.0},
		{"=COMBINA(4,3)", 20},
		{"=COMBINA(10,3)", 220},
		{"=COMBINA(2,3)", 4},
		{"=COMBINA(0,0)", 1},
		{"=COMBINA(-1,2)", f.ErrNum},
		{"=GCD(5,2)", 1},
		{"=GCD(24,36)", 12},
		{"=GCD(7,1)", 1},
		{"=GCD(5,0)", 5},
		{"=GCD(24.9,36)", 12},
		{"=GCD(A1:C1)", 12},
		{"=GCD(A1:A2,36)", 12}, // blank cells are skipped
		{"=GCD({24,36,12})", 12},
		{"=GCD(-1,2)", f.ErrNum},
		{"=GCD(A2:B2)", f.ErrValue}, // text in a range
		{"=GCD(\"x\")", f.ErrValue},
		{"=GCD(2^53)", f.ErrNum},
		{"=LCM(5,2)", 10},
		{"=LCM(24,36)", 72},
		{"=LCM(A1:C1)", 72},
		{"=LCM(3,0)", 0},
		{"=LCM(-3,2)", f.ErrNum},
		{"=MULTINOMIAL(2,3,4)", 1260},
		{"=MULTINOMIAL(A1:A1,1)", 25},
		{"=MULTINOMIAL(-1)", f.ErrNum},
		{"=SERIESSUM(PI()/4,0,2,{1,-0.5,0.041666667,-0.001388889})", near(0.707103215)},
		{"=SERIESSUM(2,1,1,{1,1,1})", 14},
		{"=SERIESSUM(2,1,1,{1,\"a\"})", f.ErrValue},
		{"=SERIESSUM(2,1,1,A1:C1)", 24*2 + 36*4 + 12*8},
		{"=RAND()", between(0, 1, false)},
		{"=RANDBETWEEN(1,6)", between(1, 6, true)},
		{"=RANDBETWEEN(-3,-3)", -3},
		{"=RANDBETWEEN(1.5,2.5)", 2},
		{"=RANDBETWEEN(5,1)", f.ErrNum},
		{"=RANDARRAY()", between(0, 1, false)},
		{"=RANDARRAY(2,3)", arrayOf(2, 3, between(0, 1, false))},
		{"=RANDARRAY(4,1,1,10,TRUE)", arrayOf(4, 1, between(1, 10, true))},
		{"=RANDARRAY(2,2,-5,-2)", arrayOf(2, 2, between(-5, -2, false))},
		{"=RANDARRAY(1,1,5,1)", f.ErrValue},
		{"=RANDARRAY(2,2,1.5,3,TRUE)", f.ErrValue},
		{"=RANDARRAY(0,2)", f.ErrCalc},
		{"=RANDARRAY(-1)", f.ErrValue},
		{"=SEQUENCE(2,3)", ftest.Arr{{1, 2, 3}, {4, 5, 6}}},
		{"=SEQUENCE(3,1,0,2)", ftest.Arr{{0}, {2}, {4}}},
		{"=SEQUENCE(4)", ftest.Arr{{1}, {2}, {3}, {4}}},
		{"=SEQUENCE(1,3,10,-5)", ftest.Arr{{10, 5, 0}}},
		{"=SEQUENCE(2,,5)", ftest.Arr{{5}, {6}}},
		{"=SEQUENCE(1,1,7)", 7},
		{"=SEQUENCE(2.9,1)", ftest.Arr{{1}, {2}}},
		{"=SEQUENCE(0)", f.ErrCalc},
		{"=SEQUENCE(-1)", f.ErrValue},
		{"=SEQUENCE(\"x\")", f.ErrValue},
		{"=ROMAN(499,0)", "CDXCIX"},
		{"=ROMAN(499,1)", "LDVLIV"},
		{"=ROMAN(499,2)", "XDIX"},
		{"=ROMAN(499,3)", "VDIV"},
		{"=ROMAN(499,4)", "ID"},
		{"=ROMAN(499,TRUE)", "CDXCIX"},
		{"=ROMAN(499,FALSE)", "ID"},
		{"=ROMAN(499)", "CDXCIX"},
		{"=ROMAN(1999)", "MCMXCIX"},
		{"=ROMAN(1999,4)", "MIM"},
		{"=ROMAN(3999)", "MMMCMXCIX"},
		{"=ROMAN(2026)", "MMXXVI"},
		{"=ROMAN(45,1)", "VL"},
		{"=ROMAN(0)", ""},
		{"=ROMAN(4.9)", "IV"},
		{"=ROMAN(4000)", f.ErrValue},
		{"=ROMAN(-1)", f.ErrValue},
		{"=ROMAN(1,5)", f.ErrValue},
		{"=ARABIC(\"LVII\")", 57},
		{"=ARABIC(\"mcmxii\")", 1912},
		{"=ARABIC(\"MMXXVI\")", 2026},
		{"=ARABIC(\"  XC \")", 90},
		{"=ARABIC(\"-MMXI\")", -2011},
		{"=ARABIC(\"ID\")", 499},
		{"=ARABIC(\"\")", 0},
		{"=ARABIC(\"ABC\")", f.ErrValue},
		{"=ARABIC(ROMAN(1234,3))", 1234},
		{"=BASE(7,2)", "111"},
		{"=BASE(100,16)", "64"},
		{"=BASE(15,2,10)", "0000001111"},
		{"=BASE(0,2)", "0"},
		{"=BASE(255,36)", "73"},
		{"=BASE(-1,2)", f.ErrNum},
		{"=BASE(1,37)", f.ErrNum},
		{"=BASE(1,1)", f.ErrNum},
		{"=BASE(1,2,256)", f.ErrNum},
		{"=DECIMAL(\"FF\",16)", 255},
		{"=DECIMAL(111,2)", 7},
		{"=DECIMAL(\"zap\",36)", 45745},
		{"=DECIMAL(\"\",2)", 0},
		{"=DECIMAL(\"12\",2)", f.ErrNum},
		{"=DECIMAL(\"1\",37)", f.ErrNum},
		{"=DECIMAL(BASE(123456,7),7)", 123456},
	})
}

// TestTrig checks the trigonometric and hyperbolic functions and their
// domain errors.
func TestTrig(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=SIN(PI())", near(0)},
		{"=SIN(PI()/2)", 1},
		{"=SIN(30*PI()/180)", near(0.5)},
		{"=SIN(RADIANS(30))", near(0.5)},
		{"=SIN(2^27)", f.ErrNum},
		{"=COS(1.047)", near(0.500171075)},
		{"=COS(60*PI()/180)", near(0.5)},
		{"=TAN(0.785)", near(0.99920399)},
		{"=TAN(45*PI()/180)", near(1)},
		{"=COT(30)", near(-0.156119952)},
		{"=COT(0)", f.ErrDiv0},
		{"=CSC(15)", near(1.537780562)},
		{"=CSC(0)", f.ErrDiv0},
		{"=SEC(45)", near(1.903594407)},
		{"=SEC(0)", 1},
		{"=SINH(1)", near(1.175201194)},
		{"=COSH(4)", near(27.30823284)},
		{"=COSH(EXP(1))", near(7.610125139)},
		{"=COSH(1000)", f.ErrNum},
		{"=TANH(-2)", near(-0.96402758)},
		{"=TANH(0.5)", near(0.462117157)},
		{"=COTH(2)", near(1.037314721)},
		{"=COTH(0)", f.ErrDiv0},
		{"=CSCH(1.5)", near(0.469642441)},
		{"=CSCH(0)", f.ErrDiv0},
		{"=SECH(45)", near(5.725037e-20)},
		{"=ASIN(-0.5)", near(-0.523598776)},
		{"=DEGREES(ASIN(-0.5))", near(-30)},
		{"=ASIN(2)", f.ErrNum},
		{"=ACOS(-0.5)", near(2.094395102)},
		{"=DEGREES(ACOS(-0.5))", near(120)},
		{"=ACOS(1.1)", f.ErrNum},
		{"=ATAN(1)", near(0.785398163)},
		{"=ACOT(2)", near(0.463647609)},
		{"=ACOT(0)", near(math.Pi / 2)},
		{"=ASINH(-2.5)", near(-1.647231146)},
		{"=ASINH(10)", near(2.99822295)},
		{"=ACOSH(1)", 0},
		{"=ACOSH(10)", near(2.993222846)},
		{"=ACOSH(0.5)", f.ErrNum},
		{"=ATANH(0.76159416)", near(1.00000)},
		{"=ATANH(-0.1)", near(-0.100335348)},
		{"=ATANH(1)", f.ErrNum},
		{"=ACOTH(6)", near(0.168236118)},
		{"=ACOTH(0.5)", f.ErrNum},
		{"=ATAN2(1,1)", near(0.785398163)},
		{"=ATAN2(-1,-1)", near(-2.35619449)},
		{"=ATAN2(-1,0)", near(math.Pi)},
		{"=ATAN2(0,0)", f.ErrDiv0},
		{"=DEGREES(PI())", 180},
		{"=RADIANS(270)", near(4.712389)},
		{"=SIN({0,1})", ftest.Arr{{0, near(0.841470985)}}},
	})
}

// TestRegistered checks that Register adds the expected names and marks
// the random functions volatile.
func TestRegistered(t *testing.T) {
	r := registry()
	for _, name := range strings.Fields(`ABS ACOS ACOSH ACOT ACOTH AGGREGATE ARABIC ASIN ASINH ATAN ATAN2
		ATANH BASE CEILING CEILING.MATH CEILING.PRECISE COMBIN COMBINA COS COSH COT COTH CSC CSCH
		DECIMAL DEGREES EVEN EXP FACT FACTDOUBLE FLOOR FLOOR.MATH FLOOR.PRECISE GCD INT ISO.CEILING
		LCM LN LOG LOG10 MDETERM MINVERSE MMULT MOD MROUND MULTINOMIAL MUNIT ODD PERCENTOF PI POWER
		PRODUCT QUOTIENT RADIANS RAND RANDARRAY RANDBETWEEN ROMAN ROUND ROUNDDOWN ROUNDUP SEC SECH
		SERIESSUM SEQUENCE SIGN SIN SINH SQRT SQRTPI SUBTOTAL SUM SUMIF SUMIFS SUMPRODUCT SUMSQ
		SUMX2MY2 SUMX2PY2 SUMXMY2 TAN TANH TRUNC
		COUNT COUNTA COUNTBLANK COUNTIF COUNTIFS AVERAGE AVERAGEA AVERAGEIF AVERAGEIFS MIN MAX MINA
		MAXA MINIFS MAXIFS MEDIAN MODE MODE.SNGL STDEV.S STDEV.P VAR.S VAR.P STDEV STDEVP VAR VARP
		LARGE SMALL PERCENTILE.INC PERCENTILE.EXC QUARTILE.INC QUARTILE.EXC PERCENTILE QUARTILE`) {
		if r.Lookup(name) == nil {
			t.Errorf("%s is not registered", name)
		}
	}
	for _, name := range []string{"RAND", "RANDARRAY", "RANDBETWEEN"} {
		if !r.Lookup(name).Volatile {
			t.Errorf("%s should be volatile", name)
		}
	}
}
