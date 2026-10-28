package layout

import (
	"bytes"
	"image"
	pngenc "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/gui"
	"officelite/common/pdf"
	"officelite/word/fileio"
	"officelite/word/model"
)

func TestExportPDF(t *testing.T) {
	bold := model.Run{Text: "Bold words ", Props: model.RunProps{Bold: model.On}}
	under := model.Run{Text: "underlined", Props: model.RunProps{Underline: model.UnderlineSingle, Highlight: "yellow"}}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var png bytes.Buffer
	if err := pngenc.Encode(&png, img); err != nil {
		t.Fatal(err)
	}
	pic := model.Run{Kind: model.RunOpaque, Text: "[picture]", Opaque: &model.Opaque{Width: 40, Height: 30, Picture: &model.Picture{Data: png.Bytes(), Ext: "png"}}}
	p := &model.Paragraph{Runs: []model.Run{bold, under, pic}}
	tbl := &model.Table{Borders: true, Rows: []*model.TableRow{
		{Cells: []*model.TableCell{{Blocks: []model.Block{para("A1")}}, {Blocks: []model.Block{para("B1")}}}},
	}}
	blocks := []model.Block{p, tbl}
	for range 80 {
		blocks = append(blocks, para(lorem))
	}
	doc := docOf(blocks...)
	h := doc.NewHeaderFooter(model.HFDefault, false)
	h.Blocks[0].(*model.Paragraph).Runs = []model.Run{{Text: "Report"}}
	e := layoutDoc(doc)
	if len(e.Pages) < 2 {
		t.Fatalf("want several pages, got %d", len(e.Pages))
	}
	d := pdf.New()
	d.NoCompress = true
	e.ExportPDF(d)
	if d.Pages() != len(e.Pages) {
		t.Fatalf("%d PDF pages for %d layout pages", d.Pages(), len(e.Pages))
	}
	var out bytes.Buffer
	if err := d.Write(&out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	// Bold text uses the bold Go font; the table's borders and the
	// highlight are filled rectangles in yellow and black.
	for _, want := range []string{"Go-Bold", "GoRegular", "/Subtype /Image", "40 0 0 30 ", "1 1 0 rg", "0 0 0 rg", " TJ", "<0052>"} {
		if !strings.Contains(s, want) {
			t.Errorf("PDF lacks %q", want)
		}
	}
}

// TestExportPDFSamples writes the test documents as PDFs into the folder
// named by PDF_SAMPLES, with the fonts the app would use, for checking
// them by eye.
func TestExportPDFSamples(t *testing.T) {
	dir := os.Getenv("PDF_SAMPLES")
	if dir == "" {
		t.Skip("PDF_SAMPLES not set")
	}
	files, _ := filepath.Glob("../fileio/*/testdata/*.doc*")
	lib := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	for _, path := range files {
		doc, err := fileio.Open(path)
		if err != nil {
			t.Logf("%s: %v", path, err)
			continue
		}
		fams := []string{"Calibri", "Aptos", "Times New Roman", "Arial", "Symbol"}
		doc.Paragraphs(func(p *model.Paragraph) bool {
			for _, r := range p.Runs {
				fams = append(fams, r.Props.Font)
			}
			return true
		})
		for _, st := range doc.Styles.Styles {
			fams = append(fams, st.Run.Font)
		}
		e := New(doc, lib.Shaper(fams...))
		e.Layout()
		d := pdf.New()
		d.Title, d.Producer = doc.Name, "OfficeLite test"
		d.Fonts, d.Fallbacks = lib, gui.FallbackFamilies
		e.ExportPDF(d)
		var out bytes.Buffer
		if err := d.Write(&out); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, "word-"+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+filepath.Ext(path)[1:]+".pdf")
		if err := os.WriteFile(name, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d pages, %d bytes", name, d.Pages(), out.Len())
	}
}
