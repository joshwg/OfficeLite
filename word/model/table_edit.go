package model

import (
	"math"
	"slices"
	"strings"
)

// Table editing: Insert > Table and the table commands (insert and
// delete rows and columns, Tab between cells). Every change is one undo
// step.

// cellPad is Word's default left and right cell margin, points; a cell's
// text is its grid width less twice this.
const cellPad = 5.4

// MaxTableSize is the largest number of rows or columns Insert Table
// creates (Word allows 63 columns).
const MaxTableSize = 63

// TableLoc says where a cell is: its table, the table's container and the
// cell's row, index in the row and grid column.
type TableLoc struct {
	Table  *Table
	Blocks *[]Block // the table's container (the body or a cell)
	Index  int      // the table's index in *Blocks
	Row    int
	Cell   int // index in Table.Rows[Row].Cells
	Grid   int // first grid column the cell covers
}

// CellAt returns the table cell the caret position p is in (the innermost
// one for nested tables).
func (e *Editor) CellAt(p Pos) (TableLoc, bool) {
	p = e.clamp(p)
	c := e.locs[p.Para].Cell
	if c == nil {
		return TableLoc{}, false
	}
	return findCell(&e.Doc.Body, c)
}

// InTable reports whether the caret is in a table.
func (e *Editor) InTable() bool {
	_, ok := e.CellAt(e.Sel.Caret)
	return ok
}

func findCell(blocks *[]Block, c *TableCell) (TableLoc, bool) {
	for i, b := range *blocks {
		t, ok := b.(*Table)
		if !ok {
			continue
		}
		for r, row := range t.Rows {
			g := 0
			for k, cell := range row.Cells {
				if cell == c {
					return TableLoc{Table: t, Blocks: blocks, Index: i, Row: r, Cell: k, Grid: g}, true
				}
				if tl, ok := findCell(&cell.Blocks, c); ok {
					return tl, true
				}
				g += max(cell.Span, 1)
			}
		}
	}
	return TableLoc{}, false
}

