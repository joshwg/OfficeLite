package layout

import (
	"image"
	"math"
	"strconv"
	"strings"

	"gioui.org/op"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// Band is a header or footer laid out on one page. Its lines, fills and
// rules are in page coordinates like the page's own, so anything that
// draws a Page (the page view, PDF export) draws a band the same way:
// Engine.DrawPage draws them, and other renderers can walk Lines (with
// Fills and Rules) exactly as they walk Page.Lines. Placed.Para of a
// band line indexes the header's own paragraphs, not the body's.
type Band struct {
	HF     *model.HeaderFooter
	Kind   model.HFKind
	Footer bool
	Lines  []Placed
	Fills  []Fill
	Rules  []Rect
	// Top and Bottom are the extent of the band's content on the page.
	Top, Bottom float64
}

// hfState caches the layouts of a document's headers and footers.
type hfState struct {
	subs  map[subKey]*hfSub
	dirty bool
}

// hfSub lays out one header or footer: an engine over a document holding
// its blocks, and its layouts by field values (one when it has no page
// fields).
type hfSub struct {
	eng    *Engine
	fields bool
	bands  map[string]*bandLayout
}

type bandLayout struct {
	lines  []Placed
	fills  []Fill
	rules  []Rect
	height float64
}

// InvalidateHF marks the headers and footers as changed (after editing
// one), so they are laid out again by the next Layout.
func (e *Engine) InvalidateHF() { e.hf.dirty = true }

// bandSetup is the page setup a header or footer is laid out with: the
// page's width and side margins, no top margin and no page breaks.
func bandSetup(pg model.PageSetup) model.PageSetup {
	pg.MarginTop, pg.MarginBottom = 0, 0
	pg.Height = 1e6
	return pg
}

func (e *Engine) prepareHF(all bool) {
	if all || e.hf.dirty || e.hf.subs == nil {
		e.hf.subs = map[subKey]*hfSub{}
		e.hf.dirty = false
	}
}

// FieldValue formats the value of a PAGE, NUMPAGES or SECTIONPAGES field
// on page (1-based) of n, honoring the \* roman, Roman, alphabetic and
// ALPHABETIC format switches; ok is false for other fields.
func FieldValue(instr string, page, n int) (string, bool) {
	return FieldValueFmt(instr, page, n, "")
}

// FieldValueFmt is FieldValue with the number format of the page's
// section ("" = decimal; see model.FormatNumber) for PAGE fields without
// a format switch.
func FieldValueFmt(instr string, page, n int, format string) (string, bool) {
	v := 0
	switch model.FieldName(instr) {
	case model.FieldPage:
		v = page
	case model.FieldNumPages, "SECTIONPAGES":
		v = n
	default:
		return "", false
	}
	f := strings.Fields(instr)
	for i := 1; i+1 < len(f); i++ {
		if f[i] != `\*` {
			continue
		}
		switch f[i+1] {
		case "roman":
			return model.FormatNumber(v, "lowerRoman"), true
		case "Roman", "ROMAN":
			return model.FormatNumber(v, "upperRoman"), true
		case "alphabetic":
			return model.FormatNumber(v, "lowerLetter"), true
		case "ALPHABETIC":
			return model.FormatNumber(v, "upperLetter"), true
		}
	}
	if format != "" && format != "decimal" && model.FieldName(instr) == model.FieldPage {
		return model.FormatNumber(v, format), true
	}
	return strconv.Itoa(v), true
}

// PageFieldValue is the value of a PAGE or NUMPAGES field on page i
// (0-based).
func (e *Engine) PageFieldValue(i int, instr string) (string, bool) {
	if i < 0 || i >= len(e.Pages) {
		return FieldValue(instr, i+1, len(e.Pages))
	}
	pg := e.Pages[i]
	return FieldValueFmt(instr, pg.Number, len(e.Pages), e.sectionOf(pg).PageNumFormat)
}

// sectionOf returns a page's section.
func (e *Engine) sectionOf(pg *Page) *model.Section {
	if pg.Section < len(e.secs) {
		return e.secs[pg.Section]
	}
	return &e.Doc.Section
}

// PageHF returns the headers and footers of page i's section (what
// editing them on that page changes) and the kind of header the page
// shows.
func (e *Engine) PageHF(i int) (*model.HeadersFooters, model.HFKind) {
	if i < 0 || i >= len(e.Pages) {
		return &e.Doc.HF, model.HFDefault
	}
	pg := e.Pages[i]
	return &e.sectionOf(pg).HF, e.hfKind(pg)
}

// PageHeader returns the header (or footer) page i shows, which may be
// a previous section's (nil when none).
func (e *Engine) PageHeader(i int, footer bool) *model.HeaderFooter {
	if i < 0 || i >= len(e.Pages) {
		return nil
	}
	h, _ := e.pageHF(e.Pages[i], footer)
	return h
}

// hfKind is the kind of header a page shows: its section's first-page
// header on the section's first page when the section has one, the even
// header on even pages when the document has them.
func (e *Engine) hfKind(pg *Page) model.HFKind {
	switch {
	case pg.First && e.sectionOf(pg).HF.TitlePage:
		return model.HFFirst
	case e.Doc.HF.EvenOdd && pg.Number%2 == 0:
		return model.HFEven
	}
	return model.HFDefault
}

// pageHF returns the header (or footer) page pg shows: its section's
// or, when the section has none of that kind, the previous section's
// (Word's Link to Previous); nil when none.
func (e *Engine) pageHF(pg *Page, footer bool) (*model.HeaderFooter, model.HFKind) {
	k := e.hfKind(pg)
	if pg.Section >= len(e.secs) {
		return nil, k
	}
	return model.HeaderFor(e.secs, pg.Section, k, footer), k
}

// fieldText is the text shown for an opaque run: its field's current
// value when the engine computes it, else its stored text.
func (e *Engine) fieldText(r *model.Run) string {
	if e.FieldValue != nil && r.Opaque != nil && r.Opaque.Field != "" {
		if v, ok := e.FieldValue(r.Opaque.Field); ok {
			return v
		}
	}
	return r.Text
}

// subKey is a header laid out for a page width and side margins
// (sections linked to the same header may differ).
type subKey struct {
	hf        *model.HeaderFooter
	w, ml, mr float64
}

func (e *Engine) sub(hf *model.HeaderFooter) *hfSub {
	pg := e.setup
	if e.page != nil {
		pg = e.pageSetup(e.page)
	}
	key := subKey{hf, pg.Width, pg.MarginLeft, pg.MarginRight}
	if s := e.hf.subs[key]; s != nil {
		return s
	}
	doc := e.Doc.SubDocument(hf.Blocks)
	doc.Page = bandSetup(pg)
	s := &hfSub{eng: New(doc, e.m.shaper), fields: model.HasPageFields(hf.Blocks), bands: map[string]*bandLayout{}}
	e.hf.subs[key] = s
	return s
}

// band lays out hf for page (1-based) of n.
func (e *Engine) band(hf *model.HeaderFooter, page, n int) *bandLayout {
	return e.bandFmt(hf, page, n, "")
}

// bandFmt lays out hf for page number page of n, PAGE fields showing the
// number in format.
func (e *Engine) bandFmt(hf *model.HeaderFooter, page, n int, format string) *bandLayout {
	s := e.sub(hf)
	key := ""
	if s.fields {
		key = strconv.Itoa(page) + "/" + strconv.Itoa(n) + "/" + format
	}
	if b := s.bands[key]; b != nil {
		return b
	}
	eng := s.eng
	eng.FieldValue = func(instr string) (string, bool) { return FieldValueFmt(instr, page, n, format) }
	eng.all = true
	eng.Layout()
	b := &bandLayout{}
	y0 := 0.0
	for _, pg := range eng.Pages {
		for _, pl := range pg.Lines {
			pl.Y += y0
			b.lines = append(b.lines, pl)
			b.height = max(b.height, pl.Y+pl.Line.Height)
		}
		for _, f := range pg.Fills {
			f.R.Y0, f.R.Y1 = f.R.Y0+y0, f.R.Y1+y0
			b.fills = append(b.fills, f)
		}
		for _, r := range pg.Rules {
			r.Y0, r.Y1 = r.Y0+y0, r.Y1+y0
			b.rules = append(b.rules, r)
			b.height = max(b.height, r.Y1)
		}
		if len(pg.Lines) > 0 {
			l := pg.Lines[len(pg.Lines)-1] // pg.Lines keeps the page-local Y
			y0 += l.Y + l.Line.Height
		}
	}
	s.bands[key] = b
	return b
}

// hfTop is the lowest the body may start because of the page's header
// (0 without one): Word pushes the body down when the header is taller
// than the top margin leaves room for. A negative (exact) top margin
// doesn't move.
func (e *Engine) hfTop() float64 {
	if e.setup.MarginTop < 0 || e.page == nil {
		return 0
	}
	hf, _ := e.pageHF(e.page, false)
	if hf == nil {
		return 0
	}
	return e.setup.Header + e.band(hf, 1, 1).height
}

// hfBottom is the highest the body may end because of the footer.
func (e *Engine) hfBottom() float64 {
	if e.setup.MarginBottom < 0 || e.page == nil {
		return math.Inf(1)
	}
	hf, _ := e.pageHF(e.page, true)
	if hf == nil {
		return math.Inf(1)
	}
	return e.setup.Height - e.setup.Footer - e.band(hf, 1, 1).height
}

// bandOffset is where a band's top goes on the page (of the page setup
// e.setup).
func (e *Engine) bandOffset(footer bool, height float64) float64 {
	if footer {
		return e.setup.Height - e.setup.Footer - height
	}
	return e.setup.Header
}

// pageSetup is the page setup of page pg's section.
func (e *Engine) pageSetup(pg *Page) model.PageSetup {
	s := e.sectionOf(pg).Page
	if s.Width <= 0 || s.Height <= 0 {
		s = model.LetterPage()
	}
	return s
}

// placeHF puts every page's header and footer on it.
func (e *Engine) placeHF() {
	n := len(e.Pages)
	for _, pg := range e.Pages {
		e.page = pg
		e.setup = e.pageSetup(pg)
		e.notes.footH = 0
		pg.BodyTop, pg.BodyBottom = e.textTop(), e.areaBottom()
		pg.Header, pg.Footer = nil, nil
		for _, footer := range []bool{false, true} {
			hf, kind := e.pageHF(pg, footer)
			if hf == nil {
				continue
			}
			bl := e.bandFmt(hf, pg.Number, n, e.sectionOf(pg).PageNumFormat)
			off := e.bandOffset(footer, bl.height)
			b := &Band{HF: hf, Kind: kind, Footer: footer, Top: off, Bottom: off + bl.height}
			for _, pl := range bl.lines {
				pl.Y += off
				b.Lines = append(b.Lines, pl)
			}
			for _, f := range bl.fills {
				f.R.Y0, f.R.Y1 = f.R.Y0+off, f.R.Y1+off
				b.Fills = append(b.Fills, f)
			}
			for _, r := range bl.rules {
				r.Y0, r.Y1 = r.Y0+off, r.Y1+off
				b.Rules = append(b.Rules, r)
			}
			if footer {
				pg.Footer = b
			} else {
				pg.Header = b
			}
		}
	}
}

// InHFArea reports whether a point at y (page coordinates) of page i is
// in its header area (above the body) or footer area (below it).
func (e *Engine) InHFArea(i int, y float64) (footer, ok bool) {
	if i < 0 || i >= len(e.Pages) {
		return false, false
	}
	pg := e.Pages[i]
	switch {
	case y < pg.BodyTop:
		return false, true
	case y > pg.BodyBottom:
		return true, true
	}
	return false, false
}

// EditView lays out doc (the header or footer of page i being edited,
// see model.Document.SubDocument) where it shows on that page, in an
// engine whose pages have the positions of e's: its CaretAt, HitView,
// Vertical, LineBounds and SelectionRects then work in page-view
// coordinates like e's. Call it again after every edit.
func (e *Engine) EditView(i int, footer bool, doc *model.Document) *Engine {
	if n := len(e.Pages); n > 0 {
		i = min(max(i, 0), n-1) // the lines must land on a page of the view
		e.setup = e.pageSetup(e.Pages[i])
	} else {
		i = 0
	}
	doc.Page = bandSetup(e.setup)
	ed := New(doc, e.m.shaper)
	ed.FieldValue = func(instr string) (string, bool) { return e.PageFieldValue(i, instr) }
	ed.Layout()
	var lines []Placed
	var fills []Fill
	var rules []Rect
	remap := map[LineRef]LineRef{}
	h, y0 := 0.0, 0.0
	for pi, pg := range ed.Pages {
		for li, pl := range pg.Lines {
			pl.Y += y0
			remap[LineRef{pi, li}] = LineRef{i, len(lines)}
			lines = append(lines, pl)
			h = max(h, pl.Y+pl.Line.Height)
		}
		for _, f := range pg.Fills {
			f.R.Y0, f.R.Y1 = f.R.Y0+y0, f.R.Y1+y0
			fills = append(fills, f)
		}
		for _, r := range pg.Rules {
			r.Y0, r.Y1 = r.Y0+y0, r.Y1+y0
			rules = append(rules, r)
			h = max(h, r.Y1)
		}
		if len(pg.Lines) > 0 {
			l := pg.Lines[len(pg.Lines)-1] // pg.Lines keeps the page-local Y
			y0 += l.Y + l.Line.Height
		}
	}
	off := e.bandOffset(footer, h)
	for k := range lines {
		lines[k].Y += off
	}
	for k := range fills {
		fills[k].R.Y0 += off
		fills[k].R.Y1 += off
	}
	for k := range rules {
		rules[k].Y0 += off
		rules[k].Y1 += off
	}
	pages := make([]*Page, max(len(e.Pages), 1))
	for k := range pages {
		pages[k] = &Page{Index: k}
		if k < len(e.Pages) {
			pg := e.Pages[k]
			*pages[k] = Page{Index: k, Width: pg.Width, Height: pg.Height, Top: pg.Top, X: pg.X, BodyTop: pg.BodyTop, BodyBottom: pg.BodyBottom,
				Section: pg.Section, First: pg.First, Number: pg.Number}
		}
	}
	pages[i].Lines, pages[i].Fills, pages[i].Rules = lines, fills, rules
	for p := range ed.paraLines {
		for k, ref := range ed.paraLines[p] {
			ed.paraLines[p][k] = remap[ref]
		}
	}
	for k, ref := range ed.order {
		ed.order[k] = remap[ref]
	}
	ed.Pages = pages
	ed.Height, ed.Width = e.Height, e.Width
	return ed
}

var (
	dimColor    = colors.RGBA(0xFF, 0xFF, 0xFF, 0x99)
	hfLineColor = colors.RGB(0x2B579A)
)

// drawBands draws the page's header and footer: dimmed in the page view
// while the body is edited (o.HFDim), or with the body dimmed and the
// header/footer boundary shown while they are edited (o.BodyDim).
func (e *Engine) drawBands(ops *op.Ops, pg *Page, o DrawOptions) {
	s := o.Scale
	px := func(v float64) int { return int(math.Round(v * s)) }
	w := px(pg.Width)
	if o.BodyDim {
		gui.FillRect(ops, image.Rect(0, px(pg.BodyTop), w, px(pg.BodyBottom)), dimColor)
	}
	for _, b := range []*Band{pg.Header, pg.Footer} {
		if b == nil {
			continue
		}
		e.DrawBand(ops, b, s)
		if o.ShowMarks {
			e.drawMarks(ops, e.Marks(b.Lines, nil), 0, 0, s)
		}
	}
	if o.HFDim {
		if pg.Header != nil {
			gui.FillRect(ops, image.Rect(0, 0, w, px(pg.BodyTop)), dimColor)
		}
		if pg.Footer != nil {
			gui.FillRect(ops, image.Rect(0, px(pg.BodyBottom), w, px(pg.Height)), dimColor)
		}
	}
	if o.BodyDim {
		// Word's dashed boundaries under the header and over the footer.
		dash := func(y int) {
			for x := 0; x < w; x += px(6) + 1 {
				gui.HLine(ops, x, min(x+px(3)+1, w), y, 1, hfLineColor)
			}
		}
		dash(px(pg.BodyTop))
		dash(px(pg.BodyBottom))
	}
}

// DrawBand draws a header or footer at scale s (pixels per point) with the
// page's top-left corner at the current origin.
func (e *Engine) DrawBand(ops *op.Ops, b *Band, s float64) {
	px := func(v float64) int { return int(math.Round(v * s)) }
	rect := func(r Rect) image.Rectangle {
		return image.Rect(px(r.X0), px(r.Y0), max(px(r.X1), px(r.X0)+1), max(px(r.Y1), px(r.Y0)+1))
	}
	for _, f := range b.Fills {
		gui.FillRect(ops, rect(f.R), f.C)
	}
	for i := range b.Lines {
		e.drawLine(ops, &b.Lines[i], s)
	}
	for _, r := range b.Rules {
		rr := rect(r)
		if rr.Dx() < rr.Dy() {
			rr.Max.X = rr.Min.X + max(1, px(0.75))
		} else {
			rr.Max.Y = rr.Min.Y + max(1, px(0.75))
		}
		gui.FillRect(ops, rr, ruleColor)
	}
}
