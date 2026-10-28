package docfile

import (
	"slices"
	"testing"

	"officelite/word/model"
)

func TestHeadersWarnAndCustomStyles(t *testing.T) {
	doc := model.New("x")
	e := model.NewEditor(doc)
	e.InsertText("Text")
	e.SelectAll()
	if _, err := e.NewStyle(model.StyleSpec{Name: "My Style", Type: model.ParagraphStyle}); err != nil {
		t.Fatal(err)
	}
	if ws := Warnings(doc); slices.Contains(ws, WarnHeadersLost) {
		t.Errorf("headers warning without headers: %v", ws)
	}
	b := roundTrip(t, doc)
	st := b.Styles.FindStyleByName("My Style")
	if st == nil || !st.Custom {
		t.Fatalf("custom style not kept: %+v", st)
	}
	if n := b.Styles.Default(model.ParagraphStyle); n == nil || n.Custom {
		t.Errorf("Normal is custom")
	}
	doc.NewHeaderFooter(model.HFDefault, false)
	if ws := Warnings(doc); !slices.Contains(ws, WarnHeadersLost) {
		t.Errorf("no headers warning: %v", ws)
	}
}
