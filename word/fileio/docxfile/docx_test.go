package docxfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"officelite/word/model"
)

func readFile(t *testing.T, name string) (*model.Document, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Skip(err)
	}
	doc, err := Read(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return doc, data
}

func write(t *testing.T, doc *model.Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func paras(doc *model.Document) []*model.Paragraph {
	var out []*model.Paragraph
	doc.Paragraphs(func(p *model.Paragraph) bool { out = append(out, p); return true })
	return out
}

func TestReadPyDocx(t *testing.T) {
	doc, _ := readFile(t, "pydocx.docx")
	ps := paras(doc)
	var texts []string
	for _, p := range ps {
		texts = append(texts, p.Text())
	}
	all := strings.Join(texts, "|")
	for _, want := range []string{"Fixture Title", "Heading One", "Plain bold and italic red big Arial\ttabbed  two spaces", "r0c0", "before break\fafter break"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %q", want, all)
		}
	}
	if ps[0].Style != "Title" || ps[1].Style != "Heading1" {
		t.Errorf("styles %q %q", ps[0].Style, ps[1].Style)
	}
	p := ps[2]
	if p.Runs[1].Text != "bold" || p.Runs[1].Props.Bold != model.On {
		t.Errorf("bold run %+v", p.Runs[1])
	}
	if r := p.Runs[3]; r.Props.Italic != model.On || r.Props.Color != "FF0000" {
		t.Errorf("italic run %+v", r)
	}
	if r := p.Runs[5]; r.Props.Size != 16 || r.Props.Font != "Arial" {
		t.Errorf("arial run %+v", r)
	}
	if doc.Styles.Find("Heading1") == nil || doc.Styles.Find("ListBullet") == nil {
		t.Error("styles missing")
	}
	// Theme font resolution: python-docx's template uses minorHAnsi = Calibri.
	if doc.Styles.DefaultRun.Font == "" {
		t.Errorf("default font unresolved: %+v", doc.Styles.DefaultRun)
	}
	q := ps[3]
	if q.Props.Align != model.AlignCenter || *q.Props.IndentFirst != -18 || *q.Props.IndentLeft != 36 || *q.Props.SpaceBefore != 6 {
		t.Errorf("quote props %+v", q.Props)
	}
	// list styles reference numbering
	lb := doc.Styles.Find("ListBullet")
	if lb.Para.List == nil || doc.Lists[lb.Para.List.NumID] == nil || doc.Lists[lb.Para.List.NumID].Levels[0].Format != "bullet" {
		t.Errorf("list bullet %+v", lb.Para.List)
	}
	var tbl *model.Table
	var pic *model.Run
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tbl = tb
		}
		if p, ok := b.(*model.Paragraph); ok {
			for i := range p.Runs {
				if p.Runs[i].Kind == model.RunOpaque && p.Runs[i].Text == "[picture]" {
					pic = &p.Runs[i]
				}
			}
		}
	}
	if tbl == nil || len(tbl.Rows) != 3 || tbl.Rows[0].Cells[0].Span != 2 || tbl.Rows[1].Cells[2].VMerge != 1 || tbl.Rows[2].Cells[2].VMerge != 2 {
		t.Errorf("table %+v", tbl)
	} else if !tbl.Borders || tbl.Style != "TableGrid" || len(tbl.Columns) != 3 {
		t.Errorf("table props %+v", tbl)
	}
	if pic == nil || pic.Opaque.Width != 72 {
		t.Errorf("picture %+v", pic)
	}
	if doc.Page.Width != 612 || doc.Page.MarginLeft != 90 {
		t.Errorf("page %+v", doc.Page)
	}
}

