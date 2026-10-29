package model

// Sections. A document is split into sections, each with its own page
// setup, columns, headers and footers and page numbering. As in a .docx,
// a section's properties are stored at its end: a body paragraph whose
// Sect is set ends a section (its mark is the section break), and the
// Document's own Section holds the last section's properties. Deleting a
// section break's paragraph mark merges the two sections, and the
// following section's properties win, as in Word.

// SectionBreak is how a section starts.
type SectionBreak int8

const (
	BreakNextPage   SectionBreak = iota // on a new page (Word's default)
	BreakContinuous                     // on the same page
	BreakEvenPage                       // on the next even page
	BreakOddPage                        // on the next odd page
	BreakNextColumn                     // in the next column
)

// BreakNames are the .docx w:type values of the break kinds.
var BreakNames = [...]string{"nextPage", "continuous", "evenPage", "oddPage", "nextColumn"}

// BreakLabels are the names Word shows for section breaks with
// formatting marks on ("Section Break (Next Page)").
var BreakLabels = [...]string{"Next Page", "Continuous", "Even Page", "Odd Page", "Next Column"}

// Columns are a section's text columns.
type Columns struct {
	Count int     // 0 or 1: one column
	Space float64 // space between equal columns, points (0 = 36)
	// Widths and Spaces are the widths of unequal columns and the space
	// after each (nil: equal columns).
	Widths, Spaces []float64
	Separator      bool // a line between columns
}

// N is the number of columns (at least 1).
func (c *Columns) N() int {
	if len(c.Widths) > 1 {
		return len(c.Widths)
	}
	return max(c.Count, 1)
}

// Layout returns the left offset and width of each column within a text
// area width wide.
func (c *Columns) Layout(width float64) (x, w []float64) {
	n := c.N()
	if n == 1 {
		return []float64{0}, []float64{width}
	}
	if len(c.Widths) == n {
		total := 0.0
		for i, cw := range c.Widths {
			total += cw
			if i < n-1 && i < len(c.Spaces) {
				total += c.Spaces[i]
			}
		}
		k := 1.0
		if total > 0 {
			k = width / total // the columns fill the text area
		}
		pos := 0.0
		for i, cw := range c.Widths {
			x, w = append(x, pos), append(w, cw*k)
			pos += cw * k
			if i < len(c.Spaces) {
				pos += c.Spaces[i] * k
			}
		}
		return x, w
	}
	sp := c.Space
	if sp <= 0 {
		sp = 36
	}
	cw := max((width-sp*float64(n-1))/float64(n), 18)
	for i := range n {
		x, w = append(x, float64(i)*(cw+sp)), append(w, cw)
	}
	return x, w
}

// Section is one section's properties.
type Section struct {
	Page PageSetup
	// HF holds the section's headers and footers. In every section but the
	// first a nil header or footer is linked to the previous section's
	// (Word's "Link to Previous"); see Document.HeaderFor. HF.EvenOdd is
	// a document setting, kept in the last section (Document.HF).
	HF      HeadersFooters
	Break   SectionBreak
	Columns Columns
	// PageNumStart restarts page numbering at the section's first page
	// when PageNumRestart is set; PageNumFormat is the number format of
	// PAGE fields in the section ("" = decimal; see FormatNumber).
	PageNumRestart bool
	PageNumStart   int
	PageNumFormat  string
	// TitlePage etc. live in HF. DocxXML is the original <w:sectPr>
	// content of a section read from .docx, so the writer keeps the
	// properties the model doesn't cover (line numbering, page borders...).
	DocxXML string
}

// Clone deep-copies the section (headers and footers included).
func (s *Section) Clone() *Section {
	c := *s
	c.HF = s.HF.Clone()
	c.Columns.Widths = append([]float64(nil), s.Columns.Widths...)
	c.Columns.Spaces = append([]float64(nil), s.Columns.Spaces...)
	return &c
}

// Sections lists the document's sections in order: those ended by body
// paragraphs (Paragraph.Sect), then the last one (&d.Section).
func (d *Document) Sections() []*Section {
	var out []*Section
	for _, b := range d.Body {
		if p, ok := b.(*Paragraph); ok && p.Sect != nil {
			out = append(out, p.Sect)
		}
	}
	return append(out, &d.Section)
}

// HeaderFor returns the header (or footer) of a kind shown in section i
// of secs (see Sections): the section's own or, when it has none, the
// nearest previous section's, as Word links headers to the previous
// section's.
func HeaderFor(secs []*Section, i int, kind HFKind, footer bool) *HeaderFooter {
	for ; i >= 0; i-- {
		if h := *secs[i].HF.Slot(kind, footer); h != nil {
			return h
		}
	}
	return nil
}

// AnyHF reports whether any section has a header or footer.
func AnyHF(secs []*Section) bool {
	for _, s := range secs {
		if s.HF.Any() {
			return true
		}
	}
	return false
}

// AllHF calls f for the headers and footers of every section.
func (d *Document) AllHF(f func(hf *HeaderFooter)) {
	for _, s := range d.Sections() {
		s.HF.All(f)
	}
}

// InsertSectionBreak ends the section at the caret's paragraph with a
// section break: the text before the caret ends a section with the
// current section's properties and headers, and the rest stays in the
// current section, which now starts with a break of kind k and shows the
// headers through the link to the previous section, as in Word. Inside a
// table nothing happens.
func (e *Editor) InsertSectionBreak(k SectionBreak) {
	defer e.Op("Section Break")()
	if e.locs[e.Sel.Caret.Para].Cell != nil {
		return
	}
	from, to := e.span()
	end := e.sectionEnd(to)
	if end >= 0 {
		to = end // the section's properties change with its blocks
	}
	saved := e.Doc.Section
	e.edit("insert", from, to, false, func() {
		p := e.deleteSelection()
		cur := &e.Doc.Section
		if end >= 0 {
			cur = e.Doc.Body[e.sectionEnd(e.locs[p.Para].Top)].(*Paragraph).Sect
		}
		q := e.splitPara(p)
		first := e.locs[q.Para-1].P
		first.Sect = cur.Clone()
		first.Sect.HF = cur.HF
		first.Sect.HF.EvenOdd = false
		cur.Break = k
		cur.HF = HeadersFooters{TitlePage: cur.HF.TitlePage, EvenOdd: cur.HF.EvenOdd}
		e.Sel = Selection{q, q}
	})
	if end < 0 && len(e.undo) > 0 {
		e.undo[len(e.undo)-1].sect = &saved
	}
}

// sectionEnd returns the index of the body paragraph that ends the
// section holding body block i, or -1 for the last section.
func (e *Editor) sectionEnd(i int) int {
	for ; i < len(e.Doc.Body); i++ {
		if p, ok := e.Doc.Body[i].(*Paragraph); ok && p.Sect != nil {
			return i
		}
	}
	return -1
}
