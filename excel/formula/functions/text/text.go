// Package text implements Excel's text functions: LEN, LEFT, MID, FIND,
// SEARCH, SUBSTITUTE, CONCAT, TEXTJOIN, TEXT, FIXED, DOLLAR, VALUE,
// TEXTBEFORE, TEXTAFTER, TEXTSPLIT, the REGEX functions, BAHTTEXT and the
// rest of the category.
//
// Lengths and positions are counted in UTF-16 code units, as Excel does:
// LEN("😀") is 2. Cutting a surrogate pair in half (LEFT("😀",1)) yields
// U+FFFD where Excel would keep the lone surrogate.
//
// The byte-counting variants (LENB, LEFTB, MIDB, RIGHTB, FINDB, SEARCHB,
// REPLACEB) behave exactly like their character versions, which is what
// Excel does unless a DBCS language (Japanese, Chinese, Korean) is set as
// the default editing language.
package text

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf16"

	f "officelite/excel/formula"
)

// maxText is the longest text a cell can hold; longer results are #VALUE!.
const maxText = 32767

// Register adds the text functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "LEN", Min: 1, Max: 1, Fn: lenFn},
		f.Spec{Name: "LEFT", Min: 1, Max: 2, Fn: left},
		f.Spec{Name: "RIGHT", Min: 1, Max: 2, Fn: right},
		f.Spec{Name: "MID", Min: 3, Max: 3, Fn: mid},
		f.Spec{Name: "REPLACE", Min: 4, Max: 4, Fn: replace},
		f.Spec{Name: "FIND", Min: 2, Max: 3, Fn: func(e *f.Env, a []f.Value) f.Value { return find(a, false) }},
		f.Spec{Name: "SEARCH", Min: 2, Max: 3, Fn: func(e *f.Env, a []f.Value) f.Value { return find(a, true) }},
		f.Spec{Name: "SUBSTITUTE", Min: 3, Max: 4, Fn: substitute},
		f.Spec{Name: "REPT", Min: 2, Max: 2, Fn: rept},
		f.Spec{Name: "UPPER", Min: 1, Max: 1, Fn: mapText(strings.ToUpper)},
		f.Spec{Name: "LOWER", Min: 1, Max: 1, Fn: mapText(strings.ToLower)},
		f.Spec{Name: "PROPER", Min: 1, Max: 1, Fn: mapText(proper)},
		f.Spec{Name: "TRIM", Min: 1, Max: 1, Fn: mapText(trim)},
		f.Spec{Name: "CLEAN", Min: 1, Max: 1, Fn: mapText(clean)},
		f.Spec{Name: "ASC", Min: 1, Max: 1, Fn: mapText(asc)},
		f.Spec{Name: "DBCS", Min: 1, Max: 1, Fn: mapText(dbcs)},
		f.Spec{Name: "EXACT", Min: 2, Max: 2, Fn: exact},
		f.Spec{Name: "T", Min: 1, Max: 1, Fn: tFn},
		f.Spec{Name: "CHAR", Min: 1, Max: 1, Fn: char},
		f.Spec{Name: "CODE", Min: 1, Max: 1, Fn: code},
		f.Spec{Name: "UNICHAR", Min: 1, Max: 1, Fn: unichar},
		f.Spec{Name: "UNICODE", Min: 1, Max: 1, Fn: unicodeFn},
		f.Spec{Name: "CONCATENATE", Min: 1, Max: 255, Fn: concatenate},
		f.Spec{Name: "CONCAT", Min: 1, Max: 253, Arrays: f.All, Fn: concat},
		f.Spec{Name: "TEXTJOIN", Min: 3, Max: 254, Arrays: f.All, Fn: textJoin},
		f.Spec{Name: "PHONETIC", Min: 1, Max: 1, Refs: f.All, Fn: phonetic},
		f.Spec{Name: "DETECTLANGUAGE", Min: 1, Max: 1, Fn: online},
		f.Spec{Name: "TRANSLATE", Min: 1, Max: 3, Fn: online},

		f.Spec{Name: "TEXT", Min: 2, Max: 2, Fn: textFn},
		f.Spec{Name: "FIXED", Min: 1, Max: 3, Fn: fixed},
		f.Spec{Name: "DOLLAR", Min: 1, Max: 2, Fn: dollar},
		f.Spec{Name: "VALUE", Min: 1, Max: 1, Fn: value},
		f.Spec{Name: "NUMBERVALUE", Min: 1, Max: 3, Fn: numberValue},
		f.Spec{Name: "VALUETOTEXT", Min: 1, Max: 2, Fn: valueToText},
		f.Spec{Name: "ARRAYTOTEXT", Min: 1, Max: 2, Arrays: f.Only(0), Fn: arrayToText},
		f.Spec{Name: "BAHTTEXT", Min: 1, Max: 1, Fn: bahtText},

		f.Spec{Name: "TEXTBEFORE", Min: 2, Max: 6, Arrays: f.Only(1), Fn: func(e *f.Env, a []f.Value) f.Value {
			return textBeforeAfter(a, false)
		}},
		f.Spec{Name: "TEXTAFTER", Min: 2, Max: 6, Arrays: f.Only(1), Fn: func(e *f.Env, a []f.Value) f.Value {
			return textBeforeAfter(a, true)
		}},
		f.Spec{Name: "TEXTSPLIT", Min: 2, Max: 6, Arrays: f.Only(0, 1, 2), Fn: textSplit},

		f.Spec{Name: "REGEXTEST", Min: 2, Max: 3, Fn: regexTest},
		f.Spec{Name: "REGEXEXTRACT", Min: 2, Max: 4, Fn: regexExtract},
		f.Spec{Name: "REGEXREPLACE", Min: 3, Max: 5, Fn: regexReplace},
	)
	// In a non-DBCS language setting the B variants count characters.
	for _, n := range []string{"LEN", "LEFT", "RIGHT", "MID", "REPLACE", "FIND", "SEARCH"} {
		s := *r.Lookup(n)
		s.Name = n + "B"
		r.Add(s)
	}
}

