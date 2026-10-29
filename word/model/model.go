// Package model is WordLite's document model, shared by the .docx and .doc
// readers/writers and the editor: blocks (paragraphs and tables) made of
// runs, with direct formatting layered over a style sheet the way Word
// resolves it (document defaults → paragraph style chain → character style
// chain → direct formatting).
//
// Units: lengths are points (1/72 inch), font sizes are points, colors are
// "RRGGBB" hex ("" means unset/automatic).
package model

// Tri is an on/off property that may be unset (inherit).
type Tri int8

const (
	Unset Tri = iota
	On
	Off
)

// Bool resolves t, falling back to def when unset.
func (t Tri) Bool(def bool) bool {
	switch t {
	case On:
		return true
	case Off:
		return false
	}
	return def
}

// TriOf converts a bool into On/Off.
func TriOf(b bool) Tri {
	if b {
		return On
	}
	return Off
}

// Align is a paragraph's horizontal alignment.
type Align int8

const (
	AlignUnset Align = iota
	AlignLeft
	AlignCenter
	AlignRight
	AlignJustify
)

// VertAlign is superscript/subscript.
type VertAlign int8

const (
	VertUnset VertAlign = iota
	VertBaseline
	VertSuper
	VertSub
)

// Underline styles; only presence is rendered, the kind round-trips.
type Underline int8

const (
	UnderlineUnset Underline = iota
	UnderlineNone
	UnderlineSingle
	UnderlineDouble
	UnderlineDotted
	UnderlineDashed
	UnderlineWave
	UnderlineWords
	UnderlineThick
)

// RunProps is character formatting. Zero values mean "inherit".
type RunProps struct {
	Font      string  // e.g. "Calibri"
	Size      float64 // points; 0 = inherit
	Bold      Tri
	Italic    Tri
	Underline Underline
	Strike    Tri
	Caps      Tri
	SmallCaps Tri
	Hidden    Tri
	Color     string // RRGGBB
	Highlight string // Word highlight name ("yellow") or RRGGBB shading
	Vert      VertAlign
	// DocxXML is the original <w:rPr> content of a run read from .docx, so
	// the .docx writer can keep properties the model doesn't cover (language,
	// kerning, complex-script sizes...). Fields above override it where they
	// differ; other formats ignore it.
	DocxXML string
}

// Merge returns p with every property set in over replacing it.
func (p RunProps) Merge(over RunProps) RunProps {
	if over.Font != "" {
		p.Font = over.Font
	}
	if over.Size != 0 {
		p.Size = over.Size
	}
	mergeTri(&p.Bold, over.Bold)
	mergeTri(&p.Italic, over.Italic)
	if over.Underline != UnderlineUnset {
		p.Underline = over.Underline
	}
	mergeTri(&p.Strike, over.Strike)
	mergeTri(&p.Caps, over.Caps)
	mergeTri(&p.SmallCaps, over.SmallCaps)
	mergeTri(&p.Hidden, over.Hidden)
	if over.Color != "" {
		p.Color = over.Color
	}
	if over.Highlight != "" {
		p.Highlight = over.Highlight
	}
	if over.Vert != VertUnset {
		p.Vert = over.Vert
	}
	return p
}

func mergeTri(dst *Tri, over Tri) {
	if over != Unset {
		*dst = over
	}
}

// LineRule says how ParaProps.Line is measured.
type LineRule int8

const (
	LineUnset   LineRule = iota
	LineAuto             // Line is a multiple of single spacing (1, 1.15, 2...)
	LineExact            // Line is points
	LineAtLeast          // Line is a minimum in points
)

// TabStop is a custom tab position.
type TabStop struct {
	Pos     float64 // points from the left margin
	Align   Align   // AlignLeft, AlignCenter or AlignRight
	Decimal bool    // aligns on the decimal point (Align is ignored)
	Leader  byte    // 0, '.', '-', '_'
	Clear   bool    // removes an inherited stop at Pos
}

// ListRef puts a paragraph in a numbered or bulleted list.
type ListRef struct {
	NumID int // key into Document.Lists; 0 = no list (explicitly removed)
	Level int // 0..8
}

// ParaProps is paragraph formatting. Nil pointers / zero values inherit.
type ParaProps struct {
	Align           Align
	SpaceBefore     *float64
	SpaceAfter      *float64
	Line            float64 // see LineRule
	LineRule        LineRule
	IndentLeft      *float64
	IndentRight     *float64
	IndentFirst     *float64 // negative = hanging indent
	KeepNext        Tri
	KeepLines       Tri
	PageBreakBefore Tri
	WidowControl    Tri
	// ContextualSpacing drops the space before and after between
	// paragraphs of the same style (Word's "Don't add space between
	// paragraphs of the same style").
	ContextualSpacing Tri
	// Borders are the paragraph's borders (top, left, bottom, right; see
	// TableCell.Borders); nil leaves an edge to the style.
	Borders      [4]*Border
	Shading      string // RRGGBB background
	Tabs         []TabStop
	List         *ListRef
	OutlineLevel int // 0 = body text, 1..9 = heading levels (from styles)
	// DocxXML is the original <w:pPr> content (as RunProps.DocxXML).
	DocxXML string
}

