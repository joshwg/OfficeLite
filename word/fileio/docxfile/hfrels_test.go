// hfrels_test.go checks that a header copied from a source part into a new
// part (an unlinked section's, or one restored by undo) takes the source
// part's relationships along, so its pictures and charts stay as they were.

package docxfile

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/opc"
	"officelite/word/model"
)

// hfDrawingDocx is pydocx.docx with two sections, the first with a header
// holding a picture and a chart (whose r:ids point into the header's own
// relationships), the second linked to it.
func hfDrawingDocx(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pydocx.docx"))
	if err != nil {
		t.Skip(err)
	}
	const rels = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	pic := `<w:r><w:drawing><wp:inline xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><wp:extent cx="914400" cy="914400"/><wp:docPr id="1" name="Picture 1"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="image.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdP"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr/></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	chart := `<w:r><w:drawing><wp:inline xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><wp:extent cx="914400" cy="914400"/><wp:docPr id="2" name="Chart 2"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" r:id="rIdC"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	header := `<w:hdr xmlns:w="` + nsW + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><w:p><w:r><w:t>Logo</w:t></w:r>` + pic + chart + `</w:p></w:hdr>`
	add := map[string]string{
		"word/header1.xml": header,
		"word/_rels/header1.xml.rels": rels +
			`<Relationship Id="rIdP" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>` +
			`<Relationship Id="rIdC" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="charts/chart1.xml"/></Relationships>`,
		"word/charts/chart1.xml": `<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"/>`,
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		b := readZipFile(t, f)
		switch f.Name {
		case "word/document.xml":
			s := string(b)
			i := strings.Index(s, "<w:body>") + len("<w:body>")
			j := strings.Index(s, "<w:sectPr")
			s = s[:i] + `<w:p><w:pPr><w:sectPr><w:headerReference w:type="default" r:id="rIdH"/></w:sectPr></w:pPr><w:r><w:t>one</w:t></w:r></w:p>` +
				`<w:p><w:r><w:t>two</w:t></w:r></w:p>` + s[j:]
			b = []byte(s)
		case "word/_rels/document.xml.rels":
			b = []byte(strings.Replace(string(b), "</Relationships>",
				`<Relationship Id="rIdH" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/></Relationships>`, 1))
		case "[Content_Types].xml":
			b = []byte(strings.Replace(string(b), "</Types>",
				`<Override PartName="/word/header1.xml" ContentType="`+ctHeader+`"/></Types>`, 1))
		}
		writeZipFile(t, zw, f.Name, b)
	}
	for name, s := range add {
		writeZipFile(t, zw, name, []byte(s))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestUnlinkedHeaderKeepsDrawings unlinks the second section's header (a
// copy of the first's, written to a new part) and saves: both parts keep
// the picture's and the chart's original XML, with relationships that
// resolve to the same media and chart parts, no new media is added and no
// "[object]" placeholder or lost-content warning appears.
func TestUnlinkedHeaderKeepsDrawings(t *testing.T) {
	doc := mustRead(t, hfDrawingDocx(t))
	if n := len(doc.Sections()); n != 2 {
		t.Fatalf("%d sections, want 2", n)
	}
	if h := doc.UnlinkHeader(1, model.HFDefault, false); h == nil || h.Part == "" {
		t.Fatalf("unlinked header %+v keeps no source part", h)
	}
	doc.Warnings = nil
	out := write(t, doc)
	for _, w := range doc.Warnings {
		if w == warnLostContent {
			t.Errorf("warning given: %q", w)
		}
	}
	parts := zipParts(t, out)
	var headers []string
	for name := range parts {
		if strings.HasPrefix(name, "word/media/") && name != "word/media/image1.png" {
			t.Errorf("picture saved again as %s", name)
		}
		if strings.HasPrefix(name, "word/header") {
			headers = append(headers, name)
		}
	}
	if len(headers) != 2 {
		t.Fatalf("header parts %v, want 2", headers)
	}
	p := &pkg{files: map[string]*zip.File{}}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		p.files[f.Name] = f
	}
	for _, name := range headers {
		x := string(parts[name])
		if strings.Contains(x, "[object]") || strings.Contains(x, "[chart]") || !strings.Contains(x, `r:embed="rIdP"`) || !strings.Contains(x, `r:id="rIdC"`) {
			t.Errorf("%s lost its drawings: %s", name, x)
		}
		targets := map[string]string{}
		for _, rl := range p.rels(name) {
			targets[rl.ID] = opc.Resolve(name, rl.Target)
		}
		if targets["rIdP"] != "word/media/image1.png" || targets["rIdC"] != "word/charts/chart1.xml" {
			t.Errorf("%s relationships resolve to %v", name, targets)
		}
	}
	back := mustRead(t, out)
	secs := back.Sections()
	for i, s := range secs {
		h := s.HF.Headers[model.HFDefault]
		if h == nil {
			t.Fatalf("section %d has no header of its own", i)
		}
		var pics, charts int
		model.WalkBlocks(h.Blocks, func(p *model.Paragraph) bool {
			for _, r := range p.Runs {
				if r.Opaque != nil && r.Opaque.Picture != nil {
					pics++
				}
				if r.Text == "[chart]" {
					charts++
				}
			}
			return true
		})
		if pics != 1 || charts != 1 {
			t.Errorf("section %d header: %d pictures, %d charts; want 1 each", i, pics, charts)
		}
	}
}
