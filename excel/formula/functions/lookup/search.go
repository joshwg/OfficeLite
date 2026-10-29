package lookup

import (
	"strings"

	f "officelite/excel/formula"
)

// Match modes (XLOOKUP/XMATCH numbering).
const (
	exact      = 0
	nextSmall  = -1
	nextLarge  = 1
	wildcard   = 2
	firstLast  = 1
	lastFirst  = -1
	binaryAsc  = 2
	binaryDesc = -2
)

// rank is a lookup value's type class: lookups only ever match values of
// the lookup value's own type. Blanks and errors (-1) never match.
func rank(v f.Value) int {
	switch v.(type) {
	case f.Number:
		return 0
	case f.String:
		return 1
	case f.Bool:
		return 2
	}
	return -1
}

// key normalizes a lookup value: an empty cell looks up 0.
func key(x f.Value) f.Value {
	switch x.(type) {
	case f.Blank, f.Missing:
		return f.Number(0)
	}
	return x
}

// equal reports an exact match of v against the lookup value x; with wild,
// text lookup values are wildcard patterns (* ? ~).
func equal(v, x f.Value, wild bool) bool {
	if rank(v) != rank(x) || rank(v) < 0 {
		return false
	}
	if wild {
		if p, ok := x.(f.String); ok && strings.ContainsAny(string(p), "*?~") {
			return f.WildcardMatch(string(p), string(v.(f.String)))
		}
	}
	return f.Equal(v, x)
}

// find returns the index in vals of the match for x, or -1. mode is an
// XLOOKUP match_mode (0, -1, 1, 2) and search a search_mode (1, -1, 2, -2).
// Binary searches assume the data is sorted, as Excel does: values of other
// types than x are skipped and the rest searched by bisection, so unsorted
// data gives the same kind of arbitrary-looking answers Excel gives.
func find(vals []f.Value, x f.Value, mode, search int) int {
	x = key(x)
	if search == binaryAsc || search == binaryDesc {
		return bfind(vals, x, mode, search == binaryDesc)
	}
	best := -1
	for k := range vals {
		i := k
		if search == lastFirst {
			i = len(vals) - 1 - k
		}
		v := vals[i]
		if equal(v, x, mode == wildcard) {
			return i
		}
		if (mode != nextSmall && mode != nextLarge) || rank(v) != rank(x) {
			continue
		}
		c, _ := f.Compare(v, x)
		if mode == nextSmall && c < 0 || mode == nextLarge && c > 0 {
			if best < 0 {
				best = i
				continue
			}
			// Closer than the best so far; ties keep the first in search order.
			d, _ := f.Compare(v, vals[best])
			if mode == nextSmall && d > 0 || mode == nextLarge && d < 0 {
				best = i
			}
		}
	}
	return best
}

