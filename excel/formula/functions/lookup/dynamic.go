package lookup

import (
	"math"
	"slices"
	"strconv"
	"strings"

	f "officelite/excel/formula"
)

// FILTER(array, include, [if_empty]). include is a column as tall as array
// (keeping rows) or a row as wide as it (keeping columns); an error in it
// is returned. With nothing kept the result is if_empty, or #CALC!.
func filter(e *f.Env, a []f.Value) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	inc, err := array(e, a[1])
	if err != nil {
		return err
	}
	byRow := inc.Cols == 1 && inc.Rows == arr.Rows
	if !byRow && !(inc.Rows == 1 && inc.Cols == arr.Cols) {
		return f.ErrValue
	}
	var keep []int
	for i, v := range inc.V {
		if _, ok := v.(f.String); ok {
			return f.ErrValue // text, even "TRUE", is not a condition here
		}
		b, err := f.Truth(v)
		if err != nil {
			return err
		}
		if b {
			keep = append(keep, i)
		}
	}
	if len(keep) == 0 {
		if omitted(a, 2) {
			return f.ErrCalc
		}
		return single(a[2])
	}
	return pickLines(arr, keep, byRow)
}

// pickLines builds an array from the listed rows (byRow) or columns of a.
func pickLines(a *f.Array, lines []int, byRow bool) f.Value {
	var out *f.Array
	if byRow {
		out = &f.Array{Rows: len(lines), Cols: a.Cols, V: make([]f.Value, 0, len(lines)*a.Cols)}
		for _, r := range lines {
			out.V = append(out.V, a.Row(r)...)
		}
	} else {
		out = &f.Array{Rows: a.Rows, Cols: len(lines), V: make([]f.Value, 0, len(lines)*a.Rows)}
		for r := 0; r < a.Rows; r++ {
			for _, c := range lines {
				out.V = append(out.V, a.At(r, c))
			}
		}
	}
	return single(out)
}

// line returns row (byRow) or column i of a.
func line(a *f.Array, i int, byRow bool) []f.Value {
	if byRow {
		return a.Row(i)
	}
	col := make([]f.Value, a.Rows)
	for r := range col {
		col[r] = a.At(r, i)
	}
	return col
}

// identity is a value's key for UNIQUE: values of different types never
// match (an empty cell is not "" or 0) and text ignores case.
func identity(v f.Value) string {
	switch x := v.(type) {
	case f.Number:
		return "n" + strconv.FormatFloat(float64(x), 'g', -1, 64)
	case f.String:
		return "s" + strings.ToLower(string(x))
	case f.Bool:
		if x {
			return "bT"
		}
		return "bF"
	case f.Error:
		return "e" + string(x)
	}
	return "_"
}

// UNIQUE(array, [by_col], [exactly_once]) keeps the first of each distinct
// row (column), or with exactly_once only those occurring once.
func unique(e *f.Env, a []f.Value) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	byCol, err := boolArg(a, 1, false)
	if err != nil {
		return err
	}
	once, err := boolArg(a, 2, false)
	if err != nil {
		return err
	}
	n := arr.Rows
	if byCol {
		n = arr.Cols
	}
	keys := make([]string, n)
	counts := map[string]int{}
	for i := range keys {
		var b strings.Builder
		for _, v := range line(arr, i, !byCol) {
			b.WriteString(identity(v))
			b.WriteByte(0)
		}
		keys[i] = b.String()
		counts[keys[i]]++
	}
	var keep []int
	seen := map[string]bool{}
	for i, k := range keys {
		if once && counts[k] != 1 || seen[k] {
			continue
		}
		seen[k] = true
		keep = append(keep, i)
	}
	if len(keep) == 0 {
		return f.ErrCalc
	}
	return pickLines(arr, keep, !byCol)
}

// sortClass orders types as Excel's sort does: numbers, text, logical
// values, errors, and blanks last in either direction.
func sortClass(v f.Value) int {
	switch v.(type) {
	case f.Number:
		return 0
	case f.String:
		return 1
	case f.Bool:
		return 2
	case f.Error:
		return 3
	}
	return 4
}

