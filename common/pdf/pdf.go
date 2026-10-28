// Package pdf writes PDF 1.7 documents in pure Go: pages of vector
// graphics (filled and stroked rectangles, lines, clipping), text in
// embedded, subsetted TrueType/OpenType fonts (Type0 fonts with
// Identity-H encoding and a ToUnicode map, so text is selectable and
// searchable) and images. Content streams are Flate-compressed.
//
// Coordinates are points (1/72 inch) with the origin at the page's top-left
// corner and y growing downwards, as in the apps' layouts; the writer
// converts them to PDF's bottom-up space.
package pdf

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"fmt"
	"image/color"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Paper sizes in points.
var (
	Letter = Size{612, 792}
	A4     = Size{595.28, 841.89}
)

// Size is a page size in points.
type Size struct{ W, H float64 }

// Document is a PDF being built. Add pages, draw on them, then Write.
type Document struct {
	Title    string
	Author   string
	Producer string
	// Fonts finds font files by family; Fallbacks are families tried for
	// characters a font lacks (and when a font may not be embedded). The
	// Go fonts are the last resort, so text always renders.
	Fonts     FontSource
	Fallbacks []string
	// NoCompress leaves content streams uncompressed (for tests).
	NoCompress bool
	// Created is the creation date; zero means now.
	Created time.Time

	pages    []*Page
	fonts    map[FontSpec]*Font
	famCache map[string][]*face
	embeds   []*embedded
	byFace   map[*face]*embedded
	images   []*imageObj
}

// New returns an empty document.
func New() *Document {
	return &Document{
		fonts:    map[FontSpec]*Font{},
		famCache: map[string][]*face{},
		byFace:   map[*face]*embedded{},
	}
}

// embedded is a face used by the document: the glyphs it draws and the
// text they stand for.
type embedded struct {
	face *face
	name string // resource name, e.g. F1
	used map[uint16]string
	obj  int // Type0 font object number, at write time
}

func (d *Document) embed(fc *face) *embedded {
	if e, ok := d.byFace[fc]; ok {
		return e
	}
	e := &embedded{face: fc, name: "F" + strconv.Itoa(len(d.embeds)+1), used: map[uint16]string{}}
	d.byFace[fc] = e
	d.embeds = append(d.embeds, e)
	return e
}

// Pages returns the number of pages.
func (d *Document) Pages() int { return len(d.pages) }

// AddPage appends a page of the given size and returns it.
func (d *Document) AddPage(sz Size) *Page {
	p := &Page{doc: d, W: sz.W, H: sz.H, alpha: map[string]float64{}}
	p.st.fill, p.st.stroke = color.NRGBA{A: 255}, color.NRGBA{A: 255}
	p.st.fa, p.st.sa = 255, 255
	p.st.lw = 1
	d.pages = append(d.pages, p)
	return p
}

// gstate is the part of the PDF graphics state the page tracks to avoid
// repeating operators.
type gstate struct {
	fill, stroke color.NRGBA
	fa, sa       uint8 // fill and stroke alpha
	lw           float64
	dash         string
}

// Page is one page's content.
type Page struct {
	W, H  float64
	doc   *Document
	buf   bytes.Buffer
	fonts []*embedded
	imgs  []*imageObj
	alpha map[string]float64 // ExtGState name -> alpha
	st    gstate
	stack []gstate
}

// num formats a coordinate compactly.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

func (p *Page) printf(format string, args ...any) { fmt.Fprintf(&p.buf, format, args...) }

func (p *Page) op(parts ...string) {
	p.buf.WriteString(strings.Join(parts, " "))
	p.buf.WriteByte('\n')
}

func colorOps(c color.NRGBA) (string, string, string) {
	return num(float64(c.R) / 255), num(float64(c.G) / 255), num(float64(c.B) / 255)
}

func (p *Page) setAlpha(fill bool, a uint8) {
	cur := &p.st.sa
	key := "CA"
	if fill {
		cur, key = &p.st.fa, "ca"
	}
	if *cur == a {
		return
	}
	name := fmt.Sprintf("G%s%d", key, a)
	p.alpha[name] = float64(a) / 255
	p.op("/"+name, "gs")
	*cur = a
}

