package lookup

import (
	"strconv"
	"strings"

	f "officelite/excel/formula"
	"officelite/excel/model"
)

// INDEX(array, row_num, [column_num], [area_num]). Given a reference the
// result is a reference (so INDEX(...):A5 and ROW(INDEX(...)) work). A row
// or column number of 0 selects the whole column or row; a one-row or
// one-column array takes a single number as the position along it. Only
// area 1 exists, as multi-area references are not supported. Arrays of
// row/column numbers give an array of (first) values.
func index(e *f.Env, a []f.Value) f.Value {
	src := a[0]
	if err, ok := src.(f.Error); ok {
		return err
	}
	if _, ok := src.(f.Ref); !ok {
		src = e.ToArray(src)
	}
	if !omitted(a, 3) {
		area, err := f.Int(one(a[3]))
		if err != nil {
			return err
		}
		if area < 1 {
			return f.ErrValue
		}
		if area > 1 {
			return f.ErrRef
		}
	}
	var rowArg, colArg f.Value = f.Missing{}, f.Missing{}
	if len(a) > 1 {
		rowArg = a[1]
	}
	if len(a) > 2 {
		colArg = a[2]
	}
	rows, cols := shape(src)
	pick := func(rv, cv f.Value) f.Value {
		r, err := f.Int(rv)
		if err != nil {
			return err
		}
		c, err := f.Int(cv)
		if err != nil {
			return err
		}
		if len(a) < 3 && rows == 1 {
			r, c = 1, r // INDEX({1,2,3},2): a position in a single row
		}
		switch {
		case r < 0 || c < 0:
			return f.ErrValue
		case r > rows || c > cols:
			return f.ErrRef
		}
		r0, r1, c0, c1 := r-1, r-1, c-1, c-1
		if r == 0 {
			r0, r1 = 0, rows-1
		}
		if c == 0 {
			c0, c1 = 0, cols-1
		}
		return sub(e, src, r0, c0, r1, c1)
	}
	ra, rMulti := multi(rowArg)
	ca, cMulti := multi(colArg)
	if !rMulti && !cMulti {
		return pick(one(rowArg), one(colArg))
	}
	return lift([]f.Value{ra, ca}, func(v []f.Value) f.Value { return topLeft(e, pick(v[0], v[1])) })
}

// multi returns v as an array when it has more than one element; the
// second result is false otherwise (and the first is v reduced to a
// scalar).
func multi(v f.Value) (f.Value, bool) {
	if a, ok := v.(*f.Array); ok && !a.Single() {
		return a, true
	}
	return one(v), false
}

// lift applies fn to the broadcast elements of inputs (arrays and
// scalars), giving an array the size of the largest input.
func lift(inputs []f.Value, fn func(v []f.Value) f.Value) *f.Array {
	rows, cols := 1, 1
	for _, v := range inputs {
		if a, ok := v.(*f.Array); ok {
			rows, cols = max(rows, a.Rows), max(cols, a.Cols)
		}
	}
	out := &f.Array{Rows: rows, Cols: cols, V: make([]f.Value, 0, rows*cols)}
	el := make([]f.Value, len(inputs))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			for i, v := range inputs {
				if a, ok := v.(*f.Array); ok {
					v = a.Broadcast(r, c)
				}
				el[i] = v
			}
			out.V = append(out.V, fn(el))
		}
	}
	return out
}

// CHOOSE(index_num, value1, [value2], ...). The chosen value is returned
// as is (references stay references). An array of indexes chooses element
// by element, broadcasting the values, so CHOOSE({1,2},B1:B3,A1:A3) builds
// a two-column array.
func choose(e *f.Env, a []f.Value) f.Value {
	pick := func(iv f.Value) (f.Value, f.Value) {
		i, err := f.Int(iv)
		if err != nil {
			return nil, err
		}
		if i < 1 || i >= len(a) {
			return nil, f.ErrValue
		}
		return a[i], nil
	}
	idx, ok := multi(a[0])
	if !ok {
		v, err := pick(idx)
		if err != nil {
			return err
		}
		return v
	}
	inputs := []f.Value{idx}
	for _, v := range a[1:] {
		inputs = append(inputs, e.Deref(v))
	}
	return lift(inputs, func(v []f.Value) f.Value {
		i, err := f.Int(v[0])
		if err != nil {
			return err
		}
		if i < 1 || i >= len(v) {
			return f.ErrValue
		}
		if _, ok := v[i].(f.Missing); ok {
			return f.Number(0)
		}
		return v[i]
	})
}

// ROW([reference]) / COLUMN([reference]): 1-based numbers, an array of
// them for a multi-row (multi-column) reference, the formula's own cell
// when omitted.
func rowCol(e *f.Env, a []f.Value, row bool) f.Value {
	r := f.Ref{Sheet: e.Sheet, R1: e.Row, C1: e.Col, R2: e.Row, C2: e.Col}
	if len(a) > 0 {
		switch v := a[0].(type) {
		case f.Ref:
			r = v
		case f.Missing:
		case f.Error:
			return v
		default:
			return f.ErrValue
		}
	}
	first, n := r.C1, r.Cols()
	if row {
		first, n = r.R1, r.Rows()
	}
	if n == 1 {
		return f.Number(first + 1)
	}
	out := &f.Array{Rows: 1, Cols: n, V: make([]f.Value, n)}
	if row {
		out.Rows, out.Cols = n, 1
	}
	for i := range out.V {
		out.V[i] = f.Number(first + i + 1)
	}
	return out
}

