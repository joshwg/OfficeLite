package layout

import (
	"testing"

	"officelite/word/model"
)

func bandText(b *Band) string {
	s := ""
	for _, pl := range b.Lines {
		for _, it := range pl.Line.Items {
			s += it.Text
		}
	}
	return s
}

func TestHeadersFootersOnPages(t *testing.T) {
	var blocks []model.Block
	for range 60 {
		blocks = append(blocks, para(lorem))
	}
	d := docOf(blocks...)
	h := d.NewHeaderFooter(model.HFDefault, false)
	h.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Report"}}
	f := d.NewHeaderFooter(model.HFDefault, true)
	f.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Page "},
		{Kind: model.RunOpaque, Text: "9", Opaque: &model.Opaque{Field: "PAGE"}},
		{Text: " of "},
		{Kind: model.RunOpaque, Text: "9", Opaque: &model.Opaque{Field: `NUMPAGES \* roman`}}}
	e := layoutDoc(d)
	n := len(e.Pages)
	if n < 3 {
		t.Fatalf("want several pages, got %d", n)
	}
	for i, pg := range e.Pages {
		if pg.Header == nil || pg.Footer == nil {
			t.Fatalf("page %d lacks header/footer", i)
		}
		if got := bandText(pg.Header); got != "Report" {
			t.Errorf("page %d header %q", i, got)
		}
		want := "Page " + itoa(i+1) + " of " + lowerRoman(n)
		if got := bandText(pg.Footer); got != want {
			t.Errorf("page %d footer %q, want %q", i, got, want)
		}
		if pg.Header.Top != d.Page.Header || pg.Footer.Bottom > pg.Height-d.Page.Footer+0.01 {
			t.Errorf("band positions %v %v", pg.Header.Top, pg.Footer.Bottom)
		}
		for _, pl := range pg.Lines {
			if pl.Y < pg.BodyTop-0.01 || pl.Y+pl.Line.Height > pg.BodyBottom+0.01 {
				t.Errorf("body line outside the body area on page %d", i)
			}
		}
	}
	// Different first page: page 1 has no header (none defined).
	d.HF.TitlePage = true
	e.Invalidate(nil, true)
	e.Layout()
	if e.Pages[0].Header != nil || e.Pages[1].Header == nil {
		t.Errorf("first page header with titlePg")
	}
	// A tall header pushes the body down.
	for range 8 {
		h.Blocks = append(h.Blocks, para("line"))
	}
	e.InvalidateHF()
	e.Layout()
	if pg := e.Pages[1]; pg.BodyTop <= d.Page.MarginTop || pg.Lines[0].Y < pg.Header.Bottom {
		t.Errorf("tall header: body top %v, header bottom %v", pg.BodyTop, pg.Header.Bottom)
	}
	if footer, ok := e.InHFArea(1, 10); !ok || footer {
		t.Errorf("header area hit")
	}
	if footer, ok := e.InHFArea(1, e.Pages[1].Height-5); !ok || !footer {
		t.Errorf("footer area hit")
	}
	// Editing view: the footer of page 2 in page-view coordinates.
	sub := d.SubDocument(f.Blocks)
	ev := e.EditView(1, true, sub)
	c, ok := ev.CaretAt(model.Pos{Para: 0, Off: 2})
	if !ok || c.Page != 1 || c.Y < e.Pages[1].BodyBottom {
		t.Errorf("edit view caret %+v %v", c, ok)
	}
	pg := e.Pages[1]
	if p := ev.HitView(c.X+1, pg.Top+c.Y+c.H/2); p.Para != 0 || p.Off != 2 {
		t.Errorf("edit view hit %v", p)
	}
}

func lowerRoman(n int) string {
	v, _ := FieldValue(`NUMPAGES \* roman`, 1, n)
	return v
}

func itoa(n int) string {
	v, _ := FieldValue("PAGE", n, n)
	return v
}

func TestFieldValue(t *testing.T) {
	for _, c := range []struct {
		instr string
		want  string
	}{{"PAGE", "7"}, {` PAGE  \* MERGEFORMAT `, "7"}, {`PAGE \* Roman`, "VII"}, {`PAGE \* alphabetic`, "g"}, {"NUMPAGES", "12"}} {
		if got, _ := FieldValue(c.instr, 7, 12); got != c.want {
			t.Errorf("%q = %q, want %q", c.instr, got, c.want)
		}
	}
	if _, ok := FieldValue("DATE", 1, 1); ok {
		t.Errorf("DATE computed")
	}
}
