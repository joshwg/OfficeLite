package lookup

import (
	f "officelite/excel/formula"
	"officelite/excel/model"
)

// The array-shaping functions: CHOOSECOLS, CHOOSEROWS, TAKE, DROP, EXPAND,
// HSTACK, VSTACK, TOCOL, TOROW, WRAPCOLS, WRAPROWS and TRIMRANGE.

// chooseLines implements CHOOSECOLS(array, col_num1, ...) and
// CHOOSEROWS(array, row_num1, ...). Each number may be an array of
// numbers; negative numbers count from the end, and 0 or a number beyond
// the array is #VALUE!.
func chooseLines(e *f.Env, a []f.Value, byRow bool) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	n := arr.Cols
	if byRow {
		n = arr.Rows
	}
	var lines []int
	for _, v := range a[1:] {
		if err, ok := v.(f.Error); ok {
			return err
		}
		idx, err := numbers(v)
		if err != nil {
			return err
		}
		for _, i := range idx {
			if i < 0 {
				i += n + 1
			}
			if i < 1 || i > n {
				return f.ErrValue
			}
			lines = append(lines, i-1)
		}
	}
	return pickLines(arr, lines, byRow)
}

// span is the part of n lines that TAKE (take) or DROP (!take) keeps for a
// count k: from the start when k > 0, from the end when k < 0. A Missing
// count keeps everything.
func span(v f.Value, n int, take bool) (lo, hi int, err f.Value) {
	if _, ok := v.(f.Missing); ok {
		return 0, n, nil
	}
	k, err := f.Int(v)
	if err != nil {
		return 0, 0, err
	}
	switch {
	case take && k >= 0:
		return 0, min(k, n), nil
	case take:
		return max(n+k, 0), n, nil
	case k >= 0:
		return min(k, n), n, nil
	}
	return 0, max(n+k, 0), nil
}

// takeDrop implements TAKE(array, rows, [columns]) and DROP(array, rows,
// [columns]). An empty result is #CALC!. A reference gives a reference,
// so TAKE(A:A,-1) is the sheet's last cell rather than the used area's.
func takeDrop(e *f.Env, a []f.Value, take bool) f.Value {
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	src := a[0]
	if _, ok := src.(f.Ref); !ok {
		src = e.ToArray(src)
	}
	rows, cols := shape(src)
	var colArg f.Value = f.Missing{}
	if len(a) > 2 {
		colArg = a[2]
	}
	r0, r1, err := span(a[1], rows, take)
	if err != nil {
		return err
	}
	c0, c1, err := span(colArg, cols, take)
	if err != nil {
		return err
	}
	if r0 >= r1 || c0 >= c1 {
		return f.ErrCalc
	}
	return sub(e, src, r0, c0, r1-1, c1-1)
}

// sizeArg is an optional row or column count, def when omitted.
func sizeArg(a []f.Value, i, def int) (int, f.Value) {
	if omitted(a, i) {
		return def, nil
	}
	return f.Int(one(a[i]))
}

// padArg is the optional pad_with argument (#N/A by default).
func padArg(a []f.Value, i int) f.Value {
	if i >= len(a) {
		return f.ErrNA
	}
	if _, ok := a[i].(f.Missing); ok {
		return f.ErrNA
	}
	return a[i]
}

// EXPAND(array, rows, [columns], [pad_with]): array grown to rows×columns,
// the new cells pad_with. A size smaller than array's is #VALUE!.
func expand(e *f.Env, a []f.Value) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	rows, err := sizeArg(a, 1, arr.Rows)
	if err != nil {
		return err
	}
	cols, err := sizeArg(a, 2, arr.Cols)
	if err != nil {
		return err
	}
	switch {
	case rows < arr.Rows || cols < arr.Cols:
		return f.ErrValue
	case rows > model.MaxRows || cols > model.MaxCols:
		return f.ErrNum
	}
	pad := padArg(a, 3)
	out := &f.Array{Rows: rows, Cols: cols, V: make([]f.Value, rows*cols)}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			v := pad
			if r < arr.Rows && c < arr.Cols {
				v = arr.At(r, c)
			}
			out.Set(r, c, v)
		}
	}
	return single(out)
}

// stack implements HSTACK(array1, ...) (side by side) and VSTACK(array1,
// ...) (one below the other); the shorter (narrower) arrays are padded
// with #N/A.
func stack(e *f.Env, a []f.Value, vertical bool) f.Value {
	arrs := make([]*f.Array, len(a))
	rows, cols := 0, 0
	for i, v := range a {
		if _, ok := v.(f.Missing); ok {
			v = f.Number(0) // an empty argument, as in VSTACK(A1:B2,)
		}
		arrs[i] = e.ToArray(v)
		if vertical {
			rows, cols = rows+arrs[i].Rows, max(cols, arrs[i].Cols)
		} else {
			rows, cols = max(rows, arrs[i].Rows), cols+arrs[i].Cols
		}
	}
	if rows > model.MaxRows || cols > model.MaxCols {
		return f.ErrNum
	}
	out := &f.Array{Rows: rows, Cols: cols, V: make([]f.Value, rows*cols)}
	for i := range out.V {
		out.V[i] = f.ErrNA
	}
	r0, c0 := 0, 0
	for _, arr := range arrs {
		for r := 0; r < arr.Rows; r++ {
			for c := 0; c < arr.Cols; c++ {
				out.Set(r0+r, c0+c, arr.At(r, c))
			}
		}
		if vertical {
			r0 += arr.Rows
		} else {
			c0 += arr.Cols
		}
	}
	return single(out)
}