// --- helpers --------------------------------------------------------------

// units returns s as UTF-16 code units, Excel's unit of text length.
func units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func fromUnits(u []uint16) string { return string(utf16.Decode(u)) }

// ulen is the length of s in UTF-16 code units.
func ulen(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// result returns s, or #VALUE! when it is too long for a cell.
func result(s string) f.Value {
	if len(s) > maxText && ulen(s) > maxText {
		return f.ErrValue
	}
	return f.String(s)
}

// intArg converts v to an integer argument (truncated), def when omitted.
// Huge magnitudes are clamped rather than rejected: LEFT("abc",1E10) is
// "abc".
func intArg(v f.Value, def int) (int, f.Value) {
	if _, ok := v.(f.Missing); ok {
		return def, nil
	}
	x, err := f.Num(v)
	if err != nil {
		return 0, err
	}
	x = math.Max(-1e12, math.Min(1e12, math.Trunc(x)))
	return int(x), nil
}

// arg returns args[i], or Missing when the argument list is shorter.
func arg(args []f.Value, i int) f.Value {
	if i < len(args) {
		return args[i]
	}
	return f.Missing{}
}

func mapText(fn func(string) string) func(*f.Env, []f.Value) f.Value {
	return func(_ *f.Env, a []f.Value) f.Value {
		s, err := f.Str(a[0])
		if err != nil {
			return err
		}
		return f.String(fn(s))
	}
}

// --- length and slicing ---------------------------------------------------

// LEN(text)
func lenFn(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	return f.Number(ulen(s))
}

// LEFT(text, [num_chars])
func left(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	n, err := intArg(arg(a, 1), 1)
	if err != nil {
		return err
	}
	if n < 0 {
		return f.ErrValue
	}
	u := units(s)
	if n >= len(u) {
		return f.String(s)
	}
	return f.String(fromUnits(u[:n]))
}

// RIGHT(text, [num_chars])
func right(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	n, err := intArg(arg(a, 1), 1)
	if err != nil {
		return err
	}
	if n < 0 {
		return f.ErrValue
	}
	u := units(s)
	if n >= len(u) {
		return f.String(s)
	}
	return f.String(fromUnits(u[len(u)-n:]))
}

// MID(text, start_num, num_chars)
func mid(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	start, err := intArg(a[1], 1)
	if err != nil {
		return err
	}
	n, err := intArg(a[2], 0)
	if err != nil {
		return err
	}
	if start < 1 || n < 0 {
		return f.ErrValue
	}
	u := units(s)
	if start > len(u) {
		return f.String("")
	}
	return f.String(fromUnits(u[start-1 : min(len(u), start-1+n)]))
}

// REPLACE(old_text, start_num, num_chars, new_text)
func replace(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	start, err := intArg(a[1], 1)
	if err != nil {
		return err
	}
	n, err := intArg(a[2], 0)
	if err != nil {
		return err
	}
	repl, err := f.Str(a[3])
	if err != nil {
		return err
	}
	if start < 1 || n < 0 {
		return f.ErrValue
	}
	u := units(s)
	i := min(start-1, len(u))
	j := min(i+n, len(u))
	return result(fromUnits(u[:i]) + repl + fromUnits(u[j:]))
}

// --- searching ----------------------------------------------------------------

// find implements FIND(find_text, within_text, [start_num]) and, with
// search set, SEARCH: case-insensitive with wildcards (? any character,
// * any run, ~ escapes the next character).
func find(a []f.Value, search bool) f.Value {
	needle, err := f.Str(a[0])
	if err != nil {
		return err
	}
	hay, err := f.Str(a[1])
	if err != nil {
		return err
	}
	start, err := intArg(arg(a, 2), 1)
	if err != nil {
		return err
	}
	p, t := units(needle), units(hay)
	if needle == "" {
		// An empty find_text matches at start_num.
		if start < 1 || start > len(t)+1 {
			return f.ErrValue
		}
		return f.Number(start)
	}
	if start < 1 || start > len(t) {
		return f.ErrValue
	}
	if search {
		pos := wildcardIndex(foldUnits(p), foldUnits(t), start-1)
		if pos < 0 {
			return f.ErrValue
		}
		return f.Number(pos + 1)
	}
	for i := start - 1; i+len(p) <= len(t); i++ {
		if equalUnits(t[i:i+len(p)], p) {
			return f.Number(i + 1)
		}
	}
	return f.ErrValue
}

func equalUnits(a, b []uint16) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// foldUnits lower-cases UTF-16 text unit by unit (surrogates unchanged).
func foldUnits(u []uint16) []uint16 {
	out := make([]uint16, len(u))
	for i, c := range u {
		out[i] = c
		if !utf16.IsSurrogate(rune(c)) {
			if l := unicode.ToLower(rune(c)); l < 0x10000 {
				out[i] = uint16(l)
			}
		}
	}
	return out
}

// wild is one element of a SEARCH pattern.
type wild struct {
	kind byte // 'c' literal, '?' any unit, '*' any run
	c    uint16
}

func parseWild(p []uint16) []wild {
	var out []wild
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case c == '~' && i+1 < len(p):
			i++
			out = append(out, wild{'c', p[i]})
		case c == '?' || c == '*':
			out = append(out, wild{byte(c), 0})
		default:
			out = append(out, wild{'c', c})
		}
	}
	return out
}