// equalDocs compares what a docx → docx round trip must keep.
func equalDocs(t *testing.T, name string, a, b *model.Document) {
	t.Helper()
	if !reflect.DeepEqual(a.Styles, b.Styles) {
		for i := range a.Styles.Styles {
			if i >= len(b.Styles.Styles) || !reflect.DeepEqual(a.Styles.Styles[i], b.Styles.Styles[i]) {
				t.Errorf("%s: style %d differs:\n%+v\n%+v", name, i, a.Styles.Styles[i], at(b.Styles.Styles, i))
				break
			}
		}
		if !reflect.DeepEqual(a.Styles.DefaultRun, b.Styles.DefaultRun) || !reflect.DeepEqual(a.Styles.DefaultPara, b.Styles.DefaultPara) {
			t.Errorf("%s: defaults differ:\n%+v %+v\n%+v %+v", name, a.Styles.DefaultRun, a.Styles.DefaultPara, b.Styles.DefaultRun, b.Styles.DefaultPara)
		}
	}
	if !reflect.DeepEqual(a.Lists, b.Lists) {
		t.Errorf("%s: lists differ", name)
	}
	if a.Page != b.Page {
		t.Errorf("%s: page %+v != %+v", name, a.Page, b.Page)
	}
	if len(a.Body) != len(b.Body) {
		t.Errorf("%s: %d blocks != %d", name, len(a.Body), len(b.Body))
	}
	for i := range min(len(a.Body), len(b.Body)) {
		if !reflect.DeepEqual(a.Body[i], b.Body[i]) {
			t.Errorf("%s: block %d differs:\n%s\n%s", name, i, dump(a.Body[i]), dump(b.Body[i]))
			return
		}
	}
}

func at[T any](s []T, i int) any {
	if i < len(s) {
		return s[i]
	}
	return nil
}

func dump(b model.Block) string {
	switch b := b.(type) {
	case *model.Paragraph:
		s := fmt.Sprintf("P style=%q props=%+v mark=%+v wrap=%v before=%d\n", b.Style, b.Props, b.Mark, b.Wrap != nil, len(b.Before))
		for _, r := range b.Runs {
			s += fmt.Sprintf("  run %d %q style=%q %+v span=%+v", r.Kind, r.Text, r.Style, r.Props, r.Span)
			if r.Opaque != nil {
				s += fmt.Sprintf(" opaque=%s field=%q", r.Opaque.Data, r.Opaque.Field)
			}
			s += "\n"
		}
		return s
	case *model.Table:
		s := fmt.Sprintf("T %+v\n", *b)
		for _, r := range b.Rows {
			s += fmt.Sprintf(" row h=%v hdr=%v x=%q\n", r.Height, r.Header, r.DocxXML)
			for _, c := range r.Cells {
				s += fmt.Sprintf("  cell span=%d vm=%d w=%v shd=%q va=%d x=%q\n", c.Span, c.VMerge, c.Width, c.Shading, c.VAlign, c.DocxXML)
				for _, cb := range c.Blocks {
					s += "   " + dump(cb)
				}
			}
		}
		return s
	}
	return "?"
}

func zipParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

func TestRoundTripFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.docx")
	files2, _ := filepath.Glob("testdata/*.docm")
	files = append(files, files2...)
	if len(files) == 0 {
		t.Skip("no fixtures")
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			a, orig := readFile(t, name)
			out := write(t, a)
			b, err := Read(out)
			if err != nil {
				t.Fatal(err)
			}
			equalDocs(t, name, a, b)
			if dir := os.Getenv("DOCX_OUT"); dir != "" {
				os.WriteFile(filepath.Join(dir, "resaved-"+name), out, 0o644)
				e := mustRead(t, orig)
				editDoc(e)
				os.WriteFile(filepath.Join(dir, "edited-"+name), write(t, e), 0o644)
			}
			// second save is stable
			out2 := write(t, b)
			p1, p2 := zipParts(t, out), zipParts(t, out2)
			for n := range p1 {
				if !bytes.Equal(p1[n], p2[n]) {
					t.Errorf("part %s changed on second save", n)
				}
			}
			// other parts kept byte for byte
			po := zipParts(t, orig)
			for n, data := range po {
				switch {
				case strings.HasSuffix(n, "document.xml"), strings.HasSuffix(n, "styles.xml"), strings.HasSuffix(n, "numbering.xml"):
					continue
				case strings.Contains(n, "vbaProject"), strings.Contains(n, "vbaData"), n == "[Content_Types].xml", strings.HasSuffix(n, "document.xml.rels"):
					continue
				}
				if !bytes.Equal(p1[n], data) {
					t.Errorf("part %s not preserved", n)
				}
			}
		})
	}
}

