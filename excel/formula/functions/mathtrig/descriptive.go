// Descriptive statistics beyond what SUBTOTAL/AGGREGATE need: GEOMEAN,
// KURT, SKEW, TRIMMEAN, RANK, PERCENTRANK, FREQUENCY, ...

package mathtrig

import (
	"math"
	"sort"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/stats"
)

// registerDescriptive adds the descriptive statistics functions to r.
func registerDescriptive(r *f.Registry) {
	r.Add(
		listSpec("GEOMEAN", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.ErrNum
			}
			s := 0.0
			for _, x := range xs {
				if x <= 0 {
					return f.ErrNum
				}
				s += math.Log(x)
			}
			return f.NumberOrErr(math.Exp(s / float64(len(xs))))
		}),
		listSpec("HARMEAN", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.ErrNum
			}
			s := 0.0
			for _, x := range xs {
				if x <= 0 {
					return f.ErrNum
				}
				s += 1 / x
			}
			return f.NumberOrErr(float64(len(xs)) / s)
		}),
		listSpec("AVEDEV", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.ErrNum
			}
			m := mean(xs)
			s := 0.0
			for _, x := range xs {
				s += math.Abs(x - m)
			}
			return f.NumberOrErr(s / float64(len(xs)))
		}),
		listSpec("DEVSQ", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.ErrNum
			}
			return f.NumberOrErr(devsq(xs))
		}),
		listSpec("KURT", func(xs []float64) f.Value {
			n := float64(len(xs))
			s := math.Sqrt(devsq(xs) / (n - 1))
			if n < 4 || s == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr(n*(n+1)/((n-1)*(n-2)*(n-3))*moment(xs, s, 4) - 3*(n-1)*(n-1)/((n-2)*(n-3)))
		}),
		listSpec("SKEW", func(xs []float64) f.Value {
			n := float64(len(xs))
			s := math.Sqrt(devsq(xs) / (n - 1))
			if n < 3 || s == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr(n / ((n - 1) * (n - 2)) * moment(xs, s, 3))
		}),
		listSpec("SKEW.P", func(xs []float64) f.Value {
			n := float64(len(xs))
			s := math.Sqrt(devsq(xs) / n)
			if n == 0 || s == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr(moment(xs, s, 3) / n)
		}),
		f.Spec{Name: "TRIMMEAN", Min: 2, Max: 2, Refs: f.Only(0), Arrays: f.Only(0), Fn: trimmean},
		f.Spec{Name: "RANK", Min: 2, Max: 3, Refs: f.Only(1), Arrays: f.Only(1), Fn: rank(false)},
		f.Spec{Name: "RANK.EQ", Min: 2, Max: 3, Refs: f.Only(1), Arrays: f.Only(1), Fn: rank(false)},
		f.Spec{Name: "RANK.AVG", Min: 2, Max: 3, Refs: f.Only(1), Arrays: f.Only(1), Fn: rank(true)},
		f.Spec{Name: "PERCENTRANK", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: percentRank(false)},
		f.Spec{Name: "PERCENTRANK.INC", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: percentRank(false)},
		f.Spec{Name: "PERCENTRANK.EXC", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: percentRank(true)},
		f.Spec{Name: "FREQUENCY", Min: 2, Max: 2, Refs: f.All, Arrays: f.All, Fn: frequency},
	)
}

// listSpec builds a variadic function of the numbers of its arguments
// (numbers in ranges, numbers and numeric text typed directly).
func listSpec(name string, g func(xs []float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All,
		Fn: func(e *f.Env, a []f.Value) f.Value {
			xs, err := gather(e, a, opts{k: kNum})
			if err != nil {
				return err
			}
			return g(xs)
		}}
}

// mean returns the arithmetic mean of xs (NaN when xs is empty).
func mean(xs []float64) float64 { return stats.Sum(xs) / float64(len(xs)) }

// devsq is the sum of squared deviations from the mean.
func devsq(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := mean(xs)
	s := 0.0
	for _, x := range xs {
		s += (x - m) * (x - m)
	}
	return s
}

