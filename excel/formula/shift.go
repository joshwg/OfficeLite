// Package formula handles formula text. Evaluation (the function library)
// will live alongside it; for now it only rewrites references.
package formula

import (
	"strconv"
	"strings"

	"officelite/excel/model"
)

// Shift moves every relative A1 reference in f by dRow rows and dCol
// columns, as Excel does when a formula is copied or a shared formula is
// expanded. Absolute parts ($A, $1) stay put; string literals and quoted
// sheet names are left alone.
func Shift(f string, dRow, dCol int) string {
	if dRow == 0 && dCol == 0 {
		return f
	}
	var b strings.Builder
	b.Grow(len(f) + 8)
	for i := 0; i < len(f); {
		c := f[i]
		switch {
		case c == '"' || c == '\'':
			j := skipQuoted(f, i)
			b.WriteString(f[i:j])
			i = j
		case isRefStart(c) && (i == 0 || !isNameChar(f[i-1])):
			if ref, n, ok := parseShiftRef(f[i:]); ok {
				b.WriteString(ref.shift(dRow, dCol))
				i += n
				continue
			}
			// Not a reference: copy the whole identifier.
			j := i + 1
			for j < len(f) && isNameChar(f[j]) {
				j++
			}
			b.WriteString(f[i:j])
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
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

func (r cellRef) shift(dRow, dCol int) string {
	if !r.colAbs {
		r.col += dCol
	}
	if !r.rowAbs {
		r.row += dRow
	}
	if r.col < 0 || r.row < 0 {
		return "#REF!"
	}
	var b strings.Builder
	if r.colAbs {
		b.WriteByte('$')
	}
	b.WriteString(model.ColName(r.col))
	if r.rowAbs {
		b.WriteByte('$')
	}
	b.WriteString(strconv.Itoa(r.row + 1))
	return b.String()
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

func isLetter(c byte) bool   { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }
func isRefStart(c byte) bool { return c == '$' || isLetter(c) }
func isNameChar(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '\\'
}
