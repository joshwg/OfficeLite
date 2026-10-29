// Package stats has the small numeric helpers shared by the statistical
// and database worksheet functions.
package stats

// Sum adds xs.
func Sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

// Variance is the sample (ddof 1) or population (ddof 0) variance of xs;
// false when there are no more than ddof values (#DIV/0! in Excel).
func Variance(xs []float64, ddof int) (float64, bool) {
	n := len(xs) - ddof
	if n <= 0 {
		return 0, false
	}
	mean := Sum(xs) / float64(len(xs))
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return ss / float64(n), true
}
