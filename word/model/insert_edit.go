package model

import (
	"strings"
	"unicode/utf8"
)

// Insert menu commands other than tables: pictures and hyperlinks.

// InsertPicture inserts pic inline at the caret, replacing the selection,
// at its natural size w × h points scaled down to fit the text column
// (and the page height). The picture ends up selected, as in Word.
func (e *Editor) InsertPicture(pic *Picture, w, h float64) {
	defer e.Op("Insert Picture")()
	if pic == nil {
		return
	}
	from, to := e.span()
	e.edit("insert", from, to, false, func() {
		r := e.caretRun(e.Sel.Caret)
		p := e.deleteSelection()
		loc := e.locs[p.Para]
		maxW := e.textWidth(loc)
		pg := e.Doc.Page
		if pg.Height <= 0 {
			pg = LetterPage()
		}
		maxH := max(pg.Height-pg.MarginTop-pg.MarginBottom-24, 72)
		if w <= 0 || h <= 0 {
			w, h = maxW, maxW*0.75
		}
		if s := min(maxW/w, maxH/h, 1); s < 1 {
			w, h = w*s, h*s
		}
		run := PictureRun(pic, w, h)
		run.Style, run.Props = r.Style, r.Props
		insertRuns(loc.P, p.Off, run)
		e.Sel = Selection{p, Pos{p.Para, p.Off + 1}}
	})
}

// PictureAt returns the picture run at caret offset p (the character
// after p), or nil.
func (e *Editor) PictureAt(p Pos) *Run {
	p = e.clamp(p)
	para := e.locs[p.Para].P
	if i := para.RunAt(p.Off); i >= 0 {
		if r := &para.Runs[i]; r.Kind == RunOpaque && r.Opaque != nil && r.Opaque.Picture != nil {
			return r
		}
	}
	return nil
}

// SelectedPicture returns the picture run when the selection is exactly
// one picture.
func (e *Editor) SelectedPicture() *Run {
	a, b := e.Sel.Ordered()
	if a.Para != b.Para || b.Off != a.Off+1 {
		return nil
	}
	return e.PictureAt(a)
}

// ResizePicture sets the size of the picture just after p (undoable).
func (e *Editor) ResizePicture(p Pos, w, h float64) {
	defer e.Op("Picture Size")()
	if e.PictureAt(p) == nil || w <= 0 || h <= 0 {
		return
	}
	top := e.locs[p.Para].Top
	e.edit("format", top, top, false, func() {
		r := e.PictureAt(p)
		o := *r.Opaque
		o.Width, o.Height = w, h
		if o.Format == "docx" {
			// The original XML has the old size; the writer makes new XML
			// from the picture.
			o.Format, o.Data = "", nil
		}
		r.Opaque = &o
	})
}

// linkOf returns the hyperlink span around a run, or nil.
func linkOf(s *Span) *Span {
	for ; s != nil; s = s.Parent {
		if s.Kind == SpanLink {
			return s
		}
	}
	return nil
}

// Link is a hyperlink at a position: its span and the range of text it
// covers in its paragraph.
type Link struct {
	Span       *Span
	Start, End Pos
}

// Text returns the link's text.
func (l Link) Text(e *Editor) string {
	rs := []rune(e.locs[l.Start.Para].P.Text())
	return strings.ReplaceAll(string(rs[l.Start.Off:l.End.Off]), "￼", "")
}

// Address returns what the link points at: its URL, or "#bookmark".
func (l Link) Address() string {
	if l.Span.URL == "" && l.Span.Anchor != "" {
		return "#" + l.Span.Anchor
	}
	return l.Span.URL
}

