// Package engineering implements Excel's engineering functions: the
// number-base conversions BIN2DEC, BIN2HEX, ..., OCT2HEX and CONVERT.
// Binary, octal and hexadecimal numbers have at most 10 digits, negative
// numbers in 10-digit two's complement, as in Excel.
package engineering

import (
	"math"
	"strings"

	f "officelite/excel/formula"
)

// Register adds the engineering functions to r.
func Register(r *f.Registry) {
	bases := []struct {
		name  string
		radix int
	}{{"BIN", 2}, {"OCT", 8}, {"DEC", 10}, {"HEX", 16}}
	for _, from := range bases {
		for _, to := range bases {
			if from == to {
				continue
			}
			fn := from.name + "2" + to.name
			from, to := from.radix, to.radix
			if to == 10 {
				r.Add(f.Spec{Name: fn, Min: 1, Max: 1, Fn: func(_ *f.Env, a []f.Value) f.Value {
					x, err := parse(a[0], from)
					if err != nil {
						return err
					}
					return f.Number(x)
				}})
				continue
			}
			r.Add(f.Spec{Name: fn, Min: 1, Max: 2, Fn: func(_ *f.Env, a []f.Value) f.Value {
				var x float64
				if from == 10 {
					n, err := num(a[0])
					if err != nil {
						return err
					}
					x = math.Trunc(n)
				} else {
					n, err := parse(a[0], from)
					if err != nil {
						return err
					}
					x = float64(n)
				}
				return format(x, to, a)
			}})
		}
	}
	r.Add(f.Spec{Name: "CONVERT", Min: 3, Max: 3, Fn: convert})
}

// bits is the number of bits 10 digits of radix hold.
func bits(radix int) uint {
	return map[int]uint{2: 10, 8: 30, 16: 40}[radix]
}

// parse reads a binary, octal or hex number of at most 10 digits, the
// 10-digit ones with the top bit set being negative. Numbers are read as
// their digits (BIN2DEC(1010) is 10).
func parse(v f.Value, radix int) (int64, f.Value) {
	if _, ok := v.(f.Bool); ok {
		return 0, f.ErrValue // as Excel's other Analysis ToolPak functions
	}
	s, err := f.Str(v)
	if err != nil {
		return 0, err
	}
	if len(s) > 10 {
		return 0, f.ErrNum
	}
	var n int64
	for _, c := range strings.ToUpper(s) {
		d := strings.IndexRune(digits, c)
		if d < 0 || d >= radix {
			return 0, f.ErrNum
		}
		n = n*int64(radix) + int64(d)
	}
	if b := bits(radix); n >= 1<<(b-1) {
		n -= 1 << b
	}
	return n, nil
}

// num is f.Num without logical values, which these functions reject.
func num(v f.Value) (float64, f.Value) {
	if _, ok := v.(f.Bool); ok {
		return 0, f.ErrValue
	}
	return f.Num(v)
}

const digits = "0123456789ABCDEF"

// format writes x in radix: non-negative numbers padded to the optional
// places argument a[1], negative ones as 10 digits of two's complement.
func format(x float64, radix int, a []f.Value) f.Value {
	b := bits(radix)
	lim := float64(int64(1) << (b - 1))
	if x < -lim || x >= lim {
		return f.ErrNum
	}
	places := -1
	if len(a) > 1 {
		if _, missing := a[1].(f.Missing); !missing {
			p, err := num(a[1])
			if err != nil {
				return err
			}
			p = math.Trunc(p)
			if p < 0 || p > 10 {
				return f.ErrNum
			}
			places = int(p)
		}
	}
	n := int64(x)
	if n < 0 {
		n += 1 << b
		places = -1
	}
	var buf []byte
	for {
		buf = append(buf, digits[n%int64(radix)])
		n /= int64(radix)
		if n == 0 {
			break
		}
	}
	if places >= 0 {
		if places < len(buf) {
			return f.ErrNum
		}
		for len(buf) < places {
			buf = append(buf, '0')
		}
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return f.String(buf)
}