func (p *Page) setFill(c color.NRGBA) {
	p.setAlpha(true, c.A)
	c.A = 255
	if p.st.fill != c {
		r, g, b := colorOps(c)
		p.op(r, g, b, "rg")
		p.st.fill = c
	}
}

func (p *Page) setStroke(c color.NRGBA) {
	p.setAlpha(false, c.A)
	c.A = 255
	if p.st.stroke != c {
		r, g, b := colorOps(c)
		p.op(r, g, b, "RG")
		p.st.stroke = c
	}
}

func (p *Page) setLine(w float64, dash []float64) {
	if w != p.st.lw {
		p.op(num(w), "w")
		p.st.lw = w
	}
	d := "[] 0"
	if len(dash) > 0 {
		parts := make([]string, len(dash))
		for i, v := range dash {
			parts[i] = num(v)
		}
		d = "[" + strings.Join(parts, " ") + "] 0"
	}
	if d != p.st.dash && (d != "[] 0" || p.st.dash != "") {
		p.op(d, "d")
	}
	p.st.dash = d
}

// y converts a top-down y to PDF space.
func (p *Page) y(v float64) float64 { return p.H - v }

func (p *Page) rect(x, y, w, h float64) string {
	return num(x) + " " + num(p.y(y+h)) + " " + num(w) + " " + num(h) + " re"
}

// FillRect fills a rectangle.
func (p *Page) FillRect(x, y, w, h float64, c color.NRGBA) {
	if w <= 0 || h <= 0 || c.A == 0 {
		return
	}
	p.setFill(c)
	p.op(p.rect(x, y, w, h), "f")
}

// StrokeRect outlines a rectangle with a line of width lw.
func (p *Page) StrokeRect(x, y, w, h, lw float64, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	p.setStroke(c)
	p.setLine(lw, nil)
	p.op(p.rect(x, y, w, h), "S")
}

// Line strokes a straight line of width lw, dashed when dash (on/off
// lengths) is given.
func (p *Page) Line(x0, y0, x1, y1, lw float64, c color.NRGBA, dash ...float64) {
	if c.A == 0 || lw <= 0 {
		return
	}
	p.setStroke(c)
	p.setLine(lw, dash)
	p.op(num(x0), num(p.y(y0)), "m", num(x1), num(p.y(y1)), "l", "S")
}

// Save pushes the graphics state (Restore pops it); clips set in
// between end at Restore.
func (p *Page) Save() {
	p.stack = append(p.stack, p.st)
	p.op("q")
}

// Restore pops the graphics state.
func (p *Page) Restore() {
	if len(p.stack) == 0 {
		return
	}
	p.st = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.op("Q")
}

// ClipRect intersects the clip with a rectangle (until Restore).
func (p *Page) ClipRect(x, y, w, h float64) {
	p.op(p.rect(x, y, max(w, 0), max(h, 0)), "W n")
}

// Transform concatenates a matrix mapping (x, y) to
// (a*x + c*y + e, b*x + d*y + f) in top-down page coordinates.
func (p *Page) Transform(a, b, c, d, e, f float64) {
	// Conjugate with the flip y' = H - y.
	H := p.H
	p.op(num(a), num(-b), num(-c), num(d), num(e+c*H), num(H-f-d*H), "cm")
}

// Run is a piece of text drawn with DrawText.
type Run struct {
	Font  *Font
	Size  float64 // points
	X, Y  float64 // start of the baseline
	Text  string
	Color color.NRGBA
	// Adv, when set, is the advance of each rune (points): glyphs are
	// placed exactly there, as the app's layout measured them.
	Adv []float32
	// HScale compresses (< 1) the text horizontally; 0 means 1.
	HScale float64
}

