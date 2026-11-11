// Tests for sections, columns, notes, the table of contents and header
// linking.

package model

import (
	"strconv"
	"strings"
	"testing"
)

// TestSectionsAndBreaks checks that a section break keeps the setup and
// headers on the section it ends, links the next section's headers, stays
// at the paragraph's end on Enter, merges on Delete with the following
// section's properties winning, and undoes.
func TestSectionsAndBreaks(t *testing.T) {
	d, e := newDoc("one two", "three")
	d.Page.Landscape, d.Page.Width, d.Page.Height = true, 792, 612
	h := d.NewHeaderFooter(HFDefault, false)
	e.SetCaret(Pos{0, 3}, false)
	e.InsertSectionBreak(BreakContinuous)
	secs := d.Sections()
	if len(secs) != 2 || texts(d) != "one| two|three" {
		t.Fatalf("%d sections, %q", len(secs), texts(d))
	}
	// The new break's section keeps the setup and headers; the one after
	// starts with the break and shows the headers through the link.
	if !secs[0].Page.Landscape || secs[0].HF.Headers[HFDefault] != h || secs[1].Break != BreakContinuous || secs[1].HF.Any() {
		t.Errorf("sections = %+v / %+v", secs[0], secs[1])
	}
	if HeaderFor(secs, 1, HFDefault, false) != h {
		t.Error("the second section's header is linked to the first's")
	}
	// Enter at the end of the section's last paragraph: the break stays
	// at the end.
	e.SetCaret(Pos{0, 3}, false)
	e.Enter()
	if ps := d.Body; ps[0].(*Paragraph).Sect != nil || ps[1].(*Paragraph).Sect == nil {
		t.Errorf("section break moved: %v %v", ps[0].(*Paragraph).Sect, ps[1].(*Paragraph).Sect)
	}
	// Deleting the break merges the sections: the following one's
	// properties win.
	e.SetCaret(Pos{1, 0}, false)
	e.Delete()
	if n := len(d.Sections()); n != 1 || texts(d) != "one| two|three" && texts(d) != "one|two|three" {
		t.Errorf("after delete: %d sections, %q", n, texts(d))
	}
	if d.Break != BreakContinuous {
		t.Errorf("merged section break = %d", d.Break)
	}
	// Undo brings the section back, and undoing the insertion restores
	// the last section as it was.
	e.Undo()
	if len(d.Sections()) != 2 {
		t.Errorf("undo: %d sections", len(d.Sections()))
	}
	e.Undo()
	e.Undo()
	if len(d.Sections()) != 1 || d.Break != BreakNextPage || d.HF.Headers[HFDefault] != h || texts(d) != "one two|three" {
		t.Errorf("undo all: %d sections, break %d, %q", len(d.Sections()), d.Break, texts(d))
	}
}

// TestColumnsLayout checks the offsets and widths of equal and unequal
// columns.
func TestColumnsLayout(t *testing.T) {
	c := Columns{Count: 2, Space: 36}
	x, w := c.Layout(468)
	if len(x) != 2 || w[0] != 216 || x[1] != 252 {
		t.Errorf("equal columns: %v %v", x, w)
	}
	c = Columns{Widths: []float64{100, 200}, Spaces: []float64{50}}
	x, w = c.Layout(350)
	if x[1] != 150 || w[1] != 200 {
		t.Errorf("unequal columns: %v %v", x, w)
	}
}

