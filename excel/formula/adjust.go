package formula

import (
	"strconv"
	"strings"

	"officelite/excel/model"
)

// Excel's sheet limits, which bound references (wider than model's).
const (
	excelRows = 1048576
	excelCols = 16384
)

// AdjustRows rewrites the references in formula f (without '=') after n
// rows were inserted before row at (n > 0) or rows at..at-n-1 were deleted
// (n < 0) on sheet target. home is the sheet the formula lives on: its
// unqualified references point at home; pass "" for a defined name (then
// only sheet-qualified references can match). Sheet names compare
// case-insensitively. Absolute and relative references move alike; a
// reference to deleted cells becomes #REF!.
func AdjustRows(f, home, target string, at, n int) string {
	return rewriteRefs(f, func(t *refTok) string {
		if t.name != "" || !t.on(home, target) || t.r.kind == refCols {
			return t.text
		}
		r := t.r
		var ok bool
		r.r1, r.r2, ok = shiftSpan(r.r1, r.r2, at, n, excelRows)
		return t.result(r, ok)
	})
}

// AdjustCols is AdjustRows for inserted or deleted columns.
func AdjustCols(f, home, target string, at, n int) string {
	return rewriteRefs(f, func(t *refTok) string {
		if t.name != "" || !t.on(home, target) || t.r.kind == refRows {
			return t.text
		}
		r := t.r
		var ok bool
		r.c1, r.c2, ok = shiftSpan(r.c1, r.c2, at, n, excelCols)
		return t.result(r, ok)
	})
}

// MoveRefs re-points references that lie entirely inside src (on sheet
// target) by dRow, dCol, as Excel does when cells are cut and pasted.
// Absolute and relative parts both move.
func MoveRefs(f, home, target string, src model.Range, dRow, dCol int) string {
	return rewriteRefs(f, func(t *refTok) string {
		r := t.r
		if t.name != "" || !t.on(home, target) || r.kind != refCell {
			return t.text
		}
		if !src.Contains(r.r1, r.c1) || !src.Contains(r.r2, r.c2) {
			return t.text
		}
		r.r1, r.r2, r.c1, r.c2 = r.r1+dRow, r.r2+dRow, r.c1+dCol, r.c2+dCol
		ok := min(r.r1, r.r2, r.c1, r.c2) >= 0 && max(r.r1, r.r2) < excelRows && max(r.c1, r.c2) < excelCols
		return t.result(r, ok)
	})
}

// RenameSheetRefs replaces the sheet prefix old with new, quoting new when
// it needs it.
func RenameSheetRefs(f, old, new string) string {
	return rewriteRefs(f, func(t *refTok) string {
		if t.sheet == "" || !strings.EqualFold(t.sheet, old) {
			return t.text
		}
		if t.name != "" {
			return QuoteSheet(new) + "!" + t.name
		}
		return QuoteSheet(new) + "!" + t.r.String()
	})
}

// DeleteSheetRefs turns every reference to sheet into #REF!.
func DeleteSheetRefs(f, sheet string) string {
	return rewriteRefs(f, func(t *refTok) string {
		if t.sheet == "" || !strings.EqualFold(t.sheet, sheet) {
			return t.text
		}
		return string(ErrRef)
	})
}