// editDoc makes typical editor changes: text, styles, formatting, a new
// hyperlink, a new list item and a deleted paragraph.
func editDoc(d *model.Document) {
	ps := paras(d)
	for _, p := range ps {
		for i := range p.Runs {
			if p.Runs[i].Kind == model.RunText && p.Runs[i].Span == nil {
				p.Runs[i].Text = "Edited " + p.Runs[i].Text
				p.Runs[i].Props.Bold = model.On
				p.Runs[i].Props.Color = "00B050"
				p.Style = "Heading2"
				p.Props.Align = model.AlignCenter
				p.Runs = append(p.Runs, model.Run{Text: "new link", Style: "Hyperlink", Span: &model.Span{Kind: model.SpanLink, URL: "https://example.org/new"}})
				goto done
			}
		}
	}
done:
	d.Lists[999] = &model.List{Levels: []model.ListLevel{{Format: "decimal", Text: "%1)", Start: 1, Indent: 36, Hang: 18}}}
	d.Body = append(d.Body, &model.Paragraph{Props: model.ParaProps{List: &model.ListRef{NumID: 999}}, Runs: []model.Run{{Text: "added item"}}})
	if st := d.Styles.Find("Heading2"); st != nil {
		st.Run.Italic = model.On
	}
	d.Page.MarginLeft = 54
}

func TestNewDocument(t *testing.T) {
	doc := model.New("x")
	doc.Lists[1] = &model.List{Levels: []model.ListLevel{{Format: "bullet", Text: "", Start: 1, Indent: 36, Hang: 18, Font: "Symbol"}}}
	link := &model.Span{Kind: model.SpanLink, URL: "https://example.com/a?b=1&c=2"}
	doc.Body = []model.Block{
		&model.Paragraph{Style: "Heading1", Runs: []model.Run{{Text: "Title <&>"}}},
		&model.Paragraph{Runs: []model.Run{
			{Text: " lead space"}, {Kind: model.RunTab}, {Text: "bold", Props: model.RunProps{Bold: model.On, Size: 14, Color: "FF0000", Highlight: "yellow"}},
			{Kind: model.RunBreak}, {Text: "link", Style: "Hyperlink", Span: link},
			{Kind: model.RunOpaque, Text: "picture", Opaque: &model.Opaque{Format: "doc", Data: []byte{1}}},
			{Kind: model.RunOpaque, Text: "3", Opaque: &model.Opaque{Format: "doc", Field: "PAGE"}},
		}},
		&model.Paragraph{Props: model.ParaProps{List: &model.ListRef{NumID: 1}, Align: model.AlignJustify, SpaceAfter: model.F(0)}, Runs: []model.Run{{Text: "item"}}},
		&model.Table{Borders: true, Rows: []*model.TableRow{{Cells: []*model.TableCell{
			{Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: "a"}}}}, Shading: "D9D9D9"},
			{Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: "b"}}}}},
		}}}},
		&model.Paragraph{Runs: []model.Run{{Kind: model.RunPageBreak}, {Text: "end"}}},
	}
	out := write(t, doc)
	if os.Getenv("DOCX_OUT") != "" {
		os.WriteFile(filepath.Join(os.Getenv("DOCX_OUT"), "new.docx"), out, 0o644)
	}
	b, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	ps := paras(b)
	got := []string{}
	for _, p := range ps {
		got = append(got, p.Text())
	}
	want := "Title <&>| lead space\tbold\nlink[picture]￼|item|a|b|\fend"
	if strings.Join(got, "|") != want {
		t.Errorf("text %q\nwant %q", strings.Join(got, "|"), want)
	}
	if ps[0].Style != "Heading1" || b.Styles.Find("Heading1").Run.Size != 16 {
		t.Error("heading style lost")
	}
	r := ps[1].Runs[2]
	if r.Props.Bold != model.On || r.Props.Size != 14 || r.Props.Color != "FF0000" || r.Props.Highlight != "yellow" {
		t.Errorf("props %+v", r.Props)
	}
	if l := ps[1].Runs[4]; l.Span == nil || l.Span.URL != link.URL || l.Style != "Hyperlink" {
		t.Errorf("link %+v", l)
	}
	if f := ps[1].Runs[6]; f.Kind != model.RunOpaque || f.Opaque.Field != "PAGE" || f.Text != "3" {
		t.Errorf("field %+v", f)
	}
	if ps[2].Props.List == nil || b.Lists[1] == nil || b.Lists[1].Levels[0].Font != "Symbol" || ps[2].Props.Align != model.AlignJustify {
		t.Errorf("list %+v %+v", ps[2].Props, b.Lists)
	}
	tbl := b.Body[3].(*model.Table)
	if !tbl.Borders || tbl.Rows[0].Cells[0].Shading != "D9D9D9" {
		t.Errorf("table %+v", tbl)
	}
	if len(doc.Warnings) == 0 {
		t.Error("expected a warning for the .doc picture")
	}
	// a re-save of the new file round-trips
	equalDocs(t, "new", b, mustRead(t, write(t, b)))
}