// DrawText draws a run and returns its width.
func (p *Page) DrawText(r Run) float64 {
	if r.Text == "" || r.Size <= 0 || r.Font == nil || r.Color.A == 0 {
		return 0
	}
	hs := r.HScale
	if hs <= 0 {
		hs = 1
	}
	type seg struct {
		m      *member
		x      float64
		glyphs []uint16
		adj    []float64 // TJ adjustment after each glyph (1/1000 em)
	}
	var segs []*seg
	x := r.X
	i := 0
	for _, ch := range r.Text {
		m, g := r.Font.pick(ch)
		w := m.face.advance(g) * r.Size / 1000 * hs
		adv := w
		if r.Adv != nil && i < len(r.Adv) {
			adv = float64(r.Adv[i])
		}
		i++
		if ch == '\n' || ch == '\r' || ch == '\t' {
			x += adv
			continue
		}
		var s *seg
		if n := len(segs); n > 0 && segs[n-1].m == m {
			s = segs[n-1]
		} else {
			s = &seg{m: m, x: x}
			segs = append(segs, s)
		}
		if _, ok := m.descendants.used[g]; !ok {
			m.descendants.used[g] = string(ch)
		}
		s.glyphs = append(s.glyphs, g)
		// Positive TJ numbers move left: the glyph's own advance minus
		// the wanted one.
		s.adj = append(s.adj, (w-adv)*1000/r.Size/hs)
		x += adv
	}
	if len(segs) == 0 {
		return 0
	}
	p.setFill(r.Color)
	for _, s := range segs {
		e := s.m.descendants
		if !slices.Contains(p.fonts, e) {
			p.fonts = append(p.fonts, e)
		}
		synth := s.m.fakeBold
		if synth {
			p.Save()
			p.setStroke(r.Color)
			p.setLine(r.Size*0.03, nil)
		}
		p.op("BT")
		p.op("/"+e.name, num(r.Size), "Tf")
		if synth {
			p.op("2 Tr")
		}
		if hs != 1 {
			p.op(num(hs*100), "Tz")
		}
		skew := 0.0
		if s.m.fakeItalic {
			skew = 0.2
		}
		p.op("1 0", num(skew), "1", num(s.x), num(p.y(r.Y)), "Tm")
		var b strings.Builder
		b.WriteString("[<")
		for k, g := range s.glyphs {
			fmt.Fprintf(&b, "%04X", g)
			if a := s.adj[k]; k < len(s.glyphs)-1 && (a > 0.05 || a < -0.05) {
				b.WriteString("> " + num(a) + " <")
			}
		}
		b.WriteString(">] TJ")
		p.op(b.String())
		if hs != 1 {
			p.op("100 Tz")
		}
		p.op("ET")
		if synth {
			p.Restore()
		}
	}
	return x - r.X
}

// Write writes the document.
func (d *Document) Write(w io.Writer) error {
	ow := &objWriter{w: w, compress: !d.NoCompress}
	ow.raw("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	catalog, pagesObj, info := ow.alloc(), ow.alloc(), ow.alloc()
	pageObjs := make([]int, len(d.pages))
	for i := range d.pages {
		pageObjs[i] = ow.alloc()
	}
	for _, e := range d.embeds {
		if len(e.used) > 0 {
			e.obj = ow.alloc()
		}
	}
	for _, im := range d.images {
		im.obj = ow.alloc()
	}

	ow.object(catalog, "<< /Type /Catalog /Pages "+ref(pagesObj)+" >>")
	kids := make([]string, len(pageObjs))
	for i, n := range pageObjs {
		kids[i] = ref(n)
	}
	ow.object(pagesObj, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageObjs)))
	created := d.Created
	if created.IsZero() {
		created = time.Now()
	}
	infoDict := "<< /Producer " + textString(d.Producer) + " /CreationDate " + textString(pdfDate(created))
	if d.Title != "" {
		infoDict += " /Title " + textString(d.Title)
	}
	if d.Author != "" {
		infoDict += " /Author " + textString(d.Author)
	}
	ow.object(info, infoDict+" >>")

	for i, p := range d.pages {
		content := ow.alloc()
		var res []string
		if len(p.fonts) > 0 {
			var fs []string
			for _, e := range p.fonts {
				fs = append(fs, "/"+e.name+" "+ref(e.obj))
			}
			res = append(res, "/Font << "+strings.Join(fs, " ")+" >>")
		}
		if len(p.imgs) > 0 {
			var xs []string
			for _, im := range p.imgs {
				xs = append(xs, "/"+im.name+" "+ref(im.obj))
			}
			res = append(res, "/XObject << "+strings.Join(xs, " ")+" >>")
		}
		if len(p.alpha) > 0 {
			names := make([]string, 0, len(p.alpha))
			for n := range p.alpha {
				names = append(names, n)
			}
			slices.Sort(names)
			var gs []string
			for _, n := range names {
				key := "/CA"
				if strings.HasPrefix(n, "Gca") {
					key = "/ca"
				}
				gs = append(gs, "/"+n+" << "+key+" "+num(p.alpha[n])+" >>")
			}
			res = append(res, "/ExtGState << "+strings.Join(gs, " ")+" >>")
		}
		ow.object(pageObjs[i], fmt.Sprintf("<< /Type /Page /Parent %s /MediaBox [0 0 %s %s] /Resources << %s >> /Contents %s >>",
			ref(pagesObj), num(p.W), num(p.H), strings.Join(res, " "), ref(content)))
		ow.stream(content, "", p.buf.Bytes(), true)
	}
	for _, e := range d.embeds {
		if e.obj != 0 {
			if err := ow.font(e); err != nil {
				return err
			}
		}
	}
	for _, im := range d.images {
		ow.image(im)
	}
	return ow.finish(catalog, info)
}

