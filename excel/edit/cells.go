// cells.go holds Insert Cells / Delete Cells with a shift, and undoable
// note and hyperlink changes.

package edit

import (
	"errors"
	"image/color"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Shift is the direction of Insert Cells / Delete Cells.
type Shift int

const (
	ShiftRight Shift = iota // insert: cells move right
	ShiftDown               // insert: cells move down
	ShiftLeft               // delete: cells move left
	ShiftUp                 // delete: cells move up
	EntireRow               // the selection's rows
	EntireCol               // the selection's columns
)

// ErrMergedShift is Excel's refusal to shift part of a merged area.
var ErrMergedShift = errors.New("We can't do that to a merged cell")

// cellsOp is an insertion or deletion of a block with a shift.
type cellsOp struct {
	rng      model.Range
	vertical bool
	insert   bool
}

// InsertCells inserts empty cells in place of r, shifting the cells
// there right or down (or inserts entire rows or columns), as Excel's
// Insert Cells dialog does. Formulas pointing into the moved cells
// follow them.
func (b *Book) InsertCells(sheet int, r model.Range, how Shift) (calc.Result, error) {
	defer b.Op("Insert Cells")()
	switch how {
	case EntireRow:
		return b.InsertRows(sheet, r.R1, r.R2-r.R1+1)
	case EntireCol:
		return b.InsertCols(sheet, r.C1, r.C2-r.C1+1)
	}
	return b.shiftCells(sheet, r, how == ShiftDown, true)
}

// DeleteCells deletes r, shifting the cells below it up or those to its
// right left (or deletes entire rows or columns). References to deleted
// cells become #REF!.
func (b *Book) DeleteCells(sheet int, r model.Range, how Shift) (calc.Result, error) {
	defer b.Op("Delete Cells")()
	switch how {
	case EntireRow:
		return b.DeleteRows(sheet, r.R1, r.R2-r.R1+1)
	case EntireCol:
		return b.DeleteCols(sheet, r.C1, r.C2-r.C1+1)
	}
	return b.shiftCells(sheet, r, how == ShiftUp, false)
}

// shiftCells inserts or deletes the block r, shifting cells down/up when
// vertical, else right/left; a block spanning whole columns (rows) shifted
// along them becomes a row (column) insertion or deletion.
func (b *Book) shiftCells(sheet int, r model.Range, vertical, insert bool) (calc.Result, error) {
	// A block spanning whole columns (rows) shifted along them is a
	// row (column) insertion.
	switch {
	case vertical && r.C1 == 0 && r.C2 == model.MaxCols-1:
		if insert {
			return b.InsertRows(sheet, r.R1, r.R2-r.R1+1)
		}
		return b.DeleteRows(sheet, r.R1, r.R2-r.R1+1)
	case !vertical && r.R1 == 0 && r.R2 == model.MaxRows-1:
		if insert {
			return b.InsertCols(sheet, r.C1, r.C2-r.C1+1)
		}
		return b.DeleteCols(sheet, r.C1, r.C2-r.C1+1)
	}
	op := &structural{kind: shiftCells, sheet: sheet, cells: &cellsOp{r, vertical, insert}}
	return b.structural(op, Where{sheet, r})
}

// applyShiftCells performs a shiftCells operation: it refuses to split a
// merged area or pivot table or push cells off the sheet, saves the sheet
// for undo, adjusts the formulas pointing into the moved strip, shifts the
// cells and recalculates.
func (b *Book) applyShiftCells(op *structural) (calc.Result, error) {
	sh := b.WB.Sheets[op.sheet]
	c := op.cells
	r := c.rng
	strip := model.ShiftStrip(r, c.vertical)
	if sh.MergeCrosses(strip) || !c.insert && sh.MergeCrosses(r) {
		return calc.Result{}, ErrMergedShift
	}
	if PivotOverlap(sh, strip) != nil {
		return calc.Result{}, ErrPivotChange
	}
	at, n := r.R1, r.R2-r.R1+1
	if !c.vertical {
		at, n = r.C1, r.C2-r.C1+1
	}
	if c.insert && !stripFits(sh, r, c.vertical, n) {
		return calc.Result{}, ErrShiftOffSheet
	}
	if !c.insert {
		n = -n
	}
	op.saved = sh.Clone()
	adjust := func(f, home string) string {
		if c.vertical {
			return formula.AdjustRowsIn(f, home, sh.Name, at, n, r.C1, r.C2)
		}
		return formula.AdjustColsIn(f, home, sh.Name, at, n, r.R1, r.R2)
	}
	b.rewriteFormulas(op, op.sheet, adjust)
	adjustSheet(sh, adjust)
	sh.ShiftCells(r, c.vertical, c.insert)
	return b.recalculateAll(), nil
}

// stripFits reports whether inserting n cells in the strip of r pushes no
// nonblank cell off the sheet.
func stripFits(sh *model.Sheet, r model.Range, vertical bool, n int) bool {
	if vertical {
		for row := model.MaxRows - n; row < sh.UsedRows(); row++ {
			for c := r.C1; c <= min(r.C2, sh.RowLen(row)-1); c++ {
				if !sh.Get(row, c).IsBlank() {
					return false
				}
			}
		}
		return true
	}
	for row := r.R1; row <= min(r.R2, sh.UsedRows()-1); row++ {
		for c := model.MaxCols - n; c < sh.RowLen(row); c++ {
			if !sh.Get(row, c).IsBlank() {
				return false
			}
		}
	}
	return true
}

// annotChange is a change to a cell's note or hyperlink, for undo.
type annotChange struct {
	sheet, row, col int
	link            bool // the hyperlink changed (else the note)
	beforeNote      *model.Note
	afterNote       *model.Note
	beforeLink      *model.Hyperlink
	afterLink       *model.Hyperlink
}

// setAnnots writes the after (or before) side of note and link changes.
func (b *Book) setAnnots(list []annotChange, after bool) {
	for _, a := range list {
		sh := b.WB.Sheets[a.sheet]
		if a.link {
			v := a.beforeLink
			if after {
				v = a.afterLink
			}
			sh.SetLink(a.row, a.col, v)
			continue
		}
		v := a.beforeNote
		if after {
			v = a.afterNote
		}
		sh.SetNote(a.row, a.col, v)
	}
}

// notePtr is a copy of the note at r, c, or nil when there is none.
func notePtr(sh *model.Sheet, r, c int) *model.Note {
	if n, ok := sh.NoteAt(r, c); ok {
		return &n
	}
	return nil
}

// linkPtr is a copy of the hyperlink at r, c, or nil when there is none.
func linkPtr(sh *model.Sheet, r, c int) *model.Hyperlink {
	if h, ok := sh.LinkAt(r, c); ok {
		return &h
	}
	return nil
}

// SetNote sets the note of a cell (nil deletes it). Undoable.
func (b *Book) SetNote(sheet, row, col int, n *model.Note) {
	defer b.Op("Edit Note")()
	sh := b.WB.Sheets[sheet]
	ch := annotChange{sheet: sheet, row: row, col: col, beforeNote: notePtr(sh, row, col), afterNote: n}
	if ch.beforeNote == nil && n == nil {
		return
	}
	b.setAnnots([]annotChange{ch}, true)
	b.record(&change{where: Where{sheet, model.Range{R1: row, C1: col, R2: row, C2: col}}, annots: []annotChange{ch}})
}

// ClearNotes deletes the notes in r. Undoable as one step.
func (b *Book) ClearNotes(sheet int, r model.Range) bool {
	defer b.Op("Clear Notes")()
	sh := b.WB.Sheets[sheet]
	var list []annotChange
	for k, n := range sh.Notes {
		if r.Contains(k[0], k[1]) {
			n := n
			list = append(list, annotChange{sheet: sheet, row: k[0], col: k[1], beforeNote: &n})
		}
	}
	if len(list) == 0 {
		return false
	}
	b.setAnnots(list, true)
	b.record(&change{where: Where{sheet, r}, annots: list})
	return true
}

// HyperlinkStyle is st with the font of Excel's Hyperlink cell style (blue,
// underlined).
func HyperlinkStyle(st model.Style) model.Style {
	st.Font.Color = color.NRGBA{R: 0x05, G: 0x63, B: 0xC1, A: 0xFF}
	st.Font.Underline = true
	return st
}

// SetHyperlink gives a cell a hyperlink and, when display is not empty,
// that text (an empty cell gets the address). The cell takes the
// Hyperlink style (blue, underlined), as in Excel. Undoable as one step.
func (b *Book) SetHyperlink(sheet, row, col int, display string, h model.Hyperlink) (calc.Result, error) {
	defer b.Op("Insert Hyperlink")()
	sh := b.WB.Sheets[sheet]
	before := snapshot(sh, spillChildren(sh), row, col)
	after := before
	if display == "" && before.IsBlank() {
		display = h.Address()
		if h.Target == "" {
			display = h.Location
		}
	}
	if display != "" && display != before.Text {
		after = model.Cell{Style: before.Style, Kind: model.KindText, Text: display}
	}
	after.Style = b.addStyle(HyperlinkStyle(b.style(before.Style)))
	where := Where{sheet, model.Range{R1: row, C1: col, R2: row, C2: col}}
	ch := annotChange{sheet: sheet, row: row, col: col, link: true, beforeLink: linkPtr(sh, row, col), afterLink: &h}
	b.setAnnots([]annotChange{ch}, true)
	cells := []cellChange{{sheet, row, col, before, after}}
	res := b.setCells(cells, true)
	b.record(&change{where: where, cells: cells, annots: []annotChange{ch}})
	return res, nil
}

// ClearHyperlinks removes the links in r, leaving the text (Excel's
// Clear Hyperlinks). Undoable as one step.
func (b *Book) ClearHyperlinks(sheet int, r model.Range) bool {
	defer b.Op("Clear Hyperlinks")()
	sh := b.WB.Sheets[sheet]
	var list []annotChange
	for k, h := range sh.Links {
		if r.Contains(k[0], k[1]) {
			h := h
			list = append(list, annotChange{sheet: sheet, row: k[0], col: k[1], link: true, beforeLink: &h})
		}
	}
	if len(list) == 0 {
		return false
	}
	b.setAnnots(list, true)
	b.record(&change{where: Where{sheet, r}, annots: list})
	return true
}
