package layout

import (
	"image/color"
	"math"
	"time"

	"gioui.org/text"

	"officelite/common/colors"
	"officelite/word/model"
)

// PageGap is the space between pages in the page view, in points.
const PageGap = 12

// Rect is a rectangle in page coordinates (points).
type Rect struct{ X0, Y0, X1, Y1 float64 }

// Fill is a shaded rectangle (paragraph or cell shading).
type Fill struct {
	R Rect
	C color.NRGBA
}

// Placed is a line placed on a page. Item X positions are relative to X.
type Placed struct {
	Line *Line
	Para int   // paragraph index (model.Pos.Para)
	Box  *Para // the paragraph's layout
	X, Y float64
	W    float64 // width of the text column (page text area or cell)

	order int // index in document order of all placed lines
}

// Page is a laid-out page.
type Page struct {
	Index         int
	Width, Height float64
	Top           float64 // offset of the page in the page view
	Lines         []Placed
	Fills         []Fill
	Rules         []Rect // table borders, as thin rectangles
	// Header and Footer are the page's header and footer (nil when it has
	// none); BodyTop and BodyBottom bound the body text area (see
	// hdrftr.go).
	Header, Footer      *Band
	BodyTop, BodyBottom float64
	// TableRows are the rows of the body's tables placed on the page, for
	// resizing columns and rows by their borders.
	TableRows []TableRowBox
}

// TableRowBox is a table row placed on a page: its top and bottom and the
// x of each cell edge, left to right, in page coordinates.
type TableRowBox struct {
	Table  *model.Table
	Row    int
	Y0, Y1 float64
	Edges  []TableEdge
}

// TableEdge is a vertical cell border: its x and the grid column
// boundary it is on (0 is the table's left edge, n its right edge).
type TableEdge struct {
	X    float64
	Grid int
}

// LineRef points at a placed line.
type LineRef struct{ Page, Line int }

// Engine lays out one document.
type Engine struct {
	Doc   *model.Document
	Pages []*Page
	// Height is the page view's total height in points.
	Height float64
	// LastRelaid and LastDuration describe the last Layout call: how many
	// paragraphs were broken into lines again and how long it took.
	LastRelaid   int
	LastDuration time.Duration

	m      measurer
	glyphs []text.Glyph
	adv    []float32
	styles map[TextStyle]*TextStyle
	pics   map[*model.Picture]picCache // see picture.go
	cache  map[*model.Paragraph]*Para
	used   map[*model.Paragraph]bool
	dirty  map[*model.Paragraph]bool
	all    bool

	locs      []model.ParaLoc
	index     map[*model.Paragraph]int
	labels    map[*model.Paragraph]string
	paraLines [][]LineRef
	order     []LineRef

	// pagination state
	page      *Page
	y         float64
	empty     bool // nothing placed on the page yet
	hardBreak bool // the page was started by a page break
	setup     model.PageSetup

	// FieldValue, when set, gives the shown value of fields (PAGE in a
	// header); see hdrftr.go.
	FieldValue func(instr string) (string, bool)
	hf         hfState
}

// New returns an engine for doc measuring text with shaper.
func New(doc *model.Document, shaper *text.Shaper) *Engine {
	return &Engine{
		Doc:    doc,
		m:      measurer{shaper: shaper},
		styles: map[TextStyle]*TextStyle{},
		cache:  map[*model.Paragraph]*Para{},
		dirty:  map[*model.Paragraph]bool{},
		all:    true,
	}
}

// SetShaper replaces the shaper (after loading fonts); everything is laid
// out again.
func (e *Engine) SetShaper(s *text.Shaper) {
	e.m = measurer{shaper: s}
	e.all = true
}

// Shaper returns the engine's text shaper.
func (e *Engine) Shaper() *text.Shaper { return e.m.shaper }

// Invalidate marks paragraphs as changed; all relays out everything.
func (e *Engine) Invalidate(paras []*model.Paragraph, all bool) {
	for _, p := range paras {
		e.dirty[p] = true
	}
	e.all = e.all || all
}

