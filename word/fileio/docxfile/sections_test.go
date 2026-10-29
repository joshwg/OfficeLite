package docxfile

import (
	"testing"

	"officelite/word/model"
)

// word2.docx has a second section in landscape with two columns, an
// endnote and a text box (DrawingML with a VML fallback).
func TestSectionsNotesTextBox(t *testing.T) {
	d, _ := readFile(t, "word2.docx")
	secs := d.Sections()
	if len(secs) != 2 || secs[0].Page.Landscape || !secs[1].Page.Landscape || secs[1].Columns.N() != 2 || secs[1].Columns.Space != 36 {
		t.Fatalf("sections = %+v / %+v", secs[0], secs[len(secs)-1])
	}
	refs := model.NoteRefs(d.Body)
	if len(refs) != 1 || !refs[0].Note.Endnote || refs[0].Para.Runs[refs[0].Run].Text != "i" {
		t.Fatalf("notes = %+v", refs)
	}
	np := refs[0].Note.Blocks[0].(*model.Paragraph)
	if o := np.Runs[0].Opaque; o == nil || !o.NoteMark {
		t.Errorf("note text starts with %+v", np.Runs[0])
	}
	var floats []*model.Floating
	model.Floats(d.Body, func(_ *model.Paragraph, _ int, f *model.Floating) { floats = append(floats, f) })
	if len(floats) != 1 {
		t.Fatalf("%d floats", len(floats))
	}
	f := floats[0]
	if f.X != 64 || f.Y != 102.75 || f.W != 150 || f.H != 60 || f.RelX != model.RelColumn || f.RelY != model.RelParagraph ||
		f.Fill != "" || f.Line != "000000" || f.LineWidth != 0.5 || !f.IsTextBox() {
		t.Errorf("text box = %+v", f)
	}
	if got := f.Blocks[0].(*model.Paragraph).Text(); got != "Text box content" {
		t.Errorf("text box text = %q", got)
	}
	for _, w := range d.Warnings {
		if w == "Text boxes are shown as placeholders." {
			t.Error(w)
		}
	}

	// Saved again: the sections, the endnote (with its id) and the text
	// box's original XML.
	b, err := Read(write(t, d))
	if err != nil {
		t.Fatal(err)
	}
	bs := b.Sections()
	if len(bs) != 2 || bs[1].Columns.N() != 2 || !bs[1].Page.Landscape {
		t.Errorf("saved sections = %+v", bs[len(bs)-1])
	}
	if r := model.NoteRefs(b.Body); len(r) != 1 || r[0].Note.DocxID != refs[0].Note.DocxID {
		t.Errorf("saved notes = %+v", r)
	}
}

// A document from another format gets sections, notes and a text box
// written as .docx.
func TestWriteSectionsNotesFloats(t *testing.T) {
	d := model.New("x")
	p0 := &model.Paragraph{Runs: []model.Run{{Text: "Portrait"}}}
	p0.Sect = &model.Section{Page: model.LetterPage()}
	note := model.NewNote(&d.Styles, false)
	note.Blocks[0].(*model.Paragraph).Runs = append(note.Blocks[0].(*model.Paragraph).Runs, model.Run{Text: "Foot"})
	box := &model.Floating{Shape: "ellipse", X: 10, Y: 20, W: 100, H: 50, RelY: model.RelPage, Wrap: model.WrapTopBottom,
		Fill: "FF0000", Line: "0000FF", LineWidth: 1, Inset: model.DefaultInset, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: "In"}}}}}
	p1 := &model.Paragraph{Runs: []model.Run{{Text: "Landscape"}, {Kind: model.RunOpaque, Opaque: &model.Opaque{Note: note}},
		{Kind: model.RunOpaque, Opaque: &model.Opaque{Float: box}}}}
	d.Body = []model.Block{p0, p1}
	d.Page = model.PageSetup{Width: 792, Height: 612, MarginTop: 72, MarginBottom: 72, MarginLeft: 72, MarginRight: 72, Header: 36, Footer: 36, Landscape: true}
	d.Break, d.Columns = model.BreakOddPage, model.Columns{Count: 3, Space: 18, Separator: true}
	d.PageNumRestart, d.PageNumStart, d.PageNumFormat = true, 3, "upperRoman"
	d.NewHeaderFooter(model.HFDefault, true).Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Foot of section 2"}}
	b, err := Read(write(t, d))
	if err != nil {
		t.Fatal(err)
	}
	s := b.Sections()
	if len(s) != 2 || s[0].Page.Landscape || s[1].Break != model.BreakOddPage || s[1].Columns.N() != 3 || !s[1].Columns.Separator ||
		!s[1].PageNumRestart || s[1].PageNumStart != 3 || s[1].PageNumFormat != "upperRoman" || s[1].HF.Footers[model.HFDefault] == nil || s[0].HF.Any() {
		t.Fatalf("sections = %+v / %+v", s[0], s[len(s)-1])
	}
	r := model.NoteRefs(b.Body)
	if len(r) != 1 || r[0].Note.Endnote || r[0].Para.Runs[r[0].Run].Text != "1" {
		t.Fatalf("notes = %+v", r)
	}
	if got := r[0].Note.Blocks[0].(*model.Paragraph).Text(); got != "￼ Foot" {
		t.Errorf("note text = %q", got)
	}
	var f *model.Floating
	model.Floats(b.Body, func(_ *model.Paragraph, _ int, x *model.Floating) { f = x })
	if f == nil || f.Shape != "ellipse" || f.X != 10 || f.Y != 20 || f.W != 100 || f.H != 50 || f.RelY != model.RelPage ||
		f.Wrap != model.WrapTopBottom || f.Fill != "FF0000" || f.Line != "0000FF" || f.LineWidth != 1 || len(f.Blocks) != 1 {
		t.Errorf("float = %+v", f)
	}
}
