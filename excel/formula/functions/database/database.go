// Package database implements Excel's database functions: DSUM, DAVERAGE,
// DCOUNT, DCOUNTA, DGET, DMAX, DMIN, DPRODUCT, DSTDEV, DSTDEVP, DVAR and
// DVARP. Each takes FN(database, field, criteria):
//
//   - database is a range whose first row holds the field labels and whose
//     other rows are the records;
//   - field picks the column to aggregate, by label (case-insensitive) or
//     1-based number; DCOUNT and DCOUNTA may omit it to count records;
//   - criteria is a range with labels in its first row and conditions
//     below. Conditions on one row must all hold (AND); a record matching
//     any row is selected (OR). A blank condition cell, or a wholly blank
//     row, matches everything.
//
// Conditions use the COUNTIF syntax (">10", "<>x", wildcards) with
// Advanced Filter's twist for plain text: "Ap" matches text beginning with
// "Ap", "=Ap" only "Ap" itself (see f.ParseDatabaseCriteria).
//
// Computed criteria: a condition under a label that is no field of the
// database (or a blank label) is a formula such as =C7>AVERAGE($C$7:$C$10)
// written for the first record; it is evaluated for each record with its
// relative references moved down to that record, and the record is
// selected when the result is TRUE (or a non-zero number).
package database

import (
	"math"
	"strings"

	f "officelite/excel/formula"
)

// Register adds the database functions to r.
func Register(r *f.Registry) {
	r.Add(
		spec("DSUM", func(xs []float64) f.Value { return f.NumberOrErr(sum(xs)) }),
		spec("DAVERAGE", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr(sum(xs) / float64(len(xs)))
		}),
		spec("DMIN", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.Number(0)
			}
			lo := math.Inf(1)
			for _, x := range xs {
				lo = math.Min(lo, x)
			}
			return f.Number(lo)
		}),
		spec("DMAX", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.Number(0)
			}
			hi := math.Inf(-1)
			for _, x := range xs {
				hi = math.Max(hi, x)
			}
			return f.Number(hi)
		}),
		spec("DPRODUCT", func(xs []float64) f.Value {
			if len(xs) == 0 {
				return f.Number(0)
			}
			p := 1.0
			for _, x := range xs {
				p *= x
			}
			return f.NumberOrErr(p)
		}),
		spec("DVAR", func(xs []float64) f.Value { return variance(xs, 1, false) }),
		spec("DVARP", func(xs []float64) f.Value { return variance(xs, 0, false) }),
		spec("DSTDEV", func(xs []float64) f.Value { return variance(xs, 1, true) }),
		spec("DSTDEVP", func(xs []float64) f.Value { return variance(xs, 0, true) }),
		f.Spec{Name: "DCOUNT", Min: 3, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2), Fn: func(e *f.Env, a []f.Value) f.Value {
			return count(e, a, func(v f.Value) bool { _, ok := v.(f.Number); return ok })
		}},
		f.Spec{Name: "DCOUNTA", Min: 3, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2), Fn: func(e *f.Env, a []f.Value) f.Value {
			return count(e, a, func(v f.Value) bool { _, blank := v.(f.Blank); return !blank })
		}},
		f.Spec{Name: "DGET", Min: 3, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2), Fn: dget},
	)
}

// spec builds an aggregate over the numbers in the field of the selected
// records. Text and logical values there are ignored; an error is
// returned.
func spec(name string, agg func([]float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: 3, Max: 3, Refs: f.Only(0, 2), Arrays: f.Only(0, 2),
		Fn: func(e *f.Env, a []f.Value) f.Value {
			q, err := newQuery(e, a, false)
			if err != nil {
				return err
			}
			var xs []float64
			var bad f.Value
			q.each(func(_ int, v f.Value) bool {
				switch x := v.(type) {
				case f.Number:
					xs = append(xs, float64(x))
				case f.Error:
					bad = x
					return false
				}
				return true
			})
			if bad != nil {
				return bad
			}
			return agg(xs)
		}}
}

func sum(xs []float64) float64 {
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t
}

