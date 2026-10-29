package model

import "slices"

// Footnotes and endnotes. A note is referenced from the text by an
// opaque run (Opaque.Note) showing its mark; the note's own blocks start
// with the same mark. Automatically numbered notes (Mark "") are
// numbered in document order: footnotes at the bottom of the page that
// references them, endnotes after the last section, each after a short
// separator line.

// Note is a footnote or an endnote.
type Note struct {
	Endnote bool
	Blocks  []Block
	// Mark is a custom reference mark ("*"); "" numbers the note
	// automatically.
	Mark string
	// DocxID is the note's w:id in the .docx it was read from, so an
	// unchanged notes part can be written back as it was.
	DocxID string
}

// NoteSettings are the document's footnote and endnote options.
type NoteSettings struct {
	// FootFormat and EndFormat are the number formats (see FormatNumber;
	// "" means "decimal" for footnotes and "lowerRoman" for endnotes, as
	// Word's defaults).
	FootFormat, EndFormat string
	// FootStart and EndStart are the first numbers (0 = 1).
	FootStart, EndStart int
	// DocxSep holds the separator notes of a .docx footnotes.xml and
	// endnotes.xml (w:footnote elements with a w:type), kept as they were.
	DocxFootSep, DocxEndSep [][]byte
}

// Format returns the number format of footnotes or endnotes.
func (s *NoteSettings) Format(endnote bool) string {
	if endnote {
		if s.EndFormat != "" {
			return s.EndFormat
		}
		return "lowerRoman"
	}
	if s.FootFormat != "" {
		return s.FootFormat
	}
	return "decimal"
}

func (s *NoteSettings) start(endnote bool) int {
	n := s.FootStart
	if endnote {
		n = s.EndStart
	}
	return max(n, 1)
}

// NoteRef is a note reference found in the body.
type NoteRef struct {
	Note *Note
	Para *Paragraph
	Run  int // index in Para.Runs
}

// NoteRefs lists the note references in blocks, in document order.
func NoteRefs(blocks []Block) []NoteRef {
	var out []NoteRef
	WalkBlocks(blocks, func(p *Paragraph) bool {
		for i := range p.Runs {
			if o := p.Runs[i].Opaque; o != nil && o.Note != nil {
				out = append(out, NoteRef{o.Note, p, i})
			}
		}
		return true
	})
	return out
}

// NumberNotes sets the shown mark of every note reference (and of the
// mark that starts each note's text) from the notes' order; it returns
// the paragraphs whose marks changed.
func (d *Document) NumberNotes() []*Paragraph {
	var changed []*Paragraph
	n := [2]int{d.Notes.start(false), d.Notes.start(true)}
	for _, ref := range NoteRefs(d.Body) {
		note := ref.Note
		k := 0
		if note.Endnote {
			k = 1
		}
		text := note.Mark
		if text == "" {
			text = FormatNumber(n[k], d.Notes.Format(note.Endnote))
			n[k]++
		}
		if r := &ref.Para.Runs[ref.Run]; r.Text != text {
			r.Text = text
			changed = append(changed, ref.Para)
		}
		WalkBlocks(note.Blocks, func(p *Paragraph) bool {
			for i := range p.Runs {
				if o := p.Runs[i].Opaque; o != nil && o.NoteMark && p.Runs[i].Text != text {
					p.Runs[i].Text = text
				}
			}
			return true
		})
	}
	return changed
}

// NoteRefStyle and NoteTextStyle return the IDs of the footnote (or
// endnote) reference and text styles, adding Word's definitions when the
// style sheet lacks them.
func NoteRefStyle(ss *StyleSheet, endnote bool) string {
	name, id := "footnote reference", "FootnoteReference"
	if endnote {
		name, id = "endnote reference", "EndnoteReference"
	}
	return ensureStyle(ss, name, id, CharacterStyle, func(st *Style) {
		st.Run.Vert = VertSuper
	})
}

func NoteTextStyle(ss *StyleSheet, endnote bool) string {
	name, id := "footnote text", "FootnoteText"
	if endnote {
		name, id = "endnote text", "EndnoteText"
	}
	return ensureStyle(ss, name, id, ParagraphStyle, func(st *Style) {
		st.Run.Size = 10
		st.Para = ParaProps{SpaceAfter: F(0), Line: 1, LineRule: LineAuto}
	})
}

func ensureStyle(ss *StyleSheet, name, id string, t StyleType, init func(*Style)) string {
	for _, st := range ss.Styles {
		if st.Type == t && (st.ID == id || equalFold(st.Name, name)) {
			return st.ID
		}
	}
	for ss.Find(id) != nil {
		id += "1"
	}
	st := &Style{ID: id, Name: name, Type: t}
	if t == ParagraphStyle {
		st.BasedOn = "Normal"
		if d := ss.Default(ParagraphStyle); d != nil {
			st.BasedOn = d.ID
		}
	} else if d := ss.Default(CharacterStyle); d != nil {
		st.BasedOn = d.ID
	}
	init(st)
	ss.Styles = append(ss.Styles, st)
	return id
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// NewNote returns a note holding one paragraph that starts with its
// mark, in the note text style.
func NewNote(ss *StyleSheet, endnote bool) *Note {
	ref := NoteRefStyle(ss, endnote)
	p := &Paragraph{Style: NoteTextStyle(ss, endnote)}
	p.Runs = []Run{
		{Kind: RunOpaque, Style: ref, Opaque: &Opaque{NoteMark: true}},
		{Kind: RunText, Text: " "},
	}
	return &Note{Endnote: endnote, Blocks: []Block{p}}
}

// InsertNote inserts a footnote (or endnote) reference at the caret,
// replacing the selection, and returns the new note (its text is one
// paragraph holding the mark and a space).
func (e *Editor) InsertNote(endnote bool) *Note {
	defer e.Op(either(endnote, "Insert Endnote", "Insert Footnote"))()
	from, to := e.span()
	var note *Note
	e.edit("insert", from, to, true, func() {
		note = NewNote(&e.Doc.Styles, endnote)
		r := e.caretRun(e.Sel.Caret)
		p := e.deleteSelection()
		r.Kind, r.Text, r.Style, r.Span = RunOpaque, "", NoteRefStyle(&e.Doc.Styles, endnote), nil
		r.Props.Vert = VertUnset
		r.Opaque = &Opaque{Note: note}
		insertRuns(e.locs[p.Para].P, p.Off, r)
		p.Off++
		e.Sel = Selection{p, p}
		for _, q := range e.Doc.NumberNotes() {
			e.dirty[q] = true
		}
	})
	return note
}

// NotesIn returns the notes referenced from blocks, in order.
func NotesIn(blocks []Block) []*Note {
	var out []*Note
	for _, r := range NoteRefs(blocks) {
		if !slices.Contains(out, r.Note) {
			out = append(out, r.Note)
		}
	}
	return out
}
