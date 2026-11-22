// Text input from input methods: phone keyboards (Gboard, Samsung
// Keyboard) and desktop IMEs. They don't send one key per character: they
// rewrite the word being typed ("h", then "he" in place of "h", then
// "hel" in place of "he"), autocorrect it, and capitalize it from the text
// before the caret. So the page view tells the platform where the caret
// is (rune offsets within its paragraph) and applies each edit to the
// range it names rather than inserting it at the caret: inserting each
// version gave "hhehel".
//
// The platform is given only the selected text, not the text around the
// caret: Gio's browser backend reads its whole hidden <textarea> as the
// composed word, and a keyboard that sees the word before the caret
// starts rewriting it when you backspace into it, at ranges Gio gets
// wrong. Without that context the keyboard can't capitalize sentences, so
// the browser build turns its auto-capitals off (common/gui).

package ui

import (
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"

	"officelite/word/model"
)

// imeReport is what the page view last told the platform: the caret's
// paragraph, the selection within it and the selected text.
type imeReport struct {
	valid bool
	para  int
	text  string
	sel   key.Range
}

// imeSel returns the caret's paragraph and the selection within it, in
// that paragraph's rune offsets. A selection reaching into another
// paragraph is reported as the caret alone.
func imeSel(ed *model.Editor) (int, key.Range) {
	s := ed.Sel
	p := s.Caret.Para
	if s.Anchor.Para != p {
		return p, key.Range{Start: s.Caret.Off, End: s.Caret.Off}
	}
	a, b := s.Ordered()
	return p, key.Range{Start: a.Off, End: b.Off}
}

// ordered returns r with Start <= End.
func ordered(r key.Range) key.Range {
	if r.Start > r.End {
		r.Start, r.End = r.End, r.Start
	}
	return r
}

// imeEdit applies a text edit from the platform: replace the range r
// (rune offsets in the caret's paragraph) with text.
func (d *window) imeEdit(r key.Range, text string) {
	ed := d.ed
	r = ordered(r)
	para, sel := imeSel(ed)
	if r == sel || r.Start == r.End {
		// At the caret, over the whole selection. A plain insertion goes
		// there even when the platform names another place: it counts
		// from what the view last reported, and a click or command may
		// have moved the caret since.
		ed.InsertText(text)
		return
	}
	n := ed.Para(para).Len()
	a, b := min(max(r.Start, 0), n), min(max(r.End, 0), n)
	if ed.Sel.Empty() && b == sel.Start {
		// The usual case: the word being typed grows at the caret
		// ("he" for "h"). Typing only the new part keeps it one undo
		// step and the run's formatting.
		old := []rune(ed.Para(para).Text())[a:b]
		if rest, ok := strings.CutPrefix(text, string(old)); ok {
			ed.InsertText(rest)
			return
		}
	}
	ed.Select(model.Pos{Para: para, Off: a}, model.Pos{Para: para, Off: b})
	if text == "" {
		if a != b {
			ed.Backspace()
		}
		return
	}
	ed.InsertText(text)
}

// reportIME tells the platform the caret's position and the selected
// text when they changed. Call it after the frame's editing, while the
// view has the focus. Reporting after each edit also empties Gio's
// browser <textarea> of what was typed (a space left there became part of
// the next word).
func (d *window) reportIME(gtx layout.Context) {
	if !d.v.focused || d.opening() {
		d.v.ime.valid = false
		return
	}
	para, sel := imeSel(d.ed)
	text := string([]rune(d.ed.Para(para).Text())[sel.Start:sel.End])
	rep := imeReport{valid: true, para: para, text: text, sel: sel}
	if rep == d.v.ime {
		return
	}
	d.v.ime = rep
	tag := &d.v
	gtx.Execute(key.SnippetCmd{Tag: tag, Snippet: key.Snippet{Range: sel, Text: text}})
	gtx.Execute(key.SelectionCmd{Tag: tag, Range: sel})
}
