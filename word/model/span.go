package model

// SpanKind says what a Span is.
type SpanKind int8

const (
	SpanOther SpanKind = iota // content control, smart tag, tracked insertion...: kept, not interpreted
	SpanLink                  // hyperlink
)

// Span is an element wrapped around a stretch of runs (Run.Span) or blocks
// (Paragraph.Wrap, Table.Wrap) that WordLite shows as ordinary content but
// keeps so a save in the same format can wrap the content again. All runs
// (or consecutive blocks) inside one span share the same *Span.
type Span struct {
	Kind    SpanKind
	URL     string // SpanLink: external target ("https://...")
	Anchor  string // SpanLink: bookmark in this document
	Tooltip string // SpanLink
	// Format names the source of Start/End ("docx"); Start and End are the
	// original opening and closing XML, written back by that format.
	Format     string
	Start, End []byte
	Parent     *Span // enclosing span, if nested
}

// Chain returns s and its ancestors, outermost first.
func (s *Span) Chain() []*Span {
	var out []*Span
	for ; s != nil; s = s.Parent {
		out = append([]*Span{s}, out...)
	}
	return out
}
