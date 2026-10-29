// Package financial implements Excel's financial functions: loans and
// annuities (PV, FV, PMT, NPER, RATE, IPMT, PPMT, CUMIPMT, CUMPRINC,
// ISPMT), returns (NPV, IRR, XNPV, XIRR, MIRR, FVSCHEDULE, EFFECT,
// NOMINAL, RRI, PDURATION), depreciation (SLN, SYD, DB, DDB, VDB), DOLLARDE
// / DOLLARFR and the Treasury-bill functions. Cash paid out is negative and
// cash received positive, as in Excel.
package financial

import (
	"math"

	f "officelite/excel/formula"
)

// The number formats Excel gives these results in a General cell.
const (
	currency = `"$"#,##0.00_);[Red]\("$"#,##0.00\)`
	percent  = "0%"
)

// Register adds the financial functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "FV", Min: 3, Max: 5, Fn: fv, Format: currency},
		f.Spec{Name: "PV", Min: 3, Max: 5, Fn: pv, Format: currency},
		f.Spec{Name: "PMT", Min: 3, Max: 5, Fn: pmt, Format: currency},
		f.Spec{Name: "NPER", Min: 3, Max: 5, Fn: nper},
		f.Spec{Name: "RATE", Min: 3, Max: 6, Fn: rate, Format: percent},
		f.Spec{Name: "IPMT", Min: 4, Max: 6, Fn: func(e *f.Env, a []f.Value) f.Value { return ipmtFn(e, a, false) }, Format: currency},
		f.Spec{Name: "PPMT", Min: 4, Max: 6, Fn: func(e *f.Env, a []f.Value) f.Value { return ipmtFn(e, a, true) }, Format: currency},
		f.Spec{Name: "CUMIPMT", Min: 6, Max: 6, Fn: func(e *f.Env, a []f.Value) f.Value { return cumipmt(e, a, false) }},
		f.Spec{Name: "CUMPRINC", Min: 6, Max: 6, Fn: func(e *f.Env, a []f.Value) f.Value { return cumipmt(e, a, true) }},
		f.Spec{Name: "ISPMT", Min: 4, Max: 4, Fn: ispmt},
		f.Spec{Name: "NPV", Min: 2, Max: f.Variadic, Refs: f.From(1), Arrays: f.From(1), Fn: npv, Format: currency},
	)
	registerReturns(r)
	registerDepreciation(r)
	registerSecurities(r)
}

// nums coerces the arguments to numbers (date text in the workbook's
// date system); omitted optional ones are 0.
func nums(e *f.Env, a []f.Value, n int) ([]float64, f.Value) {
	out := make([]float64, n)
	for i := range a {
		x, err := e.Num(a[i])
		if err != nil {
			return nil, err
		}
		out[i] = x
	}
	return out, nil
}

// given reports whether optional argument i was supplied.
func given(a []f.Value, i int) bool {
	if i >= len(a) {
		return false
	}
	_, missing := a[i].(f.Missing)
	return !missing
}

// numSpec is a function of numeric arguments only.
func numSpec(name string, min, max int, fn func(x []float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: min, Max: max, Fn: func(e *f.Env, a []f.Value) f.Value {
		x, err := nums(e, a, max)
		if err != nil {
			return err
		}
		return fn(x)
	}}
}

// due is 1 for payments at the beginning of each period (type <> 0).
func due(t float64) float64 {
	if t != 0 {
		return 1
	}
	return 0
}

// growth returns (1+rate)^nper and (1+rate)^nper - 1, the latter without
// cancellation for small rates.
func growth(rate, nper float64) (g, gm1 float64) {
	if rate <= -1 {
		g = math.Pow(1+rate, nper)
		return g, g - 1
	}
	l := nper * math.Log1p(rate)
	return math.Exp(l), math.Expm1(l)
}

// fvOf is FV(rate, nper, pmt, pv, type).
func fvOf(rate, nper, pay, pv, typ float64) float64 {
	if rate == 0 {
		return -(pv + pay*nper)
	}
	g, gm1 := growth(rate, nper)
	return -(pv*g + pay*(1+rate*typ)*gm1/rate)
}

// pmtOf is PMT(rate, nper, pv, fv, type) for nper <> 0.
func pmtOf(rate, nper, pv, fv, typ float64) float64 {
	if rate == 0 {
		return -(pv + fv) / nper
	}
	g, gm1 := growth(rate, nper)
	return -rate * (fv + pv*g) / ((1 + rate*typ) * gm1)
}

// FV(rate, nper, pmt, [pv], [type])
func fv(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 5)
	if err != nil {
		return err
	}
	return f.NumberOrErr(fvOf(x[0], x[1], x[2], x[3], due(x[4])))
}

// PV(rate, nper, pmt, [fv], [type])
func pv(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 5)
	if err != nil {
		return err
	}
	rate, nper, pay, fv, typ := x[0], x[1], x[2], x[3], due(x[4])
	if rate == 0 {
		return f.NumberOrErr(-(fv + pay*nper))
	}
	g, gm1 := growth(rate, nper)
	return f.NumberOrErr(-(fv + pay*(1+rate*typ)*gm1/rate) / g)
}

// PMT(rate, nper, pv, [fv], [type])
func pmt(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 5)
	if err != nil {
		return err
	}
	if x[1] == 0 {
		return f.ErrNum
	}
	return f.NumberOrErr(pmtOf(x[0], x[1], x[2], x[3], due(x[4])))
}