// sortCmp compares two values for sorting in order (1 ascending, -1
// descending).
func sortCmp(x, y f.Value, order int) int {
	cx, cy := sortClass(x), sortClass(y)
	switch {
	case cx == 4 || cy == 4:
		return cmpInt(cx, cy) // blanks last regardless of order
	case cx != cy:
		return order * cmpInt(cx, cy)
	case cx == 3:
		return order * cmpInt(f.ErrorCodes[x.(f.Error)], f.ErrorCodes[y.(f.Error)])
	}
	c, _ := f.Compare(x, y)
	return order * c
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// sortKey is one sort level: the values to sort by, one per line, and the
// direction.
type sortKey struct {
	vals  []f.Value
	order int
}

// sortLines stably sorts the rows (byRow) or columns of a by keys.
func sortLines(a *f.Array, keys []sortKey, byRow bool) f.Value {
	n := a.Rows
	if !byRow {
		n = a.Cols
	}
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	slices.SortStableFunc(perm, func(i, j int) int {
		for _, k := range keys {
			if c := sortCmp(k.vals[i], k.vals[j], k.order); c != 0 {
				return c
			}
		}
		return 0
	})
	return pickLines(a, perm, byRow)
}

// numbers reads an argument that may be a number or an array of numbers.
func numbers(v f.Value) ([]int, f.Value) {
	var vals []f.Value
	if arr, ok := v.(*f.Array); ok {
		vals = arr.V
	} else {
		vals = []f.Value{v}
	}
	out := make([]int, len(vals))
	for i, x := range vals {
		n, err := f.Num(x)
		if err != nil {
			return nil, err
		}
		out[i] = int(math.Trunc(n))
	}
	return out, nil
}

// SORT(array, [sort_index], [sort_order], [by_col]). sort_index and
// sort_order may be arrays for several sort levels; sort_order is 1
// (ascending) or -1.
func sortFn(e *f.Env, a []f.Value) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	idx, ord := []int{1}, []int{1}
	if !omitted(a, 1) {
		if idx, err = numbers(a[1]); err != nil {
			return err
		}
	}
	if !omitted(a, 2) {
		if ord, err = numbers(a[2]); err != nil {
			return err
		}
	}
	byCol, err := boolArg(a, 3, false)
	if err != nil {
		return err
	}
	if len(ord) == 1 {
		for len(ord) < len(idx) {
			ord = append(ord, ord[0])
		}
	}
	if len(ord) != len(idx) {
		return f.ErrValue
	}
	width := arr.Cols
	if byCol {
		width = arr.Rows
	}
	var keys []sortKey
	for i, k := range idx {
		if k < 1 || k > width || ord[i] != 1 && ord[i] != -1 {
			return f.ErrValue
		}
		// The sort column (row) holds one key per line.
		keys = append(keys, sortKey{line(arr, k-1, byCol), ord[i]})
	}
	return sortLines(arr, keys, !byCol)
}

// SORTBY(array, by_array1, [sort_order1], [by_array2, sort_order2], ...).
// Every by_array is a column as tall as array (sorting rows) or a row as
// wide as it (sorting columns), all the same way.
func sortBy(e *f.Env, a []f.Value) f.Value {
	arr, err := array(e, a[0])
	if err != nil {
		return err
	}
	var keys []sortKey
	byRow := true
	for i := 1; i < len(a); i += 2 {
		by, err := array(e, a[i])
		if err != nil {
			return err
		}
		rowWise := by.Cols == 1 && by.Rows == arr.Rows
		if !rowWise && !(by.Rows == 1 && by.Cols == arr.Cols) || i > 1 && rowWise != byRow {
			return f.ErrValue
		}
		byRow = rowWise
		order, err := intArg(a, i+1, 1)
		if err != nil {
			return err
		}
		if order != 1 && order != -1 {
			return f.ErrValue
		}
		keys = append(keys, sortKey{by.V, order})
	}
	return sortLines(arr, keys, byRow)
}