func mustRead(t *testing.T, data []byte) *model.Document {
	t.Helper()
	d, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestEditAndSave(t *testing.T) {
	doc, _ := readFile(t, "pydocx.docx")
	ps := paras(doc)
	ps[2].Runs[0].Text = "Edited "
	ps[2].Runs[1].Props.Bold = model.Off
	ps[2].Runs[3].Props.Color = "00FF00"
	ps[3].Style = "Heading2"
	ps[3].Props.Align = model.AlignRight
	h := doc.Styles.Find("Heading1")
	h.Run.Size = 20
	out := write(t, doc)
	if os.Getenv("DOCX_OUT") != "" {
		os.WriteFile(filepath.Join(os.Getenv("DOCX_OUT"), "edited.docx"), out, 0o644)
	}
	b := mustRead(t, out)
	qs := paras(b)
	if !strings.HasPrefix(qs[2].Text(), "Edited bold") || qs[2].Runs[1].Props.Bold != model.Off || qs[2].Runs[3].Props.Color != "00FF00" || qs[2].Runs[3].Props.Italic != model.On {
		t.Errorf("edit lost: %s", dump(qs[2]))
	}
	if qs[3].Style != "Heading2" || qs[3].Props.Align != model.AlignRight {
		t.Errorf("para edit lost: %s", dump(qs[3]))
	}
	if b.Styles.Find("Heading1").Run.Size != 20 {
		t.Error("style edit lost")
	}
	// media kept
	parts := zipParts(t, out)
	found := false
	for n := range parts {
		if strings.HasPrefix(n, "word/media/") {
			found = true
		}
	}
	if !found {
		t.Error("media part dropped")
	}
}

func TestMacrosDropped(t *testing.T) {
	_, data := readFile(t, "pydocx.docx")
	// Turn the fixture into a .docm with a fake vbaProject.
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		switch f.Name {
		case "[Content_Types].xml":
			s := strings.Replace(string(b), ctMain, "application/vnd.ms-word.document.macroEnabled.main+xml", 1)
			s = strings.Replace(s, "<Default ", `<Default Extension="bin" ContentType="application/vnd.ms-office.vbaProject"/><Default `, 1)
			b = []byte(s)
		case "word/_rels/document.xml.rels":
			b = []byte(strings.Replace(string(b), "</Relationships>", `<Relationship Id="rIdVba" Type="http://schemas.microsoft.com/office/2006/relationships/vbaProject" Target="vbaProject.bin"/></Relationships>`, 1))
		}
		w, _ := zw.Create(f.Name)
		w.Write(b)
	}
	w, _ := zw.Create("word/vbaProject.bin")
	w.Write([]byte("macro"))
	zw.Close()
	doc, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(doc.Warnings, "\n"), "macros") {
		t.Errorf("no macro warning: %v", doc.Warnings)
	}
	out := write(t, doc)
	parts := zipParts(t, out)
	if _, ok := parts["word/vbaProject.bin"]; ok {
		t.Error("vbaProject kept")
	}
	if s := string(parts["[Content_Types].xml"]); strings.Contains(s, "macroEnabled") || strings.Contains(s, "vbaProject") {
		t.Errorf("content types: %s", s)
	}
	if strings.Contains(string(parts["word/_rels/document.xml.rels"]), "vbaProject") {
		t.Error("vba rel kept")
	}
	if !strings.Contains(strings.Join(doc.Warnings, "\n"), "Macros were removed") {
		t.Error("no removal warning")
	}
}

func TestWithDecls(t *testing.T) {
	ns := map[string]string{"w": nsW, "foo": "urn:foo", "w14": "http://schemas.microsoft.com/office/word/2010/wordml"}
	got := string(withDecls([]byte(`<w:r><foo:x w14:a="1"/></w:r>`), ns))
	if got != `<w:r xmlns:foo="urn:foo"><foo:x w14:a="1"/></w:r>` {
		t.Error(got)
	}
}
