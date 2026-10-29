package formula

import (
	"math"
	"strconv"
	"strings"
)

// Round15 rounds x to 15 significant digits, the precision Excel works
// in, so that e.g. 1.3/0.2 (6.499999999999999) is the 6.5 it displays as.
func Round15(x float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 15, 64), 64)
	return v
}

// RoundMode selects how RoundDigits treats the dropped digits.
type RoundMode int

const (
	RoundHalf RoundMode = iota // half away from zero (ROUND)
	RoundUp                    // away from zero (ROUNDUP)
	RoundDown                  // toward zero (ROUNDDOWN, TRUNC)
)

// RoundDigits rounds x to d decimal places (d < 0 rounds to tens,
// hundreds ...). Like Excel it works on the 15-significant-digit decimal
// representation of x, so ROUND(1.005,2) is 1.01 although the double
// 1.005 is slightly below it.
func RoundDigits(x float64, d int, mode RoundMode) float64 {
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
		if mode == RoundUp {
			res = math.Pow10(-d)
		}
	default:
		kept, rest := digits[:keep], digits[keep:]
		n, _ := strconv.ParseInt("0"+kept, 10, 64)
		switch mode {
		case RoundHalf:
			if rest[0] >= '5' {
				n++
			}
		case RoundUp:
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
