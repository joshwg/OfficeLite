package financial

import (
	"math"

	f "officelite/excel/formula"
)

// DOLLARDE / DOLLARFR (fractional dollar prices) and the Treasury-bill
// functions TBILLEQ, TBILLPRICE and TBILLYIELD.

func registerSecurities(r *f.Registry) {
	r.Add(
		numSpec("DOLLARDE", 2, 2, func(x []float64) f.Value { return dollar(x[0], x[1], true) }),
		numSpec("DOLLARFR", 2, 2, func(x []float64) f.Value { return dollar(x[0], x[1], false) }),
		f.Spec{Name: "TBILLEQ", Min: 3, Max: 3, Fn: func(e *f.Env, a []f.Value) f.Value {
			return tbill(e, a, func(dsm, disc float64) float64 {
				if dsm <= 182 {
					return 365 * disc / (360 - disc*dsm)
				}
				// Over half a year: the bond-equivalent yield with one
				// coupon reinvested.
				price := 100 * (1 - disc*dsm/360)
				t := dsm / 365
				return (-t + math.Sqrt(t*t-(2*t-1)*(1-100/price))) / (t - 0.5)
			})
		}},
		f.Spec{Name: "TBILLPRICE", Min: 3, Max: 3, Fn: func(e *f.Env, a []f.Value) f.Value {
			return tbill(e, a, func(dsm, disc float64) float64 { return 100 * (1 - disc*dsm/360) })
		}},
		f.Spec{Name: "TBILLYIELD", Min: 3, Max: 3, Fn: func(e *f.Env, a []f.Value) f.Value {
			return tbill(e, a, func(dsm, pr float64) float64 { return (100 - pr) / pr * 360 / dsm })
		}},
	)
}

// dollar converts between a decimal price and one whose fractional part
// counts 1/fraction units (DOLLARFR(1.125,16) = 1.02: 1 and 2/16).
func dollar(x, fraction float64, toDecimal bool) f.Value {
	fraction = math.Trunc(fraction)
	if fraction < 0 {
		return f.ErrNum
	}
	if fraction < 1 {
		return f.ErrDiv0
	}
	scale := math.Pow(10, math.Ceil(math.Log10(fraction)))
	whole := math.Trunc(x)
	part := x - whole
	if toDecimal {
		return f.NumberOrErr(whole + part*scale/fraction)
	}
	return f.NumberOrErr(whole + part*fraction/scale)
}

// tbill checks TBILL*(settlement, maturity, x) — dates truncated,
// maturity after settlement and at most a year later, x > 0 — and applies
// fn to the days to maturity and x.
func tbill(e *f.Env, a []f.Value, fn func(dsm, x float64) float64) f.Value {
	v, err := nums(a, 3)
	if err != nil {
		return err
	}
	settle, mat, x := math.Floor(v[0]), math.Floor(v[1]), v[2]
	if settle < 0 || mat <= settle || x <= 0 {
		return f.ErrNum
	}
	y, m, d := f.SerialToYMD(settle, e.Date1904())
	if limit, ok := f.DateSerial(y+1, m, d, e.Date1904()); !ok || mat > limit {
		return f.ErrNum
	}
	res := fn(mat-settle, x)
	if res <= 0 {
		return f.ErrNum
	}
	return f.NumberOrErr(res)
}
