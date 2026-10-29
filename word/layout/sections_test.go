package layout

import (
	"strings"
	"testing"

	"officelite/word/model"
)

// A document with a portrait section, a landscape one with two columns
// and a header only in the first (linked into the second).
func TestSectionsPaginate(t *testing.T) {
	first := para("Portrait")
	first.Sect = &model.Section{Page: model.LetterPage()}
	head := &model.HeaderFooter{Blocks: []model.Block{para("Head")}}
	first.Sect.HF.Headers[model.HFDefault] = head
	var body []model.Block
	body = append(body, first)
	for range 40 {
		body = append(body, para(lorem))
	}
	d := docOf(body...)
	d.Page = model.PageSetup{Width: 792, Height: 612, MarginTop: 36, MarginBottom: 36, MarginLeft: 72, MarginRight: 72, Header: 18, Footer: 18, Landscape: true}
	d.Columns = model.Columns{Count: 2, Space: 36}
	d.PageNumRestart, d.PageNumStart, d.PageNumFormat = true, 5, "lowerRoman"
	e := layoutDoc(d)
	if len(e.Pages) < 3 {
		t.Fatalf("%d pages", len(e.Pages))
	}
	p0, p1 := e.Pages[0], e.Pages[1]
	if p0.Width != 612 || p1.Width != 792 || p1.Height != 612 || !p1.First || p1.Section != 1 || p1.Number != 5 {
		t.Errorf("pages = %+v / %+v", *p0, *p1)
	}
	if e.Width != 792 || p0.X != 90 || p1.X != 0 {
		t.Errorf("view: width %v, x %v %v", e.Width, p0.X, p1.X)
	}
	if len(p0.Lines) != 1 {
		t.Errorf("the next-page section break leaves %d lines on page 1", len(p0.Lines))
	}
	// Both columns of the landscape page are used.
	xs := map[float64]bool{}
	for _, pl := range p1.Lines {
		xs[pl.X] = true
	}
	if !xs[72] || !xs[72+(648-36)/2+36] {
		t.Errorf("column lefts = %v", xs)
	}
	if p1.Header == nil || bandText(p1.Header) != "Head" {
		t.Errorf("linked header on page 2: %+v", p1.Header)
	}
	if v, _ := e.PageFieldValue(1, "PAGE"); v != "v" {
		t.Errorf("PAGE on page 2 = %q", v)
	}
	// Show/Hide ¶ shows the break with the next section's kind.
	ms := e.Marks(p0.Lines, nil)
	if m := ms[len(ms)-1]; m.Kind != MarkSection || m.Break != model.BreakNextPage || m.W <= 0 {
		t.Errorf("section mark = %+v", m)
	}
}

func TestContinuousSection(t *testing.T) {
	first := para("One column")
	first.Sect = &model.Section{Page: model.LetterPage()}
	d := docOf(first, para("Two columns"))
	d.Break, d.Columns = model.BreakContinuous, model.Columns{Count: 2}
	e := layoutDoc(d)
	if len(e.Pages) != 1 || len(e.Pages[0].Lines) != 2 || e.Pages[0].Lines[1].Y <= e.Pages[0].Lines[0].Y {
		t.Fatalf("continuous break: %d pages", len(e.Pages))
	}
}

func TestFootnotesAndEndnotes(t *testing.T) {
	d := docOf()
	var body []model.Block
	for i := range 60 {
		p := para("Paragraph " + itoa(i) + " ")
		if i == 30 || i == 31 {
			n := model.NewNote(&d.Styles, false)
			n.Blocks[0].(*model.Paragraph).Runs = append(n.Blocks[0].(*model.Paragraph).Runs, model.Run{Text: "Note " + itoa(i)})
			p.Runs = append(p.Runs, model.Run{Kind: model.RunOpaque, Opaque: &model.Opaque{Note: n}})
		}
		if i == 2 {
			n := model.NewNote(&d.Styles, true)
			n.Blocks[0].(*model.Paragraph).Runs = append(n.Blocks[0].(*model.Paragraph).Runs, model.Run{Text: "The end note"})
			p.Runs = append(p.Runs, model.Run{Kind: model.RunOpaque, Opaque: &model.Opaque{Note: n}})
		}
		body = append(body, p)
	}
	d.Body = body
	e := layoutDoc(d)
	// The footnotes go at the bottom of the page whose lines refer to
	// them, under the separator; the text ends above them.
	var foot *Band
	var page *Page
	for _, pg := range e.Pages {
		for _, pl := range pg.Lines {
			if strings.HasPrefix(pl.Box.P.Text(), "Paragraph 30 ") {
				page = pg
			}
		}
	}
	if page == nil || len(page.Notes) != 1 {
		t.Fatalf("footnotes on the page: %+v", page)
	}
	foot = page.Notes[0]
	if !strings.Contains(bandText(foot), "Note 30") || len(foot.Rules) != 1 || foot.Rules[0].X1-foot.Rules[0].X0 != separatorWidth {
		t.Errorf("footnote area = %q %+v", bandText(foot), foot.Rules)
	}
	for _, pl := range page.Lines {
		if pl.Y+pl.Line.Height > foot.Top+0.01 {
			t.Errorf("body line at %v runs into the footnotes at %v", pl.Y, foot.Top)
		}
	}
	if foot.Bottom > page.BodyBottom+0.01 {
		t.Errorf("footnotes end at %v, below the text area %v", foot.Bottom, page.BodyBottom)
	}
	// Footnote marks are numbered 1, 2; the endnote i, after the text.
	if refs := model.NoteRefs(d.Body); refs[0].Para.Runs[refs[0].Run].Text != "i" || refs[1].Para.Runs[refs[1].Run].Text != "1" ||
		refs[2].Para.Runs[refs[2].Run].Text != "2" {
		t.Errorf("marks: %+v", refs)
	}
	last := e.Pages[len(e.Pages)-1]
	end := last.Notes[len(last.Notes)-1]
	if !strings.Contains(bandText(end), "The end note") {
		t.Errorf("endnotes = %q", bandText(end))
	}
	if l := last.Lines[len(last.Lines)-1]; end.Top < l.Y+l.Line.Height-0.01 {
		t.Errorf("endnotes at %v before the text's end %v", end.Top, l.Y+l.Line.Height)
	}
}