// NPER(rate, pmt, pv, [fv], [type])
func nper(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 5)
	if err != nil {
		return err
	}
	rate, pay, pv, fv, typ := x[0], x[1], x[2], x[3], due(x[4])
	if rate == 0 {
		if pay == 0 {
			return f.ErrNum
		}
		return f.NumberOrErr(-(pv + fv) / pay)
	}
	if rate <= -1 {
		return f.ErrNum
	}
	k := pay * (1 + rate*typ) / rate
	q := (k - fv) / (k + pv)
	if !(q > 0) {
		return f.ErrNum
	}
	return f.NumberOrErr(math.Log(q) / math.Log1p(rate))
}

// RATE(nper, pmt, pv, [fv], [type], [guess]) solves the annuity equation
// by Newton's method from guess (10%); like Excel it gives up with #NUM!
// when 20 steps don't bring successive results within 1e-7.
func rate(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 6)
	if err != nil {
		return err
	}
	n, pay, pv, fv, typ := x[0], x[1], x[2], x[3], due(x[4])
	r := 0.1
	if given(a, 5) {
		r = x[5]
	}
	if n <= 0 {
		return f.ErrNum
	}
	// y(r) = pv*(1+r)^n + pay*(1+r*typ)*((1+r)^n-1)/r + fv and its slope.
	eval := func(r float64) (y, dy float64) {
		if math.Abs(r) < 1e-12 {
			return pv + pay*n + fv, n*pv + pay*n*(typ+(n-1)/2)
		}
		g, gm1 := growth(r, n)
		a := gm1 / r              // annuity factor
		da := (n*g/(1+r) - a) / r // its derivative
		y = pv*g + pay*(1+r*typ)*a + fv
		dy = n*pv*g/(1+r) + pay*(typ*a+(1+r*typ)*da)
		return y, dy
	}
	if res, ok := newton(r, 20, 1e-7, eval); ok {
		return f.NumberOrErr(res)
	}
	return f.ErrNum
}

// newton runs Newton's method from x for up to max steps; it succeeds when
// a step is smaller than tol (one more step then polishes the result).
// Rates at or below -100% fail.
func newton(x float64, max int, tol float64, fn func(float64) (y, dy float64)) (float64, bool) {
	for i := 0; i < max; i++ {
		y, dy := fn(x)
		if dy == 0 || math.IsNaN(y) || math.IsNaN(dy) {
			return 0, false
		}
		step := y / dy
		x -= step
		if x <= -1 || math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		if math.Abs(step) < tol {
			if y, dy := fn(x); dy != 0 && !math.IsNaN(y/dy) && x-y/dy > -1 {
				x -= y / dy
			}
			return x, true
		}
	}
	return 0, false
}

// ipmtOf is the interest part of payment per (1-based) of an annuity.
func ipmtOf(rate, per, nper, pv, fv, typ float64) float64 {
	if rate == 0 || (per == 1 && typ == 1) {
		return 0
	}
	pay := pmtOf(rate, nper, pv, fv, typ)
	i := fvOf(rate, per-1, pay, pv, typ) * rate
	if typ == 1 {
		i /= 1 + rate
	}
	return i
}

// IPMT / PPMT(rate, per, nper, pv, [fv], [type]): the interest or the
// principal part of payment per (truncated), which must be in 1..nper.
func ipmtFn(e *f.Env, a []f.Value, principal bool) f.Value {
	x, err := nums(e, a, 6)
	if err != nil {
		return err
	}
	rate, per, n, pv, fv, typ := x[0], math.Trunc(x[1]), x[2], x[3], x[4], due(x[5])
	if per < 1 || per > n {
		return f.ErrNum
	}
	i := ipmtOf(rate, per, n, pv, fv, typ)
	if principal {
		return f.NumberOrErr(pmtOf(rate, n, pv, fv, typ) - i)
	}
	return f.NumberOrErr(i)
}

// CUMIPMT / CUMPRINC(rate, nper, pv, start_period, end_period, type): the
// interest or principal paid over periods start..end (truncated). These
// insist on rate, nper and pv > 0 and type 0 or 1.
func cumipmt(e *f.Env, a []f.Value, principal bool) f.Value {
	x, err := nums(e, a, 6)
	if err != nil {
		return err
	}
	rate, n, pv := x[0], x[1], x[2]
	start, end, typ := math.Trunc(x[3]), math.Trunc(x[4]), math.Trunc(x[5])
	if rate <= 0 || n <= 0 || pv <= 0 || start < 1 || end < start || end > n || (typ != 0 && typ != 1) {
		return f.ErrNum
	}
	pay := pmtOf(rate, n, pv, 0, typ)
	sum := 0.0
	for p := start; p <= end; p++ {
		i := ipmtOf(rate, p, n, pv, 0, typ)
		if principal {
			i = pay - i
		}
		sum += i
	}
	return f.NumberOrErr(sum)
}

// ISPMT(rate, per, nper, pv): interest paid in period per (counted from 0)
// of a loan repaid in even principal installments.
func ispmt(e *f.Env, a []f.Value) f.Value {
	x, err := nums(e, a, 4)
	if err != nil {
		return err
	}
	rate, per, n, pv := x[0], x[1], x[2], x[3]
	if n == 0 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(pv * rate * (per/n - 1))
}

// NPV(rate, value1, [value2], ...): values are discounted one period each,
// in order. Numbers in ranges and arrays count while their text, logical
// values and blank cells are skipped; values typed directly are coerced.
func npv(e *f.Env, a []f.Value) f.Value {
	rate, err := e.Num(a[0])
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
				if n, bad = e.Num(x); bad != nil {
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
