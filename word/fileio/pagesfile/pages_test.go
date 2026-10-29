package pagesfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/word/model"
)

func readSample(t *testing.T, name string) *model.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "Samples", name))
	if err != nil {
		t.Skip(err)
	}
	doc, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func dumpBlocks(b *strings.Builder, doc *model.Document, blocks []model.Block, indent string) {
	for _, bl := range blocks {
		switch bl := bl.(type) {
		case *model.Paragraph:
			rp := doc.ResolveRun(bl, &model.Run{Props: bl.Mark})
			list := ""
			if bl.Props.List != nil {
				list = fmt.Sprintf(" list%d/%d", bl.Props.List.NumID, bl.Props.List.Level)
			}
			fmt.Fprintf(b, "%s[%s%s %s %.0f%s] %q\n", indent, bl.Style, list, rp.Font, rp.Size, map[bool]string{true: " B"}[rp.Bold == model.On], bl.Text())
		case *model.Table:
			fmt.Fprintf(b, "%sTABLE %d rows cols %v\n", indent, len(bl.Rows), bl.Columns)
			for _, row := range bl.Rows {
				var cells []string
				for _, c := range row.Cells {
					var sb strings.Builder
					for _, cb := range c.Blocks {
						if p, ok := cb.(*model.Paragraph); ok {
							sb.WriteString(p.Text())
						}
					}
					cells = append(cells, sb.String())
				}
				fmt.Fprintf(b, "%s  | %s\n", indent, strings.Join(cells, " | "))
			}
		}
	}
}

func TestDumpSamples(t *testing.T) {
	if os.Getenv("PAGES_DUMP") == "" {
		t.Skip("set PAGES_DUMP")
	}
	for _, name := range []string{"PagesSample1.pages", "PagesInvoice.pages", "PagesBrochure.pages", "Avery5371BusinessCardsWide.pages"} {
		doc := readSample(t, name)
		var b strings.Builder
		fmt.Fprintf(&b, "%s: page %+v\nwarnings %q\n", name, doc.Page, doc.Warnings)
		dumpBlocks(&b, doc, doc.Body, "  ")
		t.Log(b.String())
	}
}

// paragraphs lists the body's paragraphs (tables skipped).
func paragraphs(doc *model.Document) []*model.Paragraph {
	var out []*model.Paragraph
	for _, b := range doc.Body {
		if p, ok := b.(*model.Paragraph); ok {
			out = append(out, p)
		}
	}
	return out
}

func find(doc *model.Document, text string) *model.Paragraph {
	for _, p := range paragraphs(doc) {
		if p.Text() == text {
			return p
		}
	}
	return nil
}

func TestPagesSample1(t *testing.T) {
	doc := readSample(t, "PagesSample1.pages")
	if doc.Page.Width != 612 || doc.Page.Height != 792 || doc.Page.MarginLeft != 108 {
		t.Errorf("page %+v", doc.Page)
	}
	title := paragraphs(doc)[0]
	if title.Text() != "Sample\nMcSampleFace" {
		t.Fatalf("first paragraph %q", title.Text())
	}
	rp := doc.ResolveRun(title, &title.Runs[0])
	if rp.Size != 60 || rp.Bold != model.On || rp.Font != "Helvetica Neue" || rp.Color != "D8267D" {
		t.Errorf("title run %+v", rp)
	}
	p := find(doc, "Profile")
	if p == nil {
		t.Fatal("no Profile paragraph")
	}
	// Subheading: a hanging indent, the first line at the margin.
	pp := doc.ResolvePara(p)
	if pp.IndentLeft == nil || *pp.IndentLeft != 120 || pp.IndentFirst == nil || *pp.IndentFirst != -120 {
		t.Errorf("Profile indents %v %v", pp.IndentLeft, pp.IndentFirst)
	}
	// The contact details are in a text box, added at the end.
	if find(doc, "M  123.456.7891") == nil {
		t.Error("text box text missing")
	}
	link := find(doc, "sample@Example.com")
	if link == nil || link.Runs[0].Span == nil || link.Runs[0].Span.URL != "mailto:sample@Example.com" {
		t.Errorf("hyperlink paragraph %+v", link)
	}
}

func TestPagesInvoice(t *testing.T) {
	doc := readSample(t, "PagesInvoice.pages")
	if p := paragraphs(doc)[0]; p.Text() != "Company Name" || doc.ResolveRun(p, &p.Runs[0]).Caps != model.On {
		t.Errorf("first paragraph %q", p.Text())
	}
	if p := find(doc, "INVOICE"); p == nil || p.Style != "Heading1" {
		t.Errorf("INVOICE paragraph %+v", p)
	}
	var tbl *model.Table
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tbl = tb
		}
	}
	if tbl == nil {
		t.Fatal("no table")
	}
	if len(tbl.Rows) != 8 || len(tbl.Columns) != 4 || !tbl.Rows[0].Header {
		t.Errorf("table %d rows %d columns", len(tbl.Rows), len(tbl.Columns))
	}
	cell := func(r, c int) string {
		return tbl.Rows[r].Cells[c].Blocks[0].(*model.Paragraph).Text()
	}
	for _, tt := range []struct {
		r, c int
		want string
	}{{0, 0, "Description"}, {1, 3, "$5,500.00"}, {6, 2, "8.25%"}, {7, 3, "$8,573.40"}} {
		if got := cell(tt.r, tt.c); got != tt.want {
			t.Errorf("cell %d,%d = %q, want %q", tt.r, tt.c, got, tt.want)
		}
	}
	if tbl.Rows[0].Cells[0].Shading == "" {
		t.Error("header cells have no fill")
	}
	if find(doc, "Sincerely yours,") == nil {
		t.Error("closing paragraph missing")
	}
}

func TestPagesLayoutDocuments(t *testing.T) {
	doc := readSample(t, "PagesBrochure.pages")
	if !doc.Page.Landscape || doc.Page.Width != 792 {
		t.Errorf("page %+v", doc.Page)
	}
	if find(doc, "TRAVEL IN HAWAII") == nil {
		t.Error("title text box missing")
	}
	pictures := 0
	model.WalkBlocks(doc.Body, func(p *model.Paragraph) bool {
		for _, r := range p.Runs {
			if r.Kind == model.RunOpaque && r.Opaque.Picture != nil {
				pictures++
			}
		}
		return true
	})
	if pictures == 0 {
		t.Error("no pictures")
	}
	doc = readSample(t, "Avery5371BusinessCardsWide.pages")
	n := 0
	for _, p := range paragraphs(doc) {
		if p.Text() == "Place Text Here" {
			n++
		}
	}
	if n != 9 || find(doc, "Sample Card") == nil {
		t.Errorf("%d cards", n)
	}
}

func TestNotPages(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "Samples", "Sample2.numbers"))
	if err != nil {
		t.Skip(err)
	}
	if _, err := Read(data); err == nil || !strings.Contains(err.Error(), "Numbers") {
		t.Errorf("reading a Numbers document: %v", err)
	}
}

func TestPagesFooter(t *testing.T) {
	doc := readSample(t, "PagesInvoice.pages")
	f := doc.HF.Footers[model.HFDefault]
	if f == nil {
		t.Fatal("no footer")
	}
	if !model.HasPageFields(f.Blocks) {
		t.Errorf("footer has no PAGE field: %+v", f.Blocks)
	}
}
