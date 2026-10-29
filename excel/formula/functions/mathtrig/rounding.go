package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

func registerRounding(r *f.Registry) {
	r.Add(
		f.Spec{Name: "ROUND", Min: 2, Max: 2, Fn: rounder(f.RoundHalf)},
		f.Spec{Name: "ROUNDUP", Min: 2, Max: 2, Fn: rounder(f.RoundUp)},
		f.Spec{Name: "ROUNDDOWN", Min: 2, Max: 2, Fn: rounder(f.RoundDown)},
		f.Spec{Name: "TRUNC", Min: 1, Max: 2, Fn: rounder(f.RoundDown)},
		fn1("INT", plain(math.Floor)),
		fn1("EVEN", func(x float64) f.Value { return f.NumberOrErr(evenOdd(x, true)) }),
		fn1("ODD", func(x float64) f.Value { return f.NumberOrErr(evenOdd(x, false)) }),
		fn2("MROUND", mround),
		fn2("CEILING", func(n, s float64) f.Value {
			if n > 0 && s < 0 {
				return f.ErrNum
			}
			return multiple(n, s, math.Ceil)
		}),
		fn2("FLOOR", func(n, s float64) f.Value {
			if n > 0 && s < 0 {
				return f.ErrNum
			}
			if s == 0 {
				if n == 0 {
					return f.Number(0)
				}
				return f.ErrDiv0
			}
			return multiple(n, s, math.Floor)
		}),
		f.Spec{Name: "CEILING.MATH", Min: 1, Max: 3, Fn: mathRound(true)},
		f.Spec{Name: "FLOOR.MATH", Min: 1, Max: 3, Fn: mathRound(false)},
		f.Spec{Name: "CEILING.PRECISE", Min: 1, Max: 2, Fn: precise(math.Ceil)},
		f.Spec{Name: "ISO.CEILING", Min: 1, Max: 2, Fn: precise(math.Ceil)},
		f.Spec{Name: "FLOOR.PRECISE", Min: 1, Max: 2, Fn: precise(math.Floor)},
	)
}

// rounder implements ROUND, ROUNDUP, ROUNDDOWN and TRUNC (whose digits
// argument is optional).
func rounder(mode f.RoundMode) func(*f.Env, []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		x, err := f.Num(a[0])
		if err != nil {
			return err
		}
		dv, err := optNum(a, 1, 0)
		if err != nil {
			return err
		}
		d := int(math.Trunc(math.Max(-400, math.Min(400, dv))))
		return f.NumberOrErr(f.RoundDigits(x, d, mode))
	}
}

// evenOdd rounds away from zero to the nearest even or odd integer.
func evenOdd(x float64, even bool) float64 {
	sign := 1.0
	if x < 0 {
		sign, x = -1, -x
	}
	n := math.Ceil(f.Round15(x))
	if even {
		if math.Mod(n, 2) != 0 {
			n++
		}
	} else if math.Mod(n, 2) == 0 {
		n++
	}
	return sign * n
}

// MROUND(number, multiple): the nearest multiple, halves away from zero.
func mround(n, m float64) f.Value {
	if m == 0 {
		return f.Number(0)
	}
	if (n > 0 && m < 0) || (n < 0 && m > 0) {
		return f.ErrNum
	}
	q := f.Round15(n / m)
	return f.NumberOrErr(f.Round15(m * f.RoundDigits(q, 0, f.RoundHalf)))
}

// multiple rounds n to a multiple of s with op (math.Ceil or math.Floor)
// applied to the quotient; the sign rules are handled by the callers.
func multiple(n, s float64, op func(float64) float64) f.Value {
	if s == 0 || n == 0 {
		return f.Number(0)
	}
	return f.NumberOrErr(f.Round15(op(f.Round15(n/s)) * s))
}

// mathRound implements CEILING.MATH / FLOOR.MATH(number, [significance],
// [mode]): the significance's sign is ignored (default 1) and a non-zero
// mode rounds negative numbers the other way (away from zero for
// CEILING.MATH, toward zero for FLOOR.MATH).
func mathRound(ceil bool) func(*f.Env, []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		n, err := f.Num(a[0])
		if err != nil {
			return err
		}
		s, err := optNum(a, 1, 1)
		if err != nil {
			return err
		}
		mode, err := optNum(a, 2, 0)
		if err != nil {
			return err
		}
		s = math.Abs(s)
		if s == 0 || n == 0 {
			return f.Number(0)
		}
		up := ceil
		if n < 0 && mode != 0 {
			up = !up
		}
		op := math.Floor
		if up {
			op = math.Ceil
		}
		return multiple(n, s, op)
	}
}

// precise implements CEILING.PRECISE / ISO.CEILING / FLOOR.PRECISE(number,
// [significance]): always toward +∞ (ceiling) or -∞ (floor) whatever the
// signs.
func precise(op func(float64) float64) func(*f.Env, []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		n, err := f.Num(a[0])
		if err != nil {
			return err
		}
		s, err := optNum(a, 1, 1)
		if err != nil {
			return err
		}
		return multiple(n, math.Abs(s), op)
	}
}
