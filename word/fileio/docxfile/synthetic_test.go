package docxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/word/model"
)

// syntheticBody exercises constructs the Word-made fixtures don't: a table
// of contents in a block content control, a field inside a hyperlink, an
// anchor hyperlink, a smart tag, a simple field, markers between blocks,
// elements in an ignorable foreign namespace, a nested table and an empty
// content control.
const syntheticBody = `<w:bookmarkStart w:id="0" w:name="top"/>` +
	`<w:sdt><w:sdtPr><w:docPartObj><w:docPartGallery w:val="Table of Contents"/><w:docPartUnique/></w:docPartObj></w:sdtPr><w:sdtContent>` +
	`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Contents</w:t></w:r></w:p>` +
	`<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> TOC \o "1-3" \h </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>` +
	`<w:hyperlink w:anchor="_Toc1" w:history="1"><w:r><w:t>Intro</w:t></w:r><w:r><w:tab/></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGEREF _Toc1 \h </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:hyperlink></w:p>` +
	`<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
	`</w:sdtContent></w:sdt>` +
	`<x:note xmlns:x="urn:example:x" mc:Ignorable="x">kept</x:note>` +
	`<w:p><w:bookmarkStart w:id="1" w:name="_Toc1"/><w:r><w:t xml:space="preserve">Intro </w:t></w:r><w:bookmarkEnd w:id="1"/>` +
	`<w:smartTag w:uri="urn:schemas-microsoft-com:office:smarttags" w:element="City"><w:r><w:t>Paris</w:t></w:r></w:smartTag>` +
	`<w:fldSimple w:instr=" DATE \@ &quot;yyyy&quot; "><w:r><w:rPr><w:b/></w:rPr><w:t>2026</w:t></w:r></w:fldSimple>` +
	`<w:sdt><w:sdtPr><w:id w:val="5"/></w:sdtPr><w:sdtContent/></w:sdt>` +
	`<w:r><w:rPr><w:b w:val="false"/><w:lang w:val="fr-FR"/></w:rPr><w:t>Bonjour</w:t></w:r></w:p>` +
	`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="4000" w:type="dxa"/></w:tcPr>` +
	`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr><w:tblGrid><w:gridCol w:w="2000"/></w:tblGrid><w:tr><w:tc><w:p><w:r><w:t>inner</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
	`<w:p/></w:tc></w:tr></w:tbl>` +
	`<w:p><w:r><w:t>end</w:t></w:r></w:p><w:bookmarkEnd w:id="0"/>`

