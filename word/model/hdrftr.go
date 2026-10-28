package model

import (
	"strings"
)

// HFKind says which pages a header or footer is for, as a Word section
// defines them.
type HFKind int8

const (
	HFDefault HFKind = iota // every page (odd pages when EvenOdd is set)
	HFFirst                 // the first page, when TitlePage is set
	HFEven                  // even pages, when EvenOdd is set
)

// HFKindNames are the .docx w:type values of the kinds.
var HFKindNames = [3]string{"default", "first", "even"}

// HeaderFooter is the content of one header or footer.
type HeaderFooter struct {
	Blocks []Block
	// Part is the .docx part it was read from ("word/header1.xml"), so
	// the writer can keep that part's relationships (pictures) and root
	// namespaces; "" for one created in WordLite.
	Part string
}

// HeadersFooters are a document's headers and footers (those of its last
// section; WordLite has one section).
type HeadersFooters struct {
	Headers, Footers [3]*HeaderFooter // by HFKind; nil = none
	TitlePage        bool             // different first page (w:titlePg)
	EvenOdd          bool             // different odd and even pages (settings' w:evenAndOddHeaders)
}

// Any reports whether there is any header or footer.
func (h *HeadersFooters) Any() bool {
	for k := range 3 {
		if h.Headers[k] != nil || h.Footers[k] != nil {
			return true
		}
	}
	return false
}

// Slot returns where the header (or footer) of a kind is kept.
func (h *HeadersFooters) Slot(kind HFKind, footer bool) **HeaderFooter {
	if footer {
		return &h.Footers[kind]
	}
	return &h.Headers[kind]
}

// KindFor returns the kind of header/footer page (0-based index) shows.
func (h *HeadersFooters) KindFor(page int) HFKind {
	switch {
	case page == 0 && h.TitlePage:
		return HFFirst
	case page%2 == 1 && h.EvenOdd:
		return HFEven
	}
	return HFDefault
}

// For returns the header (or footer) shown on a page and its kind; the
// header is nil when that kind has none (the page shows no header).
func (h *HeadersFooters) For(page int, footer bool) (*HeaderFooter, HFKind) {
	k := h.KindFor(page)
	return *h.Slot(k, footer), k
}

// Clone deep-copies the headers and footers.
func (h HeadersFooters) Clone() HeadersFooters {
	out := h
	for k := range 3 {
		for _, s := range []**HeaderFooter{&out.Headers[k], &out.Footers[k]} {
			if *s != nil {
				c := **s
				c.Blocks = CopyBlocks(c.Blocks)
				*s = &c
			}
		}
	}
	return out
}

// All calls f for every header and footer.
func (h *HeadersFooters) All(f func(hf *HeaderFooter)) {
	for k := range 3 {
		if h.Headers[k] != nil {
			f(h.Headers[k])
		}
		if h.Footers[k] != nil {
			f(h.Footers[k])
		}
	}
}

// SubDocument returns a document holding blocks (a header's) that shares
// d's styles, lists and page setup, for laying out and editing them.
func (d *Document) SubDocument(blocks []Block) *Document {
	if len(blocks) == 0 {
		blocks = []Block{&Paragraph{}}
	}
	return &Document{Name: d.Name, Styles: d.Styles, Body: blocks, Lists: d.Lists, Page: d.Page,
		DefaultTabStop: d.DefaultTabStop, SourceFormat: d.SourceFormat}
}

// HFStyleID returns the ID of the "header" (or "footer") style, adding
// Word's definition to the style sheet when it is missing: Normal with
// a center tab in the middle and a right tab at the margin of a Letter
// page, and no space after.
func HFStyleID(ss *StyleSheet, footer bool) string {
	name, id := "header", "Header"
	if footer {
		name, id = "footer", "Footer"
	}
	for _, st := range ss.Styles {
		if st.Type == ParagraphStyle && strings.EqualFold(st.Name, name) {
			return st.ID
		}
	}
	for ss.Find(id) != nil {
		id += "1"
	}
	base := "Normal"
	if st := ss.Default(ParagraphStyle); st != nil {
		base = st.ID
	}
	ss.Styles = append(ss.Styles, &Style{ID: id, Name: name, Type: ParagraphStyle, BasedOn: base,
		Para: ParaProps{SpaceAfter: F(0), Line: 1, LineRule: LineAuto,
			Tabs: []TabStop{{Pos: 234, Align: AlignCenter}, {Pos: 468, Align: AlignRight}}}})
	return id
}

// NewHeaderFooter creates an empty header (or footer) of a kind, adding
// its style when needed. It returns the existing one if there is one.
func (d *Document) NewHeaderFooter(kind HFKind, footer bool) *HeaderFooter {
	slot := d.HF.Slot(kind, footer)
	if *slot != nil {
		return *slot
	}
	id := HFStyleID(&d.Styles, footer)
	*slot = &HeaderFooter{Blocks: []Block{&Paragraph{Style: id}}}
	return *slot
}

// Field names WordLite computes.
const (
	FieldPage     = "PAGE"
	FieldNumPages = "NUMPAGES"
)

// FieldName returns a field instruction's field type, upper-cased
// ("PAGE" for `PAGE \* MERGEFORMAT`).
func FieldName(instr string) string {
	f := strings.Fields(instr)
	if len(f) == 0 {
		return ""
	}
	return strings.ToUpper(f[0])
}

// HasPageFields reports whether blocks hold a PAGE, NUMPAGES or
// SECTIONPAGES field, whose shown value depends on the page.
func HasPageFields(blocks []Block) bool {
	found := false
	WalkBlocks(blocks, func(p *Paragraph) bool {
		for i := range p.Runs {
			if o := p.Runs[i].Opaque; o != nil && o.Field != "" {
				switch FieldName(o.Field) {
				case FieldPage, FieldNumPages, "SECTIONPAGES":
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// InsertSimpleField inserts a field (e.g. `PAGE`) showing result at the caret,
// replacing the selection, with the caret's formatting.
func (e *Editor) InsertSimpleField(instr, result string) {
	from, to := e.span()
	e.edit("insert", from, to, false, func() {
		r := e.caretRun(e.Sel.Caret)
		p := e.deleteSelection()
		r.Kind, r.Text = RunOpaque, result
		r.Opaque = &Opaque{Field: instr}
		insertRuns(e.locs[p.Para].P, p.Off, r)
		p.Off++
		e.Sel = Selection{p, p}
	})
}