func TestFloatingTextBox(t *testing.T) {
	box := &model.Floating{X: 100, Y: 20, W: 120, H: 60, RelX: model.RelColumn, RelY: model.RelParagraph,
		Line: "000000", LineWidth: 0.5, Fill: "FFFFFF", Inset: model.DefaultInset, Blocks: []model.Block{para("Inside box")}}
	anchor := para("Anchor ")
	anchor.Runs = append(anchor.Runs, model.Run{Kind: model.RunOpaque, Opaque: &model.Opaque{Float: box}})
	band := &model.Floating{Y: 300, W: 468, H: 100, RelY: model.RelPage, Wrap: model.WrapTopBottom, Shape: "ellipse", Fill: "FF0000"}
	anchor2 := para("")
	anchor2.Runs = []model.Run{{Kind: model.RunOpaque, Opaque: &model.Opaque{Float: band}}}
	body := []model.Block{anchor, anchor2}
	for range 30 {
		body = append(body, para("Line"))
	}
	d := docOf(body...)
	e := layoutDoc(d)
	pg := e.Pages[0]
	if len(pg.Floats) != 2 {
		t.Fatalf("%d floats", len(pg.Floats))
	}
	b := pg.Floats[0]
	top := pg.Lines[0].Y
	if b.R != (Rect{72 + 100, top + 20, 72 + 220, top + 80}) {
		t.Errorf("box = %+v (paragraph top %v)", b.R, top)
	}
	if len(b.Lines) != 1 || b.Lines[0].X != b.R.X0+7.2 || b.Lines[0].Y != b.R.Y0+3.6 {
		t.Errorf("text box text at %+v", b.Lines)
	}
	// Text stays clear of the object wrapped top and bottom.
	for _, pl := range pg.Lines {
		if pl.Y+pl.Line.Height > 300 && pl.Y < 400 {
			t.Errorf("line at %v overlaps the wrapped shape", pl.Y)
		}
	}
	// The anchor takes no room in its line.
	items := pg.Lines[0].Line.Items
	if it := items[len(items)-1]; it.Kind != ItemObject || it.W != 0 {
		t.Errorf("anchor item = %+v", it)
	}
}

func TestNestedTableCell(t *testing.T) {
	inner := &model.Table{Columns: []float64{100, 100}, Rows: []*model.TableRow{{Cells: []*model.TableCell{
		{Blocks: []model.Block{para("n1")}}, {Blocks: []model.Block{para("n4")}}}}}}
	outer := &model.Table{Columns: []float64{200, 220}, Rows: []*model.TableRow{{Cells: []*model.TableCell{
		{Blocks: []model.Block{para("Tall")}}, {Blocks: []model.Block{inner, para("")}}}}}}
	d := docOf(outer, para("after"))
	e := layoutDoc(d)
	var n1, n4, empty *Placed
	for i := range e.Pages[0].Lines {
		pl := &e.Pages[0].Lines[i]
		switch pl.Box.P.Text() {
		case "n1":
			n1 = pl
		case "n4":
			n4 = pl
		case "":
			empty = pl
		}
	}
	if n1 == nil || n4 == nil || n1.Y != n4.Y || n4.X <= n1.X {
		t.Fatalf("nested cells side by side: %+v %+v", n1, n4)
	}
	// The cell's paragraph after the nested table takes no room: the row
	// is as high as without it.
	if empty == nil || !empty.collapsed || len(e.Marks([]Placed{*empty}, nil)) != 0 {
		t.Errorf("paragraph after the nested table: %+v", empty)
	}
	h := e.Pages[0].TableRows[0].Y1 - e.Pages[0].TableRows[0].Y0
	outer.Rows[0].Cells[1].Blocks = []model.Block{inner}
	e = layoutDoc(d)
	if h2 := e.Pages[0].TableRows[0].Y1 - e.Pages[0].TableRows[0].Y0; h != h2 {
		t.Errorf("row height %v, %v without the paragraph", h, h2)
	}
}

// A next-column section break moves to the next column of the same page
// with the new section's setup, and to a new page from the last column.
func TestSectionNextColumn(t *testing.T) {
	setup := model.PageSetup{Width: 612, Height: 792, MarginTop: 72, MarginBottom: 72, MarginLeft: 72, MarginRight: 72}
	build := func(cols int) *Engine {
		first := para("One")
		first.Sect = &model.Section{Page: setup, Columns: model.Columns{Count: cols, Space: 36}}
		d := docOf(first, para("Two"))
		d.Page, d.Columns, d.Break = setup, model.Columns{Count: cols, Space: 36}, model.BreakNextColumn
		return layoutDoc(d)
	}
	e := build(2)
	if len(e.Pages) != 1 || len(e.Pages[0].Lines) != 2 || e.Pages[0].Lines[1].X <= e.Pages[0].Lines[0].X {
		t.Errorf("two columns: %d pages, lines %+v", len(e.Pages), e.Pages[0].Lines)
	}
	if e := build(1); len(e.Pages) != 2 {
		t.Errorf("one column: %d pages, want the break to start a new page", len(e.Pages))
	}
}
