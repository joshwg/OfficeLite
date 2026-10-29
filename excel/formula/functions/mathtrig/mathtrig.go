// Package mathtrig implements Excel's math & trigonometry functions (SUM,
// ROUND, SUMIFS, SUBTOTAL, AGGREGATE, MMULT, SEQUENCE, ROMAN, ...) and the
// statistical functions (COUNT, COUNTIFS, AVERAGE, MEDIAN, STDEV,
// PERCENTILE, RANK, CORREL, LINEST, NORM.DIST, T.TEST, BINOM.DIST, ...).
package mathtrig

import (
	"math"

	f "officelite/excel/formula"
	"officelite/excel/model"
)

// Sheet limits, for clamping references.
const (
	maxRows = model.MaxRows
	maxCols = model.MaxCols
)

// Register adds the math, trig and statistical functions to r.
func Register(r *f.Registry) {
	registerBasic(r)
	registerTrig(r)
	registerRounding(r)
	registerAggregate(r)
	registerConditional(r)
	registerMatrix(r)
	registerStats(r)
	registerDescriptive(r)
	registerRegression(r)
	registerHypothesis(r)
	registerDist(r)
}

// --- argument helpers -------------------------------------------------------

// fn1 builds a one-argument numeric function.
func fn1(name string, g func(x float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
		x, err := f.Num(a[0])
		if err != nil {
			return err
		}
		return g(x)
	}}
}

// fn2 builds a two-argument numeric function.
func fn2(name string, g func(x, y float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: 2, Max: 2, Fn: func(e *f.Env, a []f.Value) f.Value {
		x, y, err := nums2(a[0], a[1])
		if err != nil {
			return err
		}
		return g(x, y)
	}}
}

// plain wraps a float function, mapping NaN/Inf to #NUM!.
func plain(g func(float64) float64) func(float64) f.Value {
	return func(x float64) f.Value { return f.NumberOrErr(g(x)) }
}