// variance is the sample (ddof 1) or population (ddof 0) variance, or its
// square root.
func variance(xs []float64, ddof int, sqrt bool) f.Value {
	n := len(xs) - ddof
	if n <= 0 {
		return f.ErrDiv0
	}
	mean := sum(xs) / float64(len(xs))
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	v := ss / float64(n)
	if sqrt {
		v = math.Sqrt(v)
	}
	return f.NumberOrErr(v)
}

// count implements DCOUNT / DCOUNTA: the selected records whose field
// value passes keep, or every selected record when field is omitted.
func count(e *f.Env, a []f.Value, keep func(f.Value) bool) f.Value {
	q, err := newQuery(e, a, true)
	if err != nil {
		return err
	}
	n := 0
	q.each(func(times int, v f.Value) bool {
		if q.field < 0 || keep(v) {
			n += times
		}
		return true
	})
	return f.Number(n)
}

// dget returns the field of the only selected record: #VALUE! when none
// is, #NUM! when several are.
func dget(e *f.Env, a []f.Value) f.Value {
	q, err := newQuery(e, a, false)
	if err != nil {
		return err
	}
	n := 0
	var got f.Value
	q.each(func(times int, v f.Value) bool {
		n += times
		got = v
		return n < 2
	})
	switch {
	case n == 0:
		return f.ErrValue
	case n > 1:
		return f.ErrNum
	}
	if _, blank := got.(f.Blank); blank {
		return f.Number(0)
	}
	return got
}

// --- ranges ------------------------------------------------------------------

// table is a database or criteria argument: a reference or an array.
type table struct {
	e          *f.Env
	ref        f.Ref
	isRef      bool
	arr        *f.Array
	rows, cols int // shape
	ur, uc     int // part that may hold non-blank values (≥ 1×1)
}

func newTable(e *f.Env, v f.Value) (*table, f.Value) {
	switch x := v.(type) {
	case f.Ref:
		t := e.Trim(x)
		return &table{e: e, ref: x, isRef: true, rows: x.Rows(), cols: x.Cols(),
			ur: max(1, t.R2-x.R1+1), uc: max(1, t.C2-x.C1+1)}, nil
	case *f.Array:
		return &table{e: e, arr: x, rows: x.Rows, cols: x.Cols, ur: x.Rows, uc: x.Cols}, nil
	case f.Error:
		return nil, x
	}
	// A single value has no records.
	return nil, f.ErrValue
}

func (t *table) at(i, j int) f.Value {
	if t.isRef {
		return t.e.Ctx.CellValue(t.ref.Sheet, t.ref.R1+i, t.ref.C1+j)
	}
	if i >= t.arr.Rows || j >= t.arr.Cols {
		return f.Blank{}
	}
	return t.arr.At(i, j)
}

// formula is the formula of the cell at i, j ("" for constants and
// arrays).
func (t *table) formula(i, j int) string {
	if !t.isRef {
		return ""
	}
	return t.e.Ctx.CellFormula(t.ref.Sheet, t.ref.R1+i, t.ref.C1+j)
}

// label finds the database column whose label is name, or -1.
func (t *table) label(name f.Value) int {
	s, err := f.Str(name)
	if err != nil || s == "" {
		return -1
	}
	for j := 0; j < t.uc; j++ {
		if h, err := f.Str(t.at(0, j)); err == nil && strings.EqualFold(h, s) {
			return j
		}
	}
	return -1
}

// --- queries -----------------------------------------------------------------

// cond is one condition cell: a criteria on a database column, or a
// computed criteria formula.
type cond struct {
	col      int
	c        f.Criteria
	formula  f.Node  // computed criteria (nil: c applies to col)
	src      string  // its text, shifted per record
	constant f.Value // a constant under a label that is no field

	sheet, row, cellCol int // the criteria cell
}

type query struct {
	e     *f.Env
	db    *table
	field int      // column, or -1 for an omitted field
	rows  [][]cond // OR of ANDs; an empty row matches everything
}

