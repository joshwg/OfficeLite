package model

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// StyleSpec describes a new style (Home > New Style…).
type StyleSpec struct {
	Name    string
	Type    StyleType // ParagraphStyle or CharacterStyle
	BasedOn string    // style ID ("" = none for character styles, Normal for paragraph styles)
	Next    string    // paragraph styles: style for the following paragraph ("" = itself)
}

// FindStyleByName returns the style whose display name is name (ignoring
// case), or nil.
func (s *StyleSheet) FindStyleByName(name string) *Style {
	for _, st := range s.Styles {
		if strings.EqualFold(st.Name, name) || st.Name == "" && strings.EqualFold(st.ID, name) {
			return st
		}
	}
	return nil
}

// styleIDFor makes a unique style ID from a name, as Word does (letters
// and digits only).
func styleIDFor(ss *StyleSheet, name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	base := b.String()
	if base == "" {
		base = "Style"
	}
	id := base
	for n := 1; ss.Find(id) != nil; n++ {
		id = base + strconv.Itoa(n)
	}
	return id
}

// Errors from style management.
var (
	ErrStyleName    = errors.New("type a name for the style")
	ErrStyleExists  = errors.New("a style with this name already exists")
	ErrStyleBuiltIn = errors.New("built-in styles can't be deleted")
)

// directRun is the direct character formatting of the first selected
// character (or the caret), without format-specific extras.
func (e *Editor) directRun() Run {
	var r Run
	if e.Sel.Empty() {
		r = e.caretRun(e.Sel.Caret)
	} else {
		r = e.caretRun(e.selStartPlusOne())
	}
	r.Props.DocxXML = ""
	return r
}

// NewStyle creates a style from the selection's formatting and applies it
// to the selection, as one undoable step; it returns the new style's ID.
// A paragraph style gets the paragraph's direct formatting and the first
// character's direct character formatting on top of BasedOn; a character
// style gets the character formatting.
func (e *Editor) NewStyle(spec StyleSpec) (string, error) {
	defer e.Op("New Style")()
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return "", ErrStyleName
	}
	if e.Doc.Styles.FindStyleByName(name) != nil {
		return "", ErrStyleExists
	}
	a, _ := e.Sel.Ordered()
	para := e.locs[a.Para].P
	run := e.directRun()
	st := &Style{Name: name, Type: spec.Type, BasedOn: spec.BasedOn, Custom: true}
	st.ID = styleIDFor(&e.Doc.Styles, name)
	switch spec.Type {
	case CharacterStyle:
		// A character style based on the run's style gets its formatting.
		if st.BasedOn == "" && run.Style != "" {
			st.BasedOn = run.Style
		}
		st.Run = run.Props
	default:
		st.Type = ParagraphStyle
		if st.BasedOn == "" {
			st.BasedOn = styleOrDefault(e.Doc, para.Style)
		}
		st.Para = copyParaProps(para.Props)
		st.Para.List, st.Para.DocxXML = nil, ""
		st.Run = run.Props
		st.Next = spec.Next
		if st.Next == "" {
			st.Next = st.ID
		}
	}
	if st.BasedOn == st.ID {
		st.BasedOn = ""
	}
	from, to := e.span()
	e.edit("style", from, to, true, func() {
		e.Doc.Styles.Styles = append(e.Doc.Styles.Styles, st)
		if st.Type == CharacterStyle {
			e.eachSelectedRun(func(p *Paragraph, r *Run) {
				r.Style = st.ID
				r.Props = clearSame(r.Props, st.Run)
			})
			e.normalizeSelected()
			return
		}
		for _, i := range e.SelectedParas() {
			p := e.locs[i].P
			list := p.Props.List
			p.Style, p.Props = st.ID, ParaProps{List: list}
			for k := range p.Runs {
				p.Runs[k].Props = clearSame(p.Runs[k].Props, st.Run)
			}
			p.Mark = clearSame(p.Mark, st.Run)
			normalize(p)
		}
	})
	return st.ID, nil
}

func (e *Editor) normalizeSelected() {
	for _, i := range e.SelectedParas() {
		normalize(e.locs[i].P)
	}
}

