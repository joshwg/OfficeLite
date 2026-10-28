package xlsxfile

import (
	"testing"

	"officelite/excel/model"
)

func TestNotesLinksRoundTrip(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	sh2 := model.NewSheet("Other")
	sh.Set(0, 0, model.Cell{Kind: model.KindText, Text: "Example"})
	sh.Set(1, 0, model.Cell{Kind: model.KindText, Text: "Jump"})
	sh.Set(2, 0, model.Cell{Kind: model.KindText, Text: "Mail"})
	sh.SetLink(0, 0, &model.Hyperlink{Target: "https://example.com/a?b=1", Tooltip: "tip"})
	sh.SetLink(1, 0, &model.Hyperlink{Location: "Other!B2"})
	sh.SetLink(2, 0, &model.Hyperlink{Target: "mailto:x@example.com"})
	sh.SetNote(0, 1, &model.Note{Author: "Josh", Text: "first line\nsecond"})
	sh.SetNote(5, 3, &model.Note{Text: "no author"})
	sh2.SetNote(0, 0, &model.Note{Author: "A", Text: "on other"})
	wb := &model.Workbook{Sheets: []*model.Sheet{sh, sh2}, Styles: []model.Style{model.DefaultStyle()}}
	got, _ := roundTrip(t, wb)
	g := got.Sheets[0]
	for _, k := range [][2]int{{0, 0}, {1, 0}, {2, 0}} {
		if l, _ := g.LinkAt(k[0], k[1]); l != sh.Links[k] {
			t.Errorf("link %v = %+v, want %+v", k, l, sh.Links[k])
		}
	}
	if n, _ := g.NoteAt(0, 1); n != (model.Note{Author: "Josh", Text: "first line\nsecond"}) {
		t.Errorf("note = %+v", n)
	}
	if n, _ := g.NoteAt(5, 3); n.Text != "no author" {
		t.Errorf("note 2 = %+v", n)
	}
	if n, _ := got.Sheets[1].NoteAt(0, 0); n.Text != "on other" || len(got.Sheets[1].Notes) != 1 {
		t.Errorf("other sheet notes = %+v", got.Sheets[1].Notes)
	}
	if g.Get(0, 0).Text != "Example" {
		t.Errorf("cell text = %q", g.Get(0, 0).Text)
	}
}
