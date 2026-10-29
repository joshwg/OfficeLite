package layout

import (
	"image/color"
	"math"
	"slices"
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
	// collapsed is set on the empty paragraph that ends a table cell
	// after a nested table: Word gives it no room and shows no mark.
	collapsed bool
}

// Page is a laid-out page.
type Page struct {
	Index         int
	Width, Height float64
	Top           float64 // offset of the page in the page view
	// X is the page's left edge in the page view, relative to the widest
	// page's (pages of different sizes are centered under each other).
	X float64
	// Section is the index of the page's section (model.Document.Sections),
	// First is set on a section's first page, and Number is the page's
	// number as PAGE fields show it (sections may restart numbering).
	Section int
	First   bool
	Number  int
	Lines   []Placed
	Fills   []Fill
	Rules   []Rect // table borders, as thin rectangles
	// Header and Footer are the page's header and footer (nil when it has
	// none); BodyTop and BodyBottom bound the body text area (see
	// hdrftr.go).
	Header, Footer      *Band
	BodyTop, BodyBottom float64
	// TableRows are the rows of the body's tables placed on the page, for
	// resizing columns and rows by their borders.
	TableRows []TableRowBox
	// Notes are the page's footnotes (at the bottom of the text area) and
	// endnotes, each area with its separator line; see notes.go.
	Notes []*Band
	// Floats are the floating objects (text boxes, shapes, pictures)
	// anchored on the page; see float.go.
	Floats []*FloatBox
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
	// Height is the page view's total height in points, Width the
	// widest page's width.
	Height, Width float64
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
	secs      []*model.Section // the document's sections
	sec       int              // the section being laid out
	secFirst  bool             // the next page is its section's first
	pageNum   int              // the number of the last page started
	cols      colState
	breaks    map[*model.Paragraph]model.SectionBreak // section break paragraphs: the next section's break
	notes     noteState                               // see notes.go
	fields    []model.FieldRange                      // fields spanning paragraphs, shaded when the caret is in one
	paraTop   float64                                 // top of the paragraph being placed, on its page
	lastBox   *Para

	// FieldValue, when set, gives the shown value of fields (PAGE in a
	// header); see hdrftr.go.
	FieldValue func(instr string) (string, bool)
	hf         hfState

	showHidden bool // hidden text is laid out (see marks.go)
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
	e.fields = model.FieldRangesIn(e.locs)
	e.paraLines = make([][]LineRef, len(e.locs))
	e.order = e.order[:0]
	e.used = make(map[*model.Paragraph]bool, len(e.locs))

	e.secs = e.Doc.Sections()
	e.breaks = map[*model.Paragraph]model.SectionBreak{}
	e.Pages = nil
	e.page = nil
	e.prepareHF(relayAll)
	e.prepareNotes()
	e.sec, e.secFirst, e.pageNum = 0, true, 0
	e.useSection()
	if s := e.secs[0]; s.PageNumRestart {
		e.pageNum = s.PageNumStart - 1
	}
	e.newPage(false)
	body := e.Doc.Body
	for bi, b := range body {
		switch b := b.(type) {
		case *model.Paragraph:
			next := 0.0
			if bi+1 < len(body) && b.Sect == nil {
				next = e.firstLineHeight(body[bi+1])
			}
			pl := e.para(b, e.textWidth(), nil)
			sp := e.spacing(pl, body, bi)
			e.placePara(pl, sp, next)
			if b.Sect != nil && e.sec+1 < len(e.secs) {
				e.breaks[b] = e.secs[e.sec+1].Break
				e.startSection(e.sec + 1)
			}
		case *model.Table:
			e.placeTable(b)
		}
	}
	e.placeEndnotes()
	e.finishPage()
	for p := range e.cache {
		if !e.used[p] {
			delete(e.cache, p)
		}
	}
	e.placeHF()
	top := float64(PageGap)
	e.Width = 0
	for _, pg := range e.Pages {
		pg.Top = top
		top += pg.Height + PageGap
		e.Width = max(e.Width, pg.Width)
	}
	for _, pg := range e.Pages {
		pg.X = math.Round((e.Width - pg.Width) / 2)
	}
	e.Height = top
	e.LastDuration = time.Since(start)
}

// colState is the text columns of the section being laid out.
type colState struct {
	x, w []float64 // each column's left offset (from the left margin) and width
	i    int       // the column being filled
	top  float64   // where the columns start on the page
	maxY float64   // the lowest y reached in the columns before i
}

