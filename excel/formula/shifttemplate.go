// shifttemplate.go scans a formula's references once for Shift at many
// offsets: a shared formula (.xlsx <f t="shared">) expands to every cell
// of its range, and rescanning the master's text per cell is what made
// large shared ranges slow to open.

package formula

import "strings"

// ShiftTemplate is a formula's text split once into the runs between its
// references and the references themselves; Shift then rebuilds the text
// at any offset without scanning it again.
type ShiftTemplate struct {
	text string   // the formula as given
	lits []string // the runs around refs: len(refs)+1 of them
	refs []refTok // the references, in order
}

// NewShiftTemplate scans f's references once.
func NewShiftTemplate(f string) *ShiftTemplate {
	t := &ShiftTemplate{text: f}
	last := 0
	rewriteRefsAt(f, func(tok *refTok, at int) string {
		t.lits = append(t.lits, f[last:at])
		t.refs = append(t.refs, *tok)
		last = at + len(tok.text)
		return tok.text
	})
	t.lits = append(t.lits, f[last:])
	return t
}

// Shift returns what Shift(f, dRow, dCol) returns for the template's f.
func (t *ShiftTemplate) Shift(dRow, dCol int) string {
	if dRow == 0 && dCol == 0 {
		return t.text
	}
	var b strings.Builder
	b.Grow(len(t.text) + 8)
	for i := range t.refs {
		b.WriteString(t.lits[i])
		b.WriteString(shiftTok(&t.refs[i], dRow, dCol))
	}
	b.WriteString(t.lits[len(t.refs)])
	return b.String()
}
