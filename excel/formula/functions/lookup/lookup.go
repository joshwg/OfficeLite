// Package lookup implements Excel's lookup and reference functions:
// XLOOKUP, XMATCH, VLOOKUP, HLOOKUP, LOOKUP, MATCH, INDEX, CHOOSE, ROW,
// ROWS, COLUMN, COLUMNS, ADDRESS, GETPIVOTDATA, OFFSET, INDIRECT, AREAS, FORMULATEXT,
// HYPERLINK, TRANSPOSE and the
// dynamic-array functions FILTER, UNIQUE, SORT, SORTBY, GROUPBY and PIVOTBY
// and the array-shaping CHOOSECOLS, CHOOSEROWS, TAKE, DROP, EXPAND, HSTACK,
// VSTACK, TOCOL, TOROW, WRAPCOLS, WRAPROWS and TRIMRANGE.
package lookup

import (
	f "officelite/excel/formula"
)

// Register adds the lookup and reference functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "XLOOKUP", Min: 3, Max: 6, Refs: f.Only(1, 2), Arrays: f.All, Fn: xlookup},
		f.Spec{Name: "XMATCH", Min: 2, Max: 4, Arrays: f.Only(1), Fn: xmatch},
		f.Spec{Name: "MATCH", Min: 2, Max: 3, Arrays: f.Only(1), Fn: match},
		f.Spec{Name: "VLOOKUP", Min: 3, Max: 4, Arrays: f.Only(1), Fn: func(e *f.Env, a []f.Value) f.Value {
			return vhlookup(e, a, true)
		}},
		f.Spec{Name: "HLOOKUP", Min: 3, Max: 4, Arrays: f.Only(1), Fn: func(e *f.Env, a []f.Value) f.Value {
			return vhlookup(e, a, false)
		}},
		f.Spec{Name: "LOOKUP", Min: 2, Max: 3, Arrays: f.Only(1, 2), Refs: f.Only(2), Fn: lookupFn},
		f.Spec{Name: "GETPIVOTDATA", Min: 2, Max: 254, Refs: f.Only(1), Volatile: true, Fn: getPivotData},
		f.Spec{Name: "INDEX", Min: 2, Max: 4, Refs: f.Only(0), Arrays: f.All, Fn: index},
		f.Spec{Name: "CHOOSE", Min: 2, Max: 255, Refs: f.From(1), Arrays: f.All, Fn: choose},
		f.Spec{Name: "ROW", Min: 0, Max: 1, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return rowCol(e, a, true)
		}},
		f.Spec{Name: "COLUMN", Min: 0, Max: 1, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return rowCol(e, a, false)
		}},
		f.Spec{Name: "ROWS", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return count(a[0], true)
		}},
		f.Spec{Name: "COLUMNS", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return count(a[0], false)
		}},
		f.Spec{Name: "ADDRESS", Min: 2, Max: 5, Fn: address},
		f.Spec{Name: "OFFSET", Min: 3, Max: 5, Refs: f.Only(0), Arrays: f.Only(0), Volatile: true, Fn: offset},
		f.Spec{Name: "INDIRECT", Min: 1, Max: 2, Volatile: true, Fn: indirect},
		f.Spec{Name: "AREAS", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: areas},
		f.Spec{Name: "FORMULATEXT", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: formulaText},
		f.Spec{Name: "HYPERLINK", Min: 1, Max: 2, Fn: hyperlink},
		f.Spec{Name: "TRANSPOSE", Min: 1, Max: 1, Arrays: f.All, Fn: transpose},
		f.Spec{Name: "FILTER", Min: 2, Max: 3, Arrays: f.All, Fn: filter},
		f.Spec{Name: "UNIQUE", Min: 1, Max: 3, Arrays: f.All, Fn: unique},
		f.Spec{Name: "SORT", Min: 1, Max: 4, Arrays: f.All, Fn: sortFn},
		f.Spec{Name: "SORTBY", Min: 2, Max: f.Variadic, Arrays: f.All, Fn: sortBy},
		f.Spec{Name: "CHOOSECOLS", Min: 2, Max: f.Variadic, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return chooseLines(e, a, false)
		}},
		f.Spec{Name: "CHOOSEROWS", Min: 2, Max: f.Variadic, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return chooseLines(e, a, true)
		}},
		f.Spec{Name: "TAKE", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return takeDrop(e, a, true)
		}},
		f.Spec{Name: "DROP", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return takeDrop(e, a, false)
		}},
		f.Spec{Name: "EXPAND", Min: 2, Max: 4, Arrays: f.Only(0), Fn: expand},
		f.Spec{Name: "HSTACK", Min: 1, Max: f.Variadic, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return stack(e, a, false)
		}},
		f.Spec{Name: "VSTACK", Min: 1, Max: f.Variadic, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return stack(e, a, true)
		}},
		f.Spec{Name: "TOCOL", Min: 1, Max: 3, Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return flatten(e, a, true)
		}},
		f.Spec{Name: "TOROW", Min: 1, Max: 3, Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return flatten(e, a, false)
		}},
		f.Spec{Name: "WRAPROWS", Min: 2, Max: 3, Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return wrap(e, a, true)
		}},
		f.Spec{Name: "WRAPCOLS", Min: 2, Max: 3, Arrays: f.Only(0), Fn: func(e *f.Env, a []f.Value) f.Value {
			return wrap(e, a, false)
		}},
		f.Spec{Name: "GROUPBY", Min: 3, Max: 8, Arrays: f.All, Fn: groupBy},
		f.Spec{Name: "PIVOTBY", Min: 4, Max: 11, Arrays: f.All, Fn: pivotBy},
		f.Spec{Name: "TRIMRANGE", Min: 1, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: trimRange},
	)
}

