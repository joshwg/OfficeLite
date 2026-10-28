package formula

import "officelite/excel/model"

// RefSpan is a reference found in formula text, for colouring the
// references of a formula being edited.
type RefSpan struct {
	Start, End int    // byte offsets in the text
	Sheet      string // the sheet named in the reference; "" when unqualified
	Range      model.Range
}

// RefSpans lists the cell, range, column and row references of formula
// text f (without its "="), in order. Names, strings and function names
// are skipped.
func RefSpans(f string) []RefSpan {
	var out []RefSpan
	rewriteRefsAt(f, func(t *refTok, start int) string {
		pos := start + len(t.text)
		if t.name != "" || pos < len(f) && f[pos] == '(' {
			return t.text // a sheet-level name, or a function like LOG10(
		}
		r := t.r
		rg := model.Range{R1: min(r.r1, r.r2), C1: min(r.c1, r.c2), R2: max(r.r1, r.r2), C2: max(r.c1, r.c2)}
		switch r.kind {
		case refCols:
			rg.R1, rg.R2 = 0, model.MaxRows-1
		case refRows:
			rg.C1, rg.C2 = 0, model.MaxCols-1
		}
		out = append(out, RefSpan{Start: start, End: pos, Sheet: t.sheet, Range: rg})
		return t.text
	})
	return out
}