// useSection takes the page setup and columns of section e.sec.
func (e *Engine) useSection() {
	s := e.secs[e.sec]
	e.setup = s.Page
	if e.setup.Width <= 0 || e.setup.Height <= 0 {
		e.setup = model.LetterPage()
	}
	e.cols.x, e.cols.w = s.Columns.Layout(e.areaWidth())
	e.cols.i = 0
}

// startSection starts section k after the paragraph that ended the
// previous one: on a new page (an even or odd one, leaving a page blank
// when needed) or, for a continuous break on a page of the same size,
// below what is on the page with the new section's margins and columns.
func (e *Engine) startSection(k int) {
	s := e.secs[k]
	pg := s.Page
	if pg.Width <= 0 || pg.Height <= 0 {
		pg = model.LetterPage()
	}
	sameSize := math.Abs(pg.Width-e.page.Width) < 0.5 && math.Abs(pg.Height-e.page.Height) < 0.5
	// A next-column break in the last column starts a new page instead.
	nextCol := s.Break == model.BreakNextColumn && e.cols.i+1 < len(e.cols.x)
	if (s.Break == model.BreakContinuous || nextCol) && sameSize {
		if nextCol {
			e.nextColumn()
			col := e.cols.i
			e.sec = k
			e.useSection() // the new section's margins and columns, from this column on
			e.cols.i = min(col, len(e.cols.x)-1)
			return
		}
		y := max(e.y, e.cols.maxY)
		e.sec = k
		e.useSection()
		e.y, e.cols.top, e.cols.maxY = y, y, y
		return
	}
	e.finishPage() // the footnotes go with the old page's geometry
	next := e.pageNum + 1
	if s.PageNumRestart {
		next = s.PageNumStart
	}
	if s.Break == model.BreakEvenPage && next%2 == 1 || s.Break == model.BreakOddPage && next%2 == 0 {
		e.newPage(true) // Word leaves this page blank
		if !s.PageNumRestart {
			next++
		}
	}
	e.sec = k
	e.secFirst = true
	e.pageNum = next - 1
	e.useSection()
	e.newPage(true)
}

// nextColumn continues in the next column, or on a new page after the
// last one.
func (e *Engine) nextColumn() {
	if e.cols.i+1 >= len(e.cols.x) {
		e.newPage(false)
		return
	}
	e.cols.maxY = max(e.cols.maxY, e.y)
	e.cols.i++
	e.y = e.cols.top
	e.empty = true
	e.hardBreak = false
}

// areaWidth is the width between the page's margins (all columns).
func (e *Engine) areaWidth() float64 {
	return max(e.setup.Width-math.Abs(e.setup.MarginLeft)-math.Abs(e.setup.MarginRight), 72)
}

func (e *Engine) textLeft() float64 {
	x := math.Abs(e.setup.MarginLeft)
	if e.cols.i < len(e.cols.x) {
		x += e.cols.x[e.cols.i]
	}
	return x
}

func (e *Engine) textWidth() float64 {
	if e.cols.i < len(e.cols.w) {
		return max(e.cols.w[e.cols.i], 18)
	}
	return e.areaWidth()
}

func (e *Engine) textTop() float64 { return max(math.Abs(e.setup.MarginTop), e.hfTop()) }

// textBottom is where the body text must end on the page: above the
// bottom margin (or a footer reaching higher) and the page's footnotes.
func (e *Engine) textBottom() float64 {
	return max(e.areaBottom()-e.notes.footH, e.textTop()+72)
}

// areaBottom is the bottom of the text area without the footnotes.
func (e *Engine) areaBottom() float64 {
	return max(min(e.setup.Height-math.Abs(e.setup.MarginBottom), e.hfBottom()), e.textTop()+72)
}

func (e *Engine) newPage(hard bool) {
	e.finishPage()
	e.pageNum++
	e.page = &Page{Index: len(e.Pages), Width: e.setup.Width, Height: e.setup.Height,
		Section: e.sec, First: e.secFirst, Number: e.pageNum}
	e.secFirst = false
	e.Pages = append(e.Pages, e.page)
	e.cols.i, e.cols.maxY = 0, 0
	e.y = e.textTop()
	e.cols.top = e.y
	e.empty = true
	e.hardBreak = hard
	e.lastBox = nil
}

