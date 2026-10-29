// Aggregating functions: SUM, PRODUCT, SUMSQ, SUMPRODUCT, the SUMX2*Y2
// pair sums, PERCENTOF, SUBTOTAL and AGGREGATE.

package mathtrig

import (
	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/stats"
)

// registerAggregate adds the aggregating functions to r.
func registerAggregate(r *f.Registry) {
	r.Add(
		statSpec("SUM", kNum, stSum),
		statSpec("PRODUCT", kNum, stProduct),
		f.Spec{Name: "SUMSQ", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			xs, err := gather(e, a, opts{k: kNum})
			if err != nil {
				return err
			}
			s := 0.0
			for _, x := range xs {
				s += x * x
			}
			return f.NumberOrErr(s)
		}},
		f.Spec{Name: "SUMPRODUCT", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: sumProduct},
		f.Spec{Name: "SUMX2MY2", Min: 2, Max: 2, Arrays: f.All, Fn: pairSum(func(x, y float64) float64 { return x*x - y*y })},
		f.Spec{Name: "SUMX2PY2", Min: 2, Max: 2, Arrays: f.All, Fn: pairSum(func(x, y float64) float64 { return x*x + y*y })},
		f.Spec{Name: "SUMXMY2", Min: 2, Max: 2, Arrays: f.All, Fn: pairSum(func(x, y float64) float64 { return (x - y) * (x - y) })},
		f.Spec{Name: "PERCENTOF", Min: 2, Max: 2, Refs: f.All, Arrays: f.All, Fn: percentOf},
		f.Spec{Name: "SUBTOTAL", Min: 2, Max: f.Variadic, Refs: f.From(1), Arrays: f.From(1), Fn: subtotal},
		f.Spec{Name: "AGGREGATE", Min: 3, Max: f.Variadic, Refs: f.From(2), Arrays: f.From(2), Fn: aggregate},
	)
}

// sumProduct implements SUMPRODUCT(array1, [array2], ...): the arrays must have the same shape;
// entries that are not numbers count as 0, errors propagate.
func sumProduct(e *f.Env, a []f.Value) f.Value {
	gs := make([]*grid, len(a))
	for i, v := range a {
		g, err := newGrid(e, v)
		if err != nil {
			return err
		}
		if i > 0 && !g.sameShape(gs[0]) {
			return f.ErrValue
		}
		gs[i] = g
	}
	rows, cols := extent(gs...)
	total := 0.0
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			p := 1.0
			for _, g := range gs {
				switch v := g.at(i, j).(type) {
				case f.Number:
					p *= float64(v)
				case f.Error:
					return v
				default:
					p = 0
				}
			}
			total += p
		}
	}
	return f.NumberOrErr(total)
}

// pairSum implements SUMX2MY2, SUMX2PY2 and SUMXMY2: pairs where both
// values are numbers are combined; arrays of different sizes are #N/A.
func pairSum(g func(x, y float64) float64) func(*f.Env, []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		xs, ys := e.ToArray(a[0]), e.ToArray(a[1])
		if xs.Len() != ys.Len() {
			return f.ErrNA
		}
		total := 0.0
		for i, xv := range xs.V {
			yv := ys.V[i]
			if err, ok := xv.(f.Error); ok {
				return err
			}
			if err, ok := yv.(f.Error); ok {
				return err
			}
			x, ok1 := xv.(f.Number)
			y, ok2 := yv.(f.Number)
			if ok1 && ok2 {
				total += g(float64(x), float64(y))
			}
		}
		return f.NumberOrErr(total)
	}
}

// percentOf implements PERCENTOF(data_subset, data_all) = SUM(data_subset) / SUM(data_all).
func percentOf(e *f.Env, a []f.Value) f.Value {
	part, err := gather(e, a[:1], opts{k: kNum})
	if err != nil {
		return err
	}
	all, err := gather(e, a[1:], opts{k: kNum})
	if err != nil {
		return err
	}
	t := stats.Sum(all)
	if t == 0 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(stats.Sum(part) / t)
}

// subtotal implements SUBTOTAL(function_num, ref1, [ref2], ...): function_num 1-11 (AVERAGE,
// COUNT, COUNTA, MAX, MIN, PRODUCT, STDEV, STDEVP, SUM, VAR, VARP), or
// 101-111 to also ignore rows hidden by hand. Rows the AutoFilter hid are
// always left out, as are cells that hold SUBTOTAL or AGGREGATE formulas.
func subtotal(e *f.Env, a []f.Value) f.Value {
	fn, err := f.Int(a[0])
	if err != nil {
		return err
	}
	rows := skipFiltered
	if fn > 100 {
		fn -= 100
		rows = skipHidden
	}
	if fn < 1 || fn > 11 {
		return f.ErrValue
	}
	o := opts{k: kNum, skipNested: true, skipRows: rows}
	return aggregateRefs(e, fn, a[1:], o)
}

// aggregateRefs computes statistics 1-13 over reference arguments.
func aggregateRefs(e *f.Env, fn int, refs []f.Value, o opts) f.Value {
	switch fn {
	case stCountA:
		return f.Number(countA(e, refs, o))
	case stCount:
		o.k = kCount
	}
	xs, err := gather(e, refs, o)
	if err != nil {
		return err
	}
	return stat(fn, xs, 0)
}

// aggregate implements AGGREGATE(function_num, options, ref1, [ref2], ...) for function_num
// 1-13, and AGGREGATE(function_num, options, array, k) for 14-19 (LARGE,
// SMALL, PERCENTILE.INC, QUARTILE.INC, PERCENTILE.EXC, QUARTILE.EXC).
// Options: 0/1 ignore nested SUBTOTAL/AGGREGATE cells, 2/3 also errors,
// 4/5 ignore nothing, 6/7 errors only. The odd options also ignore hidden
// rows, whether hidden by hand or by the AutoFilter.
func aggregate(e *f.Env, a []f.Value) f.Value {
	fn, err := f.Int(a[0])
	if err != nil {
		return err
	}
	opt, err := f.Int(a[1])
	if err != nil {
		return err
	}
	if fn < 1 || fn > 19 || opt < 0 || opt > 7 {
		return f.ErrValue
	}
	o := opts{k: kNum, skipNested: opt <= 3, skipErr: opt == 2 || opt == 3 || opt >= 6}
	if opt%2 == 1 {
		o.skipRows = skipHidden
	}
	if fn <= 13 {
		return aggregateRefs(e, fn, a[2:], o)
	}
	if len(a) != 4 {
		return f.ErrValue
	}
	xs, err := gather(e, a[2:3], o)
	if err != nil {
		return err
	}
	// k may be an array: AGGREGATE(14,6,A1:A9,{1,2,3}).
	k := e.Deref(a[3])
	if ka, ok := k.(*f.Array); ok && !ka.Single() {
		out := &f.Array{Rows: ka.Rows, Cols: ka.Cols, V: make([]f.Value, len(ka.V))}
		for i, kv := range ka.V {
			kn, err := f.Num(kv)
			if err != nil {
				out.V[i] = err
				continue
			}
			out.V[i] = stat(fn, xs, kn)
		}
		return out
	}
	if missing(k) {
		return f.ErrValue
	}
	kn, err := f.Num(k)
	if err != nil {
		return err
	}
	return stat(fn, xs, kn)
}
