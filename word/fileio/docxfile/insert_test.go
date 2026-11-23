package docxfile

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"officelite/word/model"
)

func insertTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			img.Set(x, y, color.NRGBA{uint8(x * 6), 100, uint8(y * 12), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zipPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			b := readZipFile(t, f)
			return string(b)
		}
	}
	return ""
}

// TestInsertedContentRoundTrip saves a new document with a table, a
// picture and a hyperlink made by the editor and reads it back.
func TestInsertedContentRoundTrip(t *testing.T) {
	doc := model.New("t")
	e := model.NewEditor(doc)
	e.InsertText("Visit ")
	e.InsertLink("Go", "go.dev")
	e.InsertText(" now")
	e.Enter()
	pic, w, h, err := model.NewPicture(insertTestPNG(t), "chart.png")
	if err != nil {
		t.Fatal(err)
	}
	e.InsertPicture(pic, w, h)
	e.SetCaret(e.Sel.Caret, false)
	e.Enter()
	e.InsertTable(2, 2)
	e.InsertText("cell")

	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if got := zipPart(t, data, "word/media/image1.png"); got != string(pic.Data) {
		t.Fatalf("media part missing (%d bytes)", len(got))
	}
	if ct := zipPart(t, data, "[Content_Types].xml"); !strings.Contains(ct, `Extension="png"`) {
		t.Errorf("no png content type: %s", ct)
	}
	if rels := zipPart(t, data, "word/_rels/document.xml.rels"); !strings.Contains(rels, "media/image1.png") || !strings.Contains(rels, "http://go.dev") {
		t.Errorf("rels: %s", rels)
	}
	docXML := zipPart(t, data, "word/document.xml")
	for _, want := range []string{"<w:drawing>", "<wp:inline", "<w:hyperlink", "<w:tblBorders>", `w:val="Hyperlink"`} {
		if !strings.Contains(docXML, want) {
			t.Errorf("document.xml lacks %s", want)
		}
	}

	back, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	checkInserted(t, back, pic)

	// Saving the read document again keeps the picture (now as the
	// original XML) and doesn't add another media part.
	buf.Reset()
	if err := Write(back, &buf); err != nil {
		t.Fatal(err)
	}
	if zipPart(t, buf.Bytes(), "word/media/image2.png") != "" {
		t.Error("picture duplicated on re-save")
	}
	again, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	checkInserted(t, again, pic)

	// A picture added to the read document goes into a new media part.
	e2 := model.NewEditor(again)
	e2.SetCaret(model.Pos{Para: 0}, false)
	e2.InsertPicture(pic, 10, 10)
	buf.Reset()
	if err := Write(again, &buf); err != nil {
		t.Fatal(err)
	}
	if zipPart(t, buf.Bytes(), "word/media/image2.png") == "" {
		t.Error("second picture not written")
	}
	third, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	third.Paragraphs(func(p *model.Paragraph) bool {
		for _, r := range p.Runs {
			if r.Opaque != nil && r.Opaque.Picture != nil {
				n++
			}
		}
		return true
	})
	if n != 2 {
		t.Errorf("%d pictures after adding one", n)
	}
}

func checkInserted(t *testing.T, doc *model.Document, pic *model.Picture) {
	t.Helper()
	var link *model.Span
	var got *model.Opaque
	doc.Paragraphs(func(p *model.Paragraph) bool {
		for _, r := range p.Runs {
			if r.Span != nil && r.Span.Kind == model.SpanLink {
				link = r.Span
			}
			if r.Opaque != nil && r.Opaque.Picture != nil {
				got = r.Opaque
			}
		}
		return true
	})
	if link == nil || link.URL != "http://go.dev" {
		t.Errorf("link: %+v", link)
	}
	if got == nil || !bytes.Equal(got.Picture.Data, pic.Data) || got.Width != 30 || got.Height != 15 {
		t.Fatalf("picture: %+v", got)
	}
	var tbl *model.Table
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tbl = tb
		}
	}
	if tbl == nil || len(tbl.Rows) != 2 || len(tbl.Rows[0].Cells) != 2 || !tbl.Borders {
		t.Fatalf("table: %+v", tbl)
	}
	if txt := tbl.Rows[0].Cells[0].Blocks[0].(*model.Paragraph).Text(); txt != "cell" {
		t.Errorf("cell text %q", txt)
	}
	for _, w := range doc.Warnings {
		if strings.Contains(w, "placeholder") {
			t.Errorf("warning: %s", w)
		}
	}
}

// TestPictureLookRoundTrip checks that a picture's crop, border,
// alternative text and decorative mark survive writing and reading, both
// inline and floating, and that a picture without alternative text gets
// none (not its file name).
func TestPictureLookRoundTrip(t *testing.T) {
	doc := model.New("t")
	e := model.NewEditor(doc)
	pic, w, h, err := model.NewPicture(insertTestPNG(t), "chart.png")
	if err != nil {
		t.Fatal(err)
	}
	e.InsertPicture(pic, w, h)
	e.SetCaret(e.Sel.Caret, false)
	e.InsertPicture(pic, w, h)
	e.SetCaret(e.Sel.Caret, false)
	e.InsertPicture(pic, w, h)
	inline := model.PictureLook{Crop: [4]float64{0.1, 0.2, 0, 0.05}, Alt: "Sales by month"}
	e.EditPicture(model.Pos{Para: 0, Off: 0}, "Format Picture", func(o *model.Opaque) {
		o.Look, o.Line, o.LineWidth = inline, "C00000", 2.25
	})
	floating := model.PictureLook{Crop: [4]float64{0, 0, 0.25, 0}, Decorative: true}
	e.EditPicture(model.Pos{Para: 0, Off: 1}, "Wrap Text", func(o *model.Opaque) {
		model.FloatPicture(o, model.WrapSquare, 36, 12)
		o.Float.Look = floating
	})
	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	if x := zipPart(t, buf.Bytes(), "word/document.xml"); strings.Contains(x, `descr="chart.png"`) {
		t.Error("the file name was written as alternative text")
	}
	again, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var runs []model.Run
	again.Paragraphs(func(p *model.Paragraph) bool {
		for _, r := range p.Runs {
			if r.Opaque != nil {
				runs = append(runs, r)
			}
		}
		return false
	})
	if len(runs) != 3 {
		t.Fatalf("%d pictures read back", len(runs))
	}
	if o := runs[0].Opaque; o.Picture == nil || o.Look != inline || o.Line != "C00000" || o.LineWidth != 2.25 {
		t.Errorf("inline picture: %+v line %q %v", o.Look, o.Line, o.LineWidth)
	}
	if f := runs[1].Opaque.Float; f == nil || f.Picture == nil || f.Look != floating || f.Wrap != model.WrapSquare || f.X != 36 || f.Y != 12 {
		t.Errorf("floating picture: %+v", f)
	}
	if o := runs[2].Opaque; o.Picture == nil || o.Look != (model.PictureLook{}) {
		t.Errorf("plain picture: %+v", o.Look)
	}
}