// clearSame removes from p the properties that style formatting s sets to
// the same values, so the style shows through.
func clearSame(p, s RunProps) RunProps {
	if s.Font != "" && p.Font == s.Font {
		p.Font = ""
	}
	if s.Size != 0 && p.Size == s.Size {
		p.Size = 0
	}
	for _, t := range []struct{ dst, src *Tri }{{&p.Bold, &s.Bold}, {&p.Italic, &s.Italic}, {&p.Strike, &s.Strike},
		{&p.Caps, &s.Caps}, {&p.SmallCaps, &s.SmallCaps}, {&p.Hidden, &s.Hidden}} {
		if *t.src != Unset && *t.dst == *t.src {
			*t.dst = Unset
		}
	}
	if s.Underline != UnderlineUnset && p.Underline == s.Underline {
		p.Underline = UnderlineUnset
	}
	if s.Color != "" && p.Color == s.Color {
		p.Color = ""
	}
	if s.Highlight != "" && p.Highlight == s.Highlight {
		p.Highlight = ""
	}
	if s.Vert != VertUnset && p.Vert == s.Vert {
		p.Vert = VertUnset
	}
	if p == (RunProps{DocxXML: p.DocxXML}) {
		p.DocxXML = ""
	}
	return p
}

// UpdateStyleToMatch redefines a style from the selection's formatting
// (Update <Style> to Match Selection): the selection's direct formatting
// moves into the style, so every paragraph (or run) using it changes.
func (e *Editor) UpdateStyleToMatch(id string) {
	defer e.Op("Update Style")()
	st := e.Doc.Styles.Find(id)
	if st == nil {
		return
	}
	a, _ := e.Sel.Ordered()
	para := e.locs[a.Para].P
	run := e.directRun()
	from, to := e.span()
	e.edit("style", from, to, true, func() {
		st := e.Doc.Styles.Find(id)
		st.Run = st.Run.Merge(run.Props)
		if st.Type == ParagraphStyle {
			pp := copyParaProps(para.Props)
			pp.List, pp.DocxXML = nil, ""
			st.Para = st.Para.Merge(pp)
			for _, i := range e.SelectedParas() {
				p := e.locs[i].P
				if styleOrDefault(e.Doc, p.Style) != id {
					continue
				}
				p.Props = ParaProps{List: p.Props.List}
				for k := range p.Runs {
					p.Runs[k].Props = clearSame(p.Runs[k].Props, st.Run)
				}
				p.Mark = clearSame(p.Mark, st.Run)
				normalize(p)
			}
			return
		}
		e.eachSelectedRun(func(p *Paragraph, r *Run) {
			if r.Style == id {
				r.Props = clearSame(r.Props, st.Run)
			}
		})
		e.normalizeSelected()
	})
}

// DeleteStyle deletes a custom style (one undoable step). Paragraphs and
// runs using it fall back to the style it was based on, as do styles based
// on it.
func (e *Editor) DeleteStyle(id string) error {
	defer e.Op("Delete Style")()
	st := e.Doc.Styles.Find(id)
	if st == nil {
		return nil
	}
	if !st.Custom || st.Default {
		return ErrStyleBuiltIn
	}
	base := st.BasedOn
	if st.Type == ParagraphStyle && (base == "" || base == normalID(e.Doc)) {
		base = ""
	}
	hf := e.Doc.HF.Clone() // headers and footers using the style change too
	e.edit("style", 0, len(e.Doc.Body)-1, true, func() {
		ss := &e.Doc.Styles
		out := ss.Styles[:0:0]
		for _, s := range ss.Styles {
			if s.ID == id {
				continue
			}
			if s.BasedOn == id {
				s.BasedOn = st.BasedOn
			}
			if s.Next == id {
				s.Next = ""
			}
			out = append(out, s)
		}
		ss.Styles = out
		fix := func(blocks []Block) {
			WalkBlocks(blocks, func(p *Paragraph) bool {
				if st.Type == ParagraphStyle && p.Style == id {
					p.Style = base
				}
				for k := range p.Runs {
					if p.Runs[k].Style == id {
						p.Runs[k].Style = base
					}
				}
				return true
			})
		}
		fix(e.Doc.Body)
		e.Doc.HF.All(func(hf *HeaderFooter) { fix(hf.Blocks) })
	})
	e.undo[len(e.undo)-1].hf = &hf
	return nil
}

// StyleInUse reports whether any paragraph or run uses the style.
func (d *Document) StyleInUse(id string) bool {
	used := false
	check := func(blocks []Block) {
		WalkBlocks(blocks, func(p *Paragraph) bool {
			if p.Style == id {
				used = true
			}
			for k := range p.Runs {
				if p.Runs[k].Style == id {
					used = true
				}
			}
			return !used
		})
	}
	check(d.Body)
	d.HF.All(func(hf *HeaderFooter) { check(hf.Blocks) })
	return used
}