// modes reads XLOOKUP/XMATCH's match_mode and search_mode arguments.
func modes(a []f.Value, mi int) (mode, search int, err f.Value) {
	if mode, err = intArg(a, mi, exact); err != nil {
		return
	}
	if search, err = intArg(a, mi+1, firstLast); err != nil {
		return
	}
	switch {
	case mode < -1 || mode > 2:
		err = f.ErrValue
	case search != 1 && search != -1 && search != 2 && search != -2:
		err = f.ErrValue
	case mode == wildcard && (search == binaryAsc || search == binaryDesc):
		err = f.ErrValue // wildcards cannot be bisected
	}
	return
}

// XLOOKUP(lookup_value, lookup_array, return_array, [if_not_found],
// [match_mode], [search_mode]). The result is the row (vertical lookup) or
// column (horizontal) of return_array at the match, as a reference when
// return_array is one. An array of lookup values gives an array of
// results, each reduced to its first value.
func xlookup(e *f.Env, a []f.Value) f.Value {
	mode, search, err := modes(a, 4)
	if err != nil {
		return err
	}
	for _, v := range a[1:3] {
		if err, ok := v.(f.Error); ok {
			return err
		}
	}
	lr, lc := shape(a[1])
	rr, rc := shape(a[2])
	if lr > 1 && lc > 1 {
		return f.ErrValue
	}
	vertical := lc == 1 && (lr > 1 || rr == 1)
	if vertical && rr != lr || !vertical && rc != lc {
		return f.ErrValue
	}
	vals := e.ToArray(a[1]).V // whole columns are trimmed; the rest is blank
	notFound := func() f.Value {
		if omitted(a, 3) {
			return f.ErrNA
		}
		return single(a[3])
	}
	result := func(x f.Value) f.Value {
		if err, ok := x.(f.Error); ok {
			return err
		}
		i := find(vals, x, mode, search)
		switch {
		case i < 0:
			return notFound()
		case vertical:
			return sub(e, a[2], i, 0, i, rc-1)
		}
		return sub(e, a[2], 0, i, rr-1, i)
	}
	x, ok := a[0].(*f.Array)
	if !ok || x.Single() {
		return result(one(a[0]))
	}
	out := &f.Array{Rows: x.Rows, Cols: x.Cols, V: make([]f.Value, len(x.V))}
	for i, v := range x.V {
		out.V[i] = topLeft(e, result(v))
	}
	return out
}

// XMATCH(lookup_value, lookup_array, [match_mode], [search_mode])
func xmatch(e *f.Env, a []f.Value) f.Value {
	mode, search, err := modes(a, 2)
	if err != nil {
		return err
	}
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	arr, err := array(e, a[1])
	if err != nil {
		return err
	}
	vals, ok := vector(arr)
	if !ok {
		return f.ErrValue
	}
	if i := find(vals, a[0], mode, search); i >= 0 {
		return f.Number(i + 1)
	}
	return f.ErrNA
}

