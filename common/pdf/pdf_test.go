// Tests of the PDF writer: file structure and xref, embedded and subsetted
// fonts, license fallbacks, text placement and images.

package pdf

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

// dirSource is a FontSource mapping lowercased family names to font file
// paths.
type dirSource map[string][]string

// FontFiles returns the files listed for family.
func (s dirSource) FontFiles(family string) []string { return s[strings.ToLower(family)] }

// parsed is a PDF split into its objects, checked against the xref table.
type parsed struct {
	objs    map[int][]byte // object body (between "obj" and "endobj")
	streams map[int][]byte // decoded stream data
	trailer string
}

var objRE = regexp.MustCompile(`(?s)^(\d+) 0 obj\n`)

// parsePDF checks the file's structure: header, startxref, every xref
// offset pointing at its object, stream lengths and Flate data.
func parsePDF(t *testing.T, b []byte) *parsed {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("%PDF-1.7\n")) {
		t.Fatalf("bad header %q", b[:min(len(b), 10)])
	}
	if !bytes.HasSuffix(b, []byte("%%EOF\n")) {
		t.Fatal("missing EOF marker")
	}
	i := bytes.LastIndex(b, []byte("startxref\n"))
	if i < 0 {
		t.Fatal("no startxref")
	}
	xref, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(string(b[i+10:]), "\n", 2)[0]))
	if err != nil || xref >= len(b) || !bytes.HasPrefix(b[xref:], []byte("xref\n")) {
		t.Fatalf("startxref %d doesn't point at the xref table", xref)
	}
	lines := strings.Split(string(b[xref:]), "\n")
	var first, count int
	if _, err := fmtSscan(lines[1], &first, &count); err != nil || first != 0 {
		t.Fatalf("bad xref subsection %q", lines[1])
	}
	p := &parsed{objs: map[int][]byte{}, streams: map[int][]byte{}}
	for n := 1; n < count; n++ {
		e := lines[2+n]
		if len(e) != 19 || !strings.HasSuffix(e, "\r") {
			t.Fatalf("xref entry %d is %q, not 20 bytes", n, e)
		}
		if e[17] != 'n' {
			continue
		}
		off, _ := strconv.Atoi(e[:10])
		m := objRE.FindSubmatch(b[off:min(off+20, len(b))])
		if m == nil || string(m[1]) != strconv.Itoa(n) {
			t.Fatalf("xref offset %d of object %d points at %q", off, n, b[off:min(off+20, len(b))])
		}
		body := b[off+len(m[0]):]
		end := bytes.Index(body, []byte("\nendobj\n"))
		if end < 0 {
			t.Fatalf("object %d has no endobj", n)
		}
		if si := bytes.Index(body, []byte(">>\nstream\n")); si >= 0 && si < end {
			p.readStream(t, n, body)
			end = bytes.Index(body, []byte("\nendstream\nendobj\n")) + len("\nendstream")
		}
		p.objs[n] = body[:end]
	}
	ti := bytes.Index(b[xref:], []byte("trailer"))
	p.trailer = string(b[xref+ti:])
	if !strings.Contains(p.trailer, "/Size "+strconv.Itoa(count)) {
		t.Fatalf("trailer %q doesn't match %d xref entries", p.trailer, count)
	}
	return p
}

// fmtSscan parses s as exactly two space-separated integers into a and b.
func fmtSscan(s string, a, b *int) (int, error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, io.ErrUnexpectedEOF
	}
	var err error
	if *a, err = strconv.Atoi(f[0]); err != nil {
		return 0, err
	}
	*b, err = strconv.Atoi(f[1])
	return 2, err
}

var lengthRE = regexp.MustCompile(`/Length (\d+)`)

