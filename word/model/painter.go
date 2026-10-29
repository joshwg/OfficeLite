package model

// FormatCopy is formatting picked up by the Format Painter: the character
// formatting of the first selected character (or the caret), and the
// paragraph formatting when a whole paragraph was picked, as Word does.
type FormatCopy struct {
	Style string // character style
	Props RunProps
	Para  *ParaFormat
}

// ParaFormat is a paragraph's style and direct formatting.
type ParaFormat struct {
	Style string
	Props ParaProps
}

// CopyFormat picks up the formatting of the selection (Ctrl+Shift+C).
func (e *Editor) CopyFormat() *FormatCopy {
	a, b := e.Sel.Ordered()
	var r Run
	if e.Sel.Empty() {
		r = e.caretRun(a)
		if e.Pending == nil && a.Off == 0 {
			// At the start of a paragraph Word copies the first character's.
			if p := e.locs[a.Para].P; len(p.Runs) > 0 {
				r = Run{Style: p.Runs[0].Style, Props: p.Runs[0].Props}
			}
		}
	} else {
		r = e.caretRun(e.selStartPlusOne())
	}
	f := &FormatCopy{Style: r.Style, Props: r.Props}
	pa := e.locs[a.Para].P
	whole := !e.Sel.Empty() && a.Off == 0 && (b.Para > a.Para || b.Off >= pa.Len())
	if e.Sel.Empty() || whole {
		// Word copies the paragraph's formatting too when nothing is
		// selected or a whole paragraph is.
		f.Para = &ParaFormat{Style: pa.Style, Props: copyParaProps(pa.Props)}
	}
	return f
}

// PasteFormat applies picked-up formatting to the selection (the word
// at the caret with nothing selected) as one undoable step. Paragraph
// formatting is applied to the selected paragraphs when it was copied
// from a whole paragraph (or a caret) and the target covers whole
// paragraphs.
func (e *Editor) PasteFormat(f *FormatCopy) {
	defer e.Op("Format Painter")()
	if f == nil {
		return
	}
	apply := func(r *Run) {
		r.Style, r.Props = f.Style, f.Props
	}
	sel := e.Sel
	restore := false
	if e.Sel.Empty() {
		c := e.Sel.Caret
		a, b := e.WordAt(c)
		rs := []rune(e.locs[c.Para].P.Text())
		for b.Off > a.Off && charClass(rs[b.Off-1]) == 0 {
			b.Off--
		}
		if a.Off < b.Off && (charClass(rs[a.Off]) == 1) {
			e.Sel = Selection{a, b}
			restore = true
		} else {
			r := e.caretRun(c)
			apply(&r)
			e.Pending = &r
			return
		}
	}
	s, t := e.Sel.Ordered()
	paraToo := f.Para != nil && !sel.Empty() && s.Off == 0 && (t.Para > s.Para || t.Off >= e.locs[t.Para].P.Len())
	from, to := e.span()
	e.edit("format", from, to, false, func() {
		touched := map[*Paragraph]bool{}
		e.eachSelectedRun(func(p *Paragraph, r *Run) {
			apply(r)
			touched[p] = true
		})
		for p := range touched {
			normalize(p)
		}
		for _, i := range e.SelectedParas() {
			p := e.locs[i].P
			if p.Len() == 0 || paraToo {
				r := Run{Props: p.Mark}
				apply(&r)
				p.Mark = r.Props
			}
			if paraToo {
				p.Style, p.Props = f.Para.Style, copyParaProps(f.Para.Props)
			}
		}
	})
	if restore {
		e.Sel = sel
	}
}