// Layout lays out changed paragraphs again and repaginates.
func (e *Engine) Layout() {
	start := time.Now()
	e.LastRelaid = 0
	relayAll := e.all
	if e.all {
		clear(e.cache)
		clear(e.styles)
		e.all = false
	}
	for p := range e.dirty {
		delete(e.cache, p)
	}
	clear(e.dirty)
	e.locs = e.Doc.Locate()
	e.index = make(map[*model.Paragraph]int, len(e.locs))
	for i, l := range e.locs {
		e.index[l.P] = i
	}
	e.labels = e.listLabels(e.locs)
	e.paraLines = make([][]LineRef, len(e.locs))
	e.order = e.order[:0]
	e.used = make(map[*model.Paragraph]bool, len(e.locs))

	e.setup = e.Doc.Page
	if e.setup.Width <= 0 || e.setup.Height <= 0 {
		e.setup = model.LetterPage()
	}
	e.Pages = nil
	e.prepareHF(relayAll)
	e.newPage(false)
	body := e.Doc.Body
	for bi, b := range body {
		switch b := b.(type) {
		case *model.Paragraph:
			next := 0.0
			if bi+1 < len(body) {
				next = e.firstLineHeight(body[bi+1])
			}
			e.placePara(b, next)
		case *model.Table:
			e.placeTable(b)
		}
	}
	for p := range e.cache {
		if !e.used[p] {
			delete(e.cache, p)
		}
	}
	e.placeHF()
	top := float64(PageGap)
	for _, pg := range e.Pages {
		pg.Top = top
		top += pg.Height + PageGap
	}
	e.Height = top
	e.LastDuration = time.Since(start)
}

func (e *Engine) textLeft() float64 { return math.Abs(e.setup.MarginLeft) }
func (e *Engine) textWidth() float64 {
	return max(e.setup.Width-math.Abs(e.setup.MarginLeft)-math.Abs(e.setup.MarginRight), 72)
}
func (e *Engine) textTop() float64 { return max(math.Abs(e.setup.MarginTop), e.hfTop()) }
func (e *Engine) textBottom() float64 {
	return max(min(e.setup.Height-math.Abs(e.setup.MarginBottom), e.hfBottom()), e.textTop()+72)
}

func (e *Engine) newPage(hard bool) {
	e.page = &Page{Index: len(e.Pages), Width: e.setup.Width, Height: e.setup.Height}
	e.Pages = append(e.Pages, e.page)
	e.y = e.textTop()
	e.empty = true
	e.hardBreak = hard
}

// para returns the (cached) layout of p for a column width wide.
func (e *Engine) para(p *model.Paragraph, width float64, inTable bool) *Para {
	e.used[p] = true
	label := e.labels[p]
	if c := e.cache[p]; c != nil && c.width == width && c.label == label && c.inTable == inTable {
		return c
	}
	c := e.layoutPara(p, width, label, inTable)
	e.cache[p] = c
	e.LastRelaid++
	return c
}

func (e *Engine) firstLineHeight(b model.Block) float64 {
	switch b := b.(type) {
	case *model.Paragraph:
		pl := e.para(b, e.textWidth(), false)
		return pl.Before + pl.Lines[0].Height
	case *model.Table:
		if len(b.Rows) > 0 {
			return 14
		}
	}
	return 0
}

// place adds a line to the current page.
func (e *Engine) place(pl Placed) {
	pg := e.page
	ref := LineRef{pg.Index, len(pg.Lines)}
	pl.order = len(e.order)
	pg.Lines = append(pg.Lines, pl)
	e.paraLines[pl.Para] = append(e.paraLines[pl.Para], ref)
	e.order = append(e.order, ref)
	e.empty = false
}

func (e *Engine) placePara(p *model.Paragraph, nextFirst float64) {
	pl := e.para(p, e.textWidth(), false)
	idx := e.index[p]
	pp := pl.Props
	bottom := e.textBottom()
	if pp.PageBreakBefore.Bool(false) && !e.empty {
		e.newPage(true)
	}
	before := pl.Before
	if e.empty && e.page.Index > 0 && !e.hardBreak {
		before = 0 // Word drops space before at the top of a page
	}
	lines := pl.Lines
	if !e.empty {
		need := 0.0
		if pp.KeepLines.Bool(false) {
			need = before + pl.Height
		}
		if pp.KeepNext.Bool(false) {
			need = before + pl.Height + pl.After + nextFirst
		}
		if need > 0 && e.y+need > bottom && need <= bottom-e.textTop() {
			e.newPage(false)
			before = 0
		}
	}
	e.y += before
	widow := pp.WidowControl.Bool(false) && len(lines) > 1
	for k, l := range lines {
		if !e.empty {
			breakHere := e.y+l.Height > bottom+0.01
			// Widow/orphan control: never leave the first or the last line
			// of a paragraph alone on a page.
			if widow && !breakHere && k == 0 && e.y+l.Height+lines[1].Height > bottom+0.01 {
				breakHere = true
			}
			if widow && !breakHere && k > 0 && k == len(lines)-2 && e.y+l.Height+lines[k+1].Height > bottom+0.01 && len(lines) > 2 {
				breakHere = true
			}
			if breakHere {
				e.newPage(false)
			}
		}
		e.place(Placed{Line: l, Para: idx, Box: pl, X: e.textLeft(), Y: e.y, W: e.textWidth()})
		if pl.Shading.A > 0 {
			e.page.Fills = append(e.page.Fills, Fill{Rect{e.textLeft() + pl.Left, e.y, e.textLeft() + e.textWidth() - pl.Right, e.y + l.Height}, pl.Shading})
		}
		e.y += l.Height
		if l.PageBreak {
			e.newPage(true)
		}
	}
	e.y += pl.After
}

