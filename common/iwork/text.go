package iwork

import (
	"sort"
	"strings"
	"unicode/utf16"
)

// Text object types.
const (
	TypeStorage            = 2001 // TSWP.StorageArchive
	TypeStorage2           = 2005 // TSWP.StorageArchive (another kind)
	TypeDrawableAttachment = 2003 // TSWP.DrawableAttachmentArchive
	TypeShapeInfo          = 2011 // TSWP.ShapeInfoArchive (a shape or text box)
	TypeHyperlinkField     = 2032 // TSWP.HyperlinkFieldArchive
	TypeDateTimeField      = 2034 // TSWP.DateTimeSmartFieldArchive
	TypePageNumber         = 2043 // TSWP.NumberAttachmentArchive
	TypeFootnoteRef        = 2008 // TSWP.FootnoteReferenceAttachmentArchive
	TypeRichTextPayload    = 6218 // TST.RichTextPayloadArchive
)

// Characters with a meaning in iWork text.
const (
	charAttachment = '￼' // an attachment (drawable, footnote, field) sits here
	charLineBreak  = ' '
	charParaBreak  = ' '
	charPageBreak  = '\u000C'
	charColBreak   = '\u000E'
	charSectBreak  = '\u0004'
)

// Span is a stretch of a paragraph with one character style.
type Span struct {
	Text      string
	CharStyle uint64 // TSWP.CharacterStyleArchive, 0 for none
	// Kind marks special content: an attachment (Object is the attachment
	// object), a line, page or column break, or a tab.
	Kind   SpanKind
	Object uint64
	Link   string // hyperlink URL, if the span is in a link
}

// SpanKind distinguishes text from special characters.
type SpanKind int8

const (
	SpanText SpanKind = iota
	SpanTab
	SpanLineBreak
	SpanPageBreak
	SpanColumnBreak
	SpanAttachment
)

// Paragraph is one paragraph of a text storage.
type Paragraph struct {
	Style     uint64 // TSWP.ParagraphStyleArchive
	ListStyle uint64 // TSWP.ListStyleArchive, 0 when not in a list
	Level     int    // list level (0-based)
	Spans     []Span
}