// readStream checks stream object n's /Length against its endstream and
// stores its data, Flate-decoded when filtered.
func (p *parsed) readStream(t *testing.T, n int, body []byte) {
	t.Helper()
	si := bytes.Index(body, []byte(">>\nstream\n"))
	dict := body[:si]
	m := lengthRE.FindSubmatch(dict)
	if m == nil {
		t.Fatalf("stream %d has no /Length", n)
	}
	l, _ := strconv.Atoi(string(m[1]))
	data := body[si+len(">>\nstream\n"):]
	if len(data) < l || !bytes.HasPrefix(data[l:], []byte("\nendstream\nendobj\n")) {
		t.Fatalf("stream %d: /Length %d doesn't end at endstream", n, l)
	}
	data = data[:l]
	if bytes.Contains(dict, []byte("/FlateDecode")) {
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("stream %d: %v", n, err)
		}
		if data, err = io.ReadAll(zr); err != nil {
			t.Fatalf("stream %d: %v", n, err)
		}
	}
	p.streams[n] = data
}

// find returns the objects whose body contains s.
func (p *parsed) find(s string) []int {
	var out []int
	for n, b := range p.objs {
		if bytes.Contains(b, []byte(s)) {
			out = append(out, n)
		}
	}
	return out
}

// refOf returns the object number of the reference following key in body.
func refOf(t *testing.T, body []byte, key string) int {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*(\d+) 0 R`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("no %s in %q", key, body)
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

// samplePDF draws two pages on d (shapes, translucency, clipped and styled
// text, a PNG and a JPEG) and returns the written file.
func samplePDF(t *testing.T, d *Document) []byte {
	t.Helper()
	pg := d.AddPage(Letter)
	pg.FillRect(72, 72, 200, 50, color.NRGBA{R: 0xFF, G: 0xEE, A: 255})
	pg.StrokeRect(72, 72, 200, 50, 1, color.NRGBA{A: 255})
	pg.Line(72, 140, 300, 140, 0.5, color.NRGBA{B: 255, A: 255}, 2, 1)
	pg.FillRect(300, 72, 50, 50, color.NRGBA{R: 255, A: 128})
	f := d.Font(FontSpec{Family: "Arial"})
	pg.Save()
	pg.ClipRect(72, 160, 200, 30)
	pg.DrawText(Run{Font: f, Size: 12, X: 76, Y: 180, Text: "Hello, PDF — Ünïcödé", Color: color.NRGBA{A: 255}})
	pg.Restore()
	b := d.Font(FontSpec{Family: "Arial", Bold: true, Italic: true})
	pg.DrawText(Run{Font: b, Size: 18, X: 72, Y: 220, Text: "Bold italic", Color: color.NRGBA{R: 0xC0, A: 255}, Adv: []float32{12, 10, 5, 10, 5, 4, 5, 5, 10, 10, 5}})

	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	var pngBuf, jpgBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	jpeg.Encode(&jpgBuf, image.NewGray(image.Rect(0, 0, 8, 8)), nil)
	pi, err := d.AddImageData(pngBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ji, err := d.AddImageData(jpgBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	pg2 := d.AddPage(A4)
	pg2.DrawImage(pi, 72, 72, 100, 100)
	pg2.DrawImage(ji, 200, 72, 100, 100)
	var out bytes.Buffer
	if err := d.Write(&out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestStructure checks the sample PDF's structure, info dictionary,
// images, ExtGState and fonts; with PDF_DUMP the PDF is kept for viewing.
func TestStructure(t *testing.T) {
	d := New()
	d.Title = "Test – document"
	d.Producer = "OfficeLite test"
	out := samplePDF(t, d)
	p := parsePDF(t, out)
	if len(p.find("/Type /Page /Parent")) != 2 {
		t.Fatalf("want 2 pages")
	}
	if !strings.Contains(string(p.objs[3]), "/Producer (OfficeLite test)") || !strings.Contains(string(p.objs[3]), "/Title <FEFF") {
		t.Fatalf("info dict %q", p.objs[3])
	}
	if len(p.find("/Filter /DCTDecode")) != 1 || len(p.find("/SMask")) != 1 {
		t.Fatal("want a DCT image and a PNG with a soft mask")
	}
	if len(p.find("/ca 0.502")) != 1 {
		t.Fatal("want an ExtGState for the translucent fill")
	}
	checkFonts(t, p, "Hello")
	if dump := os.Getenv("PDF_DUMP"); dump != "" {
		os.WriteFile(filepath.Join(dump, "pdftest.pdf"), out, 0o644)
	}
}

// checkFonts verifies every Type0 font: the subset parses as a font with
// outlines for its glyphs, and the ToUnicode map gives back want's
// characters.
func checkFonts(t *testing.T, p *parsed, want string) {
	t.Helper()
	fonts := p.find("/Subtype /Type0")
	if len(fonts) == 0 {
		t.Fatal("no Type0 fonts")
	}
	var cmaps strings.Builder
	for _, n := range fonts {
		body := p.objs[n]
		cid := refOf(t, body, "/DescendantFonts [")
		cmaps.Write(p.streams[refOf(t, body, "/ToUnicode")])
		if !bytes.Contains(body, []byte("/Encoding /Identity-H")) {
			t.Fatal("font without Identity-H")
		}
		desc := refOf(t, p.objs[cid], "/FontDescriptor")
		if !bytes.Contains(p.objs[cid], []byte("/W [")) {
			t.Fatal("CIDFont without widths")
		}
		if bytes.Contains(p.objs[desc], []byte("/FontFile3")) {
			continue
		}
		prog := p.streams[refOf(t, p.objs[desc], "/FontFile2")]
		fnt, err := sfnt.Parse(prog)
		if err != nil {
			t.Fatalf("subset font %d doesn't parse: %v", n, err)
		}
		// Glyphs listed in /W must have outlines (except spaces).
		var buf sfnt.Buffer
		w := regexp.MustCompile(`(\d+) \[`).FindAllSubmatch(p.objs[cid], -1)
		for _, m := range w {
			g, _ := strconv.Atoi(string(m[1]))
			if _, err := fnt.LoadGlyph(&buf, sfnt.GlyphIndex(g), 1000, nil); err != nil {
				t.Fatalf("glyph %d of subset %d: %v", g, n, err)
			}
		}
	}
	for _, r := range want {
		hex := strings.ToUpper(strconv.FormatInt(int64(r), 16))
		for len(hex) < 4 {
			hex = "0" + hex
		}
		if !strings.Contains(cmaps.String(), "<"+hex+">") {
			t.Fatalf("ToUnicode maps lack %q", r)
		}
	}
}

// TestSubsetIsSmall checks that Fonts/arial.ttf is embedded as a tagged
// subset well under the font's size.
func TestSubsetIsSmall(t *testing.T) {
	arial := filepath.Join("..", "..", "Fonts", "arial.ttf")
	if _, err := os.Stat(arial); err != nil {
		t.Skip("no Fonts/arial.ttf")
	}
	d := New()
	d.Fonts = dirSource{"arial": {arial}}
	pg := d.AddPage(Letter)
	f := d.Font(FontSpec{Family: "Arial"})
	if f.Embedded() != "Arial" {
		t.Fatalf("embedded %q", f.Embedded())
	}
	pg.DrawText(Run{Font: f, Size: 11, X: 72, Y: 72, Text: "Subset me", Color: color.NRGBA{A: 255}})
	var out bytes.Buffer
	if err := d.Write(&out); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(arial)
	if out.Len() > int(st.Size())/4 {
		t.Fatalf("PDF is %d bytes for a %d byte font: not subsetted", out.Len(), st.Size())
	}
	p := parsePDF(t, out.Bytes())
	checkFonts(t, p, "Subset me")
	if len(p.find("+ArialMT")) == 0 {
		t.Fatal("no subset-tagged ArialMT")
	}
}

// TestRestrictedFontFallsBack checks that a font whose license forbids
// embedding is skipped for an embeddable fallback.
func TestRestrictedFontFallsBack(t *testing.T) {
	// A copy of Go Regular marked "restricted license" must not be
	// embedded; the Go fallback takes its place.
	fs, _ := parseFaces(goFaces()[[2]bool{}].tables["head"]) // not a font
	if fs != nil {
		t.Fatal("parsed a table as a font")
	}
	b := sfntFile(goFaces()[[2]bool{}])
	fc, err := parseFaces(b)
	if err != nil {
		t.Fatal(err)
	}
	os2 := fc[0].tables["OS/2"]
	if len(os2) < 10 {
		t.Skip("no OS/2")
	}
	// Patch fsType in the file.
	i := bytes.Index(b, os2)
	b[i+8], b[i+9] = 0, 2
	dir := t.TempDir()
	path := filepath.Join(dir, "restricted.ttf")
	os.WriteFile(path, b, 0o644)
	d := New()
	d.Fonts = dirSource{"go": {path}}
	f := d.Font(FontSpec{Family: "Go"})
	if !f.chain[0].face.embeddable() {
		t.Fatal("restricted font embedded")
	}
}

// TestTextPositions checks that DrawText honors Adv with TJ adjustments
// and places the baseline in PDF space.
func TestTextPositions(t *testing.T) {
	d := New()
	d.NoCompress = true
	pg := d.AddPage(Letter)
	f := d.Font(FontSpec{Family: "Go"})
	w := pg.DrawText(Run{Font: f, Size: 10, X: 10, Y: 20, Text: "AB", Color: color.NRGBA{A: 255}, Adv: []float32{20, 7}})
	if w != 27 {
		t.Fatalf("width %v, want 27", w)
	}
	s := pg.buf.String()
	// "A" is narrower than 20pt at 10pt, so a negative adjustment follows it.
	if !regexp.MustCompile(`\[<[0-9A-F]{4}> -\d+(\.\d+)? <[0-9A-F]{4}>\] TJ`).MatchString(s) {
		t.Fatalf("no TJ adjustment in %q", s)
	}
	if !strings.Contains(s, "1 0 0 1 10 772 Tm") {
		t.Fatalf("text not placed at (10, 792-20): %q", s)
	}
}

// TestEmbedFontFiles embeds each font file listed in PDF_FONTS (OS path
// list) and checks the result; with PDF_DUMP the PDF is kept for viewing.
func TestEmbedFontFiles(t *testing.T) {
	list := filepath.SplitList(os.Getenv("PDF_FONTS"))
	if len(list) == 0 {
		t.Skip("PDF_FONTS not set")
	}
	d := New()
	pg := d.AddPage(Letter)
	y := 72.0
	for _, path := range list {
		for i, fc := range loadFile(path) {
			if !fc.embeddable() {
				t.Logf("%s #%d: not embeddable (fsType %#x)", path, i, fc.fsType)
				continue
			}
			src := dirSource{strings.ToLower(fc.family[0]): {path}}
			d.Fonts = src
			f := d.Font(FontSpec{Family: fc.family[0], Bold: fc.bold(), Italic: fc.italic})
			pg.DrawText(Run{Font: f, Size: 14, X: 72, Y: y, Text: fc.family[0] + " — The quick brown fox 0123", Color: color.NRGBA{A: 255}})
			t.Logf("%s #%d: %s (cff %v, weight %d, italic %v)", path, i, f.Embedded(), fc.cff, fc.weight, fc.italic)
			y += 24
		}
	}
	var out bytes.Buffer
	if err := d.Write(&out); err != nil {
		t.Fatal(err)
	}
	parsePDF(t, out.Bytes())
	if dump := os.Getenv("PDF_DUMP"); dump != "" {
		os.WriteFile(filepath.Join(dump, "fonts.pdf"), out.Bytes(), 0o644)
	}
}
