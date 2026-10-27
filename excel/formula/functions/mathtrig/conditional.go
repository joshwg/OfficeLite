package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

// The criteria functions take their ranges as references (so a whole
// column is only walked over its used part) while the criteria themselves
// are scalar arguments: an array of criteria lifts, e.g.
// SUMIFS(C:C,A:A,{"a","b"}) returns two sums.

func registerConditional(r *f.Registry) {
	sumRangeFirst := func(i int) bool { return i == 0 || i%2 == 1 }
	pairs := func(i int) bool { return i%2 == 0 }
	r.Add(
		f.Spec{Name: "SUMIF", Min: 2, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2), Fn: func(e *f.Env, a []f.Value) f.Value {
			return ifFn(e, a, aggSum)
		}},
		f.Spec{Name: "AVERAGEIF", Min: 2, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2), Fn: func(e *f.Env, a []f.Value) f.Value {
			return ifFn(e, a, aggAverage)
		}},
		f.Spec{Name: "COUNTIF", Min: 2, Max: 2, Refs: f.Only(0), Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return ifsFnAgg(e, nil, a, aggCount)
		}},
		f.Spec{Name: "COUNTIFS", Min: 2, Max: f.Variadic, Refs: pairs, Arrays: pairs, Fn: func(e *f.Env, a []f.Value) f.Value {
			if len(a)%2 != 0 {
				return f.ErrValue
			}
			return ifsFnAgg(e, nil, a, aggCount)
		}},
		ifsSpec("SUMIFS", aggSum, sumRangeFirst),
		ifsSpec("AVERAGEIFS", aggAverage, sumRangeFirst),
		ifsSpec("MINIFS", aggMin, sumRangeFirst),
		ifsSpec("MAXIFS", aggMax, sumRangeFirst),
	)
}

type aggKind int

const (
	aggCount aggKind = iota
	aggSum
	aggAverage
	aggMin
	aggMax
)

// ifsSpec builds FN(value_range, criteria_range1, criteria1, ...).
func ifsSpec(name string, k aggKind, ranges func(int) bool) f.Spec {
	return f.Spec{Name: name, Min: 3, Max: f.Variadic, Refs: ranges, Arrays: ranges,
		Fn: func(e *f.Env, a []f.Value) f.Value {
			if len(a)%2 != 1 {
				return f.ErrValue
			}
			vals, err := newGrid(e, a[0])
			if err != nil {
				return err
			}
			return ifsFnAgg(e, vals, a[1:], k)
		}}
}

// ifFn implements SUMIF / AVERAGEIF(range, criteria, [value_range]). The
// value range is used from its top-left cell with the shape of range, as
// in Excel (SUMIF(A1:A3,"x",B1) sums B1:B3).
func ifFn(e *f.Env, a []f.Value, k aggKind) f.Value {
	g, err := newGrid(e, a[0])
	if err != nil {
		return err
	}
	vals := g
	if len(a) > 2 && !missing(a[2]) {
		v, err := newGrid(e, a[2])
		if err != nil {
			return err
		}
		vals = v.resize(g.rows, g.cols)
	}
	return ifsFnAgg(e, vals, a[:2], k)
}

// ifsFnAgg aggregates the values of vals (or counts, for aggCount, when
// vals is nil) at the
// positions where every (range, criteria) pair in a matches. All ranges
// must have the same shape.
func ifsFnAgg(e *f.Env, vals *grid, a []f.Value, k aggKind) f.Value {
	type cond struct {
		g *grid
		c f.Criteria
	}
	conds := make([]cond, 0, len(a)/2)
	gs := make([]*grid, 0, len(a)/2+1)
	for i := 0; i+1 < len(a); i += 2 {
		g, err := newGrid(e, a[i])
		if err != nil {
			return err
		}
		if len(gs) > 0 && !g.sameShape(gs[0]) {
			return f.ErrValue
		}
		conds = append(conds, cond{g, f.ParseCriteria(a[i+1])})
		gs = append(gs, g)
	}
	if vals != nil {
		if !vals.sameShape(gs[0]) {
			return f.ErrValue
		}
		gs = append(gs, vals)
	}
	rows, cols := extent(gs...)
	n, total := 0, 0.0
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := 0; i < rows; i++ {
	cells:
		for j := 0; j < cols; j++ {
			for _, c := range conds {
				if !c.c.Match(c.g.at(i, j)) {
					continue cells
				}
			}
			if k == aggCount {
				n++
				continue
			}
			switch v := vals.at(i, j).(type) {
			case f.Number:
				x := float64(v)
				n++
				total += x
				lo, hi = math.Min(lo, x), math.Max(hi, x)
			case f.Error:
				return v
			}
		}
	}
	switch k {
	case aggCount:
		// Cells past the used area are blank in every range.
		all := true
		for _, c := range conds {
			all = all && c.c.Match(f.Blank{})
		}
		if all {
			n += gs[0].rows*gs[0].cols - rows*cols
		}
		return f.Number(n)
	case aggSum:
		return f.NumberOrErr(total)
	case aggAverage:
		if n == 0 {
			return f.ErrDiv0
		}
		return f.NumberOrErr(total / float64(n))
	case aggMin:
		if n == 0 {
			return f.Number(0)
		}
		return f.Number(lo)
	}
	if n == 0 {
		return f.Number(0)
	}
	return f.Number(hi)
}