// QuoteSheet returns a sheet name as it is written before '!' in a
// formula: quoted when it is not a plain identifier or could be read as a
// cell reference.
func QuoteSheet(name string) string {
	plain := name != "" && isIdentStart(name[0])
	for i := 0; plain && i < len(name); i++ {
		c := name[i]
		plain = isLetter(c) || c >= '0' && c <= '9' || c == '_' || c == '.' || c >= 0x80
	}
	if plain && (matchCell(name) == len(name) || looksR1C1(name)) {
		plain = false
	}
	if plain {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// looksR1C1 reports whether s reads as an R1C1 reference (R, C, R2, C3,
// R1C1), which Excel also quotes.
func looksR1C1(s string) bool {
	s = strings.ToUpper(s)
	digits := func(s string) string {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return s[i:]
	}
	if strings.HasPrefix(s, "R") {
		s = digits(s[1:])
		if s == "" {
			return true
		}
	}
	return strings.HasPrefix(s, "C") && digits(s[1:]) == ""
}

// shiftSpan moves the span a..b (in either order) for a structural change
// of n entries at at. A span wholly deleted or pushed off the sheet is
// invalid; one partly deleted or pushed off is clipped.
func shiftSpan(a, b int, at, n, limit int) (int, int, bool) {
	lo, hi := min(a, b), max(a, b)
	if n > 0 {
		if lo >= at {
			lo += n
		}
		if hi >= at {
			hi += n
		}
		if lo >= limit {
			return a, b, false
		}
		hi = min(hi, limit-1)
	} else {
		end := at - n // first index after the deleted block
		if lo >= at && hi < end {
			return a, b, false
		}
		switch {
		case lo >= end:
			lo += n
		case lo >= at:
			lo = at
		}
		switch {
		case hi >= end:
			hi += n
		case hi >= at:
			hi = at - 1
		}
	}
	if a <= b {
		return lo, hi, true
	}
	return hi, lo, true
}

// Kinds of reference recognized by rewriteRefs.
const (
	refCell = iota // A1 or A1:B2
	refCols        // A:C
	refRows        // 1:3
)

// adjRef is a parsed reference with its absolute markers.
type adjRef struct {
	kind               int
	r1, c1, r2, c2     int // zero-based; unused axes are zero
	ar1, ac1, ar2, ac2 bool
	rng                bool // a cell range rather than one cell
}

func (r adjRef) String() string {
	col := func(abs bool, c int) string {
		if abs {
			return "$" + model.ColName(c)
		}
		return model.ColName(c)
	}
	row := func(abs bool, n int) string {
		if abs {
			return "$" + strconv.Itoa(n+1)
		}
		return strconv.Itoa(n + 1)
	}
	switch r.kind {
	case refCols:
		return col(r.ac1, r.c1) + ":" + col(r.ac2, r.c2)
	case refRows:
		return row(r.ar1, r.r1) + ":" + row(r.ar2, r.r2)
	}
	s := col(r.ac1, r.c1) + row(r.ar1, r.r1)
	if r.rng {
		s += ":" + col(r.ac2, r.c2) + row(r.ar2, r.r2)
	}
	return s
}

// refTok is a reference (or sheet-qualified name) found in formula text.
type refTok struct {
	text   string // the original text, prefix included
	prefix string // "Sheet1!" or "'My Sheet'!"; "" when unqualified
	sheet  string // unquoted sheet name; "" when unqualified
	name   string // set for a sheet-qualified defined name instead of r
	r      adjRef
}

// on reports whether the reference points at sheet target.
func (t *refTok) on(home, target string) bool {
	s := t.sheet
	if s == "" {
		s = home
	}
	return s != "" && strings.EqualFold(s, target)
}

// result writes r back with the original prefix, keeping the original text
// when nothing changed.
func (t *refTok) result(r adjRef, ok bool) string {
	switch {
	case !ok:
		return string(ErrRef)
	case r == t.r:
		return t.text
	}
	return t.prefix + r.String()
}

// rewriteRefs calls fn for every reference in f and replaces it with the
// returned text. String literals and error constants are copied as is.
func rewriteRefs(f string, fn func(t *refTok) string) string {
	return rewriteRefsAt(f, func(t *refTok, _ int) string { return fn(t) })
}

// rewriteRefsAt is rewriteRefs with each reference's byte offset in f.
func rewriteRefsAt(f string, fn func(t *refTok, at int) string) string {
	var b strings.Builder
	b.Grow(len(f) + 8)
	for i := 0; i < len(f); {
		c := f[i]
		boundary := i == 0 || !isNameChar(f[i-1]) && f[i-1] != '$'
		switch {
		case c == '"':
			j := skipQuoted(f, i)
			b.WriteString(f[i:j])
			i = j
		case c == '#':
			_, n := lexError(f[i:])
			n = max(n, 1)
			b.WriteString(f[i : i+n])
			i += n
		case c == '\'' || boundary && (isIdentStart(c) || c == '$' || c >= '0' && c <= '9'):
			if t, ok := scanRef(f[i:]); ok {
				b.WriteString(fn(&t, i))
				i += len(t.text)
				continue
			}
			j := i + 1
			if c == '\'' {
				j = skipQuoted(f, i)
			} else {
				for j < len(f) && (isIdentChar(f[j]) || f[j] == '$') {
					j++
				}
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

// scanSheet reads a sheet prefix ("Sheet1!" or "'My Sheet'!") at the start
// of s and returns the sheet name and the prefix length (0 if none).
func scanSheet(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	if s[0] == '\'' {
		j := skipQuoted(s, 0)
		if j >= 2 && j < len(s) && s[j-1] == '\'' && s[j] == '!' {
			return strings.ReplaceAll(s[1:j-1], "''", "'"), j + 1
		}
		return "", 0
	}
	if !isIdentStart(s[0]) {
		return "", 0
	}
	j := 0
	for j < len(s) && isIdentChar(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '!' {
		return s[:j], j + 1
	}
	return "", 0
}

// scanRef reads an optionally sheet-qualified reference at the start of s:
// a cell, a cell range, a column range or a row range, or a
// sheet-qualified name.
func scanRef(s string) (refTok, bool) {
	var t refTok
	sheet, p := scanSheet(s)
	t.sheet, t.prefix = sheet, s[:p]
	rest := s[p:]
	var r adjRef
	switch {
	case matchColRange(rest) > 0:
		n := matchColRange(rest)
		a, b, _ := strings.Cut(rest[:n], ":")
		r = adjRef{kind: refCols}
		r.ac1, r.c1 = parseAbsCol(a)
		r.ac2, r.c2 = parseAbsCol(b)
		t.text = s[:p+n]
	case matchRowRange(rest) > 0:
		n := matchRowRange(rest)
		a, b, _ := strings.Cut(rest[:n], ":")
		r = adjRef{kind: refRows}
		r.ar1, r.r1 = parseAbsRow(a)
		r.ar2, r.r2 = parseAbsRow(b)
		t.text = s[:p+n]
	default:
		c, n, ok := parseShiftRef(rest)
		if !ok || n != matchCell(rest) {
			if sheet == "" {
				return t, false
			}
			// A sheet-level name: Sheet1!TaxRate.
			j := 0
			for j < len(rest) && isIdentChar(rest[j]) {
				j++
			}
			if j == 0 || !isIdentStart(rest[0]) {
				return t, false
			}
			t.name, t.text = rest[:j], s[:p+j]
			return t, true
		}
		r = adjRef{kind: refCell, r1: c.row, c1: c.col, r2: c.row, c2: c.col,
			ar1: c.rowAbs, ac1: c.colAbs, ar2: c.rowAbs, ac2: c.colAbs}
		t.text = s[:p+n]
		// A range: A1:B2, or Sheet1!A1:Sheet1!B2.
		if n < len(rest) && rest[n] == ':' {
			second := rest[n+1:]
			sh2, p2 := scanSheet(second)
			if p2 == 0 || strings.EqualFold(sh2, sheet) {
				if c2, n2, ok := parseShiftRef(second[p2:]); ok && n2 == matchCell(second[p2:]) {
					r.rng = true
					r.r2, r.c2, r.ar2, r.ac2 = c2.row, c2.col, c2.rowAbs, c2.colAbs
					t.text = s[:p+n+1+p2+n2]
				}
			}
		}
	}
	t.r = r
	return t, true
}

func parseAbsCol(s string) (bool, int) {
	abs := strings.HasPrefix(s, "$")
	return abs, model.ParseColName(strings.TrimPrefix(s, "$"))
}

func parseAbsRow(s string) (bool, int) {
	abs := strings.HasPrefix(s, "$")
	n, _ := strconv.Atoi(strings.TrimPrefix(s, "$"))
	return abs, n - 1
}
