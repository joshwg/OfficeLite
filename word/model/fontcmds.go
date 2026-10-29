package model

import (
	"unicode"
)

// Font commands of the Home ribbon's Font group beyond those in edit.go.

// growSizes are the steps of Grow Font / Shrink Font (Ctrl+Shift+> / <).
var growSizes = []float64{1, 8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72}

// stepSize returns the next size up (dir 1) or down (-1) from s.
func stepSize(s float64, dir int) float64 {
	if dir > 0 {
		for _, v := range growSizes {
			if v > s+0.01 {
				return v
			}
		}
		return min(s+10, 1638)
	}
	for i := len(growSizes) - 1; i >= 0; i-- {
		if growSizes[i] < s-0.01 {
			return growSizes[i]
		}
	}
	return max(s-1, 1)
}

// GrowFont makes the selected text one size step larger (dir 1) or
// smaller (-1); text of mixed sizes steps from each run's own size.
func (e *Editor) GrowFont(dir int) {
	defer e.Op(either(dir > 0, "Grow Font", "Shrink Font"))()
	if e.Sel.Empty() {
		size := e.CaretFormat().Size
		e.FormatRuns(func(r *Run) { r.Props.Size = stepSize(size, dir) })
		return
	}
	from, to := e.span()
	e.edit("format", from, to, false, func() {
		e.eachSelectedRun(func(p *Paragraph, r *Run) {
			r.Props.Size = stepSize(e.Doc.ResolveRun(p, r).Size, dir)
		})
		e.normalizeSelected()
	})
}

// ToggleVert switches superscript (VertSuper) or subscript (VertSub) on
// for the selection, or off when it all has it already.
func (e *Editor) ToggleVert(v VertAlign) {
	defer e.Op(either(v == VertSub, "Subscript", "Superscript"))()
	on := !e.AllSelected(func(p RunProps) bool { return p.Vert == v })
	e.FormatRuns(func(r *Run) {
		r.Props.Vert = VertBaseline
		if on {
			r.Props.Vert = v
		}
	})
}

// SetHighlight sets the text highlight color: a Word highlight name
// ("yellow") or "none".
func (e *Editor) SetHighlight(name string) {
	defer e.Op("Text Highlight Color")()
	e.FormatRuns(func(r *Run) { r.Props.Highlight = name })
}

// TextCase is a Change Case choice.
type TextCase int8

const (
	SentenceCase TextCase = iota
	LowerCase
	UpperCase
	CapitalizeWords
	ToggleCase
)

// ChangeCase changes the case of the selected text (the word at the caret
// with nothing selected), as Word's Change Case button does.
func (e *Editor) ChangeCase(c TextCase) {
	defer e.Op("Change Case")()
	sel := e.Sel
	if e.Sel.Empty() {
		a, b := e.WordAt(e.Sel.Caret)
		if a == b {
			return
		}
		e.Sel = Selection{a, b}
	}
	from, to := e.span()
	e.edit("format", from, to, false, func() {
		s, _ := e.Sel.Ordered()
		// The state carries across runs: a sentence or word may span them.
		sentence := s.Off == 0 || sentenceStart([]rune(e.locs[s.Para].P.Text()), s.Off)
		word := true
		if s.Off > 0 {
			rs := []rune(e.locs[s.Para].P.Text())
			word = !unicode.IsLetter(rs[s.Off-1]) && !unicode.IsDigit(rs[s.Off-1])
		}
		var last *Paragraph
		e.eachSelectedRun(func(p *Paragraph, r *Run) {
			if p != last && last != nil {
				sentence, word = true, true
			}
			last = p
			if r.Kind != RunText {
				word = true
				return
			}
			rs := []rune(r.Text)
			for i, ch := range rs {
				letter := unicode.IsLetter(ch)
				switch c {
				case LowerCase:
					rs[i] = unicode.ToLower(ch)
				case UpperCase:
					rs[i] = unicode.ToUpper(ch)
				case ToggleCase:
					if unicode.IsUpper(ch) {
						rs[i] = unicode.ToLower(ch)
					} else {
						rs[i] = unicode.ToUpper(ch)
					}
				case CapitalizeWords:
					if letter && word {
						rs[i] = unicode.ToUpper(ch)
					} else if letter {
						rs[i] = unicode.ToLower(ch)
					}
				case SentenceCase:
					if letter && sentence {
						rs[i] = unicode.ToUpper(ch)
						sentence = false
					} else if letter {
						rs[i] = unicode.ToLower(ch)
					}
					if ch == '.' || ch == '!' || ch == '?' {
						sentence = true
					}
				}
				word = !letter && !unicode.IsDigit(ch) && ch != '\'' && ch != '’'
			}
			r.Text = string(rs)
		})
		e.normalizeSelected()
	})
	if sel.Empty() {
		e.Sel = sel
	}
}

// sentenceStart reports whether off follows the end of a sentence (only
// spaces since a '.', '!' or '?').
func sentenceStart(rs []rune, off int) bool {
	for i := off - 1; i >= 0; i-- {
		switch {
		case rs[i] == '.' || rs[i] == '!' || rs[i] == '?':
			return true
		case !unicode.IsSpace(rs[i]):
			return false
		}
	}
	return true
}