// nums2 coerces a and b to numbers, returning the first coercion error.
func nums2(a, b f.Value) (float64, float64, f.Value) {
	x, err := f.Num(a)
	if err != nil {
		return 0, 0, err
	}
	y, err := f.Num(b)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// missing reports whether v is an omitted argument.
func missing(v f.Value) bool {
	_, ok := v.(f.Missing)
	return ok
}

// optNum returns the numeric value of an optional argument, def if omitted.
func optNum(a []f.Value, i int, def float64) (float64, f.Value) {
	if i >= len(a) || missing(a[i]) {
		return def, nil
	}
	return f.Num(a[i])
}

// trunc converts a number to an integer argument (truncating), #NUM! when
// it is out of range.
func trunc(x float64) (int, f.Value) {
	if math.IsNaN(x) || math.Abs(x) > 1<<53 {
		return 0, f.ErrNum
	}
	return int(x), nil
}

// --- iterating aggregate arguments --------------------------------------------

// kind selects how an aggregate treats non-numeric values.
type kind int

const (
	// kNum (SUM, AVERAGE, MAX ...): numbers in ranges and arrays count, text,
	// booleans and blanks there are ignored; scalars typed directly are
	// coerced (non-numeric text is #VALUE!); errors propagate.
	kNum kind = iota
	// kA (AVERAGEA, MAXA ...): like kNum but text in ranges counts as 0 and
	// booleans as 1/0.
	kA
	// kCount (COUNT): like kNum but values that are not numbers, including
	// errors and non-numeric direct text, are skipped silently.
	kCount
)

type opts struct {
	k          kind
	skipErr    bool // AGGREGATE options 2, 3, 6, 7
	skipNested bool // SUBTOTAL, AGGREGATE options 0-3
	skipRows   skip // SUBTOTAL, AGGREGATE: rows left out
}

// skip says which rows of a reference SUBTOTAL and AGGREGATE leave out.
type skip int

const (
	skipNone     skip = iota
	skipFiltered      // rows the AutoFilter hid (SUBTOTAL 1-11)
	skipHidden        // also rows hidden by hand (SUBTOTAL 101-111, odd AGGREGATE options)
)

// rowVisibility is implemented by contexts that know which rows are hidden
// (the calc engine); others hide none.
type rowVisibility interface {
	RowHidden(sheet, row int) bool
	RowFiltered(sheet, row int) bool
}

// skipped reports whether o leaves row out.
func (o opts) skipped(e *f.Env, sheet, row int) bool {
	rv, ok := e.Ctx.(rowVisibility)
	switch {
	case !ok || o.skipRows == skipNone:
		return false
	case o.skipRows == skipFiltered:
		return rv.RowFiltered(sheet, row)
	}
	return rv.RowHidden(sheet, row)
}

// nestedTotal reports whether a formula contains SUBTOTAL or AGGREGATE;
// such cells are skipped by SUBTOTAL and AGGREGATE to avoid double counting.
func nestedTotal(formula string) bool {
	if formula == "" {
		return false
	}
	return f.CallsFunc(formula, "SUBTOTAL", "AGGREGATE")
}

// each visits the scalars of an argument like e.Each, optionally skipping
// cells whose formula is itself a SUBTOTAL/AGGREGATE and hidden rows.
func each(e *f.Env, v f.Value, o opts, fn func(v f.Value, direct bool) bool) {
	r, ok := v.(f.Ref)
	if !ok || !o.skipNested && o.skipRows == skipNone {
		e.Each(v, fn)
		return
	}
	r = e.Trim(r)
	for row := r.R1; row <= r.R2; row++ {
		if o.skipped(e, r.Sheet, row) {
			continue
		}
		for col := r.C1; col <= r.C2; col++ {
			if o.skipNested && nestedTotal(e.Ctx.CellFormula(r.Sheet, row, col)) {
				continue
			}
			if !fn(e.Ctx.CellValue(r.Sheet, row, col), false) {
				return
			}
		}
	}
}

// gather collects the numbers of aggregate arguments following Excel's
// rules for the given kind.
func gather(e *f.Env, args []f.Value, o opts) ([]float64, f.Value) {
	var out []float64
	var errv f.Value
	visit := func(v f.Value, direct bool) bool {
		switch x := v.(type) {
		case f.Number:
			out = append(out, float64(x))
		case f.Error:
			if o.skipErr || o.k == kCount {
				return true
			}
			errv = x
			return false
		case f.Bool:
			if direct || o.k == kA {
				out = append(out, b2f(bool(x)))
			}
		case f.String:
			switch {
			case direct:
				n, ok := f.ParseNumber(string(x))
				switch {
				case ok:
					out = append(out, n)
				case o.k == kCount:
				default:
					errv = f.ErrValue
					return false
				}
			case o.k == kA:
				out = append(out, 0)
			}
		case f.Missing:
			// An empty argument, as in SUM(1,) or COUNT(,), is a zero.
			if direct {
				out = append(out, 0)
			}
		case *f.Lambda:
			if o.k != kCount {
				errv = f.ErrValue
				return false
			}
		}
		return true
	}
	for _, a := range args {
		each(e, a, o, visit)
		if errv != nil {
			return nil, errv
		}
	}
	return out, nil
}

// countA counts non-empty values (COUNTA): everything but blank cells,
// including errors and "" formula results.
func countA(e *f.Env, args []f.Value, o opts) int {
	n := 0
	for _, a := range args {
		each(e, a, o, func(v f.Value, direct bool) bool {
			switch v.(type) {
			case f.Blank:
			case f.Error:
				if !o.skipErr {
					n++
				}
			default:
				n++
			}
			return true
		})
	}
	return n
}

// b2f returns 1 for true and 0 for false.
func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// --- range-shaped arguments ---------------------------------------------------

// grid is a rectangular argument (a reference or an array) addressed by
// offset, used where several ranges are walked in parallel (SUMIFS,
// SUMPRODUCT). For references only the used part of the sheet is visited;
// cells past it are known to be blank.
type grid struct {
	e          *f.Env
	ref        f.Ref
	isRef      bool
	arr        *f.Array
	rows, cols int // logical shape
	ur, uc     int // extent that may hold non-blank values
}

// newGrid wraps a reference or array as a grid; a scalar becomes a 1×1
// grid, an error is returned as is and a lambda is #VALUE!.
func newGrid(e *f.Env, v f.Value) (*grid, f.Value) {
	switch x := v.(type) {
	case f.Ref:
		t := e.Trim(x)
		return &grid{e: e, ref: x, isRef: true, rows: x.Rows(), cols: x.Cols(),
			ur: t.R2 - x.R1 + 1, uc: t.C2 - x.C1 + 1}, nil
	case *f.Array:
		return &grid{e: e, arr: x, rows: x.Rows, cols: x.Cols, ur: x.Rows, uc: x.Cols}, nil
	case f.Error:
		return nil, x
	case *f.Lambda:
		return nil, f.ErrValue
	}
	a := &f.Array{Rows: 1, Cols: 1, V: []f.Value{v}}
	return &grid{e: e, arr: a, rows: 1, cols: 1, ur: 1, uc: 1}, nil
}

// at returns the value at offset i, j (Blank outside the underlying data).
func (g *grid) at(i, j int) f.Value {
	if g.isRef {
		if i >= g.ref.Rows() || j >= g.ref.Cols() {
			return f.Blank{}
		}
		return g.e.Ctx.CellValue(g.ref.Sheet, g.ref.R1+i, g.ref.C1+j)
	}
	if i >= g.arr.Rows || j >= g.arr.Cols {
		return f.Blank{}
	}
	return g.arr.At(i, j)
}

// resize gives g the shape rows×cols anchored at its top-left cell, as
// SUMIF/AVERAGEIF do with their sum/average range.
func (g *grid) resize(rows, cols int) *grid {
	if g.isRef {
		r := g.ref
		r.R2 = min(r.R1+rows-1, maxRows-1)
		r.C2 = min(r.C1+cols-1, maxCols-1)
		ng, _ := newGrid(g.e, r)
		g = ng
	} else {
		ng := *g
		g = &ng
	}
	g.rows, g.cols = rows, cols
	g.ur, g.uc = min(g.ur, rows), min(g.uc, cols)
	return g
}

// sameShape reports whether g and h have the same logical shape.
func (g *grid) sameShape(h *grid) bool { return g.rows == h.rows && g.cols == h.cols }

// extent is the part of the common shape of gs that may hold non-blank
// values; positions outside it are blank in every grid.
func extent(gs ...*grid) (rows, cols int) {
	for _, g := range gs {
		rows, cols = max(rows, min(g.ur, gs[0].rows)), max(cols, min(g.uc, gs[0].cols))
	}
	return rows, cols
}