// Tables.

const cellMargin = 5.4 // Word's default left/right cell margin

type cellBox struct {
	cell    *model.TableCell
	x, w    float64 // relative to the table's left edge
	rowSpan int
	cont    bool // continues a vertical merge from above
	flow    *flow
}

type rowBox struct {
	height float64
	cells  []*cellBox
}

// flow is blocks laid out without pagination (a table cell's content),
// in coordinates relative to its top-left corner.
type flow struct {
	lines  []Placed
	fills  []Fill
	rules  []Rect
	height float64
}

func (e *Engine) layoutFlow(blocks []model.Block, width float64) *flow {
	f := &flow{}
	y := 0.0
	for _, b := range blocks {
		switch b := b.(type) {
		case *model.Paragraph:
			pl := e.para(b, width, true)
			y += pl.Before
			for _, l := range pl.Lines {
				f.lines = append(f.lines, Placed{Line: l, Para: e.index[b], Box: pl, Y: y, W: width})
				if pl.Shading.A > 0 {
					f.fills = append(f.fills, Fill{Rect{pl.Left, y, width - pl.Right, y + l.Height}, pl.Shading})
				}
				y += l.Height
			}
			y += pl.After
		case *model.Table:
			rows, x0 := e.layoutTable(b, width)
			for r := range rows {
				e.emitRow(rows, r, x0, y, &f.lines, &f.fills, &f.rules, b.Borders)
				y += rows[r].height
			}
		}
	}
	f.height = y
	return f
}

// layoutTable lays out every cell and computes row heights; x0 is the
// table's left edge relative to the text column.
func (e *Engine) layoutTable(t *model.Table, avail float64) ([]*rowBox, float64) {
	cols := t.Columns
	total := 0.0
	for _, c := range cols {
		total += c
	}
	if total <= 0 {
		n := 1
		for _, row := range t.Rows {
			k := 0
			for _, c := range row.Cells {
				k += max(c.Span, 1)
			}
			n = max(n, k)
		}
		cols = make([]float64, n)
		for i := range cols {
			cols[i] = avail / float64(n)
		}
		total = avail
	}
	colX := make([]float64, len(cols)+1)
	for i, c := range cols {
		colX[i+1] = colX[i] + c
	}
	gridX := func(g int) float64 {
		if g >= len(colX) {
			// More cells than grid columns: extend with the last width.
			last := cols[len(cols)-1]
			return colX[len(colX)-1] + float64(g-len(colX)+1)*last
		}
		return colX[g]
	}
	x0 := t.Indent
	switch t.Align {
	case model.AlignCenter:
		x0 = (avail - total) / 2
	case model.AlignRight:
		x0 = avail - total
	}

	rows := make([]*rowBox, len(t.Rows))
	starts := map[int]*cellBox{} // grid column -> open vertical merge
	for r, row := range t.Rows {
		rb := &rowBox{}
		rows[r] = rb
		g := 0
		for _, c := range row.Cells {
			span := max(c.Span, 1)
			cb := &cellBox{cell: c, x: gridX(g), w: gridX(g+span) - gridX(g), rowSpan: 1}
			if c.Width > 0 && len(t.Columns) == 0 {
				cb.w = c.Width
			}
			if c.VMerge == 2 {
				cb.cont = true
				if st := starts[g]; st != nil {
					st.rowSpan++
				}
			} else {
				cb.flow = e.layoutFlow(c.Blocks, max(cb.w-2*cellMargin, 6))
				if c.VMerge == 1 {
					starts[g] = cb
				} else {
					delete(starts, g)
				}
			}
			rb.cells = append(rb.cells, cb)
			g += span
		}
		for _, cb := range rb.cells {
			if cb.flow != nil && cb.rowSpan == 1 && cb.cell.VMerge != 1 {
				rb.height = max(rb.height, cb.flow.height)
			}
		}
		rb.height = max(rb.height, row.Height)
	}
	// Second pass: vertically merged cells grow their last row.
	for r, rb := range rows {
		for _, cb := range rb.cells {
			if cb.flow == nil {
				continue
			}
			span := min(cb.rowSpan, len(rows)-r)
			h := 0.0
			for k := r; k < r+span; k++ {
				h += rows[k].height
			}
			if cb.flow.height > h {
				rows[r+span-1].height += cb.flow.height - h
			}
		}
	}
	for _, rb := range rows {
		if rb.height <= 0 {
			rb.height = 14
		}
	}
	return rows, x0
}

