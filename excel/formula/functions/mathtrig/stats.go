package mathtrig

import (
	"math"
	"sort"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/stats"
)

// Statistic numbers, as used by AGGREGATE (SUBTOTAL uses 1-11 too).
const (
	stAverage = 1 + iota
	stCount
	stCountA
	stMax
	stMin
	stProduct
	stStdevS
	stStdevP
	stSum
	stVarS
	stVarP
	stMedian
	stMode
	stLarge
	stSmall
	stPercentileInc
	stQuartileInc
	stPercentileExc
	stQuartileExc
)

func registerStats(r *f.Registry) {
	r.Add(
		f.Spec{Name: "COUNT", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			xs, _ := gather(e, a, opts{k: kCount})
			return f.Number(len(xs))
		}},
		f.Spec{Name: "COUNTA", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return f.Number(countA(e, a, opts{}))
		}},
		f.Spec{Name: "COUNTBLANK", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: countBlank},
		statSpec("AVERAGE", kNum, stAverage),
		statSpec("AVERAGEA", kA, stAverage),
		statSpec("MAX", kNum, stMax),
		statSpec("MAXA", kA, stMax),
		statSpec("MIN", kNum, stMin),
		statSpec("MINA", kA, stMin),
		statSpec("MEDIAN", kNum, stMedian),
		statSpec("MODE", kNum, stMode),
		statSpec("MODE.SNGL", kNum, stMode),
		f.Spec{Name: "MODE.MULT", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: modeMult},
		statSpec("STDEV", kNum, stStdevS),
		statSpec("STDEV.S", kNum, stStdevS),
		statSpec("STDEVA", kA, stStdevS),
		statSpec("STDEVP", kNum, stStdevP),
		statSpec("STDEV.P", kNum, stStdevP),
		statSpec("STDEVPA", kA, stStdevP),
		statSpec("VAR", kNum, stVarS),
		statSpec("VAR.S", kNum, stVarS),
		statSpec("VARA", kA, stVarS),
		statSpec("VARP", kNum, stVarP),
		statSpec("VAR.P", kNum, stVarP),
		statSpec("VARPA", kA, stVarP),
		rankSpec("LARGE", stLarge),
		rankSpec("SMALL", stSmall),
		rankSpec("PERCENTILE", stPercentileInc),
		rankSpec("PERCENTILE.INC", stPercentileInc),
		rankSpec("PERCENTILE.EXC", stPercentileExc),
		rankSpec("QUARTILE", stQuartileInc),
		rankSpec("QUARTILE.INC", stQuartileInc),
		rankSpec("QUARTILE.EXC", stQuartileExc),
	)
}

// statSpec builds a variadic statistic over all numbers of its arguments.
func statSpec(name string, k kind, id int) f.Spec {
	return f.Spec{Name: name, Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All,
		Fn: func(e *f.Env, a []f.Value) f.Value {
			xs, err := gather(e, a, opts{k: k})
			if err != nil {
				return err
			}
			return stat(id, xs, 0)
		}}
}

// rankSpec builds FN(array, k) functions; k lifts, so LARGE(A1:A9,{1,2})
// returns two values.
func rankSpec(name string, id int) f.Spec {
	return f.Spec{Name: name, Min: 2, Max: 2, Refs: f.Only(0), Arrays: f.Only(0),
		Fn: func(e *f.Env, a []f.Value) f.Value {
			xs, err := gather(e, a[:1], opts{k: kNum})
			if err != nil {
				return err
			}
			k, err := f.Num(a[1])
			if err != nil {
				return err
			}
			return stat(id, xs, k)
		}}
}

