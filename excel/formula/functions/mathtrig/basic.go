// Basic math functions: ABS, EXP, LOG, POWER, MOD, FACT, COMBIN, GCD/LCM,
// RAND, RANDARRAY, SEQUENCE, ROMAN/ARABIC, BASE/DECIMAL and the like.

package mathtrig

import (
	"math"
	"strings"

	f "officelite/excel/formula"
)

// registerBasic adds the basic math functions to r.
func registerBasic(r *f.Registry) {
	r.Add(
		fn1("ABS", plain(math.Abs)),
		fn1("SIGN", func(x float64) f.Value {
			switch {
			case x > 0:
				return f.Number(1)
			case x < 0:
				return f.Number(-1)
			}
			return f.Number(0)
		}),
		fn1("EXP", plain(math.Exp)),
		fn1("LN", func(x float64) f.Value {
			if x <= 0 {
				return f.ErrNum
			}
			return f.Number(math.Log(x))
		}),
		fn1("LOG10", func(x float64) f.Value {
			if x <= 0 {
				return f.ErrNum
			}
			return f.Number(math.Log10(x))
		}),
		f.Spec{Name: "LOG", Min: 1, Max: 2, Fn: logFn},
		fn1("SQRT", func(x float64) f.Value {
			if x < 0 {
				return f.ErrNum
			}
			return f.Number(math.Sqrt(x))
		}),
		fn1("SQRTPI", func(x float64) f.Value {
			if x < 0 {
				return f.ErrNum
			}
			return f.Number(math.Sqrt(x * math.Pi))
		}),
		fn2("POWER", power),
		f.Spec{Name: "PI", Min: 0, Max: 0, Fn: func(*f.Env, []f.Value) f.Value { return f.Number(math.Pi) }},
		fn2("MOD", func(n, d float64) f.Value {
			if d == 0 {
				return f.ErrDiv0
			}
			// The result has the sign of the divisor: MOD(3,-2) = -1.
			return f.NumberOrErr(n - d*math.Floor(n/d))
		}),
		fn2("QUOTIENT", func(n, d float64) f.Value {
			if d == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr(math.Trunc(n / d))
		}),
		fn1("FACT", func(x float64) f.Value {
			if x < 0 {
				return f.ErrNum
			}
			return f.NumberOrErr(fact(math.Floor(x)))
		}),
		fn1("FACTDOUBLE", func(x float64) f.Value {
			n := math.Floor(x)
			if n < -1 {
				return f.ErrNum
			}
			if n > 300 {
				return f.ErrNum
			}
			p := 1.0
			for ; n > 1; n -= 2 {
				p *= n
			}
			return f.NumberOrErr(p)
		}),
		fn2("COMBIN", func(n, k float64) f.Value {
			n, k = math.Trunc(n), math.Trunc(k)
			if n < 0 || k < 0 || n < k {
				return f.ErrNum
			}
			return f.NumberOrErr(combin(n, k))
		}),
		fn2("COMBINA", func(n, k float64) f.Value {
			n, k = math.Trunc(n), math.Trunc(k)
			if n < 0 || k < 0 {
				return f.ErrNum
			}
			if n == 0 {
				if k == 0 {
					return f.Number(1)
				}
				return f.Number(0)
			}
			return f.NumberOrErr(combin(n+k-1, k))
		}),
		f.Spec{Name: "GCD", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: gcdFn},
		f.Spec{Name: "LCM", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: lcmFn},
		f.Spec{Name: "MULTINOMIAL", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: multinomial},
		f.Spec{Name: "SERIESSUM", Min: 4, Max: 4, Refs: f.Only(3), Arrays: f.Only(3), Fn: seriesSum},
		f.Spec{Name: "RAND", Min: 0, Max: 0, Volatile: true, Fn: func(e *f.Env, _ []f.Value) f.Value {
			return f.Number(e.Rand())
		}},
		f.Spec{Name: "RANDBETWEEN", Min: 2, Max: 2, Volatile: true, Fn: randBetween},
		f.Spec{Name: "RANDARRAY", Min: 0, Max: 5, Volatile: true, Fn: randArray},
		f.Spec{Name: "SEQUENCE", Min: 1, Max: 4, Fn: sequence},
		f.Spec{Name: "ROMAN", Min: 1, Max: 2, Fn: roman},
		f.Spec{Name: "ARABIC", Min: 1, Max: 1, Fn: arabic},
		f.Spec{Name: "BASE", Min: 2, Max: 3, Fn: base},
		f.Spec{Name: "DECIMAL", Min: 2, Max: 2, Fn: decimal},
	)
}