// emitRow adds row r's cells (shading, text, borders) with the table's
// left edge at x0 and the row's top at y.
func (e *Engine) emitRow(rows []*rowBox, r int, x0, y float64, lines *[]Placed, fills *[]Fill, rules *[]Rect, borders bool) {
	rb := rows[r]
	const bw = 0.5
	for _, cb := range rb.cells {
		x := x0 + cb.x
		if !cb.cont {
			h := 0.0
			for k := r; k < min(r+cb.rowSpan, len(rows)); k++ {
				h += rows[k].height
			}
			if c, ok := colors.ParseHex(cb.cell.Shading); ok && cb.cell.Shading != "auto" {
				*fills = append(*fills, Fill{Rect{x, y, x + cb.w, y + h}, c})
			}
			off := 0.0
			switch cb.cell.VAlign {
			case 1:
				off = (h - cb.flow.height) / 2
			case 2:
				off = h - cb.flow.height
			}
			for _, f := range cb.flow.fills {
				*fills = append(*fills, Fill{Rect{f.R.X0 + x + cellMargin, f.R.Y0 + y + off, f.R.X1 + x + cellMargin, f.R.Y1 + y + off}, f.C})
			}
			for _, pl := range cb.flow.lines {
				pl.X += x + cellMargin
				pl.Y += y + off
				*lines = append(*lines, pl)
			}
			for _, rr := range cb.flow.rules {
				*rules = append(*rules, Rect{rr.X0 + x + cellMargin, rr.Y0 + y + off, rr.X1 + x + cellMargin, rr.Y1 + y + off})
			}
		}
		if !borders {
			continue
		}
		bottom := y + rb.height
		*rules = append(*rules, Rect{x, y, x + bw, bottom}, Rect{x + cb.w, y, x + cb.w + bw, bottom})
		if !cb.cont {
			*rules = append(*rules, Rect{x, y, x + cb.w + bw, y + bw})
		}
		if !continuesBelow(rows, r, cb.x) {
			*rules = append(*rules, Rect{x, bottom, x + cb.w + bw, bottom + bw})
		}
	}
}

func continuesBelow(rows []*rowBox, r int, x float64) bool {
	if r+1 >= len(rows) {
		return false
	}
	for _, cb := range rows[r+1].cells {
		if cb.cont && math.Abs(cb.x-x) < 0.01 {
			return true
		}
	}
	return false
}

func (e *Engine) placeTable(t *model.Table) {
	rows, x0 := e.layoutTable(t, e.textWidth())
	bottom := e.textBottom()
	for r, rb := range rows {
		if e.y+rb.height > bottom+0.01 && !e.empty {
			e.newPage(false)
		}
		pg := e.page
		var lines []Placed
		e.emitRow(rows, r, e.textLeft()+x0, e.y, &lines, &pg.Fills, &pg.Rules, t.Borders)
		box := TableRowBox{Table: t, Row: r, Y0: e.y, Y1: e.y + rb.height}
		g := 0
		for i, cb := range rb.cells {
			x := e.textLeft() + x0 + cb.x
			if i == 0 {
				box.Edges = append(box.Edges, TableEdge{X: x, Grid: g})
			}
			g += max(cb.cell.Span, 1)
			box.Edges = append(box.Edges, TableEdge{X: x + cb.w, Grid: g})
		}
		pg.TableRows = append(pg.TableRows, box)
		for _, pl := range lines {
			e.place(pl)
		}
		e.y += rb.height
		e.empty = false
	}
}
