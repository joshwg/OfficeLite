package financial

import (
	"math"

	f "officelite/excel/formula"
)

// The return functions: IRR, XNPV, XIRR, MIRR, FVSCHEDULE, EFFECT,
// NOMINAL, RRI and PDURATION.

func registerReturns(r *f.Registry) {
	r.Add(
		f.Spec{Name: "IRR", Min: 1, Max: 2, Refs: f.Only(0), Arrays: f.Only(0), Fn: irr, Format: percent},
		f.Spec{Name: "MIRR", Min: 3, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: mirr, Format: percent},
		f.Spec{Name: "XNPV", Min: 3, Max: 3, Refs: f.Only(1, 2), Arrays: f.Only(1, 2), Fn: xnpv},
		f.Spec{Name: "XIRR", Min: 2, Max: 3, Refs: f.Only(0, 1), Arrays: f.Only(0, 1), Fn: xirr},
		f.Spec{Name: "FVSCHEDULE", Min: 2, Max: 2, Refs: f.Only(1), Arrays: f.Only(1), Fn: fvschedule},
		numSpec("EFFECT", 2, 2, func(x []float64) f.Value {
			nominal, p := x[0], math.Trunc(x[1])
			if nominal <= 0 || p < 1 {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Expm1(p * math.Log1p(nominal/p)))
		}),
		numSpec("NOMINAL", 2, 2, func(x []float64) f.Value {
			effect, p := x[0], math.Trunc(x[1])
			if effect <= 0 || p < 1 {
				return f.ErrNum
			}
			return f.NumberOrErr(p * math.Expm1(math.Log1p(effect)/p))
		}),
		numSpec("RRI", 3, 3, func(x []float64) f.Value { // RRI(nper, pv, fv)
			if x[0] <= 0 || x[1] == 0 {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Pow(x[2]/x[1], 1/x[0]) - 1)
		}),
		numSpec("PDURATION", 3, 3, func(x []float64) f.Value { // PDURATION(rate, pv, fv)
			if x[0] <= 0 || x[1] <= 0 || x[2] <= 0 {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Log(x[2]/x[1]) / math.Log1p(x[0]))
		}),
	)
}

// cashflows collects the numbers of a range or array argument. Other
// values are skipped, or #VALUE! when strict (the Analysis ToolPak
// functions); an error value is returned as is.
func cashflows(e *f.Env, v f.Value, strict bool) ([]float64, f.Value) {
	var out []float64
	var bad f.Value
	e.Each(v, func(x f.Value, direct bool) bool {
		switch x := x.(type) {
		case f.Number:
			out = append(out, float64(x))
		case f.Error:
			bad = x
		default:
			if direct {
				var n float64
				if n, bad = e.Num(x); bad == nil {
					out = append(out, n)
				}
			} else if strict {
				bad = f.ErrValue
			}
		}
		return bad == nil
	})
	return out, bad
}

// mixed reports whether vs has both a positive and a negative value.
func mixed(vs []float64) bool {
	pos, neg := false, false
	for _, v := range vs {
		pos = pos || v > 0
		neg = neg || v < 0
	}
	return pos && neg
}

// guessArg is the optional guess argument i, 10% when omitted.
func guessArg(e *f.Env, a []f.Value, i int) (float64, f.Value) {
	if !given(a, i) {
		return 0.1, nil
	}
	return e.Num(a[i])
}

// IRR(values, [guess]): the rate at which the NPV of values (one per
// period, the first undiscounted) is zero, by Newton's method; #NUM! when
// 20 steps don't converge to 1e-7.
func irr(e *f.Env, a []f.Value) f.Value {
	vs, err := cashflows(e, a[0], false)
	if err != nil {
		return err
	}
	guess, err := guessArg(e, a, 1)
	if err != nil {
		return err
	}
	if !mixed(vs) {
		return f.ErrNum
	}
	r, ok := newton(guess, 20, 1e-7, func(r float64) (y, dy float64) {
		d := 1.0 // (1+r)^-i
		for i, v := range vs {
			y += v * d
			d /= 1 + r
			dy -= float64(i) * v * d
		}
		return y, dy
	})
	if !ok {
		return f.ErrNum
	}
	return f.NumberOrErr(r)
}