// TestNotes checks that inserted notes are numbered in document order
// (footnotes decimal, endnotes roman), their own marks follow, their
// styles are added, and undo removes a note.
func TestNotes(t *testing.T) {
	d, e := newDoc("abc")
	e.SetCaret(Pos{0, 1}, false)
	n1 := e.InsertNote(false)
	e.SetCaret(Pos{0, 0}, false)
	n0 := e.InsertNote(false)
	en := e.InsertNote(true)
	refs := NoteRefs(d.Body)
	if len(refs) != 3 || refs[0].Note != n0 || refs[1].Note != en || refs[2].Note != n1 {
		t.Fatalf("refs = %+v", refs)
	}
	p := d.Body[0].(*Paragraph)
	if p.Text() != "￼￼a￼bc" || p.Runs[0].Text != "1" || p.Runs[1].Text != "i" || p.Runs[3].Text != "2" {
		t.Errorf("marks = %q %q %q", p.Runs[0].Text, p.Runs[1].Text, p.Runs[3].Text)
	}
	if mark := n1.Blocks[0].(*Paragraph).Runs[0]; !mark.Opaque.NoteMark || mark.Text != "2" {
		t.Errorf("note's own mark = %+v", mark)
	}
	if d.Styles.Find(NoteTextStyle(&d.Styles, false)) == nil || d.Styles.Find(NoteRefStyle(&d.Styles, true)).Run.Vert != VertSuper {
		t.Error("note styles missing")
	}
	e.Undo()
	if len(NoteRefs(d.Body)) != 2 {
		t.Error("undo didn't remove the endnote")
	}
}

// TestUpdateTOC checks that an existing TOC field is found as one range
// and rebuilt from the headings with TOC styles, leaders and page numbers,
// and that undo restores the old entries.
func TestUpdateTOC(t *testing.T) {
	d, e := newDoc("", "end", "Intro", "Body text", "Details")
	h1, h2 := &Style{ID: "Heading1", Name: "heading 1", Type: ParagraphStyle, Para: ParaProps{OutlineLevel: 1}},
		&Style{ID: "Heading2", Name: "heading 2", Type: ParagraphStyle, Para: ParaProps{OutlineLevel: 2}}
	d.Styles.Styles = append(d.Styles.Styles, h1, h2)
	ps := d.Body
	ps[2].(*Paragraph).Style, ps[4].(*Paragraph).Style = "Heading1", "Heading2"
	// An old table of contents: begin (with the code), a stale entry, and
	// the end in a paragraph of its own.
	ps[0].(*Paragraph).Runs = []Run{{Kind: RunOpaque, Opaque: &Opaque{FieldChar: FieldStart, Field: `TOC \o "1-3"`}}, {Text: "Old\t9"}}
	ps[1].(*Paragraph).Runs = []Run{{Kind: RunOpaque, Opaque: &Opaque{FieldChar: FieldEnd}}}
	e.reindex()
	if fr := e.FieldRanges(); len(fr) != 1 || fr[0].From != (Pos{0, 0}) || fr[0].To != (Pos{1, 1}) {
		t.Fatalf("field ranges = %+v", fr)
	}
	pages := map[string]int{"Intro": 2, "Details": 3}
	if !e.UpdateTOC(468, func(p *Paragraph) int { return pages[p.Text()] }) {
		t.Fatal("no TOC")
	}
	if got := texts(d); got != "￼Intro\t2|Details\t3|￼|Intro|Body text|Details" {
		t.Errorf("updated: %q", got)
	}
	if p := d.Body[1].(*Paragraph); p.Style != "TOC2" || p.Props.Tabs[0].Leader != '.' || d.Styles.Find("TOC2").Para.IndentLeft == nil {
		t.Errorf("entry = %+v", p)
	}
	e.Undo()
	if texts(d) != "￼Old\t9|￼|Intro|Body text|Details" {
		t.Errorf("undo: %q", texts(d))
	}
}

// TestNoteUndoRenumbers checks that undoing and redoing a note's insertion
// renumbers the notes after it, and that undo snapshots keep their own
// copies of note text.
func TestNoteUndoRenumbers(t *testing.T) {
	d, e := newDoc("abcd")
	marks := func() string {
		var s string
		for _, r := range NoteRefs(d.Body) {
			s += r.Para.Runs[r.Run].Text
		}
		return s
	}
	e.SetCaret(Pos{0, 2}, false)
	e.Enter() // "ab" / "cd": the notes sit in different paragraphs
	e.SetCaret(Pos{1, 1}, false)
	e.InsertNote(false)
	e.SetCaret(Pos{0, 1}, false)
	e.InsertNote(false) // now first: the other becomes 2
	if got := marks(); got != "12" {
		t.Fatalf("marks = %q, want 12", got)
	}
	e.Undo()
	if got := marks(); got != "1" {
		t.Errorf("after undo marks = %q, want 1", got)
	}
	e.Redo()
	if got := marks(); got != "12" {
		t.Errorf("after redo marks = %q, want 12", got)
	}
}