// para returns the (cached) layout of p for a column width wide.
func (e *Engine) para(p *model.Paragraph, width float64, t *model.Table) *Para {
	e.used[p] = true
	label := e.labels[p]
	if c := e.cache[p]; c != nil && c.width == width && c.label == label && c.table == t {
		return c
	}
	c := e.layoutPara(p, width, label, t)
	e.cache[p] = c
	e.LastRelaid++
	return c
}

// paraSpacing is the space around a paragraph where it is placed, and
// its top and bottom borders (nil: none drawn).
type paraSpacing struct {
	before, after float64
	top, bottom   *model.Border
	// shadeUp and shadeDown are set when the paragraph above or below has
	// the same shading and indents: the shading then fills the space
	// between them, drawing the group as one block, as Word does.
	shadeUp, shadeDown bool
}

// spacing returns the spacing of paragraph blocks[i]: with contextual
// spacing, no space toward a neighboring paragraph of the same style.
// Neighbors with the same borders share one box, as in Word: the top
// border is drawn above the first and the bottom border below the last.
func (e *Engine) spacing(pl *Para, blocks []model.Block, i int) paraSpacing {
	sp := paraSpacing{before: pl.Before, after: pl.After}
	var prev, next *model.Paragraph
	if i > 0 {
		prev, _ = blocks[i-1].(*model.Paragraph)
	}
	if i+1 < len(blocks) {
		next, _ = blocks[i+1].(*model.Paragraph)
	}
	if pl.Props.ContextualSpacing.Bool(false) {
		style := e.paraStyle(pl.P)
		if prev != nil && e.paraStyle(prev) == style {
			sp.before = 0
		}
		if next != nil && e.paraStyle(next) == style {
			sp.after = 0
		}
	}
	bs := pl.Props.Borders
	same := func(q *model.Paragraph) bool {
		return q != nil && sameBorders(bs, e.Doc.ResolvePara(q).Borders)
	}
	if b := bs[model.EdgeTop]; b != nil && b.Width > 0 && !same(prev) {
		sp.top = b
	}
	if b := bs[model.EdgeBottom]; b != nil && b.Width > 0 && !same(next) {
		sp.bottom = b
	}
	if pl.Shading.A > 0 {
		me := e.Doc.ResolvePara(pl.P)
		sameShade := func(q *model.Paragraph) bool {
			if q == nil {
				return false
			}
			o := e.Doc.ResolvePara(q)
			return o.Shading == me.Shading && ptVal(o.IndentLeft) == ptVal(me.IndentLeft) && ptVal(o.IndentRight) == ptVal(me.IndentRight)
		}
		sp.shadeUp, sp.shadeDown = sameShade(prev), sameShade(next)
	}
	return sp
}

// ptVal is *v, or 0 for nil.
func ptVal(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func sameBorders(a, b [4]*model.Border) bool {
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) || a[i] != nil && *a[i] != *b[i] {
			return false
		}
	}
	return true
}

// borderFill is a paragraph's top or bottom border b with its top at y,
// across the paragraph's indents in a text column from x, w wide.
func borderFill(pl *Para, b *model.Border, x, w, y float64) Fill {
	c, ok := colors.ParseHex(b.Color)
	if !ok {
		c = ruleColor
	}
	return Fill{Rect{x + pl.Left, y, x + w - pl.Right, y + b.Width}, c}
}

// paraStyle returns a paragraph's style ID, the default style for none.
func (e *Engine) paraStyle(p *model.Paragraph) string {
	if p.Style != "" {
		return p.Style
	}
	if st := e.Doc.Styles.Default(model.ParagraphStyle); st != nil {
		return st.ID
	}
	return ""
}

func (e *Engine) firstLineHeight(b model.Block) float64 {
	switch b := b.(type) {
	case *model.Paragraph:
		pl := e.para(b, e.textWidth(), nil)
		return pl.Before + pl.Lines[0].Height
	case *model.Table:
		if len(b.Rows) > 0 {
			return 14
		}
	}
	return 0
}

// place adds a line to the current page, with the footnotes it
// references and the floating objects anchored in it.
func (e *Engine) place(pl Placed) {
	pg := e.page
	ref := LineRef{pg.Index, len(pg.Lines)}
	pl.order = len(e.order)
	pg.Lines = append(pg.Lines, pl)
	e.paraLines[pl.Para] = append(e.paraLines[pl.Para], ref)
	e.order = append(e.order, ref)
	e.empty = false
	if pl.Box != e.lastBox {
		e.lastBox, e.paraTop = pl.Box, pl.Y
	}
	for k := range pl.Line.Items {
		it := &pl.Line.Items[k]
		if it.Kind != ItemObject || it.Obj == nil {
			continue
		}
		if n := it.Obj.Note; n != nil && !n.Endnote {
			e.addFootnote(n)
		}
		if f := it.Obj.Float; f != nil {
			e.addFloat(f, &pl, it.X)
		}
	}
}

