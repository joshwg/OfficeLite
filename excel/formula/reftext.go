package formula

import (
	"strconv"
	"strings"

	"officelite/excel/model"
)

// Reference resolves reference text the way INDIRECT does: an A1 (a1
// true) or R1C1 reference, optionally sheet-qualified ('My Sheet'!A1),
// a range of two of them, whole columns or rows, or a defined name that
// refers to a range. R1C1 relative parts (R[-1]C) are relative to the
// environment's cell. Anything else, including references to other
// workbooks and ones off the sheet, is #REF!.
func (e *Env) Reference(text string, a1 bool) Value {
	text = strings.TrimSpace(text)
	if text == "" || text[0] == '=' {
		return ErrRef
	}
	var v Value = ErrRef
	if a1 {
		if n, err := Parse(text); err == nil && refText(n) {
			v = e.evalRefText(n)
		}
	} else if r, ok := e.r1c1(text); ok {
		v = r
	} else if n, err := Parse(text); err == nil {
		if _, ok := n.(nameNode); ok {
			v = e.evalRefText(n)
		}
	}
	r, ok := v.(Ref)
	if !ok || r.R1 < 0 || r.C1 < 0 || r.R2 >= model.MaxRows || r.C2 >= model.MaxCols {
		return ErrRef
	}
	return r
}

// refText reports whether n is made only of references, names and the
// range operator (A1, Sheet2!A1:B3, Name, A1#).
func refText(n Node) bool {
	switch n := n.(type) {
	case refNode, nameNode:
		return true
	case binaryNode:
		return n.op == ":" && refText(n.l) && refText(n.r)
	case spillNode:
		return refText(n.x)
	}
	return false
}

// evalRefText evaluates reference text outside any LET/LAMBDA scope:
// INDIRECT("x") names a defined name, never a LET variable.
func (e *Env) evalRefText(n Node) Value {
	saved := e.scope
	e.scope = nil
	defer func() { e.scope = saved }()
	return e.Eval(n)
}

// r1c1 parses an R1C1 reference: [sheet!]part[:part], where a part is
// RnCn (a cell), Rn (whole rows) or Cn (whole columns), each n being an
// absolute 1-based number, [±k] relative to the environment's cell, or
// absent (the cell's own row or column).
func (e *Env) r1c1(s string) (Ref, bool) {
	sheet := e.Sheet
	if name, rest, ok := sheetQualifier(s); ok {
		if sheet = e.Ctx.SheetIndex(name); sheet < 0 {
			return Ref{}, false
		}
		s = rest
	}
	a, b, isRange := strings.Cut(s, ":")
	r1, c1, kind1, ok := e.r1c1Part(a)
	if !ok {
		return Ref{}, false
	}
	r2, c2, kind2 := r1, c1, kind1
	if isRange {
		if r2, c2, kind2, ok = e.r1c1Part(b); !ok || kind2 != kind1 {
			return Ref{}, false
		}
	}
	r := normRef(Ref{Sheet: sheet, R1: r1, C1: c1, R2: r2, C2: c2})
	switch kind1 {
	case 'R':
		r.C1, r.C2 = 0, model.MaxCols-1
	case 'C':
		r.R1, r.R2 = 0, model.MaxRows-1
	}
	return r, true
}

// r1c1Part parses one R1C1 part, returning its row and column (zero-based)
// and its kind: 'R' for whole rows, 'C' for whole columns, 'X' for a cell.
func (e *Env) r1c1Part(s string) (row, col int, kind byte, ok bool) {
	num := func(s string, base int) (int, string, bool) {
		if strings.HasPrefix(s, "[") {
			end := strings.IndexByte(s, ']')
			if end < 0 {
				return 0, "", false
			}
			k, err := strconv.Atoi(s[1:end])
			return base + k, s[end+1:], err == nil
		}
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 {
			return base, s, true
		}
		n, err := strconv.Atoi(s[:i])
		return n - 1, s[i:], err == nil && n >= 1
	}
	s = strings.ToUpper(s)
	hasR, hasC := false, false
	if strings.HasPrefix(s, "R") {
		if row, s, ok = num(s[1:], e.Row); !ok {
			return
		}
		hasR = true
	}
	if strings.HasPrefix(s, "C") {
		if col, s, ok = num(s[1:], e.Col); !ok {
			return
		}
		hasC = true
	}
	switch {
	case s != "":
		return 0, 0, 0, false
	case hasR && hasC:
		kind = 'X'
	case hasR:
		kind = 'R'
	case hasC:
		kind = 'C'
	default:
		return 0, 0, 0, false
	}
	return row, col, kind, true
}

// sheetQualifier splits "Sheet!rest" or "'My Sheet'!rest".
func sheetQualifier(s string) (sheet, rest string, ok bool) {
	if strings.HasPrefix(s, "'") {
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			if s[i] != '\'' {
				b.WriteByte(s[i])
				continue
			}
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			if i+1 < len(s) && s[i+1] == '!' {
				return b.String(), s[i+2:], true
			}
			return "", "", false
		}
		return "", "", false
	}
	return strings.Cut(s, "!")
}