// wildcardIndex returns the first position >= from where pattern matches
// a prefix of t, or -1.
func wildcardIndex(pattern, t []uint16, from int) int {
	p := append(parseWild(pattern), wild{kind: '*'}) // match a prefix
	for i := from; i < len(t); i++ {
		if wildMatch(p, t[i:]) {
			return i
		}
	}
	return -1
}

// wildMatch reports whether p matches all of t (greedy with backtracking
// to the last '*').
func wildMatch(p []wild, t []uint16) bool {
	i, j, star, mark := 0, 0, -1, 0
	for j < len(t) {
		switch {
		case i < len(p) && (p[i].kind == '?' || p[i].kind == 'c' && p[i].c == t[j]):
			i++
			j++
		case i < len(p) && p[i].kind == '*':
			star, mark = i, j
			i++
		case star >= 0:
			mark++
			i, j = star+1, mark
		default:
			return false
		}
	}
	for i < len(p) && p[i].kind == '*' {
		i++
	}
	return i == len(p)
}

// SUBSTITUTE(text, old_text, new_text, [instance_num])
func substitute(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	old, err := f.Str(a[1])
	if err != nil {
		return err
	}
	repl, err := f.Str(a[2])
	if err != nil {
		return err
	}
	inst := 0
	if _, ok := arg(a, 3).(f.Missing); !ok {
		if inst, err = intArg(a[3], 0); err != nil {
			return err
		}
		if inst < 1 {
			return f.ErrValue
		}
	}
	if old == "" {
		return f.String(s)
	}
	if inst == 0 {
		return result(strings.ReplaceAll(s, old, repl))
	}
	pos := 0
	for k := 1; ; k++ {
		i := strings.Index(s[pos:], old)
		if i < 0 {
			return f.String(s)
		}
		if k == inst {
			return result(s[:pos+i] + repl + s[pos+i+len(old):])
		}
		pos += i + len(old)
	}
}

// --- transformations ------------------------------------------------------------

// REPT(text, number_times)
func rept(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	n, err := intArg(a[1], 0)
	if err != nil {
		return err
	}
	if n < 0 {
		return f.ErrValue
	}
	if s == "" || n == 0 {
		return f.String("")
	}
	if ulen(s)*n > maxText {
		return f.ErrValue
	}
	return f.String(strings.Repeat(s, n))
}

// proper capitalizes every letter that follows a non-letter and lower-cases
// the rest, so "it's" becomes "It'S" and "2-way" "2-Way".
func proper(s string) string {
	var b strings.Builder
	prevLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			if prevLetter {
				r = unicode.ToLower(r)
			} else {
				r = unicode.ToUpper(r)
			}
			prevLetter = true
		} else {
			prevLetter = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trim removes leading and trailing spaces and collapses inner runs to a
// single space. Only the ASCII space (32) counts, not the non-breaking
// space (160).
func trim(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' }), " ")
}

// clean removes the 7-bit control characters 0-31.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, s)
}