// flatten implements TOCOL(array, [ignore], [scan_by_column]) and
// TOROW(...). ignore is 0 (keep everything), 1 (skip blanks), 2 (skip
// errors) or 3 (both). Nothing left is #CALC!.
func flatten(e *f.Env, a []f.Value, col bool) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	ignore, err := intArg(a, 1, 0)
	if err != nil {
		return err
	}
	byCol, err := boolArg(a, 2, false)
	if err != nil {
		return err
	}
	if ignore < 0 || ignore > 3 {
		return f.ErrValue
	}
	if byCol {
		arr = transposed(arr)
	}
	var vals []f.Value
	for _, v := range arr.V {
		switch v.(type) {
		case f.Blank:
			if ignore&1 != 0 {
				continue
			}
		case f.Error:
			if ignore&2 != 0 {
				continue
			}
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return f.ErrCalc
	}
	out := &f.Array{Rows: 1, Cols: len(vals), V: vals}
	if col {
		out.Rows, out.Cols = len(vals), 1
	}
	return single(out)
}

// wrap implements WRAPROWS(vector, wrap_count, [pad_with]) (rows of
// wrap_count values) and WRAPCOLS(...) (columns of wrap_count values); the
// last row (column) is padded with pad_with, #N/A by default.
func wrap(e *f.Env, a []f.Value, rows bool) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	vals, ok := vector(arr)
	if !ok {
		return f.ErrValue
	}
	n, err := f.Int(a[1])
	if err != nil {
		return err
	}
	if n < 1 {
		return f.ErrNum
	}
	pad := padArg(a, 2)
	lines := (len(vals) + n - 1) / n
	out := &f.Array{Rows: lines, Cols: n, V: make([]f.Value, lines*n)}
	for i := range out.V {
		out.V[i] = pad
		if i < len(vals) {
			out.V[i] = vals[i]
		}
	}
	if !rows {
		out = transposed(out)
	}
	return single(out)
}

// TRIMRANGE(range, [trim_rows], [trim_cols]): range without its blank
// leading and/or trailing rows and columns. trim_rows and trim_cols are 0
// (keep), 1 (leading), 2 (trailing) or 3 (both, the default). A reference
// gives a reference, so TRIMRANGE(A:A) is the column down to its last
// value; an array gives an array. A range with nothing in it is #REF!
// (an array #CALC!).
func trimRange(e *f.Env, a []f.Value) f.Value {
	tr, err := intArg(a, 1, 3)
	if err != nil {
		return err
	}
	tc, err := intArg(a, 2, 3)
	if err != nil {
		return err
	}
	if tr < 0 || tr > 3 || tc < 0 || tc > 3 {
		return f.ErrValue
	}
	if err, ok := a[0].(f.Error); ok {
		return err
	}
	src := a[0]
	rows, cols := shape(src)
	// The bounding box of the values. A reference is scanned only within
	// the used area: beyond it everything is blank.
	scanRows, scanCols := rows, cols
	var blank func(r, c int) bool
	if ref, ok := src.(f.Ref); ok {
		used := e.Trim(ref)
		scanRows, scanCols = used.Rows(), used.Cols()
		blank = func(r, c int) bool {
			_, ok := e.Ctx.CellValue(ref.Sheet, ref.R1+r, ref.C1+c).(f.Blank)
			return ok
		}
	} else {
		arr := e.ToArray(src)
		src = arr
		blank = func(r, c int) bool {
			_, ok := arr.At(r, c).(f.Blank)
			return ok
		}
	}
	r0, c0, r1, c1 := rows, cols, -1, -1
	for r := 0; r < scanRows; r++ {
		for c := 0; c < scanCols; c++ {
			if !blank(r, c) {
				r0, c0, r1, c1 = min(r0, r), min(c0, c), max(r1, r), max(c1, c)
			}
		}
	}
	if r1 < 0 {
		if _, ok := src.(f.Ref); ok {
			return f.ErrRef
		}
		return f.ErrCalc
	}
	if tr&1 == 0 {
		r0 = 0
	}
	if tr&2 == 0 {
		r1 = rows - 1
	}
	if tc&1 == 0 {
		c0 = 0
	}
	if tc&2 == 0 {
		c1 = cols - 1
	}
	return sub(e, src, r0, c0, r1, c1)
}