// stat computes statistic id (AGGREGATE numbering) of xs; k is the second
// argument of LARGE, SMALL, PERCENTILE and QUARTILE.
func stat(id int, xs []float64, k float64) f.Value {
	n := float64(len(xs))
	switch id {
	case stAverage:
		if len(xs) == 0 {
			return f.ErrDiv0
		}
		return f.NumberOrErr(stats.Sum(xs) / n)
	case stCount, stCountA:
		return f.Number(n)
	case stMax, stMin:
		if len(xs) == 0 {
			return f.Number(0)
		}
		m := xs[0]
		for _, x := range xs[1:] {
			if (id == stMax && x > m) || (id == stMin && x < m) {
				m = x
			}
		}
		return f.Number(m)
	case stProduct:
		if len(xs) == 0 {
			return f.Number(0)
		}
		p := 1.0
		for _, x := range xs {
			p *= x
		}
		return f.NumberOrErr(p)
	case stSum:
		return f.NumberOrErr(stats.Sum(xs))
	case stStdevS, stStdevP, stVarS, stVarP:
		ddof := 0
		if id == stStdevS || id == stVarS {
			ddof = 1
		}
		v, ok := stats.Variance(xs, ddof)
		if !ok {
			return f.ErrDiv0
		}
		if id == stStdevS || id == stStdevP {
			v = math.Sqrt(v)
		}
		return f.NumberOrErr(v)
	case stMedian:
		if len(xs) == 0 {
			return f.ErrNum
		}
		s := sorted(xs)
		m := len(s) / 2
		if len(s)%2 == 1 {
			return f.Number(s[m])
		}
		return f.Number((s[m-1] + s[m]) / 2)
	case stMode:
		modes := modes(xs)
		if len(modes) == 0 {
			return f.ErrNA
		}
		return f.Number(modes[0])
	case stLarge, stSmall:
		// A fractional k is rounded up.
		i := math.Ceil(k)
		if i < 1 || i > n {
			return f.ErrNum
		}
		s := sorted(xs)
		if id == stLarge {
			return f.Number(s[len(s)-int(i)])
		}
		return f.Number(s[int(i)-1])
	case stQuartileInc, stQuartileExc:
		q := math.Trunc(k)
		if (id == stQuartileInc && (q < 0 || q > 4)) || (id == stQuartileExc && (q <= 0 || q >= 4)) {
			return f.ErrNum
		}
		return percentile(xs, q/4, id == stQuartileExc)
	case stPercentileInc:
		return percentile(xs, k, false)
	case stPercentileExc:
		return percentile(xs, k, true)
	}
	return f.ErrValue
}

func sorted(xs []float64) []float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s
}

// percentile interpolates the k-th percentile: inclusive ranks run 0..n-1
// over k in [0,1]; exclusive ones (k+1)·k-1 for k in (0,1), #NUM! when
// that falls outside the data.
func percentile(xs []float64, k float64, exclusive bool) f.Value {
	n := len(xs)
	if n == 0 {
		return f.ErrNum
	}
	var h float64
	if exclusive {
		if k <= 0 || k >= 1 {
			return f.ErrNum
		}
		h = k*float64(n+1) - 1
		if h < 0 || h > float64(n-1) {
			return f.ErrNum
		}
	} else {
		if k < 0 || k > 1 {
			return f.ErrNum
		}
		h = k * float64(n-1)
	}
	s := sorted(xs)
	lo := int(math.Floor(h))
	if lo >= n-1 {
		return f.Number(s[n-1])
	}
	return f.Number(s[lo] + (h-float64(lo))*(s[lo+1]-s[lo]))
}

// modes returns the most frequent values occurring more than once, in
// order of first appearance (MODE returns the first of them).
func modes(xs []float64) []float64 {
	count := map[float64]int{}
	best := 1
	for _, x := range xs {
		count[x]++
		best = max(best, count[x])
	}
	if best < 2 {
		return nil
	}
	var out []float64
	for _, x := range xs {
		if count[x] == best {
			out = append(out, x)
			count[x] = 0 // report each value once
		}
	}
	return out
}

// MODE.MULT(number1, ...): a vertical array of the modes.
func modeMult(e *f.Env, a []f.Value) f.Value {
	xs, err := gather(e, a, opts{k: kNum})
	if err != nil {
		return err
	}
	ms := modes(xs)
	if len(ms) == 0 {
		return f.ErrNA
	}
	out := &f.Array{Rows: len(ms), Cols: 1}
	for _, m := range ms {
		out.V = append(out.V, f.Number(m))
	}
	return out
}

// COUNTBLANK(range): empty cells and "" formula results.
func countBlank(e *f.Env, a []f.Value) f.Value {
	g, err := newGrid(e, a[0])
	if err != nil {
		return err
	}
	ur, uc := extent(g)
	n := g.rows*g.cols - ur*uc // cells past the used area
	for i := 0; i < ur; i++ {
		for j := 0; j < uc; j++ {
			switch v := g.at(i, j).(type) {
			case f.Blank:
				n++
			case f.String:
				if v == "" {
					n++
				}
			}
		}
	}
	return f.Number(n)
}