// newQuery checks the arguments of FN(database, field, criteria).
func newQuery(e *f.Env, a []f.Value, fieldOptional bool) (*query, f.Value) {
	db, err := newTable(e, a[0])
	if err != nil {
		return nil, err
	}
	q := &query{e: e, db: db, field: -1}
	switch x := a[1].(type) {
	case f.Missing:
		if !fieldOptional {
			return nil, f.ErrValue
		}
	case f.Error:
		return nil, x
	case f.Number:
		n := int(math.Trunc(float64(x)))
		if n < 1 || n > db.cols {
			return nil, f.ErrValue
		}
		q.field = n - 1
	case f.String:
		if q.field = db.label(x); q.field < 0 {
			return nil, f.ErrValue
		}
	default:
		return nil, f.ErrValue
	}
	crit, err := newTable(e, a[2])
	if err != nil {
		return nil, err
	}
	if crit.rows < 2 {
		return nil, f.ErrValue
	}
	cols := make([]int, crit.uc) // criteria column -> database column
	for j := range cols {
		cols[j] = db.label(crit.at(0, j))
	}
	for i := 1; i < crit.ur; i++ {
		var row []cond
		for j, col := range cols {
			v := crit.at(i, j)
			if col >= 0 {
				if c, ok := f.ParseDatabaseCriteria(v); ok {
					row = append(row, cond{col: col, c: c})
				}
				continue
			}
			if src := crit.formula(i, j); src != "" {
				if n, perr := f.Parse(src); perr == nil {
					row = append(row, cond{col: -1, formula: n, src: src,
						sheet: crit.ref.Sheet, row: crit.ref.R1 + i, cellCol: crit.ref.C1 + j})
					continue
				}
			}
			if !blank(v) {
				row = append(row, cond{col: -1, constant: v})
			}
		}
		q.rows = append(q.rows, row)
	}
	if crit.rows > crit.ur {
		// Rows past the used area are blank and match everything.
		q.rows = append(q.rows, nil)
	}
	return q, nil
}

func blank(v f.Value) bool {
	switch x := v.(type) {
	case f.Blank, f.Missing:
		return true
	case f.String:
		return x == ""
	}
	return false
}

// each calls fn with the field value (Blank when field is omitted) of
// every selected record, until fn returns false. Records past the used
// area are blank; when they are selected fn is called once for all of
// them, with times their count.
func (q *query) each(fn func(times int, v f.Value) bool) {
	for i := 1; i < q.db.rows; i++ {
		times := 1
		if i == q.db.ur {
			times = q.db.rows - i
		}
		if q.match(i) {
			var v f.Value = f.Blank{}
			if q.field >= 0 {
				v = q.db.at(i, q.field)
			}
			if !fn(times, v) {
				return
			}
		}
		if times > 1 {
			return
		}
	}
}

// match reports whether record i (1 is the first record) meets the
// criteria.
func (q *query) match(i int) bool {
rows:
	for _, row := range q.rows {
		for _, c := range row {
			if !q.test(c, i) {
				continue rows
			}
		}
		return true
	}
	return false
}

func (q *query) test(c cond, i int) bool {
	switch {
	case c.col >= 0:
		return c.c.Match(q.db.at(i, c.col))
	case c.formula == nil:
		return truthy(c.constant)
	}
	// The formula is written for the first record: move its relative
	// references down to record i.
	n := c.formula
	if i > 1 {
		var err error
		if n, err = f.Parse(f.Shift(c.src, i-1, 0)); err != nil {
			return false
		}
	}
	env := f.NewEnv(q.e.Ctx, q.e.Funcs, c.sheet, c.row, c.cellCol)
	v := env.Evaluate(n).Value
	if env.Volatile {
		q.e.Volatile = true
	}
	if a, ok := v.(*f.Array); ok && a.Len() > 0 {
		v = a.V[0]
	}
	return truthy(v)
}

// truthy is a computed criteria's verdict: TRUE or a non-zero number.
func truthy(v f.Value) bool {
	switch x := v.(type) {
	case f.Bool:
		return bool(x)
	case f.Number:
		return x != 0
	}
	return false
}