// Merge returns p with every property set in over replacing it.
func (p ParaProps) Merge(over ParaProps) ParaProps {
	if over.Align != AlignUnset {
		p.Align = over.Align
	}
	for _, f := range []struct{ dst, src **float64 }{
		{&p.SpaceBefore, &over.SpaceBefore}, {&p.SpaceAfter, &over.SpaceAfter},
		{&p.IndentLeft, &over.IndentLeft}, {&p.IndentRight, &over.IndentRight},
		{&p.IndentFirst, &over.IndentFirst},
	} {
		if *f.src != nil {
			v := **f.src
			*f.dst = &v
		}
	}
	if over.LineRule != LineUnset {
		p.Line, p.LineRule = over.Line, over.LineRule
	}
	mergeTri(&p.KeepNext, over.KeepNext)
	mergeTri(&p.KeepLines, over.KeepLines)
	mergeTri(&p.PageBreakBefore, over.PageBreakBefore)
	mergeTri(&p.WidowControl, over.WidowControl)
	mergeTri(&p.ContextualSpacing, over.ContextualSpacing)
	for i, b := range over.Borders {
		if b != nil {
			p.Borders[i] = b
		}
	}
	if over.Shading != "" {
		p.Shading = over.Shading
	}
	if len(over.Tabs) > 0 {
		p.Tabs = mergeTabs(p.Tabs, over.Tabs)
	}
	if over.List != nil {
		l := *over.List
		p.List = &l
	}
	if over.OutlineLevel != 0 {
		p.OutlineLevel = over.OutlineLevel
	}
	return p
}

func mergeTabs(base, over []TabStop) []TabStop {
	out := make([]TabStop, 0, len(base)+len(over))
	for _, b := range base {
		keep := true
		for _, o := range over {
			if o.Pos == b.Pos {
				keep = false
			}
		}
		if keep {
			out = append(out, b)
		}
	}
	for _, o := range over {
		if !o.Clear {
			out = append(out, o)
		}
	}
	return out
}

// F returns a pointer to v, for ParaProps fields.
func F(v float64) *float64 { return &v }

// StyleType is what a style applies to.
type StyleType int8

const (
	ParagraphStyle StyleType = iota
	CharacterStyle
	TableStyle
	NumberingStyle
)

// Style is a named style. A paragraph style's Run props apply to the whole
// paragraph's text; a character style only has Run props.
type Style struct {
	ID      string // reference key used by paragraphs/runs ("Heading1")
	Name    string // display name ("heading 1"; Word shows "Heading 1")
	Type    StyleType
	BasedOn string // ID
	Next    string // ID of the style for the following paragraph
	Default bool   // the default style of its type ("Normal", "DefaultParagraphFont")
	Hidden  bool   // not shown in the style gallery
	Custom  bool   // a user-defined style (not one of Word's built-in styles); only these can be deleted
	Para    ParaProps
	Run     RunProps
	// DocxXML is the original <w:style> content other than pPr/rPr
	// (uiPriority, qFormat, table style properties...), kept for .docx.
	DocxXML string
}

// StyleSheet holds the document defaults and named styles.
type StyleSheet struct {
	DefaultRun  RunProps
	DefaultPara ParaProps
	Styles      []*Style
}

// Find returns the style with the given ID, or nil.
func (s *StyleSheet) Find(id string) *Style {
	for _, st := range s.Styles {
		if st.ID == id {
			return st
		}
	}
	return nil
}

// Default returns the default style of type t (e.g. Normal), or nil.
func (s *StyleSheet) Default(t StyleType) *Style {
	for _, st := range s.Styles {
		if st.Type == t && st.Default {
			return st
		}
	}
	return nil
}

// chain returns the style and its BasedOn ancestors, root first.
func (s *StyleSheet) chain(id string, t StyleType) []*Style {
	var out []*Style
	seen := map[string]bool{}
	st := s.Find(id)
	if st == nil || st.Type != t {
		st = s.Default(t)
	}
	for st != nil && !seen[st.ID] {
		seen[st.ID] = true
		out = append([]*Style{st}, out...)
		st = s.Find(st.BasedOn)
	}
	return out
}

// Block is a paragraph or a table.
type Block interface{ isBlock() }

// RunKind distinguishes text from special content.
type RunKind int8

const (
	RunText      RunKind = iota // Text holds the characters
	RunTab                      // a tab character
	RunBreak                    // a line break (Shift+Enter)
	RunPageBreak                // a page break
	RunOpaque                   // content WordLite doesn't edit (pictures, fields...), kept for saving
)