// moment is Σ((x - mean)/s)^k.
func moment(xs []float64, s float64, k float64) float64 {
	m := mean(xs)
	t := 0.0
	for _, x := range xs {
		t += math.Pow((x-m)/s, k)
	}
	return t
}

// trimmean implements TRIMMEAN(array, percent): the mean without the smallest and largest
// values, percent·n of them rounded down to an even number.
func trimmean(e *f.Env, a []f.Value) f.Value {
	xs, err := gather(e, a[:1], opts{k: kNum})
	if err != nil {
		return err
	}
	pct, err := f.Num(a[1])
	if err != nil {
		return err
	}
	if pct < 0 || pct >= 1 || len(xs) == 0 {
		return f.ErrNum
	}
	k := int(math.Floor(float64(len(xs))*pct/2 + 1e-12))
	s := sorted(xs)[k : len(xs)-k]
	return f.NumberOrErr(mean(s))
}

// rank is RANK(number, ref, [order]) and RANK.EQ (ties share the best
// rank) or RANK.AVG (ties get the average of their ranks). Order 0 ranks
// descending.
func rank(avg bool) func(e *f.Env, a []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		x, err := f.Num(a[0])
		if err != nil {
			return err
		}
		xs, err := gather(e, a[1:2], opts{k: kNum})
		if err != nil {
			return err
		}
		asc, err := optNum(a, 2, 0)
		if err != nil {
			return err
		}
		better, same := 0, 0
		for _, v := range xs {
			switch {
			case v == x:
				same++
			case (asc == 0 && v > x) || (asc != 0 && v < x):
				better++
			}
		}
		if same == 0 {
			return f.ErrNA
		}
		r := float64(better + 1)
		if avg {
			r += float64(same-1) / 2
		}
		return f.Number(r)
	}
}

// percentRank is PERCENTRANK(array, x, [significance]) and its .INC/.EXC
// forms: x's relative position in the data (rank/(n-1), or (rank+1)/(n+1)
// for .EXC), interpolated between data values and truncated to
// significance digits (default 3).
func percentRank(exclusive bool) func(e *f.Env, a []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		xs, err := gather(e, a[:1], opts{k: kNum})
		if err != nil {
			return err
		}
		x, err := f.Num(a[1])
		if err != nil {
			return err
		}
		sig, err := optNum(a, 2, 3)
		if err != nil {
			return err
		}
		sig = math.Trunc(sig)
		if sig < 1 {
			return f.ErrNum
		}
		n := len(xs)
		if n == 0 {
			return f.ErrNum
		}
		s := sorted(xs)
		if x < s[0] || x > s[n-1] {
			return f.ErrNA
		}
		i := sort.SearchFloat64s(s, x) // values below x
		pos := float64(i)
		if s[i] != x {
			pos = float64(i-1) + (x-s[i-1])/(s[i]-s[i-1])
		}
		var r float64
		switch {
		case exclusive:
			r = (pos + 1) / float64(n+1)
		case n == 1:
			r = 1
		default:
			r = pos / float64(n-1)
		}
		p := math.Pow(10, sig)
		return f.NumberOrErr(math.Floor(f.Round15(r*p)) / p)
	}
}

// frequency implements FREQUENCY(data_array, bins_array): a vertical array counting the data
// up to each bin (above the previous one), plus one more count of the
// data above the largest bin. Bins need not be sorted.
func frequency(e *f.Env, a []f.Value) f.Value {
	data, err := gather(e, a[:1], opts{k: kNum})
	if err != nil {
		return err
	}
	bins, err := gather(e, a[1:2], opts{k: kNum})
	if err != nil {
		return err
	}
	order := make([]int, len(bins))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return bins[order[i]] < bins[order[j]] })
	counts := make([]float64, len(bins)+1)
	for _, x := range data {
		k := sort.Search(len(order), func(i int) bool { return x <= bins[order[i]] })
		if k == len(order) {
			counts[len(bins)]++
		} else {
			counts[order[k]]++
		}
	}
	out := &f.Array{Rows: len(counts), Cols: 1}
	for _, c := range counts {
		out.V = append(out.V, f.Number(c))
	}
	return out
}
