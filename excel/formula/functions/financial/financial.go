// Package financial implements Excel's financial functions FV, NPV and
// PMT. Cash paid out is negative and cash received positive, as in Excel.
package financial

import (
	"math"

	f "officelite/excel/formula"
)

// Register adds the financial functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "FV", Min: 3, Max: 5, Fn: fv},
		f.Spec{Name: "PMT", Min: 3, Max: 5, Fn: pmt},
		f.Spec{Name: "NPV", Min: 2, Max: f.Variadic, Refs: f.From(1), Arrays: f.From(1), Fn: npv},
	)
}

// nums coerces the arguments to numbers; omitted optional ones are 0.
func nums(a []f.Value, n int) ([]float64, f.Value) {
	out := make([]float64, n)
	for i := range a {
		x, err := f.Num(a[i])
		if err != nil {
			return nil, err
		}
		out[i] = x
	}
	return out, nil
}

// due is 1 for payments at the beginning of each period (type <> 0).
func due(t float64) float64 {
	if t != 0 {
		return 1
	}
	return 0
}

// FV(rate, nper, pmt, [pv], [type])
func fv(_ *f.Env, a []f.Value) f.Value {
	x, err := nums(a, 5)
	if err != nil {
		return err
	}
	rate, nper, pay, pv, typ := x[0], x[1], x[2], x[3], due(x[4])
	if rate == 0 {
		return f.NumberOrErr(-(pv + pay*nper))
	}
	g := math.Pow(1+rate, nper)
	return f.NumberOrErr(-(pv*g + pay*(1+rate*typ)*(g-1)/rate))
}

// PMT(rate, nper, pv, [fv], [type])
func pmt(_ *f.Env, a []f.Value) f.Value {
	x, err := nums(a, 5)
	if err != nil {
		return err
	}
	rate, nper, pv, fv, typ := x[0], x[1], x[2], x[3], due(x[4])
	if nper == 0 {
		return f.ErrNum
	}
	if rate == 0 {
		return f.NumberOrErr(-(pv + fv) / nper)
	}
	g := math.Pow(1+rate, nper)
	return f.NumberOrErr(-rate * (fv + pv*g) / ((1 + rate*typ) * (g - 1)))
}

// NPV(rate, value1, [value2], ...): values are discounted one period each,
// in order. Numbers in ranges and arrays count while their text, logical
// values and blank cells are skipped; values typed directly are coerced.
func npv(e *f.Env, a []f.Value) f.Value {
	rate, err := f.Num(a[0])
	if err != nil {
		return err
	}
	if rate == -1 {
		return f.ErrDiv0
	}
	var sum, disc float64 = 0, 1
	var bad f.Value
	for _, v := range a[1:] {
		e.Each(v, func(x f.Value, direct bool) bool {
			var n float64
			if direct {
				if n, bad = f.Num(x); bad != nil {
					return false
				}
			} else {
				switch x := x.(type) {
				case f.Number:
					n = float64(x)
				case f.Error:
					bad = x
					return false
				default:
					return true
				}
			}
			disc *= 1 + rate
			sum += n / disc
			return true
		})
		if bad != nil {
			return bad
		}
	}
	return f.NumberOrErr(sum)
}