// bfind is the binary-search part of find.
func bfind(vals []f.Value, x f.Value, mode int, desc bool) int {
	var idx []int
	for i, v := range vals {
		if rank(v) == rank(x) && rank(x) >= 0 {
			idx = append(idx, i)
		}
	}
	cmp := func(k int) int {
		c, _ := f.Compare(vals[idx[k]], x)
		return c
	}
	// count is the length of the prefix of idx for which pred holds
	// (pred is monotone on sorted data).
	count := func(pred func(c int) bool) int {
		// Closed-interval bisection probing the lower middle; this probe
		// order reproduces Excel's documented MATCH(40,{25,38,40,41},-1)
		// = #N/A on wrongly sorted data.
		lo, hi := 0, len(idx)-1
		for lo <= hi {
			mid := (lo + hi) / 2
			if pred(cmp(mid)) {
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		return lo
	}
	at := func(k int) int {
		if k < 0 || k >= len(idx) {
			return -1
		}
		return idx[k]
	}
	switch {
	case !desc && mode == nextSmall: // last value <= x
		return at(count(func(c int) bool { return c <= 0 }) - 1)
	case !desc && mode == nextLarge: // first value >= x
		return at(count(func(c int) bool { return c < 0 }))
	case !desc: // exact
		k := count(func(c int) bool { return c < 0 })
		if k < len(idx) && cmp(k) == 0 {
			return idx[k]
		}
	case mode == nextSmall: // descending: first value <= x
		return at(count(func(c int) bool { return c > 0 }))
	case mode == nextLarge: // descending: last value >= x
		return at(count(func(c int) bool { return c >= 0 }) - 1)
	default:
		k := count(func(c int) bool { return c > 0 })
		if k < len(idx) && cmp(k) == 0 {
			return idx[k]
		}
	}
	return -1
}

// approx is the approximate match of MATCH type 1, VLOOKUP/HLOOKUP with
// range_lookup TRUE and LOOKUP: the largest value <= x in ascending data.
func approx(vals []f.Value, x f.Value) int { return find(vals, x, nextSmall, binaryAsc) }

// --- argument helpers -----------------------------------------------------

// one reduces an array argument to its first element.
func one(v f.Value) f.Value {
	if a, ok := v.(*f.Array); ok {
		if a.Len() == 0 {
			return f.Blank{}
		}
		return a.V[0]
	}
	return v
}

// omitted reports whether optional argument i was left out or empty.
func omitted(a []f.Value, i int) bool {
	if i >= len(a) {
		return true
	}
	_, ok := one(a[i]).(f.Missing)
	return ok
}

// intArg is optional integer argument i, def when omitted.
func intArg(a []f.Value, i, def int) (int, f.Value) {
	if omitted(a, i) {
		return def, nil
	}
	return f.Int(one(a[i]))
}

// boolArg is optional logical argument i, def when omitted.
func boolArg(a []f.Value, i int, def bool) (bool, f.Value) {
	if omitted(a, i) {
		return def, nil
	}
	return f.Truth(one(a[i]))
}

// array turns an argument into an array: references are dereferenced,
// scalars wrapped; an error value is returned as the second result.
func array(e *f.Env, v f.Value) (*f.Array, f.Value) {
	if err, ok := v.(f.Error); ok {
		return nil, err
	}
	return e.ToArray(v), nil
}

// vector is a one-row or one-column array's values; 2-D arrays give ok
// false.
func vector(a *f.Array) (vals []f.Value, ok bool) {
	return a.V, a.Rows == 1 || a.Cols == 1
}

// single returns a 1×1 array's element, other values unchanged.
func single(v f.Value) f.Value {
	if a, ok := v.(*f.Array); ok && a.Single() {
		return a.V[0]
	}
	return v
}

// topLeft reduces a (possibly multi-cell) result to its first value, as
// Excel does when a lifted function produces an array per element.
func topLeft(e *f.Env, v f.Value) f.Value {
	if r, ok := v.(f.Ref); ok {
		return e.Ctx.CellValue(r.Sheet, r.R1, r.C1)
	}
	if _, ok := v.(f.Missing); ok {
		return f.Number(0)
	}
	return one(v)
}

// shape returns the dimensions of an argument (references untrimmed).
func shape(v f.Value) (rows, cols int) {
	switch x := v.(type) {
	case f.Ref:
		return x.Rows(), x.Cols()
	case *f.Array:
		return x.Rows, x.Cols
	}
	return 1, 1
}

// shapeWith is shape, except that a whole-column or whole-row reference is
// measured trimmed to the used area when it is combined with a calculated
// array (which Deref already trimmed): XLOOKUP(1,(B:B="x"),A:A).
func shapeWith(e *f.Env, v, other f.Value) (rows, cols int) {
	if r, ok := v.(f.Ref); ok {
		if _, arr := other.(*f.Array); arr {
			r = e.Trim(r)
			return r.Rows(), r.Cols()
		}
	}
	return shape(v)
}

// sub returns rows r0..r1 and columns c0..c1 (inclusive, zero-based) of a
// reference (as a reference) or array (as an array, or its element when
// 1×1).
func sub(e *f.Env, v f.Value, r0, c0, r1, c1 int) f.Value {
	switch x := v.(type) {
	case f.Ref:
		return f.Ref{Sheet: x.Sheet, R1: x.R1 + r0, C1: x.C1 + c0, R2: x.R1 + r1, C2: x.C1 + c1}
	case *f.Array:
		out := &f.Array{Rows: r1 - r0 + 1, Cols: c1 - c0 + 1}
		for r := r0; r <= r1; r++ {
			out.V = append(out.V, x.Row(r)[c0:c1+1]...)
		}
		return single(out)
	}
	return v
}