// cellParas returns the first and last paragraph of a cell (nested tables
// included).
func (e *Editor) cellParas(c *TableCell) (int, int) {
	first, last := -1, -1
	for i, l := range e.locs {
		if l.Cell == c {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

// CellBounds returns the start of a cell's first paragraph and the end of
// its last one.
func (e *Editor) CellBounds(c *TableCell) (Pos, Pos, bool) {
	a, b := e.cellParas(c)
	if a < 0 {
		return Pos{}, Pos{}, false
	}
	return Pos{Para: a}, Pos{Para: b, Off: e.locs[b].P.Len()}, true
}

// topOf is the body index of the block holding a table location.
func (e *Editor) topOf(tl TableLoc) int {
	for _, l := range e.locs {
		if l.Cell == tl.Table.Rows[0].Cells[0] {
			return l.Top
		}
	}
	return 0
}

// textWidth is the width available to a new table at a paragraph: the
// page's text column, or the text width of the cell it is in.
func (e *Editor) textWidth(loc ParaLoc) float64 {
	pg := e.Doc.Page
	if pg.Width <= 0 {
		pg = LetterPage()
	}
	w := max(pg.Width-math.Abs(pg.MarginLeft)-math.Abs(pg.MarginRight), 72)
	if loc.Cell == nil {
		return w
	}
	tl, ok := findCell(&e.Doc.Body, loc.Cell)
	if !ok {
		return w
	}
	cols := tl.Table.Columns
	cw := 0.0
	for g := tl.Grid; g < tl.Grid+max(loc.Cell.Span, 1) && g < len(cols); g++ {
		cw += cols[g]
	}
	if cw <= 0 {
		cw = loc.Cell.Width
	}
	if cw <= 0 {
		return w / 2
	}
	return max(cw-2*cellPad, 36)
}

// NewTable returns a rows × cols table width points wide with single
// borders around every cell, as Word's "Table Grid" draws it.
func NewTable(rows, cols int, width float64) *Table {
	rows, cols = min(max(rows, 1), 32767), min(max(cols, 1), MaxTableSize)
	t := &Table{Borders: true}
	cw := width / float64(cols)
	for range cols {
		t.Columns = append(t.Columns, cw)
	}
	for range rows {
		r := &TableRow{}
		for range cols {
			r.Cells = append(r.Cells, &TableCell{Span: 1, Width: cw, Blocks: []Block{&Paragraph{}}})
		}
		t.Rows = append(t.Rows, r)
	}
	return t
}

// InsertTable inserts a rows × cols table at the caret, replacing the
// selection, and puts the caret in its first cell. In the middle of a
// paragraph the paragraph is split; a paragraph always follows the table.
func (e *Editor) InsertTable(rows, cols int) {
	from, to := e.span()
	e.edit("table", from, to, false, func() {
		p := e.deleteSelection()
		loc := e.locs[p.Para]
		t := NewTable(rows, cols, e.textWidth(loc))
		for _, st := range e.Doc.Styles.Styles {
			if st.Type == TableStyle && strings.EqualFold(st.Name, "Table Grid") {
				t.Style = st.ID
			}
		}
		idx := loc.Index
		if p.Off > 0 {
			e.splitPara(p)
			loc = e.locs[p.Para]
			idx = loc.Index + 1
		}
		*loc.Blocks = slices.Insert(*loc.Blocks, idx, Block(t))
		e.reindex()
		a, _, _ := e.CellBounds(t.Rows[0].Cells[0])
		e.Sel = Selection{a, a}
	})
}

// tableEdit runs f as one undoable step on the table the caret is in.
func (e *Editor) tableEdit(kind string, f func(tl TableLoc)) bool {
	tl, ok := e.CellAt(e.Sel.Caret)
	if !ok {
		return false
	}
	top := e.topOf(tl)
	e.edit(kind, top, top, false, func() { f(tl) })
	return true
}

// emptyCellLike returns an empty cell with c's properties (width, span,
// shading...) and the formatting of its first paragraph.
func emptyCellLike(c *TableCell) *TableCell {
	n := &TableCell{Span: c.Span, Shading: c.Shading, VAlign: c.VAlign, Width: c.Width, DocxXML: c.DocxXML}
	p := &Paragraph{}
	if len(c.Blocks) > 0 {
		if q, ok := c.Blocks[0].(*Paragraph); ok {
			p.Style, p.Props = q.Style, copyParaProps(q.Props)
			p.Mark = q.Mark
			if len(q.Runs) > 0 {
				p.Mark = q.Runs[0].Props
			}
		}
	}
	n.Blocks = []Block{p}
	return n
}

// cellAtGrid returns the index of the cell of row covering grid column g,
// or -1.
func cellAtGrid(row *TableRow, g int) int {
	x := 0
	for i, c := range row.Cells {
		s := max(c.Span, 1)
		if g >= x && g < x+s {
			return i
		}
		x += s
	}
	return -1
}

// InsertRow inserts a row above or below the caret's row, with the same
// cells (empty); the caret moves into it.
func (e *Editor) InsertRow(below bool) {
	e.tableEdit("table", func(tl TableLoc) {
		t := tl.Table
		src := t.Rows[tl.Row]
		at := tl.Row
		if below {
			at++
		}
		n := &TableRow{Height: src.Height, DocxXML: src.DocxXML}
		g := 0
		for _, c := range src.Cells {
			nc := emptyCellLike(c)
			// Inside a vertical merge the new row continues it.
			nc.VMerge = 0
			if c.VMerge != 0 && at < len(t.Rows) {
				if k := cellAtGrid(t.Rows[at], g); k >= 0 && t.Rows[at].Cells[k].VMerge == 2 && (below || c.VMerge == 2) {
					nc.VMerge = 2
				}
			}
			n.Cells = append(n.Cells, nc)
			g += max(c.Span, 1)
		}
		t.Rows = slices.Insert(t.Rows, at, n)
		e.reindex()
		a, _, _ := e.CellBounds(n.Cells[min(tl.Cell, len(n.Cells)-1)])
		e.Sel = Selection{a, a}
	})
}

// InsertColumn inserts a column left or right of the caret's cell. The
// new column takes the cell's width and the table keeps its total width.
func (e *Editor) InsertColumn(right bool) {
	e.tableEdit("table", func(tl TableLoc) {
		t := tl.Table
		cell := t.Rows[tl.Row].Cells[tl.Cell]
		g := tl.Grid
		if right {
			g += max(cell.Span, 1)
		}
		cols := gridWidths(t)
		total := 0.0
		for _, c := range cols {
			total += c
		}
		w := 0.0
		for k := tl.Grid; k < tl.Grid+max(cell.Span, 1) && k < len(cols); k++ {
			w += cols[k]
		}
		w /= float64(max(cell.Span, 1))
		cols = slices.Insert(cols, min(g, len(cols)), w)
		scale := total / (total + w)
		for i := range cols {
			cols[i] *= scale
		}
		t.Columns = cols
		var added *TableCell
		for r, row := range t.Rows {
			x, at := 0, len(row.Cells)
			for i, c := range row.Cells {
				s := max(c.Span, 1)
				if g == x {
					at = i
					break
				}
				if g > x && g < x+s {
					at = -1 // a merged cell straddles the new column: widen it
					c.Span = s + 1
					break
				}
				x += s
			}
			if at < 0 || len(row.Cells) == 0 {
				continue
			}
			like := row.Cells[min(at, len(row.Cells)-1)]
			if right && at > 0 {
				like = row.Cells[at-1]
			}
			nc := emptyCellLike(like)
			nc.Span, nc.VMerge = 1, 0
			if like.Span <= 1 {
				nc.VMerge = like.VMerge
			}
			row.Cells = slices.Insert(row.Cells, at, nc)
			if r == tl.Row {
				added = nc
			}
		}
		fixCellWidths(t)
		e.reindex()
		if added != nil {
			a, _, _ := e.CellBounds(added)
			e.Sel = Selection{a, a}
		}
	})
}

// gridWidths returns the table's grid column widths, deriving them from
// the cells when the table has none.
func gridWidths(t *Table) []float64 {
	if len(t.Columns) > 0 {
		return slices.Clone(t.Columns)
	}
	n := 0
	for _, row := range t.Rows {
		k := 0
		for _, c := range row.Cells {
			k += max(c.Span, 1)
		}
		n = max(n, k)
	}
	cols := make([]float64, n)
	for _, row := range t.Rows {
		g := 0
		for _, c := range row.Cells {
			if max(c.Span, 1) == 1 && c.Width > 0 && cols[g] == 0 {
				cols[g] = c.Width
			}
			g += max(c.Span, 1)
		}
	}
	for i := range cols {
		if cols[i] == 0 {
			cols[i] = 72
		}
	}
	return cols
}

// fixCellWidths sets every cell's preferred width from the grid.
func fixCellWidths(t *Table) {
	for _, row := range t.Rows {
		g := 0
		for _, c := range row.Cells {
			s := max(c.Span, 1)
			w := 0.0
			for k := g; k < g+s && k < len(t.Columns); k++ {
				w += t.Columns[k]
			}
			if w > 0 {
				c.Width = w
			}
			g += s
		}
	}
}

// DeleteRows deletes the rows the selection touches (the caret's row);
// deleting every row deletes the table.
func (e *Editor) DeleteRows() {
	r0, r1, ok := e.selectedRows()
	if !ok {
		return
	}
	e.tableEdit("table", func(tl TableLoc) {
		t := tl.Table
		if r0 == 0 && r1 == len(t.Rows)-1 {
			e.removeTable(tl)
			return
		}
		// A vertical merge starting in a deleted row restarts below it.
		if r1+1 < len(t.Rows) {
			below := t.Rows[r1+1]
			g := 0
			for _, c := range below.Cells {
				if c.VMerge == 2 {
					started := false
					for r := r0; r <= r1; r++ {
						if k := cellAtGrid(t.Rows[r], g); k >= 0 && t.Rows[r].Cells[k].VMerge != 0 {
							started = true
						}
					}
					prevMerged := false
					if r0 > 0 {
						if k := cellAtGrid(t.Rows[r0-1], g); k >= 0 && t.Rows[r0-1].Cells[k].VMerge != 0 {
							prevMerged = true
						}
					}
					if started && !prevMerged {
						c.VMerge = 1
					}
				}
				g += max(c.Span, 1)
			}
		}
		t.Rows = slices.Delete(t.Rows, r0, r1+1)
		e.reindex()
		row := t.Rows[min(r0, len(t.Rows)-1)]
		a, _, _ := e.CellBounds(row.Cells[min(tl.Cell, len(row.Cells)-1)])
		e.Sel = Selection{a, a}
	})
}

// selectedRows returns the range of rows of the caret's table the
// selection touches.
func (e *Editor) selectedRows() (int, int, bool) {
	tl, ok := e.CellAt(e.Sel.Caret)
	if !ok {
		return 0, 0, false
	}
	r0, r1 := tl.Row, tl.Row
	if al, ok := e.CellAt(e.Sel.Anchor); ok && al.Table == tl.Table {
		r0, r1 = min(r0, al.Row), max(r1, al.Row)
	}
	return r0, r1, true
}

// selectedGrid returns the range of grid columns the selection touches in
// the caret's table.
func (e *Editor) selectedGrid() (int, int, bool) {
	tl, ok := e.CellAt(e.Sel.Caret)
	if !ok {
		return 0, 0, false
	}
	span := max(tl.Table.Rows[tl.Row].Cells[tl.Cell].Span, 1)
	g0, g1 := tl.Grid, tl.Grid+span-1
	if al, ok := e.CellAt(e.Sel.Anchor); ok && al.Table == tl.Table {
		as := max(al.Table.Rows[al.Row].Cells[al.Cell].Span, 1)
		g0, g1 = min(g0, al.Grid), max(g1, al.Grid+as-1)
	}
	return g0, g1, true
}

// DeleteColumns deletes the grid columns the selection touches (the
// caret's cell's columns); deleting every column deletes the table.
func (e *Editor) DeleteColumns() {
	g0, g1, ok := e.selectedGrid()
	if !ok {
		return
	}
	e.tableEdit("table", func(tl TableLoc) {
		t := tl.Table
		cols := gridWidths(t)
		if g0 == 0 && g1 >= len(cols)-1 {
			e.removeTable(tl)
			return
		}
		for _, row := range t.Rows {
			x := 0
			out := row.Cells[:0]
			for _, c := range row.Cells {
				s := max(c.Span, 1)
				lo, hi := max(x, g0), min(x+s-1, g1)
				x += s
				if lo <= hi {
					if overlap := hi - lo + 1; overlap < s {
						c.Span = s - overlap
					} else {
						continue
					}
				}
				out = append(out, c)
			}
			clear(row.Cells[len(out):])
			row.Cells = out
		}
		// Rows left without cells go.
		t.Rows = slices.DeleteFunc(t.Rows, func(r *TableRow) bool { return len(r.Cells) == 0 })
		if g0 < len(cols) {
			cols = slices.Delete(cols, g0, min(g1+1, len(cols)))
		}
		t.Columns = cols
		fixCellWidths(t)
		if len(t.Rows) == 0 {
			e.removeTable(tl)
			return
		}
		e.reindex()
		row := t.Rows[min(tl.Row, len(t.Rows)-1)]
		k := cellAtGrid(row, min(g0, len(cols)-1))
		if k < 0 {
			k = len(row.Cells) - 1
		}
		a, _, _ := e.CellBounds(row.Cells[k])
		e.Sel = Selection{a, a}
	})
}

// DeleteTable deletes the table the caret is in.
func (e *Editor) DeleteTable() {
	e.tableEdit("table", func(tl TableLoc) { e.removeTable(tl) })
}

// removeTable takes a table out of its container; the caret goes to the
// paragraph that followed it.
func (e *Editor) removeTable(tl TableLoc) {
	blocks := tl.Blocks
	*blocks = slices.Delete(*blocks, tl.Index, tl.Index+1)
	if len(*blocks) == 0 {
		*blocks = []Block{&Paragraph{}}
	}
	// The block now at Index (or the last one) gets the caret.
	i := min(tl.Index, len(*blocks)-1)
	var target *Paragraph
	WalkBlocks((*blocks)[i:i+1], func(p *Paragraph) bool {
		target = p
		return false
	})
	e.reindex()
	for k, l := range e.locs {
		if l.P == target {
			e.Sel = Selection{Pos{Para: k}, Pos{Para: k}}
			return
		}
	}
}

// NextCell moves to the next cell (dir 1) or the previous one (-1) and
// selects its content, as Tab and Shift+Tab do in Word. Tab in the last
// cell adds a row. It reports false when the caret isn't in a table.
func (e *Editor) NextCell(dir int) bool {
	tl, ok := e.CellAt(e.Sel.Caret)
	if !ok {
		return false
	}
	t := tl.Table
	r, k := tl.Row, tl.Cell+dir
	for {
		if k < 0 {
			r--
			if r < 0 {
				return true
			}
			k = len(t.Rows[r].Cells) - 1
		}
		if k >= len(t.Rows[r].Cells) {
			r, k = r+1, 0
			if r >= len(t.Rows) {
				e.SetCaret(e.Sel.Caret, false)
				e.InsertRow(true)
				row := t.Rows[len(t.Rows)-1]
				a, _, _ := e.CellBounds(row.Cells[0])
				e.Sel = Selection{a, a}
				return true
			}
		}
		if t.Rows[r].Cells[k].VMerge != 2 {
			break
		}
		k += dir
	}
	a, b, _ := e.CellBounds(t.Rows[r].Cells[k])
	e.Select(a, b)
	return true
}

// SelectCell, SelectRow and SelectTable select the caret's cell, its row
// or its whole table (Word's Layout > Select).
func (e *Editor) SelectCell() {
	if tl, ok := e.CellAt(e.Sel.Caret); ok {
		a, b, _ := e.CellBounds(tl.Table.Rows[tl.Row].Cells[tl.Cell])
		e.Select(a, b)
	}
}

func (e *Editor) SelectRow() {
	if tl, ok := e.CellAt(e.Sel.Caret); ok {
		row := tl.Table.Rows[tl.Row]
		a, _, _ := e.CellBounds(row.Cells[0])
		_, b, _ := e.CellBounds(row.Cells[len(row.Cells)-1])
		e.Select(a, b)
	}
}

func (e *Editor) SelectTable() {
	if tl, ok := e.CellAt(e.Sel.Caret); ok {
		t := tl.Table
		a, _, _ := e.CellBounds(t.Rows[0].Cells[0])
		last := t.Rows[len(t.Rows)-1]
		_, b, _ := e.CellBounds(last.Cells[len(last.Cells)-1])
		e.Select(a, b)
	}
}

// SnapToCells widens a selection that crosses table cells to whole
// cells, as Word selects cells once a drag leaves the first one.
func (e *Editor) SnapToCells(s Selection) Selection {
	ca, cb := e.locs[e.clamp(s.Anchor).Para].Cell, e.locs[e.clamp(s.Caret).Para].Cell
	if ca == cb {
		return s
	}
	grow := func(p Pos, c *TableCell, toEnd bool) Pos {
		if c == nil {
			return p
		}
		a, b, ok := e.CellBounds(c)
		if !ok {
			return p
		}
		if toEnd {
			return b
		}
		return a
	}
	if s.Anchor.Less(s.Caret) {
		return Selection{grow(s.Anchor, ca, false), grow(s.Caret, cb, true)}
	}
	return Selection{grow(s.Anchor, ca, true), grow(s.Caret, cb, false)}
}