// logFn implements LOG(number, [base]).
func logFn(e *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	b, err := optNum(a, 1, 10)
	if err != nil {
		return err
	}
	if x <= 0 || b <= 0 {
		return f.ErrNum
	}
	if b == 1 {
		return f.ErrDiv0
	}
	if b == 10 {
		return f.Number(math.Log10(x))
	}
	return f.NumberOrErr(math.Log(x) / math.Log(b))
}

// power implements POWER(number, power), which behaves like the ^ operator.
func power(x, y float64) f.Value {
	switch {
	case x == 0 && y == 0:
		return f.ErrNum
	case x == 0 && y < 0:
		return f.ErrDiv0
	}
	return f.NumberOrErr(math.Pow(x, y))
}

// fact returns n! for a whole n, +Inf past 170!.
func fact(n float64) float64 {
	if n > 170 {
		return math.Inf(1)
	}
	p := 1.0
	for i := 2.0; i <= n; i++ {
		p *= i
	}
	return p
}

// combin is the binomial coefficient n choose k for integers 0 <= k <= n.
func combin(n, k float64) float64 {
	k = math.Min(k, n-k)
	r := 1.0
	for i := 1.0; i <= k; i++ {
		r = r * (n - k + i) / i
		if math.IsInf(r, 0) {
			return r
		}
	}
	if r < 1<<53 {
		r = math.Round(r)
	}
	return r
}

// integers collects the arguments of GCD/LCM/MULTINOMIAL truncated to
// integers. Blank cells are skipped; text or booleans inside ranges are
// #VALUE!, negative values #NUM!.
func integers(e *f.Env, args []f.Value) ([]float64, f.Value) {
	var out []float64
	var errv f.Value
	for _, a := range args {
		e.Each(a, func(v f.Value, direct bool) bool {
			var x float64
			switch v := v.(type) {
			case f.Blank:
				return true
			case f.Number:
				x = float64(v)
			default:
				if !direct {
					if err, ok := v.(f.Error); ok {
						errv = err
					} else {
						errv = f.ErrValue
					}
					return false
				}
				n, err := f.Num(v)
				if err != nil {
					errv = err
					return false
				}
				x = n
			}
			x = math.Trunc(x)
			if x < 0 || x >= 1<<53 {
				errv = f.ErrNum
				return false
			}
			out = append(out, x)
			return true
		})
		if errv != nil {
			return nil, errv
		}
	}
	return out, nil
}

// gcd returns the greatest common divisor of the whole numbers a and b.
func gcd(a, b float64) float64 {
	for b != 0 {
		a, b = b, math.Mod(a, b)
	}
	return a
}

// gcdFn implements GCD(number1, [number2], ...).
func gcdFn(e *f.Env, a []f.Value) f.Value {
	xs, err := integers(e, a)
	if err != nil {
		return err
	}
	g := 0.0
	for _, x := range xs {
		g = gcd(g, x)
	}
	return f.Number(g)
}

// lcmFn implements LCM(number1, [number2], ...).
func lcmFn(e *f.Env, a []f.Value) f.Value {
	xs, err := integers(e, a)
	if err != nil {
		return err
	}
	if len(xs) == 0 {
		return f.Number(0)
	}
	l := 1.0
	for _, x := range xs {
		if x == 0 {
			return f.Number(0)
		}
		l = l / gcd(l, x) * x
	}
	return f.NumberOrErr(l)
}