// EXACT(text1, text2) compares case-sensitively.
func exact(_ *f.Env, a []f.Value) f.Value {
	x, err := f.Str(a[0])
	if err != nil {
		return err
	}
	y, err := f.Str(a[1])
	if err != nil {
		return err
	}
	return f.Bool(x == y)
}

// T(value) returns text unchanged and "" for anything else.
func tFn(_ *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case f.String, f.Error:
		return v
	}
	return f.String("")
}

// --- joining ---------------------------------------------------------------------

// CONCATENATE(text1, ...) takes scalars (ranges lift).
func concatenate(_ *f.Env, a []f.Value) f.Value {
	var b strings.Builder
	for _, v := range a {
		s, err := f.Str(v)
		if err != nil {
			return err
		}
		b.WriteString(s)
	}
	return result(b.String())
}

// flatten appends the text of every element of vals (row by row) to out.
func flatten(e *f.Env, vals []f.Value, out []string) ([]string, f.Value) {
	var bad f.Value
	for _, v := range vals {
		e.Each(v, func(x f.Value, _ bool) bool {
			s, err := f.Str(x)
			if err != nil {
				bad = err
				return false
			}
			out = append(out, s)
			return true
		})
		if bad != nil {
			return nil, bad
		}
	}
	return out, nil
}

// CONCAT(text1, ...) joins ranges and arrays row by row.
func concat(e *f.Env, a []f.Value) f.Value {
	parts, err := flatten(e, a, nil)
	if err != nil {
		return err
	}
	return result(strings.Join(parts, ""))
}

// TEXTJOIN(delimiter, ignore_empty, text1, ...). An array of delimiters is
// used in turn.
func textJoin(e *f.Env, a []f.Value) f.Value {
	delims, err := flatten(e, a[:1], nil)
	if err != nil {
		return err
	}
	if len(delims) == 0 {
		delims = []string{""}
	}
	ignore, err := f.Truth(a[1])
	if err != nil {
		return err
	}
	parts, err := flatten(e, a[2:], nil)
	if err != nil {
		return err
	}
	var b strings.Builder
	n := 0
	for _, p := range parts {
		if ignore && p == "" {
			continue
		}
		if n > 0 {
			b.WriteString(delims[(n-1)%len(delims)])
		}
		b.WriteString(p)
		n++
		if b.Len() > 4*maxText {
			return f.ErrValue
		}
	}
	return result(b.String())
}

// --- characters --------------------------------------------------------------------

// CHAR(number): a character of the Windows-1252 code page (Excel on
// Windows). Its five unassigned bytes map to the C1 control characters of
// the same number, as Windows does.
func char(_ *f.Env, a []f.Value) f.Value {
	n, err := intArg(a[0], 0)
	if err != nil {
		return err
	}
	if n < 1 || n > 255 {
		return f.ErrValue
	}
	return f.String(string(decode1252(byte(n))))
}

// CODE(text): the Windows-1252 code of the first character; characters
// outside the code page give 63 ("?").
func code(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	if s == "" {
		return f.ErrValue
	}
	r := []rune(s)[0]
	if b, ok := encode1252(r); ok {
		return f.Number(b)
	}
	return f.Number('?')
}

// UNICHAR(number)
func unichar(_ *f.Env, a []f.Value) f.Value {
	n, err := intArg(a[0], 0)
	if err != nil {
		return err
	}
	switch {
	case n < 1 || n > unicode.MaxRune:
		return f.ErrValue
	case utf16.IsSurrogate(rune(n)):
		return f.ErrNA
	}
	return f.String(string(rune(n)))
}

// UNICODE(text): the code point of the first character.
func unicodeFn(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	if s == "" {
		return f.ErrValue
	}
	return f.Number([]rune(s)[0])
}

// --- other ------------------------------------------------------------------------------

// PHONETIC(reference) returns the furigana of the top-left cell. Workbooks
// here carry no phonetic data, so that is the cell's text.
func phonetic(e *f.Env, a []f.Value) f.Value {
	v := a[0]
	if r, ok := v.(f.Ref); ok {
		v = e.Ctx.CellValue(r.Sheet, r.R1, r.C1)
	} else if arr, ok := v.(*f.Array); ok && arr.Len() > 0 {
		v = arr.V[0]
	}
	switch v := v.(type) {
	case f.String, f.Error:
		return v
	}
	return f.String("")
}

// online implements DETECTLANGUAGE and TRANSLATE, which need Microsoft's
// translation service: without it Excel shows #CONNECT!.
func online(_ *f.Env, a []f.Value) f.Value {
	for _, v := range a {
		if err, ok := v.(f.Error); ok {
			return err
		}
	}
	return f.ErrConnect
}
