package layout

import (
	"strings"
	"testing"

	"officelite/word/model"
)

// pageText is the text shown on page pg's body lines.
func pageText(pg *Page) string {
	var b strings.Builder
	for _, pl := range pg.Lines {
		for _, it := range pl.Line.Items {
			b.WriteString(it.Text)
		}
	}
	return b.String()
}

// fieldRun is a field run showing a stale result.
func fieldRun(instr, stale string) model.Run {
	return model.Run{Kind: model.RunOpaque, Text: stale, Opaque: &model.Opaque{Field: instr}}
}

// TestBodyPageFields checks that PAGE and NUMPAGES fields in the body show
// the page they're on and the page count, not their stored result, and
// follow the text when it moves to another page.
func TestBodyPageFields(t *testing.T) {
	var body []model.Block
	for i := range 3 {
		p := &model.Paragraph{Runs: []model.Run{{Text: "Page "}, fieldRun("PAGE", "9"), {Text: " of "},
			fieldRun(`NUMPAGES \* roman`, "9")}}
		if i < 2 {
			p.Runs = append(p.Runs, model.Run{Kind: model.RunPageBreak})
		}
		body = append(body, p)
	}
	d := docOf(body...)
	e := layoutDoc(d)
	if len(e.Pages) != 3 {
		t.Fatalf("%d pages", len(e.Pages))
	}
	for i, pg := range e.Pages {
		want := "Page " + itoa(i+1) + " of iii"
		if got := pageText(pg); !strings.HasPrefix(got, want) {
			t.Errorf("page %d shows %q, want %q", i+1, got, want)
		}
	}
	// The stored results are left alone: only an update writes them.
	if got := body[0].(*model.Paragraph).Runs[1].Text; got != "9" {
		t.Errorf("stored result changed to %q", got)
	}
	// A new first page moves every field one page on.
	d.Body = append([]model.Block{&model.Paragraph{Runs: []model.Run{{Text: "cover"}, {Kind: model.RunPageBreak}}}}, d.Body...)
	e.Invalidate(nil, true)
	e.Layout()
	if got := pageText(e.Pages[3]); !strings.HasPrefix(got, "Page 4 of iv") {
		t.Errorf("after a new first page, page 4 shows %q", got)
	}
	// Steady state: laying out again finds nothing to change.
	e.Layout()
	if e.LastRelaid != 0 {
		t.Errorf("relaid %d paragraphs with nothing changed", e.LastRelaid)
	}
}

// TestSectionPageFields checks SECTIONPAGES (the section's page count, not
// the document's) and SECTION in a two-section document.
func TestSectionPageFields(t *testing.T) {
	first := &model.Paragraph{Runs: []model.Run{{Text: "a"}, {Kind: model.RunPageBreak}, {Text: "b"}}}
	first.Sect = &model.Section{Page: model.LetterPage()}
	var body []model.Block
	body = append(body, first)
	for i := range 3 {
		p := para("s2")
		if i < 2 {
			p.Runs = append(p.Runs, model.Run{Kind: model.RunPageBreak})
		}
		body = append(body, p)
	}
	d := docOf(body...)
	e := layoutDoc(d)
	if len(e.Pages) != 5 {
		t.Fatalf("%d pages", len(e.Pages))
	}
	for _, c := range []struct {
		page        int
		instr, want string
	}{
		{0, "SECTIONPAGES", "2"}, {1, "SECTIONPAGES", "2"}, {2, "SECTIONPAGES", "3"},
		{4, "SECTIONPAGES", "3"}, {0, "NUMPAGES", "5"}, {0, "SECTION", "1"}, {3, "SECTION", "2"},
		{3, `SECTIONPAGES \* ALPHABETIC`, "C"},
	} {
		if got, ok := e.PageFieldValue(c.page, c.instr); !ok || got != c.want {
			t.Errorf("page %d %s = %q, %v; want %q", c.page+1, c.instr, got, ok, c.want)
		}
	}
	if _, ok := e.PageFieldValue(0, "DATE"); ok {
		t.Error("DATE is not a page field")
	}
	// The section page counts are cached: a page more in the second
	// section must show after laying out again.
	last := body[len(body)-1].(*model.Paragraph)
	last.Runs = append(last.Runs, model.Run{Kind: model.RunPageBreak}, model.Run{Text: "more"})
	e.Invalidate([]*model.Paragraph{last}, false)
	e.Layout()
	if got, _ := e.PageFieldValue(2, "SECTIONPAGES"); got != "4" {
		t.Errorf("after adding a page, SECTIONPAGES = %q; want 4", got)
	}
	if got, _ := e.PageFieldValue(0, "SECTIONPAGES"); got != "2" {
		t.Errorf("after adding a page, first section's SECTIONPAGES = %q; want 2", got)
	}
}