// TestChangeSectionsUndoesStyles checks that undoing a header change
// that added the "header" style (the document's first header) takes the
// style out again.
func TestChangeSectionsUndoesStyles(t *testing.T) {
	d, e := newDoc("abc")
	n := len(d.Styles.Styles)
	e.ChangeSections("Header", func() { d.UnlinkHeader(0, HFDefault, false) })
	if d.HF.Headers[HFDefault] == nil || len(d.Styles.Styles) != n+1 {
		t.Fatalf("header %v, %d styles (had %d)", d.HF.Headers[HFDefault], len(d.Styles.Styles), n)
	}
	e.Undo()
	if d.HF.Headers[HFDefault] != nil || len(d.Styles.Styles) != n {
		t.Errorf("after undo: header %v, %d styles; want none, %d", d.HF.Headers[HFDefault], len(d.Styles.Styles), n)
	}
	e.Redo()
	if d.HF.Headers[HFDefault] == nil || len(d.Styles.Styles) != n+1 {
		t.Errorf("after redo: header %v, %d styles", d.HF.Headers[HFDefault], len(d.Styles.Styles))
	}
}

// TestPasteNoteTwice checks that each paste of a copied note reference
// gets a note of its own: shared, editing one note's text would change
// the others, and numbering would give them all the last number.
func TestPasteNoteTwice(t *testing.T) {
	d, e := newDoc("abc")
	e.SetCaret(Pos{0, 1}, false)
	orig := e.InsertNote(false)
	e.Select(Pos{0, 1}, Pos{0, 2})
	f := e.Copy()
	e.SetCaret(Pos{0, 0}, false)
	e.Paste(f)
	e.Paste(f)
	refs := NoteRefs(d.Body)
	if len(refs) != 3 {
		t.Fatalf("%d note references", len(refs))
	}
	for i := range refs {
		for j := i + 1; j < len(refs); j++ {
			if refs[i].Note == refs[j].Note {
				t.Errorf("references %d and %d share a note", i, j)
			}
		}
	}
	orig.Blocks[0].(*Paragraph).Runs = append(orig.Blocks[0].(*Paragraph).Runs, Run{Text: "edited"})
	if strings.Contains(f.Paras[0].Runs[0].Opaque.Note.Blocks[0].(*Paragraph).Text(), "edited") {
		t.Error("editing the note changed the copied fragment")
	}
	d.NumberNotes()
	for i, r := range NoteRefs(d.Body) {
		if want := strconv.Itoa(i + 1); r.Para.Runs[r.Run].Text != want {
			t.Errorf("reference %d shows %q; want %q", i, r.Para.Runs[r.Run].Text, want)
		}
	}
}

// TestResizePictureOutOfRange checks that resizing at a position past the
// document's end (a stale position) resizes the picture there, as
// PictureAt finds it, rather than panicking.
func TestResizePictureOutOfRange(t *testing.T) {
	d, e := newDoc("abc")
	e.SetCaret(Pos{0, 3}, false)
	e.InsertPicture(&Picture{}, 40, 30)
	e.ResizePicture(Pos{5, 3}, 20, 15) // paragraph 5 doesn't exist: clamped, {0, 3}
	if r := d.Body[0].(*Paragraph).Runs[1]; r.Opaque.Width != 20 || r.Opaque.Height != 15 {
		t.Errorf("picture is %v × %v; want 20 × 15", r.Opaque.Width, r.Opaque.Height)
	}
}

// TestHeaderLinkOutOfRange checks that linking or unlinking the header of
// a section the document doesn't have (a stale index) does nothing.
func TestHeaderLinkOutOfRange(t *testing.T) {
	d, _ := newDoc("abc")
	for _, i := range []int{-1, 1, 5} {
		if h := d.UnlinkHeader(i, HFDefault, false); h != nil {
			t.Errorf("UnlinkHeader(%d) = %v", i, h)
		}
		d.LinkHeader(i, HFDefault, true)
		if d.HeaderLinked(i, HFDefault, false) {
			t.Errorf("HeaderLinked(%d)", i)
		}
	}
}
