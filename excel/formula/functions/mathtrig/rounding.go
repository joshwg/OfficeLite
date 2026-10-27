package mathtrig

import (
	"math"
	"strconv"
	"strings"

	f "officelite/excel/formula"
)

type roundMode int

const (
	roundHalf roundMode = iota // half away from zero (ROUND)
	roundUp                    // away from zero (ROUNDUP)
	roundDown                  // toward zero (ROUNDDOWN, TRUNC)
)

func registerRounding(r *f.Registry) {
	r.Add(
		f.Spec{Name: "ROUND", Min: 2, Max: 2, Fn: rounder(roundHalf)},
		f.Spec{Name: "ROUNDUP", Min: 2, Max: 2, Fn: rounder(roundUp)},
		f.Spec{Name: "ROUNDDOWN", Min: 2, Max: 2, Fn: rounder(roundDown)},
		f.Spec{Name: "TRUNC", Min: 1, Max: 2, Fn: rounder(roundDown)},
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

// roundDigits rounds x to d decimal places (d < 0 rounds to tens,
// hundreds ...). Like Excel it works on the 15-significant-digit decimal
// representation of x, so ROUND(1.005,2) is 1.01 although the double
// 1.005 is slightly below it.
func roundDigits(x float64, d int, mode roundMode) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	neg := x < 0
	s := strconv.FormatFloat(math.Abs(x), 'e', 14, 64) // d.dddddddddddddde±XX
	mant, expStr, _ := strings.Cut(s, "e")
	digits := mant[:1] + mant[2:] // 15 digits; value = 0.digits × 10^(exp+1)
	exp, _ := strconv.Atoi(expStr)
	keep := exp + 1 + d // digits before the rounding position
	var res float64
	switch {
	case keep >= len(digits):
		res, _ = strconv.ParseFloat(digits+"e"+strconv.Itoa(exp+1-len(digits)), 64)
	case keep < 0:
		// Every significant digit lies beyond the next one after the
		// rounding position.
		if mode == roundUp {
			res = math.Pow10(-d)
		}
	default:
		kept, rest := digits[:keep], digits[keep:]
		n, _ := strconv.ParseInt("0"+kept, 10, 64)
		switch mode {
		case roundHalf:
			if rest[0] >= '5' {
				n++
			}
		case roundUp:
			if strings.Trim(rest, "0") != "" {
				n++
			}
		}
		res, _ = strconv.ParseFloat(strconv.FormatInt(n, 10)+"e"+strconv.Itoa(-d), 64)
	}
	if neg {
		res = -res
	}
	return res
}

// rounder implements ROUND, ROUNDUP, ROUNDDOWN and TRUNC (whose digits
// argument is optional).
func rounder(mode roundMode) func(*f.Env, []f.Value) f.Value {
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
		return f.NumberOrErr(roundDigits(x, d, mode))
	}
}

// evenOdd rounds away from zero to the nearest even or odd integer.
func evenOdd(x float64, even bool) float64 {
	sign := 1.0
	if x < 0 {
		sign, x = -1, -x
	}
	n := math.Ceil(sig15(x))
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
	q := sig15(n / m)
	return f.NumberOrErr(sig15(m * roundDigits(q, 0, roundHalf)))
}

// multiple rounds n to a multiple of s with op (math.Ceil or math.Floor)
// applied to the quotient; the sign rules are handled by the callers.
func multiple(n, s float64, op func(float64) float64) f.Value {
	if s == 0 || n == 0 {
		return f.Number(0)
	}
	return f.NumberOrErr(sig15(op(sig15(n/s)) * s))
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
