package xlsxfile

import (
	"archive/zip"
	"reflect"
	"testing"
	"time"

	"officelite/excel"
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
	if n, _ := g.NoteAt(0, 1); n.Author != "Josh" || n.Text != "first line\nsecond" || n.Threaded {
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

// A note without an author keeps none (excelize would make it "Author"),
// and control characters in note text survive as _xHHHH_ escapes instead
// of becoming U+FFFD.
func TestNoteAuthorAndControlChars(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	notes := map[[2]int]model.Note{
		{0, 0}: {Text: "no author"},
		{1, 0}: {Author: "Josh", Text: "a\x01b\tc\x1fd\ne"},
		{2, 0}: {Text: "literal _x0041_ stays"},
	}
	for k, n := range notes {
		sh.SetNote(k[0], k[1], &n)
	}
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	got, _ := roundTrip(t, wb)
	for k, want := range notes {
		if n, _ := got.Sheets[0].NoteAt(k[0], k[1]); !reflect.DeepEqual(n, want) {
			t.Errorf("note %v = %#v, want %#v", k, n, want)
		}
	}
}

// TestHugeLinkRanges checks that hyperlinks over the whole sheet stay
// cheap: the row loop ignored the per-link bound, so each A1:XFD1048576
// hyperlink still walked all 262,144 rows, and thousands of them stalled
// the reader.
func TestHugeLinkRanges(t *testing.T) {
	r := &reader{files: map[string]*zip.File{}, wb: &model.Workbook{}}
	for range 20000 {
		r.links = append(r.links, xLink{ref: "A1:XFD1048576", location: "Sheet1!A1"})
	}
	sh := model.NewSheet("Sheet1")
	start := time.Now()
	r.readAnnotations("xl/worksheets/sheet1.xml", sh)
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
	if len(sh.Links) != excel.MaxLinkCells {
		t.Errorf("%d linked cells, want %d", len(sh.Links), excel.MaxLinkCells)
	}
}
