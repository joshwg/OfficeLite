// TEXTBEFORE, TEXTAFTER and TEXTSPLIT, built on a multi-delimiter matcher.

package text

import (
	"unicode"

	f "officelite/excel/formula"
)

// matcher finds delimiter occurrences in a text, optionally ignoring case.
// Positions are rune indexes into the original text.
type matcher struct {
	text   []rune // original
	folded []rune // compared (lower-cased when ignoring case)
	delims [][]rune
}

// newMatcher returns a matcher for delims in s, case-folded when
// ignoreCase.
func newMatcher(s string, delims []string, ignoreCase bool) *matcher {
	m := &matcher{text: []rune(s)}
	m.folded = fold(m.text, ignoreCase)
	for _, d := range delims {
		m.delims = append(m.delims, fold([]rune(d), ignoreCase))
	}
	return m
}

// fold returns rs lower-cased when ignoreCase, else rs itself.
func fold(rs []rune, ignoreCase bool) []rune {
	if !ignoreCase {
		return rs
	}
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// at returns the length of the longest delimiter starting (or, with
// ending, ending) at position i, or -1. An empty delimiter matches with
// length 0.
func (m *matcher) at(i int, ending bool) int {
	best := -1
	for _, d := range m.delims {
		if len(d) <= best {
			continue
		}
		start := i
		if ending {
			start = i - len(d)
		}
		if start < 0 || start+len(d) > len(m.folded) {
			continue
		}
		if string(m.folded[start:start+len(d)]) == string(d) {
			best = len(d)
		}
	}
	return best
}

// span is a delimiter occurrence [start, end).
type span struct{ start, end int }

// forward lists up to limit non-overlapping occurrences from the start
// (all of them when limit < 0).
func (m *matcher) forward(limit int) []span {
	var out []span
	for i := 0; i <= len(m.folded) && (limit < 0 || len(out) < limit); {
		n := m.at(i, false)
		switch {
		case n < 0:
			i++
		case n == 0: // an empty delimiter matches before every character
			out = append(out, span{i, i})
			i++
		default:
			out = append(out, span{i, i + n})
			i += n
		}
	}
	return out
}

// backward lists up to limit occurrences scanning from the end.
func (m *matcher) backward(limit int) []span {
	var out []span
	for j := len(m.folded); j >= 0 && len(out) < limit; {
		n := m.at(j, true)
		switch {
		case n < 0:
			j--
		case n == 0:
			out = append(out, span{j, j})
			j--
		default:
			out = append(out, span{j - n, j})
			j -= n
		}
	}
	return out
}

// delimList reads a delimiter argument: text or an array of texts.
func delimList(v f.Value) ([]string, f.Value) {
	var out []string
	vals := []f.Value{v}
	if a, ok := v.(*f.Array); ok {
		vals = a.V
	}
	for _, x := range vals {
		s, err := f.Str(x)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// matchModeArg reads match_mode: 0 case-sensitive, 1 case-insensitive.
func matchModeArg(v f.Value) (bool, f.Value) {
	n, err := intArg(v, 0)
	if err != nil {
		return false, err
	}
	if n != 0 && n != 1 {
		return false, f.ErrValue
	}
	return n == 1, nil
}

// textBeforeAfter implements TEXTBEFORE and TEXTAFTER(text, delimiter,
// [instance_num], [match_mode], [match_end], [if_not_found]). A negative
// instance_num counts from the end; match_end treats the end of the text
// (the start, when searching backwards) as a delimiter.
func textBeforeAfter(a []f.Value, after bool) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	delims, err := delimList(a[1])
	if err != nil {
		return err
	}
	inst, err := intArg(arg(a, 2), 1)
	if err != nil {
		return err
	}
	ignoreCase, err := matchModeArg(arg(a, 3))
	if err != nil {
		return err
	}
	matchEnd, err := f.Truth(arg(a, 4))
	if err != nil {
		return err
	}
	notFound := arg(a, 5)
	if _, ok := notFound.(f.Missing); ok {
		notFound = f.ErrNA
	}
	if inst == 0 || (s != "" && max(inst, -inst) > ulen(s)) {
		return f.ErrValue
	}
	m := newMatcher(s, delims, ignoreCase)
	var spans []span
	k := inst
	if inst > 0 {
		spans = m.forward(k)
		if matchEnd && len(spans) < k {
			spans = append(spans, span{len(m.text), len(m.text)})
		}
	} else {
		k = -inst
		spans = m.backward(k)
		if matchEnd && len(spans) < k {
			spans = append(spans, span{0, 0})
		}
	}
	if len(spans) < k {
		return notFound
	}
	sp := spans[k-1]
	if after {
		return f.String(string(m.text[sp.end:]))
	}
	return f.String(string(m.text[:sp.start]))
}

// split cuts s at every occurrence of the delimiters.
func split(s string, delims []string, ignoreCase bool) []string {
	if len(delims) == 0 {
		return []string{s}
	}
	m := newMatcher(s, delims, ignoreCase)
	var out []string
	prev := 0
	for _, sp := range m.forward(-1) {
		out = append(out, string(m.text[prev:sp.start]))
		prev = sp.end
	}
	return append(out, string(m.text[prev:]))
}

// textSplit implements TEXTSPLIT(text, col_delimiter, [row_delimiter],
// [ignore_empty], [match_mode], [pad_with]). Rows are split first, then columns; short
// rows are padded with pad_with (#N/A by default).
func textSplit(e *f.Env, a []f.Value) f.Value {
	optDelims := func(v f.Value) ([]string, f.Value) {
		if _, ok := v.(f.Missing); ok {
			return nil, nil
		}
		ds, err := delimList(v)
		if err != nil {
			return nil, err
		}
		for _, d := range ds {
			if d == "" {
				return nil, f.ErrValue
			}
		}
		return ds, nil
	}
	colDelims, err := optDelims(a[1])
	if err != nil {
		return err
	}
	rowDelims, err := optDelims(arg(a, 2))
	if err != nil {
		return err
	}
	if colDelims == nil && rowDelims == nil {
		return f.ErrValue
	}
	ignoreEmpty, err := f.Truth(arg(a, 3))
	if err != nil {
		return err
	}
	ignoreCase, err := matchModeArg(arg(a, 4))
	if err != nil {
		return err
	}
	pad := arg(a, 5)
	if _, ok := pad.(f.Missing); ok {
		pad = f.ErrNA
	}
	one := func(v f.Value) f.Value {
		s, err := f.Str(v)
		if err != nil {
			return err
		}
		var rows [][]string
		width := 0
		for _, line := range split(s, rowDelims, ignoreCase) {
			if ignoreEmpty && line == "" {
				continue
			}
			var cells []string
			for _, c := range split(line, colDelims, ignoreCase) {
				if !(ignoreEmpty && c == "") {
					cells = append(cells, c)
				}
			}
			if len(cells) == 0 {
				continue
			}
			rows = append(rows, cells)
			width = max(width, len(cells))
		}
		if len(rows) == 0 {
			return f.ErrCalc
		}
		out := f.NewArray(len(rows), width)
		for r, cells := range rows {
			for c := 0; c < width; c++ {
				if c < len(cells) {
					out.Set(r, c, f.String(cells[c]))
				} else {
					out.Set(r, c, pad)
				}
			}
		}
		return out
	}
	// With several texts Excel keeps only the first value of each split.
	if arr, ok := a[0].(*f.Array); ok && !arr.Single() {
		out := &f.Array{Rows: arr.Rows, Cols: arr.Cols, V: make([]f.Value, len(arr.V))}
		for i, v := range arr.V {
			res := one(v)
			if ra, ok := res.(*f.Array); ok {
				res = ra.V[0]
			}
			out.V[i] = res
		}
		return out
	}
	return one(e.Scalar(a[0]))
}