// multinomial implements MULTINOMIAL(number1, [number2], ...) = (Σn)! / Πn!.
func multinomial(e *f.Env, a []f.Value) f.Value {
	xs, err := integers(e, a)
	if err != nil {
		return err
	}
	r, sum := 1.0, 0.0
	for _, x := range xs {
		sum += x
		r *= combin(sum, x)
	}
	return f.NumberOrErr(r)
}

// seriesSum implements SERIESSUM(x, n, m, coefficients) = Σ c_i · x^(n+i·m).
func seriesSum(e *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	n, m, err := nums2(a[1], a[2])
	if err != nil {
		return err
	}
	sum := 0.0
	for i, c := range e.ToArray(a[3]).V {
		cv, ok := c.(f.Number)
		if !ok {
			if err, isErr := c.(f.Error); isErr {
				return err
			}
			return f.ErrValue
		}
		p := power(x, n+float64(i)*m)
		pn, ok := p.(f.Number)
		if !ok {
			return p
		}
		sum += float64(cv) * float64(pn)
	}
	return f.NumberOrErr(sum)
}

// randBetween implements RANDBETWEEN(bottom, top): a random integer between the smallest integer
// >= bottom and the largest integer <= top.
func randBetween(e *f.Env, a []f.Value) f.Value {
	lo, hi, err := nums2(a[0], a[1])
	if err != nil {
		return err
	}
	lo, hi = math.Ceil(lo), math.Floor(hi)
	if lo > hi {
		return f.ErrNum
	}
	return f.Number(lo + math.Floor(e.Rand()*(hi-lo+1)))
}

// arrayDims reads optional rows/columns arguments for SEQUENCE and
// RANDARRAY: 0 is #CALC! (an empty array), negative #VALUE!.
func arrayDims(a []f.Value) (rows, cols int, errv f.Value) {
	dims := [2]int{1, 1}
	for i := range dims {
		x, err := optNum(a, i, 1)
		if err != nil {
			return 0, 0, err
		}
		n, err := trunc(x)
		if err != nil {
			return 0, 0, f.ErrValue
		}
		switch {
		case n < 0:
			return 0, 0, f.ErrValue
		case n == 0:
			errv = f.ErrCalc
		}
		dims[i] = n
	}
	if errv != nil {
		return 0, 0, errv
	}
	if dims[0]*dims[1] > maxRows*maxCols {
		return 0, 0, f.ErrNum
	}
	return dims[0], dims[1], nil
}

// randArray implements RANDARRAY([rows], [columns], [min], [max], [whole_number]).
func randArray(e *f.Env, a []f.Value) f.Value {
	rows, cols, err := arrayDims(a)
	if err != nil {
		return err
	}
	lo, err := optNum(a, 2, 0)
	if err != nil {
		return err
	}
	hi, err := optNum(a, 3, 1)
	if err != nil {
		return err
	}
	whole := false
	if len(a) > 4 {
		b, err := f.Truth(a[4])
		if err != nil {
			return err
		}
		whole = b
	}
	if lo > hi || (whole && (lo != math.Trunc(lo) || hi != math.Trunc(hi))) {
		return f.ErrValue
	}
	out := &f.Array{Rows: rows, Cols: cols, V: make([]f.Value, rows*cols)}
	for i := range out.V {
		if whole {
			out.V[i] = f.Number(lo + math.Floor(e.Rand()*(hi-lo+1)))
		} else {
			out.V[i] = f.Number(lo + e.Rand()*(hi-lo))
		}
	}
	return out
}

// sequence implements SEQUENCE(rows, [columns], [start], [step]).
func sequence(e *f.Env, a []f.Value) f.Value {
	rows, cols, err := arrayDims(a)
	if err != nil {
		return err
	}
	start, err := optNum(a, 2, 1)
	if err != nil {
		return err
	}
	step, err := optNum(a, 3, 1)
	if err != nil {
		return err
	}
	out := &f.Array{Rows: rows, Cols: cols, V: make([]f.Value, rows*cols)}
	for i := range out.V {
		out.V[i] = f.Number(start + float64(i)*step)
	}
	return out
}