// placePara places a laid-out paragraph with its spacing and borders
// (see spacing).
func (e *Engine) placePara(pl *Para, sp paraSpacing, nextFirst float64) {
	before, after := sp.before, sp.after
	topB, botB := 0.0, 0.0 // room for the borders
	if b := sp.top; b != nil {
		topB = b.Width + b.Space
	}
	if b := sp.bottom; b != nil {
		botB = b.Width + b.Space
	}
	idx := e.index[pl.P]
	pp := pl.Props
	bottom := e.textBottom()
	if pp.PageBreakBefore.Bool(false) && !e.empty {
		e.newPage(true)
	}
	if e.empty && e.page.Index > 0 && !e.hardBreak {
		before = 0 // Word drops space before at the top of a page
	}
	lines := pl.Lines
	if !e.empty {
		need := 0.0
		if pp.KeepLines.Bool(false) {
			need = before + topB + pl.Height + botB
		}
		if pp.KeepNext.Bool(false) {
			need = before + topB + pl.Height + botB + after + nextFirst
		}
		if need > 0 && e.y+need > bottom && need <= bottom-e.textTop() {
			e.nextColumn()
			bottom = e.textBottom()
			before = 0
		}
	}
	e.y += before
	gapTop, gapPage := e.y-before, e.page // the space above, shaded with sp.shadeUp
	if b := sp.top; b != nil {
		if !e.empty && e.y+topB+lines[0].Height > bottom+0.01 {
			e.nextColumn()
			bottom = e.textBottom()
		}
		e.page.Fills = append(e.page.Fills, borderFill(pl, b, e.textLeft(), e.textWidth(), e.y))
		e.y += topB
	}
	widow := pp.WidowControl.Bool(false) && len(lines) > 1
	for k, l := range lines {
		e.y = e.clearFloats(e.y, l.Height)
		bottom = e.textBottom() - e.footNeed(l)
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
				e.nextColumn()
				e.y = e.clearFloats(e.y, l.Height)
			}
		}
		e.place(Placed{Line: l, Para: idx, Box: pl, X: e.textLeft(), Y: e.y, W: e.textWidth()})
		if pl.Shading.A > 0 {
			top, bot := e.y, e.y+l.Height
			if k == 0 && sp.shadeUp && sp.top == nil && e.page == gapPage && top >= gapTop {
				top = gapTop
			}
			if k == len(lines)-1 && sp.shadeDown && sp.bottom == nil && !l.PageBreak {
				bot = min(bot+after, e.textBottom())
			}
			e.page.Fills = append(e.page.Fills, Fill{Rect{e.textLeft() + pl.Left, top, e.textLeft() + e.textWidth() - pl.Right, bot}, pl.Shading})
		}
		e.y += l.Height
		if l.PageBreak {
			e.newPage(true)
		}
	}
	if b := sp.bottom; b != nil {
		e.page.Fills = append(e.page.Fills, borderFill(pl, b, e.textLeft(), e.textWidth(), e.y+b.Space))
		e.y += botB
	}
	e.y += after
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

