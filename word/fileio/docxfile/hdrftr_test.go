package docxfile

import (
	"bytes"
	"strings"
	"testing"

	"officelite/word/model"
)

func hfText(h *model.HeaderFooter) string {
	if h == nil {
		return "<nil>"
	}
	var out []string
	model.WalkBlocks(h.Blocks, func(p *model.Paragraph) bool { out = append(out, p.Text()); return true })
	return strings.Join(out, "|")
}

func TestReadHeadersFooters(t *testing.T) {
	doc, orig := readFile(t, "word.docx")
	hf := &doc.HF
	if got := hfText(hf.Headers[model.HFDefault]); got != "Header text" {
		t.Errorf("default header %q", got)
	}
	if got := hfText(hf.Footers[model.HFDefault]); got != "Footer text" {
		t.Errorf("default footer %q", got)
	}
	if hf.Headers[model.HFFirst] == nil || hf.Footers[model.HFEven] == nil {
		t.Errorf("first/even parts not read")
	}
	if hf.TitlePage || hf.EvenOdd {
		t.Errorf("switches: %v %v", hf.TitlePage, hf.EvenOdd)
	}
	if p := hf.Headers[model.HFDefault].Blocks[0].(*model.Paragraph); p.Style != "Header" {
		t.Errorf("header style %q", p.Style)
	}
	for _, w := range doc.Warnings {
		if strings.Contains(w, "Headers and footers") {
			t.Errorf("warning still given: %q", w)
		}
	}

	// Edit the default header, turn on a different first page and
	// odd/even pages.
	h := hf.Headers[model.HFDefault].Blocks[0].(*model.Paragraph)
	h.Runs = append(h.Runs, model.Run{Text: " - page "}, model.Run{Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "PAGE"}})
	hf.TitlePage, hf.EvenOdd = true, true
	out := write(t, doc)
	parts, po := zipParts(t, out), zipParts(t, orig)
	if bytes.Equal(parts["word/header2.xml"], po["word/header2.xml"]) {
		t.Errorf("edited header not rewritten")
	}
	if !bytes.Equal(parts["word/footer2.xml"], po["word/footer2.xml"]) {
		t.Errorf("unchanged footer not kept byte for byte")
	}
	if !bytes.Contains(parts["word/settings.xml"], []byte("<w:evenAndOddHeaders/>")) {
		t.Errorf("settings lack evenAndOddHeaders")
	}
	b := mustRead(t, out)
	if got := hfText(b.HF.Headers[model.HFDefault]); got != "Header text - page ￼" {
		t.Errorf("re-read header %q", got)
	}
	r := b.HF.Headers[model.HFDefault].Blocks[0].(*model.Paragraph).Runs
	if o := r[len(r)-1].Opaque; o == nil || model.FieldName(o.Field) != "PAGE" {
		t.Errorf("PAGE field lost: %+v", r[len(r)-1])
	}
	if !b.HF.TitlePage || !b.HF.EvenOdd {
		t.Errorf("switches not saved")
	}
	// And off again.
	b.HF.TitlePage, b.HF.EvenOdd = false, false
	c := mustRead(t, write(t, b))
	if c.HF.TitlePage || c.HF.EvenOdd || c.HF.Headers[model.HFFirst] == nil {
		t.Errorf("switches off: %v %v", c.HF.TitlePage, c.HF.EvenOdd)
	}
}

func TestNewHeaderFooter(t *testing.T) {
	for _, src := range []string{"", "pydocx.docx"} {
		doc := model.New("x")
		if src != "" {
			doc, _ = readFile(t, src)
		}
		h := doc.NewHeaderFooter(model.HFDefault, false)
		h.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Top"}}
		f := doc.NewHeaderFooter(model.HFFirst, true)
		f.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Page "},
			{Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "PAGE"}},
			{Text: " of "}, {Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "NUMPAGES"}}}
		doc.HF.TitlePage = true
		doc.HF.EvenOdd = src == ""
		out := write(t, doc)
		parts := zipParts(t, out)
		if !bytes.Contains(parts["[Content_Types].xml"], []byte(ctHeader)) || !bytes.Contains(parts["[Content_Types].xml"], []byte(ctFooter)) {
			t.Errorf("%s: content types lack header/footer", src)
		}
		if !bytes.Contains(parts["word/document.xml"], []byte(`<w:headerReference w:type="default"`)) ||
			!bytes.Contains(parts["word/document.xml"], []byte(`<w:footerReference w:type="first"`)) ||
			!bytes.Contains(parts["word/document.xml"], []byte(`<w:titlePg/>`)) {
			t.Errorf("%s: sectPr lacks references", src)
		}
		b := mustRead(t, out)
		if got := hfText(b.HF.Headers[model.HFDefault]); got != "Top" {
			t.Errorf("%s: header %q", src, got)
		}
		if got := hfText(b.HF.Footers[model.HFFirst]); got != "Page ￼ of ￼" {
			t.Errorf("%s: footer %q", src, got)
		}
		if !b.HF.TitlePage || b.HF.EvenOdd != (src == "") {
			t.Errorf("%s: switches %v %v", src, b.HF.TitlePage, b.HF.EvenOdd)
		}
		if b.Styles.Find("Header") == nil || b.Styles.Find("Footer") == nil {
			t.Errorf("%s: header/footer styles not saved", src)
		}
		// A second save is stable.
		out2 := write(t, b)
		p1, p2 := zipParts(t, out2), zipParts(t, write(t, mustRead(t, out2)))
		for n := range p1 {
			if !bytes.Equal(p1[n], p2[n]) {
				t.Errorf("%s: part %s changed on another save", src, n)
			}
		}
	}
}

func TestCustomStylesRoundTrip(t *testing.T) {
	for _, src := range []string{"", "word.docx"} {
		doc := model.New("x")
		if src != "" {
			doc, _ = readFile(t, src)
		}
		e := model.NewEditor(doc)
		e.InsertText("Styled")
		e.SelectAll()
		e.ToggleBold()
		if _, err := e.NewStyle(model.StyleSpec{Name: "My Para", Type: model.ParagraphStyle, Next: "Normal"}); err != nil {
			t.Fatal(err)
		}
		e.SelectAll()
		if _, err := e.NewStyle(model.StyleSpec{Name: "My Chars", Type: model.CharacterStyle}); err != nil {
			t.Fatal(err)
		}
		b := mustRead(t, write(t, doc))
		ps := b.Styles.Find("MyPara")
		cs := b.Styles.Find("MyChars")
		if ps == nil || cs == nil {
			t.Fatalf("%s: styles not saved", src)
		}
		if !ps.Custom || !cs.Custom || ps.Name != "My Para" || ps.Next != "Normal" || ps.Run.Bold != model.On || cs.Type != model.CharacterStyle {
			t.Errorf("%s: %+v %+v", src, ps, cs)
		}
		if b.Styles.Find("Normal").Custom {
			t.Errorf("%s: Normal marked custom", src)
		}
		p := paras(b)[0]
		if p.Style != "MyPara" || len(p.Runs) == 0 || p.Runs[0].Style != "MyChars" {
			t.Errorf("%s: usage lost: %q %+v", src, p.Style, p.Runs)
		}
		// Deleting a custom style removes it from styles.xml.
		e2 := model.NewEditor(b)
		if err := e2.DeleteStyle("MyChars"); err != nil {
			t.Fatal(err)
		}
		c := mustRead(t, write(t, b))
		if c.Styles.Find("MyChars") != nil || paras(c)[0].Runs[0].Style != "" {
			t.Errorf("%s: deleted style still there", src)
		}
	}
}