var (
	romanChars  = []byte{'M', 'D', 'C', 'L', 'X', 'V', 'I'}
	romanValues = []int{1000, 500, 100, 50, 10, 5, 1}
)

// roman implements ROMAN(number, [form]): form 0 (or TRUE) is classic, 1-3 increasingly
// concise, 4 (or FALSE) simplified: ROMAN(499,0..4) = CDXCIX, LDVLIV,
// XDIX, VDIV, ID.
func roman(e *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	mode := 0
	if len(a) > 1 {
		switch v := a[1].(type) {
		case f.Missing:
		case f.Bool:
			if !v {
				mode = 4
			}
		default:
			m, err := f.Num(v)
			if err != nil {
				return err
			}
			mode = int(m)
		}
	}
	if x < 0 || x >= 4000 || mode < 0 || mode > 4 {
		return f.ErrValue
	}
	val := int(x)
	var b strings.Builder
	maxIdx := len(romanValues) - 1
	for i := 0; i <= maxIdx/2; i++ {
		idx := 2 * i
		digit := val / romanValues[idx]
		if digit%5 == 4 {
			// A subtractive pair; higher forms let the smaller symbol
			// reach further (IL instead of XLIX ...).
			idx2 := idx - 2
			if digit == 4 {
				idx2 = idx - 1
			}
			for steps := 0; steps < mode && idx < maxIdx; {
				steps++
				if romanValues[idx2]-romanValues[idx+1] <= val {
					idx++
				} else {
					steps = mode
				}
			}
			b.WriteByte(romanChars[idx])
			b.WriteByte(romanChars[idx2])
			val += romanValues[idx] - romanValues[idx2]
		} else {
			if digit > 4 {
				b.WriteByte(romanChars[idx-1])
			}
			b.WriteString(strings.Repeat(string(romanChars[idx]), digit%5))
			val %= romanValues[idx]
		}
	}
	return f.String(b.String())
}

var romanDigit = map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}

// arabic implements ARABIC(text): a Roman numeral to its value; a leading '-' negates and ""
// is 0. Like Excel, non-canonical numerals (IIII, IM) are accepted.
func arabic(e *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) > 255 {
		return f.ErrValue
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	total := 0
	for i := 0; i < len(s); i++ {
		v, ok := romanDigit[s[i]]
		if !ok {
			return f.ErrValue
		}
		if i+1 < len(s) && romanDigit[s[i+1]] > v {
			total -= v
		} else {
			total += v
		}
	}
	if neg {
		total = -total
	}
	return f.Number(total)
}

const digits36 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// base implements BASE(number, radix, [min_length]).
func base(e *f.Env, a []f.Value) f.Value {
	x, radix, err := nums2(a[0], a[1])
	if err != nil {
		return err
	}
	minLen, err := optNum(a, 2, 0)
	if err != nil {
		return err
	}
	x, radix, minLen = math.Trunc(x), math.Trunc(radix), math.Trunc(minLen)
	if x < 0 || x >= 1<<53 || radix < 2 || radix > 36 || minLen < 0 || minLen > 255 {
		return f.ErrNum
	}
	n, rb := uint64(x), uint64(radix)
	var buf []byte
	for {
		buf = append(buf, digits36[n%rb])
		n /= rb
		if n == 0 {
			break
		}
	}
	for len(buf) < int(minLen) {
		buf = append(buf, '0')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return f.String(buf)
}

// decimal implements DECIMAL(text, radix).
func decimal(e *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	r, err := f.Num(a[1])
	if err != nil {
		return err
	}
	radix := math.Trunc(r)
	if radix < 2 || radix > 36 || len(s) > 255 {
		return f.ErrNum
	}
	total := 0.0
	for _, c := range strings.ToUpper(s) {
		d := strings.IndexRune(digits36, c)
		if d < 0 || float64(d) >= radix {
			return f.ErrNum
		}
		total = total*radix + float64(d)
	}
	return f.NumberOrErr(total)
}
