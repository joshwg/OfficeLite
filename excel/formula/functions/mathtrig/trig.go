package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

// trigLimit is the magnitude from which Excel's SIN, COS, TAN (and their
// reciprocals) return #NUM! because the argument has lost all precision.
const trigLimit = 1 << 27

func registerTrig(r *f.Registry) {
	r.Add(
		fn1("SIN", periodic(math.Sin)),
		fn1("COS", periodic(math.Cos)),
		fn1("TAN", periodic(math.Tan)),
		fn1("COT", reciprocal(math.Tan, true)),
		fn1("CSC", reciprocal(math.Sin, true)),
		fn1("SEC", reciprocal(math.Cos, true)),
		fn1("SINH", plain(math.Sinh)),
		fn1("COSH", plain(math.Cosh)),
		fn1("TANH", plain(math.Tanh)),
		fn1("COTH", reciprocal(math.Tanh, false)),
		fn1("CSCH", reciprocal(math.Sinh, false)),
		fn1("SECH", reciprocal(math.Cosh, false)),
		fn1("ASIN", domain(math.Asin, func(x float64) bool { return x < -1 || x > 1 })),
		fn1("ACOS", domain(math.Acos, func(x float64) bool { return x < -1 || x > 1 })),
		fn1("ATAN", plain(math.Atan)),
		fn1("ACOT", plain(func(x float64) float64 { return math.Pi/2 - math.Atan(x) })),
		fn1("ASINH", plain(math.Asinh)),
		fn1("ACOSH", domain(math.Acosh, func(x float64) bool { return x < 1 })),
		fn1("ATANH", domain(math.Atanh, func(x float64) bool { return x <= -1 || x >= 1 })),
		fn1("ACOTH", domain(func(x float64) float64 { return math.Atanh(1 / x) },
			func(x float64) bool { return x >= -1 && x <= 1 })),
		// ATAN2(x_num, y_num): note the x-first argument order.
		fn2("ATAN2", func(x, y float64) f.Value {
			if x == 0 && y == 0 {
				return f.ErrDiv0
			}
			return f.Number(math.Atan2(y, x))
		}),
		fn1("DEGREES", plain(func(x float64) float64 { return x * 180 / math.Pi })),
		fn1("RADIANS", plain(func(x float64) float64 { return x * math.Pi / 180 })),
	)
}

func periodic(g func(float64) float64) func(float64) f.Value {
	return func(x float64) f.Value {
		if math.Abs(x) >= trigLimit {
			return f.ErrNum
		}
		return f.NumberOrErr(g(x))
	}
}

// reciprocal builds 1/g(x); g(x) == 0 is #DIV/0!.
func reciprocal(g func(float64) float64, limited bool) func(float64) f.Value {
	return func(x float64) f.Value {
		if limited && math.Abs(x) >= trigLimit {
			return f.ErrNum
		}
		d := g(x)
		if d == 0 {
			return f.ErrDiv0
		}
		return f.NumberOrErr(1 / d)
	}
}

func domain(g func(float64) float64, bad func(float64) bool) func(float64) f.Value {
	return func(x float64) f.Value {
		if bad(x) {
			return f.ErrNum
		}
		return f.NumberOrErr(g(x))
	}
}