func ref(n int) string { return strconv.Itoa(n) + " 0 R" }

// textString encodes a PDF text string: literal for ASCII, UTF-16BE hex
// otherwise.
func textString(s string) string {
	ascii := true
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			ascii = false
			break
		}
	}
	if ascii {
		r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
		return "(" + r.Replace(s) + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}

func pdfDate(t time.Time) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	return fmt.Sprintf("D:%s%s%02d'%02d'", t.Format("20060102150405"), sign, off/3600, off/60%60)
}

// objWriter writes numbered objects and records their offsets.
type objWriter struct {
	w        io.Writer
	n        int64
	offsets  []int64 // by object number - 1
	err      error
	compress bool
	hash     []byte
}

func (o *objWriter) raw(s string) { o.bytes([]byte(s)) }

func (o *objWriter) bytes(b []byte) {
	if o.err != nil {
		return
	}
	k, err := o.w.Write(b)
	o.n += int64(k)
	o.err = err
	if len(o.hash) < 4096 {
		o.hash = append(o.hash, b[:min(len(b), 4096-len(o.hash))]...)
	}
}

func (o *objWriter) alloc() int {
	o.offsets = append(o.offsets, -1)
	return len(o.offsets)
}

func (o *objWriter) begin(n int) {
	o.offsets[n-1] = o.n
	o.raw(strconv.Itoa(n) + " 0 obj\n")
}

func (o *objWriter) object(n int, body string) {
	o.begin(n)
	o.raw(body + "\nendobj\n")
}

// stream writes a stream object; dict holds extra entries. Data is
// Flate-compressed when compress is set (and the writer compresses).
func (o *objWriter) stream(n int, dict string, data []byte, compress bool) {
	if compress && o.compress {
		var z bytes.Buffer
		zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
		zw.Write(data)
		zw.Close()
		data = z.Bytes()
		dict = "/Filter /FlateDecode " + dict
	}
	o.begin(n)
	o.raw(fmt.Sprintf("<< %s/Length %d >>\nstream\n", dict, len(data)))
	o.bytes(data)
	o.raw("\nendstream\nendobj\n")
}

func (o *objWriter) finish(catalog, info int) error {
	xref := o.n
	var b strings.Builder
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(o.offsets)+1)
	for _, off := range o.offsets {
		if off < 0 {
			b.WriteString("0000000000 65535 f\r\n")
			continue
		}
		fmt.Fprintf(&b, "%010d 00000 n\r\n", off)
	}
	id := fmt.Sprintf("%X", md5.Sum(o.hash))
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %s /Info %s /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF\n",
		len(o.offsets)+1, ref(catalog), ref(info), id, id, xref)
	o.raw(b.String())
	return o.err
}