// Text returns the paragraph's plain text (attachments dropped).
func (p *Paragraph) Text() string {
	var b strings.Builder
	for _, s := range p.Spans {
		switch s.Kind {
		case SpanText:
			b.WriteString(s.Text)
		case SpanTab:
			b.WriteByte('\t')
		case SpanLineBreak:
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// attrTable is an ObjectAttributeTable: from each character index on, an
// object applies (0 = none).
type attrTable struct {
	at  []int
	obj []uint64
}

func readAttrTable(m Message, keepPrevious bool) attrTable {
	var t attrTable
	for _, e := range m.Msgs(1) {
		obj := e.Ref(2)
		if obj == 0 && keepPrevious && len(t.obj) > 0 {
			obj = t.obj[len(t.obj)-1]
		}
		t.at = append(t.at, int(e.Uint(1)))
		t.obj = append(t.obj, obj)
	}
	return t
}

// lookup returns the object in effect at character i.
func (t attrTable) lookup(i int) uint64 {
	k := sort.SearchInts(t.at, i+1) - 1
	if k < 0 {
		return 0
	}
	return t.obj[k]
}

// exact returns the object attached at exactly character i.
func (t attrTable) exact(i int) uint64 {
	k := sort.SearchInts(t.at, i)
	if k < len(t.at) && t.at[k] == i {
		return t.obj[k]
	}
	return 0
}

// next is the first change after character i (or -1).
func (t attrTable) next(i int) int {
	k := sort.SearchInts(t.at, i+1)
	if k < len(t.at) {
		return t.at[k]
	}
	return -1
}

// Paragraphs splits a TSWP.StorageArchive (or the storage a
// TST.RichTextPayloadArchive points to) into paragraphs of styled spans.
// Character positions in the attribute tables count UTF-16 code units, as
// NSString does.
func (d *Document) Paragraphs(id uint64) []Paragraph {
	o := d.Objects[id]
	if o != nil && o.Type == TypeRichTextPayload {
		o = d.Objects[o.Msg.Ref(1)]
	}
	if o == nil || o.Type == TypeRichTextPayload {
		return nil // (a damaged file's payload pointing at a payload)
	}
	m := o.Msg
	text := utf16.Encode([]rune(strings.Join(m.Strs(3), "")))
	paraStyles := readAttrTable(m.Msg(5), true)
	listStyles := readAttrTable(m.Msg(7), false)
	charStyles := readAttrTable(m.Msg(8), false)
	attachments := readAttrTable(m.Msg(9), false)
	smartFields := readAttrTable(m.Msg(11), false)
	var levels attrTable
	for _, e := range m.Msg(6).Msgs(1) {
		levels.at = append(levels.at, int(e.Uint(1)))
		levels.obj = append(levels.obj, e.Uint(2))
	}

	var paras []Paragraph
	start := 0
	for start <= len(text) {
		end := start
		for end < len(text) && text[end] != '\n' && text[end] != charParaBreak {
			end++
		}
		if end == len(text) && start == end && len(paras) > 0 {
			break // text ending in a paragraph mark has no empty paragraph after it
		}
		p := Paragraph{
			Style:     paraStyles.lookup(start),
			ListStyle: listStyles.lookup(start),
			Level:     int(levels.lookup(start)),
		}
		// Split at character style, link and special characters.
		var cur Span
		flush := func() {
			if cur.Text != "" {
				p.Spans = append(p.Spans, cur)
			}
			cur = Span{}
		}
		for i := start; i < end; i++ {
			cs := charStyles.lookup(i)
			link := d.linkURL(smartFields.lookup(i))
			if cs != cur.CharStyle || link != cur.Link {
				flush()
			}
			cur.CharStyle, cur.Link = cs, link
			c := text[i]
			special := SpanText
			switch c {
			case '\t':
				special = SpanTab
			case charLineBreak, '\r', '\v':
				special = SpanLineBreak
			case charPageBreak, charSectBreak:
				special = SpanPageBreak
			case charColBreak:
				special = SpanColumnBreak
			case charAttachment:
				special = SpanAttachment
			}
			if special == SpanText {
				if c < 0x20 {
					continue // other control characters (field markers...)
				}
				if utf16.IsSurrogate(rune(c)) && i+1 < end {
					cur.Text += string(utf16.Decode(text[i : i+2]))
					i++
					continue
				}
				cur.Text += string(rune(c))
				continue
			}
			flush()
			sp := Span{Kind: special, CharStyle: cs}
			if special == SpanAttachment {
				sp.Object = attachments.exact(i)
				if sp.Object == 0 {
					continue
				}
			}
			p.Spans = append(p.Spans, sp)
			cur.CharStyle, cur.Link = cs, link
		}
		flush()
		paras = append(paras, p)
		start = end + 1
	}
	return paras
}

// linkURL returns the URL of a hyperlink smart field, "" for other fields.
func (d *Document) linkURL(field uint64) string {
	if field == 0 {
		return ""
	}
	if o := d.Objects[field]; o != nil && o.Type == TypeHyperlinkField {
		return o.Msg.Str(2)
	}
	return ""
}

// PlainText returns a storage's text with paragraphs separated by
// newlines.
func (d *Document) PlainText(id uint64) string {
	var lines []string
	for _, p := range d.Paragraphs(id) {
		lines = append(lines, p.Text())
	}
	return strings.Join(lines, "\n")
}

// Geometry returns a drawable's position and size on its page (or in its
// group), in points.
func (d *Document) Geometry(id uint64) (x, y, w, h float64) {
	o := d.Objects[id]
	if o == nil {
		return
	}
	drawable := o.Msg.Msg(1) // the TSD.DrawableArchive super of images, groups, tables...
	if o.Type == TypeShapeInfo {
		drawable = drawable.Msg(1) // ShapeInfoArchive -> ShapeArchive -> DrawableArchive
	}
	g := drawable.Msg(1)
	pos, size := g.Msg(1), g.Msg(2)
	return pos.Float(1), pos.Float(2), size.Float(1), size.Float(2)
}