// ROWS(array) / COLUMNS(array). References are measured, not
// materialized: ROWS(A:A) is the sheet's row count.
func count(v f.Value, rows bool) f.Value {
	if err, ok := v.(f.Error); ok {
		return err
	}
	r, c := shape(v)
	if rows {
		return f.Number(r)
	}
	return f.Number(c)
}

// Excel's sheet size, which ADDRESS validates against (it only makes
// text, so it is not limited to this program's smaller grid).
const (
	excelRows = 1048576
	excelCols = 16384
)

// ADDRESS(row_num, column_num, [abs_num], [a1], [sheet_text]). abs_num 1
// is $A$1, 2 A$1, 3 $A1, 4 A1; a1 FALSE gives R1C1 notation with relative
// parts in brackets (R2C[3]).
func address(e *f.Env, a []f.Value) f.Value {
	row, err := f.Int(a[0])
	if err != nil {
		return err
	}
	col, err := f.Int(a[1])
	if err != nil {
		return err
	}
	abs, err := intArg(a, 2, 1)
	if err != nil {
		return err
	}
	a1, err := boolArg(a, 3, true)
	if err != nil {
		return err
	}
	if row < 1 || col < 1 || row > excelRows || col > excelCols || abs < 1 || abs > 4 {
		return f.ErrValue
	}
	absRow, absCol := abs == 1 || abs == 2, abs == 1 || abs == 3
	var s string
	if a1 {
		s = dollar(absCol) + model.ColName(col-1) + dollar(absRow) + strconv.Itoa(row)
	} else {
		s = "R" + bracket(row, absRow) + "C" + bracket(col, absCol)
	}
	if !omitted(a, 4) {
		sheet, err := f.Str(a[4])
		if err != nil {
			return err
		}
		s = sheetPrefix(sheet) + s
	}
	return f.String(s)
}

func dollar(abs bool) string {
	if abs {
		return "$"
	}
	return ""
}

func bracket(n int, abs bool) string {
	if abs {
		return strconv.Itoa(n)
	}
	return "[" + strconv.Itoa(n) + "]"
}

// sheetPrefix is name followed by '!', quoted when it contains characters
// other than letters, digits, '_' and '.' or starts with a digit.
func sheetPrefix(name string) string {
	quote := name != "" && name[0] >= '0' && name[0] <= '9'
	for _, c := range name {
		if !(c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c > 127) {
			quote = true
		}
	}
	if quote {
		return "'" + strings.ReplaceAll(name, "'", "''") + "'!"
	}
	return name + "!"
}

// TRANSPOSE(array)
func transpose(e *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case *f.Array:
		return transposed(v)
	case f.Missing:
		return f.Number(0)
	}
	return a[0]
}

// OFFSET(reference, rows, cols, [height], [width]): the reference moved
// down rows and right cols, height×width in size (by default reference's
// size). A negative height or width extends up or left from the moved
// top-left cell, as current Excel does; 0 is #REF!, as is a result off the
// sheet. The result is a reference, so SUM(OFFSET(...)) and
// OFFSET(...):B5 work.
func offset(e *f.Env, a []f.Value) f.Value {
	ref, ok := a[0].(f.Ref)
	if !ok {
		if err, ok := a[0].(f.Error); ok {
			return err
		}
		return f.ErrValue
	}
	dr, err := f.Int(a[1])
	if err != nil {
		return err
	}
	dc, err := f.Int(a[2])
	if err != nil {
		return err
	}
	h, err := intArg(a, 3, ref.Rows())
	if err != nil {
		return err
	}
	w, err := intArg(a, 4, ref.Cols())
	if err != nil {
		return err
	}
	if h == 0 || w == 0 {
		return f.ErrRef
	}
	r := f.Ref{Sheet: ref.Sheet, R1: ref.R1 + dr, C1: ref.C1 + dc}
	r.R2, r.C2 = r.R1+h-1, r.C1+w-1
	if h < 0 {
		r.R1, r.R2 = r.R1+h+1, r.R1
	}
	if w < 0 {
		r.C1, r.C2 = r.C1+w+1, r.C1
	}
	if r.R1 < 0 || r.C1 < 0 || r.R2 >= model.MaxRows || r.C2 >= model.MaxCols {
		return f.ErrRef
	}
	return r
}

// INDIRECT(ref_text, [a1]): the reference ref_text names, in A1 style or,
// with a1 FALSE, R1C1 style (relative parts relative to the formula's
// cell). Text that is not a reference on this workbook is #REF!.
func indirect(e *f.Env, a []f.Value) f.Value {
	text, err := f.Str(a[0])
	if err != nil {
		return err
	}
	a1, err := boolArg(a, 1, true)
	if err != nil {
		return err
	}
	return e.Reference(text, a1)
}