func syntheticDocx(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pydocx.docx"))
	if err != nil {
		t.Skip(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "word/document.xml" {
			s := string(b)
			i := strings.Index(s, "<w:body>") + len("<w:body>")
			j := strings.Index(s, "<w:sectPr")
			s = s[:i] + syntheticBody + s[j:]
			if !strings.Contains(s[:strings.Index(s, "<w:body>")], "xmlns:mc=") {
				s = strings.Replace(s, "<w:document ", `<w:document xmlns:mc="`+nsMC+`" `, 1)
			}
			b = []byte(s)
		}
		w, _ := zw.Create(f.Name)
		w.Write(b)
	}
	zw.Close()
	return buf.Bytes()
}

func TestSyntheticConstructs(t *testing.T) {
	data := syntheticDocx(t)
	doc, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	ps := paras(doc)
	if len(ps) < 6 {
		t.Fatalf("paragraphs %d", len(ps))
	}
	// TOC content control wraps the first three paragraphs.
	if ps[0].Wrap == nil || ps[0].Wrap != ps[1].Wrap || ps[1].Wrap != ps[2].Wrap || ps[3].Wrap != nil {
		t.Errorf("toc wrap %p %p %p %p", ps[0].Wrap, ps[1].Wrap, ps[2].Wrap, ps[3].Wrap)
	}
	// Body-level bookmark moved into the first paragraph.
	if ps[0].Runs[0].Kind != model.RunOpaque || !strings.Contains(string(ps[0].Runs[0].Opaque.Data), "top") {
		t.Errorf("leading bookmark: %s", dump(ps[0]))
	}
	// TOC line: result text shown, PAGEREF is one unit inside the link.
	if got := ps[1].Text(); !strings.Contains(got, "Intro\t") {
		t.Errorf("toc text %q", got)
	}
	var pageref *model.Run
	for i := range ps[1].Runs {
		if r := &ps[1].Runs[i]; r.Opaque != nil && strings.HasPrefix(r.Opaque.Field, "PAGEREF") {
			pageref = r
		}
	}
	if pageref == nil || pageref.Text != "1" || pageref.Span == nil || pageref.Span.Anchor != "_Toc1" {
		t.Errorf("pageref %+v\n%s", pageref, dump(ps[1]))
	}
	// Foreign element kept before the next paragraph.
	if len(ps[3].Before) != 1 || !strings.Contains(string(ps[3].Before[0].Data), `xmlns:x="urn:example:x"`) {
		t.Errorf("before %+v", ps[3].Before)
	}
	p := ps[3]
	var simple, bonjour *model.Run
	for i := range p.Runs {
		r := &p.Runs[i]
		if r.Opaque != nil && strings.HasPrefix(r.Opaque.Field, "DATE") {
			simple = r
		}
		if r.Text == "Bonjour" {
			bonjour = r
		}
	}
	if simple == nil || simple.Text != "2026" || simple.Props.Bold != model.On {
		t.Errorf("fldSimple %+v", simple)
	}
	if bonjour == nil || bonjour.Props.Bold != model.Off || !strings.Contains(bonjour.Props.DocxXML, "fr-FR") {
		t.Errorf("bonjour %+v", bonjour)
	}
	if !strings.Contains(p.Text(), "Paris") {
		t.Errorf("smart tag text lost: %q", p.Text())
	}
	tbl, ok := doc.Body[len(doc.Body)-2].(*model.Table)
	if !ok {
		t.Fatalf("no table: %T", doc.Body[len(doc.Body)-2])
	}
	if _, ok := tbl.Rows[0].Cells[0].Blocks[0].(*model.Table); !ok {
		t.Error("nested table lost")
	}
	last := ps[len(ps)-1]
	if r := last.Runs[len(last.Runs)-1]; r.Kind != model.RunOpaque || !strings.Contains(string(r.Opaque.Data), "bookmarkEnd") {
		t.Errorf("trailing bookmark: %s", dump(last))
	}

	// Round trip, then an edit of runs inside the wrappers.
	out := write(t, doc)
	b := mustRead(t, out)
	equalDocs(t, "synthetic", doc, b)
	s := string(zipParts(t, out)["word/document.xml"])
	for _, want := range []string{`docPartGallery w:val="Table of Contents"`, `<x:note`, `w:smartTag`, `DATE \@ &quot;yyyy&quot;`, `<w:lang w:val="fr-FR"/>`, `w:anchor="_Toc1"`} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %s", want)
		}
	}
	bp := paras(b)
	bp[1].Runs[len(bp[1].Runs)-1].Props.Italic = model.On
	bonj := &bp[3].Runs[len(bp[3].Runs)-1]
	bonj.Text = "Salut"
	bonj.Props.Bold = model.On
	out2 := write(t, b)
	c := mustRead(t, out2)
	cp := paras(c)
	if r := cp[3].Runs[len(cp[3].Runs)-1]; r.Text != "Salut" || r.Props.Bold != model.On || !strings.Contains(r.Props.DocxXML, "fr-FR") {
		t.Errorf("edit: %+v", r)
	}
	if dir := os.Getenv("DOCX_OUT"); dir != "" {
		os.WriteFile(filepath.Join(dir, "synthetic.docx"), data, 0o644)
		os.WriteFile(filepath.Join(dir, "resaved-synthetic.docx"), out, 0o644)
		os.WriteFile(filepath.Join(dir, "edited-synthetic.docx"), out2, 0o644)
	}
}
