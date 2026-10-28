package model

import (
	"unicode"
)

// FindOptions control Find and Replace.
type FindOptions struct {
	MatchCase  bool
	WholeWords bool
}

// Match is a found occurrence: [A, B) within one paragraph.
type Match struct{ A, B Pos }

// foldRune is the case-insensitive comparison key of r.
func foldRune(r rune) rune { return unicode.ToLower(unicode.ToUpper(r)) }

// findIn returns the rune offsets of the occurrences of q in text.
func findIn(text, q []rune, o FindOptions) []int {
	var out []int
	if len(q) == 0 || len(q) > len(text) {
		return nil
	}
	eq := func(a, b rune) bool {
		if o.MatchCase {
			return a == b
		}
		return a == b || foldRune(a) == foldRune(b)
	}
	for i := 0; i+len(q) <= len(text); i++ {
		ok := true
		for k := range q {
			if !eq(text[i+k], q[k]) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if o.WholeWords {
			if i > 0 && charClass(text[i-1]) == 1 && charClass(q[0]) == 1 {
				continue
			}
			if j := i + len(q); j < len(text) && charClass(text[j]) == 1 && charClass(q[len(q)-1]) == 1 {
				continue
			}
		}
		out = append(out, i)
		i += len(q) - 1 // occurrences don't overlap
	}
	return out
}

// FindAll returns every occurrence of q in document order (table cells
// included). Matches don't cross paragraph marks; text in a match may
// span runs with different formatting.
func (e *Editor) FindAll(q string, o FindOptions) []Match {
	qr := []rune(q)
	var out []Match
	for i, l := range e.locs {
		for _, off := range findIn([]rune(l.P.Text()), qr, o) {
			out = append(out, Match{Pos{i, off}, Pos{i, off + len(qr)}})
		}
	}
	return out
}

// MatchIndex returns the index of the first match at or after p (wrapping
// to 0), or -1 when there are none. With back set it returns the last
// match that starts before p instead.
func MatchIndex(ms []Match, p Pos, back bool) int {
	if len(ms) == 0 {
		return -1
	}
	if back {
		for i := len(ms) - 1; i >= 0; i-- {
			if ms[i].A.Less(p) {
				return i
			}
		}
		return len(ms) - 1
	}
	for i, m := range ms {
		if !m.A.Less(p) {
			return i
		}
	}
	return 0
}

// replaceAt replaces [a, b) of paragraph p with text in the formatting of
// the first replaced character.
func replaceAt(p *Paragraph, a, b int, text string) {
	var fmtRun Run
	if i := p.RunAt(a); i >= 0 {
		r := p.Runs[i]
		fmtRun = Run{Style: r.Style, Props: r.Props, Span: r.Span}
	} else {
		fmtRun = Run{Props: p.Mark}
	}
	deleteRunes(p, a, b)
	if text != "" {
		fmtRun.Text = text
		insertRuns(p, a, fmtRun)
	}
}

// Replace replaces one match with text (one undoable step) and selects
// the replacement.
func (e *Editor) Replace(m Match, text string) {
	m.A, m.B = e.clamp(m.A), e.clamp(m.B)
	if m.A.Para != m.B.Para || !m.A.Less(m.B) {
		return
	}
	top := e.locs[m.A.Para].Top
	e.edit("replace", top, top, false, func() {
		replaceAt(e.locs[m.A.Para].P, m.A.Off, m.B.Off, text)
		end := Pos{m.A.Para, m.A.Off + len([]rune(text))}
		e.Sel = Selection{m.A, end}
	})
}

// ReplaceAll replaces every occurrence of q with text as one undoable
// step and returns how many were replaced.
func (e *Editor) ReplaceAll(q, text string, o FindOptions) int {
	ms := e.FindAll(q, o)
	if len(ms) == 0 {
		return 0
	}
	from, to := e.locs[ms[0].A.Para].Top, e.locs[ms[len(ms)-1].A.Para].Top
	e.edit("replace", from, to, false, func() {
		for k := len(ms) - 1; k >= 0; k-- { // last first: offsets stay valid
			m := ms[k]
			replaceAt(e.locs[m.A.Para].P, m.A.Off, m.B.Off, text)
		}
		c := e.Sel.Caret
		e.Sel = Selection{c, c}
	})
	return len(ms)
}