// MATCH(lookup_value, lookup_array, [match_type]): 1 (default) is the
// largest value <= lookup_value in ascending data, 0 the first exact match
// (text may use wildcards), -1 the smallest value >= lookup_value in
// descending data.
func match(e *f.Env, a []f.Value) f.Value {
	typ := 1
	if len(a) > 2 {
		n, err := f.Num(a[2]) // an empty argument is 0
		if err != nil {
			return err
		}
		typ = sign(n)
	}
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	arr, err := array(e, a[1])
	if err != nil {
		return err
	}
	vals, ok := vector(arr)
	if !ok {
		return f.ErrNA
	}
	var i int
	switch typ {
	case 0:
		i = find(vals, a[0], wildcard, firstLast)
	case 1:
		i = approx(vals, a[0])
	default:
		i = find(vals, a[0], nextLarge, binaryDesc)
	}
	if i < 0 {
		return f.ErrNA
	}
	return f.Number(i + 1)
}

func sign(n float64) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// VLOOKUP(lookup_value, table_array, col_index_num, [range_lookup]) and
// HLOOKUP(lookup_value, table_array, row_index_num, [range_lookup]).
// range_lookup TRUE (the default) is an approximate match in ascending
// data; FALSE (or an empty argument) an exact one, with wildcards for text.
func vhlookup(e *f.Env, a []f.Value, vertical bool) f.Value {
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	tbl, err := array(e, a[1])
	if err != nil {
		return err
	}
	n, err := f.Int(a[2])
	if err != nil {
		return err
	}
	approximate := true
	if len(a) > 3 {
		if approximate, err = f.Truth(a[3]); err != nil {
			return err
		}
	}
	if !vertical {
		tbl = transposed(tbl)
	}
	switch {
	case n < 1:
		return f.ErrValue
	case n > tbl.Cols:
		return f.ErrRef
	}
	vals := make([]f.Value, tbl.Rows)
	for r := range vals {
		vals[r] = tbl.At(r, 0)
	}
	var i int
	if approximate {
		i = approx(vals, a[0])
	} else {
		i = find(vals, a[0], wildcard, firstLast)
	}
	if i < 0 {
		return f.ErrNA
	}
	return tbl.At(i, n-1)
}

// LOOKUP(lookup_value, lookup_vector, [result_vector]) or
// LOOKUP(lookup_value, array): always an approximate match in ascending
// data. The array form searches the first row of an array wider than it is
// tall and returns from the last row, otherwise searches the first column
// and returns from the last column. A result_vector reference shorter than
// lookup_vector is extended, as in Excel.
func lookupFn(e *f.Env, a []f.Value) f.Value {
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	arr, err := array(e, a[1])
	if err != nil {
		return err
	}
	if len(a) == 2 {
		wide := arr.Cols > arr.Rows
		if wide {
			arr = transposed(arr)
		}
		vals := make([]f.Value, arr.Rows)
		for r := range vals {
			vals[r] = arr.At(r, 0)
		}
		i := approx(vals, a[0])
		if i < 0 {
			return f.ErrNA
		}
		return arr.At(i, arr.Cols-1)
	}
	vals, ok := vector(arr)
	if !ok {
		return f.ErrNA
	}
	i := approx(vals, a[0])
	if i < 0 {
		return f.ErrNA
	}
	switch res := a[2].(type) {
	case f.Error:
		return res
	case f.Ref:
		switch {
		case res.Cols() == 1:
			return e.Ctx.CellValue(res.Sheet, res.R1+i, res.C1)
		case res.Rows() == 1:
			return e.Ctx.CellValue(res.Sheet, res.R1, res.C1+i)
		}
		return f.ErrNA
	case *f.Array:
		if rv, ok := vector(res); ok && i < len(rv) {
			return rv[i]
		}
		return f.ErrNA
	default:
		if i == 0 {
			return res
		}
		return f.ErrNA
	}
}

// transposed returns a's transpose.
func transposed(a *f.Array) *f.Array {
	out := &f.Array{Rows: a.Cols, Cols: a.Rows, V: make([]f.Value, 0, len(a.V))}
	for c := 0; c < a.Cols; c++ {
		for r := 0; r < a.Rows; r++ {
			out.V = append(out.V, a.At(r, c))
		}
	}
	return out
}
