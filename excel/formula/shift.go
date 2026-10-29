// Package formula handles formula text. Evaluation (the function library)
// will live alongside it; for now it only rewrites references.
package formula

import (
	"strconv"

	"officelite/excel/model"
)

// Shift moves every relative A1 reference in f by dRow rows and dCol
// columns, as Excel does when a formula is copied or a shared formula is
// expanded. Absolute parts ($A, $1) stay put; whole columns (A:A) move
// only sideways and whole rows (1:1) only up or down; string literals and
// quoted sheet names are left alone. A reference moved off the sheet
// (any part of a range) becomes #REF!.
func Shift(f string, dRow, dCol int) string {
	if dRow == 0 && dCol == 0 {
		return f
	}
	return rewriteRefs(f, func(t *refTok) string {
		if t.name != "" {
			return t.text
		}
		r := t.r
		if r.kind != refRows {
			if !r.ac1 {
				r.c1 += dCol
			}
			if !r.ac2 {
				r.c2 += dCol
			}
		}
		if r.kind != refCols {
			if !r.ar1 {
				r.r1 += dRow
			}
			if !r.ar2 {
				r.r2 += dRow
			}
		}
		if min(r.r1, r.r2, r.c1, r.c2) < 0 || max(r.r1, r.r2) >= excelRows || max(r.c1, r.c2) >= excelCols {
			return string(ErrRef)
		}
		return t.result(r, true)
	})
}

// skipQuoted returns the index just past the quoted run starting at i; a
// doubled quote character is an escaped quote.
func skipQuoted(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		if s[j] == q {
			if j+1 < len(s) && s[j+1] == q {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(s)
}

type cellRef struct {
	colAbs, rowAbs bool
	col, row       int // zero-based
}

// parseShiftRef parses [$]COL[$]ROW at the start of s and returns how many bytes
// it used. It rejects names that merely look like references, such as
// function calls (LOG10() or longer identifiers (ABC12X).
func parseShiftRef(s string) (cellRef, int, bool) {
	var r cellRef
	i := 0
	if i < len(s) && s[i] == '$' {
		r.colAbs = true
		i++
	}
	start := i
	for i < len(s) && i-start < 4 && isLetter(s[i]) {
		i++
	}
	if i == start || i-start > 3 {
		return r, 0, false
	}
	r.col = model.ParseColName(s[start:i])
	if i < len(s) && s[i] == '$' {
		r.rowAbs = true
		i++
	}
	dstart := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == dstart {
		return r, 0, false
	}
	if i < len(s) && (isNameChar(s[i]) || s[i] == '(') {
		return r, 0, false
	}
	row, err := strconv.Atoi(s[dstart:i])
	if err != nil || row < 1 || r.col < 0 {
		return r, 0, false
	}
	r.row = row - 1
	return r, i, true
}

func isLetter(c byte) bool { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }
func isNameChar(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '\\'
}
