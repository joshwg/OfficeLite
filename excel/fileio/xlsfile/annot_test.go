package xlsfile

import (
	"bytes"
	"testing"

	"officelite/excel/model"
)

func TestHyperlinksRoundTrip(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	links := map[[2]int]model.Hyperlink{
		{0, 0}: {Target: "https://example.com/a?b=1", Tooltip: "tip"},
		{1, 0}: {Location: "Sheet2!B2"},
		{2, 0}: {Target: "mailto:x@example.com"},
		{3, 0}: {Target: `C:\docs\report.xlsx`, Location: "Data!A1"},
		{4, 1}: {Target: `..\notes.txt`},
		{5, 1}: {Target: `C:\données\日本.txt`},
	}
	for k, h := range links {
		sh.Set(k[0], k[1], model.Cell{Kind: model.KindText, Text: "link"})
		sh.SetLink(k[0], k[1], &h)
	}
	sh.SetNote(0, 3, &model.Note{Author: "Josh", Text: "first\nsecond"})
	sh.SetNote(9, 2, &model.Note{Text: "wide: 日本語 ✓"})
	wb := &model.Workbook{Sheets: []*model.Sheet{sh, model.NewSheet("Sheet2")}, Styles: []model.Style{model.DefaultStyle()}}
	if issues := Fits(wb); len(issues) != 0 {
		t.Errorf("Fits = %q", issues)
	}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	g := got.Sheets[0]
	if len(g.Links) != len(links) {
		t.Errorf("%d links, want %d", len(g.Links), len(links))
	}
	for k, want := range links {
		if l, _ := g.LinkAt(k[0], k[1]); l != want {
			t.Errorf("%v = %+v, want %+v", k, l, want)
		}
	}
	if len(g.Notes) != 2 {
		t.Errorf("notes = %+v", g.Notes)
	}
	for k, want := range sh.Notes {
		if n, _ := g.NoteAt(k[0], k[1]); n != want {
			t.Errorf("note %v = %+v, want %+v", k, n, want)
		}
	}
}