// Run is a stretch of text with one formatting.
type Run struct {
	Kind  RunKind
	Text  string // RunText; for RunOpaque, a placeholder shown to the user (e.g. "[picture]")
	Style string // character style ID
	Props RunProps
	// Opaque holds format-specific data for RunOpaque so the same format can
	// write it back unchanged: Format names the source ("docx", "doc") and
	// Data its bytes (e.g. the <w:r> XML).
	Opaque *Opaque
	// Span is the inline element around this run (a hyperlink, content
	// control, tracked insertion...); runs in one span share the pointer.
	Span *Span
}

// Opaque is content preserved, not interpreted.
type Opaque struct {
	Format string
	Data   []byte
	Width  float64 // points, for layout (0 if unknown)
	Height float64
	// Field is the field instruction (e.g. `PAGE`, `TOC \o "1-3" \h`) when
	// the content is a field; Text is then its last result. Fields are not
	// recalculated.
	Field string
	// Picture is the image the content shows (an inline picture), drawn
	// at Width × Height; nil for other content.
	Picture *Picture
	// Note is the footnote or endnote this run references (Text is its
	// mark); NoteMark is set on the mark that starts a note's own text.
	Note     *Note
	NoteMark bool
	// Float is the floating object (text box, shape, picture) anchored at
	// this run; the run itself takes no room in the line (see float.go).
	Float *Floating
	// FieldChar marks the parts of a field whose result spans paragraphs
	// (a table of contents): the result is ordinary runs between a
	// FieldBegin (or FieldStart) run and a FieldEnd run. Field is set on
	// FieldInstr and FieldStart runs.
	FieldChar FieldChar
}

// FieldChar is a part of a field kept around a result in ordinary runs.
type FieldChar int8

const (
	FieldNone  FieldChar = iota
	FieldBegin           // the begin character
	FieldInstr           // the instruction
	FieldSep             // the separator between the instruction and the result
	FieldEnd             // the end character
	FieldStart           // begin, instruction and separator in one run
)

// Paragraph is a block of runs ending in a paragraph mark.
type Paragraph struct {
	Style string // paragraph style ID; "" = default (Normal)
	Props ParaProps
	Mark  RunProps // formatting of the paragraph mark (used for empty paragraphs)
	Runs  []Run
	// Wrap is the block-level element around this paragraph (e.g. a content
	// control holding a table of contents); consecutive blocks share it.
	Wrap *Span
	// Before holds block-level content the model doesn't cover that comes
	// just before this paragraph, kept for saving in the same format.
	Before []*Opaque
	// Sect is set on a body paragraph whose mark is a section break: it
	// holds the properties of the section the paragraph ends (see
	// section.go).
	Sect *Section
}

func (*Paragraph) isBlock() {}

// Text is the paragraph's plain text (tabs as '\t', breaks as '\n').
func (p *Paragraph) Text() string {
	var b []rune
	for _, r := range p.Runs {
		switch r.Kind {
		case RunText:
			b = append(b, []rune(r.Text)...)
		case RunTab:
			b = append(b, '\t')
		case RunBreak:
			b = append(b, '\n')
		case RunPageBreak:
			b = append(b, '\f')
		case RunOpaque:
			b = append(b, '￼')
		}
	}
	return string(b)
}

// Table is a grid of cells.
type Table struct {
	Style   string    // table style ID
	Columns []float64 // grid column widths, points
	Rows    []*TableRow
	Borders bool // draw single borders around every cell
	Indent  float64
	Align   Align
	Wrap    *Span     // see Paragraph.Wrap
	Before  []*Opaque // see Paragraph.Before
	DocxXML string    // original <w:tblPr> content (see RunProps.DocxXML)
}

func (*Table) isBlock() {}

// TableRow is one row of a table.
type TableRow struct {
	Cells  []*TableCell
	Height float64 // points; 0 = auto
	Header bool    // repeats at the top of each page
	// CantSplit keeps the row on one page (Word's "Allow row to break
	// across pages" turned off).
	CantSplit bool
	// DocxXML is the original <w:trPr> content (and <w:tblPrEx>), for .docx.
	DocxXML string
}

// TableCell holds blocks (at least one paragraph).
type TableCell struct {
	Blocks  []Block
	Span    int  // grid columns spanned (1 if 0)
	VMerge  int8 // 0 none, 1 starts a vertical merge, 2 continues one
	Shading string
	VAlign  int8 // 0 top, 1 center, 2 bottom
	Width   float64
	// Borders are the cell's own borders (top, left, bottom, right), which
	// replace the table's on those edges; nil keeps the table's.
	Borders [4]*Border
	DocxXML string // original <w:tcPr> content, for .docx
}