func (e *Engine) layoutFlow(blocks []model.Block, width float64, t *model.Table) *flow {
	f := &flow{}
	y := 0.0
	for i, b := range blocks {
		switch b := b.(type) {
		case *model.Paragraph:
			pl := e.para(b, width, t)
			if _, after := prevBlock(blocks, i).(*model.Table); after && t != nil && i == len(blocks)-1 && b.Len() == 0 {
				for _, l := range pl.Lines {
					f.lines = append(f.lines, Placed{Line: l, Para: e.index[b], Box: pl, Y: max(y-l.Height, 0), W: width, collapsed: true})
				}
				continue
			}
			sp := e.spacing(pl, blocks, i)
			y += sp.before
			if b := sp.top; b != nil {
				f.fills = append(f.fills, borderFill(pl, b, 0, width, y))
				y += b.Width + b.Space
			}
			for k, l := range pl.Lines {
				f.lines = append(f.lines, Placed{Line: l, Para: e.index[b], Box: pl, Y: y, W: width})
				if pl.Shading.A > 0 {
					top, bot := y, y+l.Height
					if k == 0 && sp.shadeUp && sp.top == nil {
						top -= sp.before
					}
					if k == len(pl.Lines)-1 && sp.shadeDown && sp.bottom == nil {
						bot += sp.after
					}
					f.fills = append(f.fills, Fill{Rect{pl.Left, top, width - pl.Right, bot}, pl.Shading})
				}
				y += l.Height
			}
			if b := sp.bottom; b != nil {
				f.fills = append(f.fills, borderFill(pl, b, 0, width, y+b.Space))
				y += b.Space + b.Width
			}
			y += sp.after
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

// prevBlock returns blocks[i-1], or nil.
func prevBlock(blocks []model.Block, i int) model.Block {
	if i > 0 {
		return blocks[i-1]
	}
	return nil
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
				cb.flow = e.layoutFlow(c.Blocks, max(cb.w-2*cellMargin, 6), t)
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
		// The table's borders are hairline rules; a cell's own borders
		// are filled in their width and color, centered on its edges.
		bottom := y + rb.height
		bs := &cb.cell.Borders
		half := func(i int) float64 {
			if bs[i] != nil {
				return bs[i].Width / 2
			}
			return 0
		}
		edge := func(i int, rule, own Rect) {
			if b := bs[i]; b != nil {
				if b.Width > 0 {
					c, ok := colors.ParseHex(b.Color)
					if !ok {
						c = ruleColor
					}
					*fills = append(*fills, Fill{own, c})
				}
			} else if borders {
				*rules = append(*rules, rule)
			}
		}
		x1 := x + cb.w
		y0, y1 := y-half(model.EdgeTop), bottom+half(model.EdgeBottom)
		edge(model.EdgeLeft, Rect{x, y, x + bw, bottom}, Rect{x - half(model.EdgeLeft), y0, x + half(model.EdgeLeft), y1})
		edge(model.EdgeRight, Rect{x1, y, x1 + bw, bottom}, Rect{x1 - half(model.EdgeRight), y0, x1 + half(model.EdgeRight), y1})
		hx0, hx1 := x-half(model.EdgeLeft), x1+half(model.EdgeRight)
		if !cb.cont {
			edge(model.EdgeTop, Rect{x, y, x + cb.w + bw, y + bw}, Rect{hx0, y - half(model.EdgeTop), hx1, y + half(model.EdgeTop)})
		}
		if !continuesBelow(rows, r, cb.x) {
			edge(model.EdgeBottom, Rect{x, bottom, x + cb.w + bw, bottom + bw}, Rect{hx0, bottom - half(model.EdgeBottom), hx1, bottom + half(model.EdgeBottom)})
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

// placeTable places a table's rows. Rows linked by vertical merges are
// placed as one group. A group that doesn't fit in the rest of the page
// is split between lines, as Word lets rows break across pages, unless a
// row can't split (it then starts a new page when it fits on one).
func (e *Engine) placeTable(t *model.Table) {
	rows, x0 := e.layoutTable(t, e.textWidth())
	for r := 0; r < len(rows); {
		end := groupEnd(rows, r)
		e.placeRows(t, rows, r, end, x0)
		r = end
	}
}

// groupEnd returns the end (exclusive) of the rows from r that vertically
// merged cells hold together.
func groupEnd(rows []*rowBox, r int) int {
	end := r + 1
	for k := r; k < end; k++ {
		for _, cb := range rows[k].cells {
			if cb.flow != nil {
				end = max(end, min(k+cb.rowSpan, len(rows)))
			}
		}
	}
	return end
}

// placeRows places rows [r0, r1) of a table: laid out from y 0 as if the
// page were endless, then cut into pieces that fit the pages.
func (e *Engine) placeRows(t *model.Table, rows []*rowBox, r0, r1 int, x0 float64) {
	left := e.textLeft() + x0
	var lines []Placed
	var fills []Fill
	var rules []Rect
	var boxes []TableRowBox
	h, xr := 0.0, 0.0
	cantSplit := false
	for r := r0; r < r1; r++ {
		e.emitRow(rows, r, left, h, &lines, &fills, &rules, t.Borders)
		box := TableRowBox{Table: t, Row: r, Y0: h, Y1: h + rows[r].height}
		g := 0
		for i, cb := range rows[r].cells {
			x := left + cb.x
			if i == 0 {
				box.Edges = append(box.Edges, TableEdge{X: x, Grid: g})
			}
			g += max(cb.cell.Span, 1)
			box.Edges = append(box.Edges, TableEdge{X: x + cb.w, Grid: g})
			xr = max(xr, x+cb.w)
		}
		boxes = append(boxes, box)
		h += rows[r].height
		cantSplit = cantSplit || t.Rows[r].CantSplit
	}
	bottom := e.textBottom()
	if !e.empty && e.y+h > bottom+0.01 && cantSplit && h <= bottom-e.textTop() {
		e.nextColumn()
	}
	for s := 0.0; ; {
		bottom = e.textBottom()
		if dx := e.textLeft() + x0 - left; dx != 0 {
			// Continued in another column: move the rest over.
			left += dx
			xr += dx
			for i := range lines {
				lines[i].X += dx
			}
			for i := range fills {
				fills[i].R.X0, fills[i].R.X1 = fills[i].R.X0+dx, fills[i].R.X1+dx
			}
			for i := range rules {
				rules[i].X0, rules[i].X1 = rules[i].X0+dx, rules[i].X1+dx
			}
			for i := range boxes {
				for k := range boxes[i].Edges {
					boxes[i].Edges[k].X += dx
				}
			}
		}
		cut := math.Inf(1)
		if e.y+h-s > bottom+0.01 {
			cut = cutBefore(lines, s, s+bottom-e.y)
			if cut <= s+0.01 {
				if !e.empty {
					e.nextColumn() // nothing fits here: start in the next column or page
					continue
				}
				cut = s + bottom - e.y // a line taller than a page
			}
		}
		// Place what lies in [s, cut), moved to e.y.
		dy := e.y - s
		pg := e.page
		// The first band also keeps what hangs above the table: the upper
		// half of a thick top border is centered on y = 0.
		top := s
		if s == 0 {
			top = math.Inf(-1)
		}
		for _, f := range fills {
			if r, ok := clipY(f.R, top, cut); ok {
				pg.Fills = append(pg.Fills, Fill{shiftY(r, dy), f.C})
			}
		}
		for _, rr := range rules {
			if r, ok := clipY(rr, top, cut); ok {
				pg.Rules = append(pg.Rules, shiftY(r, dy))
			}
		}
		for _, b := range boxes {
			if r, ok := clipY(Rect{0, b.Y0, 0, b.Y1}, s, cut); ok {
				b.Y0, b.Y1 = r.Y0+dy, r.Y1+dy
				b.Edges = slices.Clone(b.Edges) // a later column moves boxes' edges, not these
				pg.TableRows = append(pg.TableRows, b)
			}
		}
		for _, pl := range lines {
			if pl.Y >= s-0.01 && pl.Y < cut-0.01 {
				pl.Y += dy
				e.place(pl)
			}
		}
		e.empty = false
		if math.IsInf(cut, 1) {
			e.y += h - s
			return
		}
		if t.Borders {
			// The split row is closed at the page bottom and reopened at the
			// top of the next page.
			pg.Rules = append(pg.Rules, Rect{left, cut + dy, xr + 0.5, cut + dy + 0.5})
			e.nextColumn()
			dx := e.textLeft() + x0 - left
			e.page.Rules = append(e.page.Rules, Rect{left + dx, e.y, xr + dx + 0.5, e.y + 0.5})
		} else {
			e.nextColumn()
		}
		s = cut
	}
}

// cutBefore returns the highest y at most limit, and above s, where no
// line from s on is cut in two; s when the first line doesn't fit.
func cutBefore(lines []Placed, s, limit float64) float64 {
	cut := limit
	for moved := true; moved; {
		moved = false
		for _, pl := range lines {
			if pl.Y >= s-0.01 && pl.Y < cut-0.01 && pl.Y+pl.Line.Height > cut+0.01 {
				cut, moved = pl.Y, true
			}
		}
	}
	return max(cut, s)
}

// clipY clips r to the band [y0, y1); ok is false when nothing is left.
func clipY(r Rect, y0, y1 float64) (Rect, bool) {
	if r.Y0 == r.Y1 {
		// A line (a horizontal border) belongs to the band it lies in.
		return r, r.Y0 >= y0 && r.Y0 < y1
	}
	// A rectangle that only touches the band (ending at y0) has nothing in it.
	r.Y0, r.Y1 = max(r.Y0, y0), min(r.Y1, y1)
	return r, r.Y1 > r.Y0
}

func shiftY(r Rect, dy float64) Rect {
	r.Y0 += dy
	r.Y1 += dy
	return r
}
