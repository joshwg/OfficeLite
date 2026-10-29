package docfile

import (
	"testing"

	"officelite/word/model"
)

func TestHeadersAndCustomStyles(t *testing.T) {
	doc := model.New("x")
	e := model.NewEditor(doc)
	e.InsertText("Text")
	e.SelectAll()
	if _, err := e.NewStyle(model.StyleSpec{Name: "My Style", Type: model.ParagraphStyle}); err != nil {
		t.Fatal(err)
	}
	b := roundTrip(t, doc)
	st := b.Styles.FindStyleByName("My Style")
	if st == nil || !st.Custom {
		t.Fatalf("custom style not kept: %+v", st)
	}
	if n := b.Styles.Default(model.ParagraphStyle); n == nil || n.Custom {
		t.Errorf("Normal is custom")
	}
	// Headers and footers are saved in the header subdocument.
	h := doc.NewHeaderFooter(model.HFDefault, false)
	h.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Head "}, {Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "PAGE"}}}
	f := doc.NewHeaderFooter(model.HFFirst, true)
	f.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "First foot"}}
	doc.HF.TitlePage = true
	if ws := Warnings(doc); len(ws) != 0 {
		t.Errorf("warnings: %v", ws)
	}
	b = roundTrip(t, doc)
	if !b.HF.TitlePage || b.HF.Headers[model.HFDefault] == nil || b.HF.Footers[model.HFFirst] == nil || b.HF.Footers[model.HFDefault] != nil {
		t.Fatalf("headers = %+v", b.HF)
	}
	p := b.HF.Headers[model.HFDefault].Blocks[0].(*model.Paragraph)
	if p.Text() != "Head ￼" || p.Runs[1].Opaque.Field != "PAGE" || p.Runs[1].Text != "1" {
		t.Errorf("header = %q %+v", p.Text(), p.Runs)
	}
	if got := b.HF.Footers[model.HFFirst].Blocks[0].(*model.Paragraph).Text(); got != "First foot" {
		t.Errorf("first footer = %q", got)
	}
}
