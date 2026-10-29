package fileio

import (
	"bytes"
	"strings"
	"testing"

	"officelite/word/model"
)

// Hyperlinks and fields read from a .doc are opaque runs; saving as .txt
// writes the text they show (it used to drop it), but not the
// placeholders of pictures and shapes.
func TestSaveTextKeepsFieldText(t *testing.T) {
	doc, err := Open("../../Samples/Tables-Pictures-Links.doc")
	if err != nil {
		t.Skip(err)
	}
	var b bytes.Buffer
	if err := Write(doc, &b, TXT); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Visit the Go site or mail someone@example.com") || strings.Contains(out, "￼") {
		t.Errorf("link text missing:\n%s", out)
	}

	p := &model.Paragraph{Runs: []model.Run{
		{Text: "See "},
		{Kind: model.RunOpaque, Text: "[1]", Opaque: &model.Opaque{Field: "CITATION x"}},
		{Kind: model.RunOpaque, Text: "[picture]", Opaque: &model.Opaque{Picture: &model.Picture{}}},
		{Kind: model.RunOpaque, Text: "[shape]", Opaque: &model.Opaque{}},
		{Text: "."},
	}}
	b.Reset()
	if err := Write(&model.Document{Body: []model.Block{p}}, &b, TXT); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "See [1].\r\n" {
		t.Errorf("text = %q", got)
	}
}