// Border is a table cell or paragraph border.
type Border struct {
	Width float64 // points; 0 = no border
	Color string  // RRGGBB; "" = automatic (black)
	Space float64 // paragraph borders: distance from the text, points
}

// Cell border edges, indexes into TableCell.Borders.
const (
	EdgeTop = iota
	EdgeLeft
	EdgeBottom
	EdgeRight
)

// ListLevel describes one level of a list definition.
type ListLevel struct {
	Format string // "bullet", "decimal", "lowerLetter", "upperLetter", "lowerRoman", "upperRoman", "none"
	Text   string // level text with %1..%9 placeholders, or the bullet character
	Start  int    // the first number (0 is a valid start)
	// Restart is set when this list overrides the start (w:startOverride):
	// its first paragraph at this level restarts the count at Start
	// instead of continuing the other lists of its Abstract.
	Restart bool
	Indent  float64 // left indent, points
	Hang    float64 // hanging indent, points
	Font    string  // bullet font (e.g. "Symbol")
}

// List is a numbering definition referenced by ListRef.NumID.
type List struct {
	Levels []ListLevel
	// Abstract names the definition the list instantiates (the .docx
	// w:abstractNum, the .doc list id); lists sharing one count together,
	// as in Word. "" means the list counts on its own.
	Abstract string
}

// PageSetup is a section's page size and margins.
type PageSetup struct {
	Width, Height                                    float64
	MarginTop, MarginBottom, MarginLeft, MarginRight float64
	Header, Footer                                   float64 // distance from edge
	Landscape                                        bool
}

// LetterPage is US Letter with Word's 1-inch margins.
func LetterPage() PageSetup {
	return PageSetup{Width: 612, Height: 792, MarginTop: 72, MarginBottom: 72, MarginLeft: 72, MarginRight: 72, Header: 36, Footer: 36}
}

// Document is a Word document.
type Document struct {
	Name   string
	Styles StyleSheet
	Body   []Block
	Lists  map[int]*List
	// Section holds the last section's properties: its page setup (Page)
	// and headers and footers (HF); earlier sections are kept on the
	// paragraphs that end them (see section.go).
	Section
	Warnings []string // things not shown or not kept
	// Source holds the original file's bytes and format so a writer for the
	// same format can carry over parts WordLite doesn't model (headers,
	// footers, images, settings). Writers must not depend on it.
	Source       []byte
	SourceFormat string // "docx", "doc", "txt"
	// DefaultTabStop is the interval of the default tab stops, points
	// (0 = 36, half an inch).
	DefaultTabStop float64
	// Notes holds the footnote and endnote settings (see notes.go).
	Notes NoteSettings
}

// New returns an empty document with Word's default styles.
func New(name string) *Document {
	d := &Document{Name: name, Section: Section{Page: LetterPage()}, Lists: map[int]*List{}}
	d.Styles = DefaultStyles()
	d.Body = []Block{&Paragraph{}}
	return d
}

// ResolvePara returns a paragraph's effective paragraph formatting.
func (d *Document) ResolvePara(p *Paragraph) ParaProps {
	pp := d.Styles.DefaultPara
	for _, st := range d.Styles.chain(p.Style, ParagraphStyle) {
		pp = pp.Merge(st.Para)
	}
	return pp.Merge(p.Props)
}

// ResolveRun returns the effective character formatting of run r in
// paragraph p (pass a zero Run with Props = p.Mark for the paragraph mark).
func (d *Document) ResolveRun(p *Paragraph, r *Run) RunProps {
	rp := d.Styles.DefaultRun
	for _, st := range d.Styles.chain(p.Style, ParagraphStyle) {
		rp = rp.Merge(st.Run)
	}
	if r.Style != "" {
		for _, st := range d.Styles.chain(r.Style, CharacterStyle) {
			rp = rp.Merge(st.Run)
		}
	}
	rp = rp.Merge(r.Props)
	if rp.Font == "" {
		rp.Font = "Calibri"
	}
	if rp.Size == 0 {
		rp.Size = 11
	}
	return rp
}

// Paragraphs calls f for every paragraph, including those in table cells,
// in document order; f returns false to stop.
func (d *Document) Paragraphs(f func(*Paragraph) bool) {
	WalkBlocks(d.Body, f)
}

// WalkBlocks calls f for every paragraph in blocks, including those in
// table cells; f returns false to stop. It reports whether it finished.
func WalkBlocks(blocks []Block, f func(*Paragraph) bool) bool {
	return walk(blocks, f)
}

func walk(blocks []Block, f func(*Paragraph) bool) bool {
	for _, b := range blocks {
		switch b := b.(type) {
		case *Paragraph:
			if !f(b) {
				return false
			}
		case *Table:
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					if !walk(c.Blocks, f) {
						return false
					}
				}
			}
		}
	}
	return true
}