// MIRR(values, finance_rate, reinvest_rate): negative cash flows are
// financed at finance_rate and positive ones reinvested at reinvest_rate.
func mirr(e *f.Env, a []f.Value) f.Value {
	vs, err := cashflows(e, a[0], false)
	if err != nil {
		return err
	}
	x, err := nums(e, a[1:], 2)
	if err != nil {
		return err
	}
	fr, rr := x[0], x[1]
	n := len(vs)
	if fr == -1 || rr == -1 || n < 2 {
		return f.ErrDiv0
	}
	var fvPos, pvNeg float64
	for i, v := range vs {
		if v > 0 {
			fvPos += v * math.Pow(1+rr, float64(n-1-i))
		} else if v < 0 {
			pvNeg += v / math.Pow(1+fr, float64(i))
		}
	}
	if fvPos == 0 || pvNeg == 0 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(math.Pow(-fvPos/pvNeg, 1/float64(n-1)) - 1)
}

// datedFlows reads XNPV/XIRR's values and dates: equal counts, dates
// truncated to days and none before the first.
func datedFlows(e *f.Env, values, dates f.Value) (vs, ts []float64, err f.Value) {
	if vs, err = cashflows(e, values, true); err != nil {
		return nil, nil, err
	}
	if ts, err = cashflows(e, dates, true); err != nil {
		return nil, nil, err
	}
	if len(vs) != len(ts) || len(vs) == 0 {
		return nil, nil, f.ErrNum
	}
	d0 := math.Floor(ts[0])
	for i, d := range ts {
		d = math.Floor(d)
		if d < d0 || d < 0 {
			return nil, nil, f.ErrNum
		}
		ts[i] = (d - d0) / 365 // years from the first date
	}
	return vs, ts, nil
}

// xnpvOf returns the NPV of dated flows and its derivative in the rate.
func xnpvOf(r float64, vs, ts []float64) (y, dy float64) {
	for i, v := range vs {
		d := math.Pow(1+r, -ts[i])
		y += v * d
		dy -= ts[i] * v * d / (1 + r)
	}
	return y, dy
}

// XNPV(rate, values, dates)
func xnpv(e *f.Env, a []f.Value) f.Value {
	r, err := e.Num(a[0])
	if err != nil {
		return err
	}
	vs, ts, err := datedFlows(e, a[1], a[2])
	if err != nil {
		return err
	}
	if r <= -1 {
		return f.ErrNum
	}
	y, _ := xnpvOf(r, vs, ts)
	return f.NumberOrErr(y)
}

// XIRR(values, dates, [guess]): the rate at which XNPV is zero; #NUM!
// without both signs among values or when 100 Newton steps don't
// converge.
func xirr(e *f.Env, a []f.Value) f.Value {
	vs, ts, err := datedFlows(e, a[0], a[1])
	if err != nil {
		return err
	}
	guess, err := guessArg(e, a, 2)
	if err != nil {
		return err
	}
	if !mixed(vs) || guess <= -1 {
		return f.ErrNum
	}
	r, ok := newton(guess, 100, 1e-8, func(r float64) (float64, float64) { return xnpvOf(r, vs, ts) })
	if !ok {
		return f.ErrNum
	}
	return f.NumberOrErr(r)
}

// FVSCHEDULE(principal, schedule): principal compounded at each rate of
// schedule; blank cells count as 0% and other non-numbers are #VALUE!.
func fvschedule(e *f.Env, a []f.Value) f.Value {
	p, err := e.Num(a[0])
	if err != nil {
		return err
	}
	var bad f.Value
	e.Each(a[1], func(x f.Value, direct bool) bool {
		switch x := x.(type) {
		case f.Number:
			p *= 1 + float64(x)
		case f.Blank, f.Missing:
		case f.Error:
			bad = x
		default:
			var n float64
			if !direct {
				bad = f.ErrValue
			} else if n, bad = e.Num(x); bad == nil {
				p *= 1 + n
			}
		}
		return bad == nil
	})
	if bad != nil {
		return bad
	}
	return f.NumberOrErr(p)
}