// LinkAt returns the hyperlink at caret position p: the one the character
// after p is in, or else the one before it.
func (e *Editor) LinkAt(p Pos) (Link, bool) {
	p = e.clamp(p)
	para := e.locs[p.Para].P
	try := func(off int) (Link, bool) {
		i := para.RunAt(off)
		if i < 0 {
			return Link{}, false
		}
		sp := linkOf(para.Runs[i].Span)
		if sp == nil {
			return Link{}, false
		}
		lo, hi := i, i
		for lo > 0 && linkOf(para.Runs[lo-1].Span) == sp {
			lo--
		}
		for hi+1 < len(para.Runs) && linkOf(para.Runs[hi+1].Span) == sp {
			hi++
		}
		start, end, pos := 0, 0, 0
		for k := range para.Runs {
			if k == lo {
				start = pos
			}
			pos += RunLen(&para.Runs[k])
			if k == hi {
				end = pos
			}
		}
		return Link{Span: sp, Start: Pos{p.Para, start}, End: Pos{p.Para, end}}, true
	}
	if l, ok := try(p.Off); ok {
		return l, true
	}
	if p.Off > 0 {
		return try(p.Off - 1)
	}
	return Link{}, false
}

// SelectionLink returns the link the selection (or caret) is in.
func (e *Editor) SelectionLink() (Link, bool) {
	a, b := e.Sel.Ordered()
	l, ok := e.LinkAt(a)
	if !ok || b.Para != a.Para || b.Off > l.End.Off {
		return Link{}, false
	}
	return l, true
}

// SelectedText is the plain text of the selection when it lies in one
// paragraph ("" otherwise), for Insert Link's "Text to display".
func (e *Editor) SelectedText() string {
	a, b := e.Sel.Ordered()
	if a.Para != b.Para || a == b {
		return ""
	}
	rs := []rune(e.locs[a.Para].P.Text())
	return string(rs[a.Off:b.Off])
}

// LinkTarget turns what the user typed as a link address into a URL:
// e-mail addresses get mailto:, "www." addresses http://.
func LinkTarget(addr string) string {
	addr = strings.TrimSpace(addr)
	low := strings.ToLower(addr)
	switch {
	case addr == "" || strings.HasPrefix(addr, "#"):
		return addr
	case strings.Contains(low, "://") || strings.HasPrefix(low, "mailto:") || strings.HasPrefix(low, "file:") ||
		strings.HasPrefix(low, "tel:") || strings.HasPrefix(low, "news:"):
		return addr
	case strings.Contains(addr, "@") && !strings.ContainsAny(addr, " /"):
		return "mailto:" + addr
	case strings.HasPrefix(low, "www."):
		return "http://" + addr
	case len(addr) > 2 && (addr[1] == ':' || strings.HasPrefix(addr, `\\`)):
		return "file:///" + strings.ReplaceAll(addr, `\`, "/")
	}
	return "http://" + addr
}

// hyperlinkStyle returns the ID of the Hyperlink character style, adding
// Word's definition to the style sheet if the document lacks it (the
// caller must be in a docLevel edit then; added reports it).
func (e *Editor) hyperlinkStyle(add bool) (id string, missing bool) {
	for _, st := range e.Doc.Styles.Styles {
		if st.Type == CharacterStyle && strings.EqualFold(st.Name, "Hyperlink") {
			return st.ID, false
		}
	}
	if !add {
		return "Hyperlink", true
	}
	for _, st := range DefaultStyles().Styles {
		if st.ID == "Hyperlink" {
			c := *st
			if d := e.Doc.Styles.Default(CharacterStyle); d != nil {
				c.BasedOn = d.ID
			} else {
				c.BasedOn = ""
			}
			e.Doc.Styles.Styles = append(e.Doc.Styles.Styles, &c)
			return c.ID, true
		}
	}
	return "", true
}

// InsertLink makes a hyperlink to addr (see LinkTarget; "#name" links to
// a bookmark) showing text. With the caret in a link it changes that
// link; otherwise the selection becomes the link when its text is text,
// else text replaces the selection.
func (e *Editor) InsertLink(text, addr string) {
	defer e.Op("Insert Hyperlink")()
	url := LinkTarget(addr)
	if url == "" {
		return
	}
	if text == "" {
		text = addr
	}
	sp := &Span{Kind: SpanLink, URL: url}
	if strings.HasPrefix(url, "#") {
		sp.URL, sp.Anchor = "", url[1:]
	}
	if l, ok := e.SelectionLink(); ok {
		e.editLink(l, sp, text)
		return
	}
	_, missing := e.hyperlinkStyle(false)
	from, to := e.span()
	sel := e.Sel
	e.edit("link", from, to, missing, func() {
		style, _ := e.hyperlinkStyle(true)
		a, b := sel.Ordered()
		if a.Para == b.Para && a != b && e.SelectedText() == text {
			p := e.locs[a.Para].P
			i, j := splitRuns(p, a.Off), splitRuns(p, b.Off)
			if i < j {
				sp.Parent = p.Runs[i].Span
			}
			for k := i; k < j; k++ {
				p.Runs[k].Span = sp
				p.Runs[k].Style = style
			}
			normalize(p)
			e.Sel = Selection{b, b}
			return
		}
		r := e.caretRun(e.Sel.Caret)
		if !e.Sel.Empty() {
			r = e.caretRun(e.selStartPlusOne())
		}
		p := e.deleteSelection()
		sp.Parent = r.Span
		if r.Span != nil && linkOf(r.Span) != nil {
			sp.Parent = nil
		}
		r.Kind, r.Text, r.Span, r.Style, r.Opaque = RunText, text, sp, style, nil
		insertRuns(e.locs[p.Para].P, p.Off, r)
		p.Off += utf8.RuneCountInString(text)
		e.Sel = Selection{p, p}
	})
}

// editLink points link l at nsp's target and, when text differs from
// its text, replaces the text.
func (e *Editor) editLink(l Link, nsp *Span, text string) {
	top := e.locs[l.Start.Para].Top
	e.edit("link", top, top, false, func() {
		para := e.locs[l.Start.Para].P
		old := l.Span
		sp := *old // spans are shared with undo copies: replace, don't change
		sp.URL, sp.Anchor = nsp.URL, nsp.Anchor
		if sp.URL != old.URL || sp.Anchor != old.Anchor {
			sp.Tooltip = ""
		}
		i, j := splitRuns(para, l.Start.Off), splitRuns(para, l.End.Off)
		replace := func(s *Span) *Span {
			// Runs may sit in spans nested inside the link.
			var chain []*Span
			for c := s; c != nil && c != old; c = c.Parent {
				chain = append(chain, c)
			}
			if len(chain) == 0 {
				return &sp
			}
			var parent = &sp
			for k := len(chain) - 1; k >= 0; k-- {
				c := *chain[k]
				c.Parent = parent
				parent = &c
			}
			return parent
		}
		for k := i; k < j; k++ {
			para.Runs[k].Span = replace(para.Runs[k].Span)
		}
		end := l.End
		if text != l.Text(e) {
			first := para.Runs[i]
			para.Runs = append(para.Runs[:i:i], para.Runs[j:]...)
			first.Kind, first.Text, first.Opaque, first.Span = RunText, text, nil, &sp
			insertRuns(para, l.Start.Off, first)
			end = Pos{l.Start.Para, l.Start.Off + utf8.RuneCountInString(text)}
		}
		normalize(para)
		e.Sel = Selection{end, end}
	})
}

// RemoveLink turns the link at the caret back into plain text.
func (e *Editor) RemoveLink() {
	defer e.Op("Remove Hyperlink")()
	l, ok := e.SelectionLink()
	if !ok {
		return
	}
	style, _ := e.hyperlinkStyle(false)
	top := e.locs[l.Start.Para].Top
	e.edit("link", top, top, false, func() {
		para := e.locs[l.Start.Para].P
		i, j := splitRuns(para, l.Start.Off), splitRuns(para, l.End.Off)
		for k := i; k < j; k++ {
			r := &para.Runs[k]
			if r.Span == l.Span {
				r.Span = l.Span.Parent
			} else {
				// Spans nested in the link lose it as their ancestor.
				var chain []*Span
				for c := r.Span; c != nil && c != l.Span; c = c.Parent {
					chain = append(chain, c)
				}
				parent := l.Span.Parent
				for m := len(chain) - 1; m >= 0; m-- {
					c := *chain[m]
					c.Parent = parent
					parent = &c
				}
				r.Span = parent
			}
			if r.Style == style {
				r.Style = ""
			}
		}
		normalize(para)
	})
}

// unlinkedStyle is the character style for text typed just after a link
// whose style is style: none instead of the Hyperlink style.
func (e *Editor) unlinkedStyle(style string) string {
	if id, _ := e.hyperlinkStyle(false); style == id {
		return ""
	}
	return style
}
