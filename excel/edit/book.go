package edit

import (
	"time"

	"officelite/excel/calc"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// maxUndo is how many changes Undo can take back.
const maxUndo = 100

// Book is a workbook being edited.
type Book struct {
	WB *model.Workbook
	// Now is the time NOW() and TODAY() see; time.Now when nil.
	Now func() time.Time

	calc   *calc.Calculator
	nf     *numfmt.Formatter
	styles map[model.Style]uint32
	undo   []*change
	redo   []*change
}

// Where is the part of a sheet a change affected, which Undo and Redo
// select as Excel does.
type Where struct {
	Sheet int
	Range model.Range
}

// change is one undoable edit: cell contents before and after, or a
// structural change.
type change struct {
	where Where
	cells []cellChange
	st    *structural
	dims  *dimChange
	// annots are note and hyperlink changes, alone or with cells; see
	// cells.go.
	annots []annotChange
	// sheet, when set, changes sheet properties (merges, hidden, default
	// width) along with cells; see format.go.
	sheet *sheetChange
}

type cellChange struct {
	sheet, row, col int
	before, after   model.Cell
}

// New starts editing wb.
func New(wb *model.Workbook) *Book {
	return &Book{
		WB:     wb,
		calc:   calc.NewCalculator(wb, functions.Default()),
		nf:     numfmt.New(wb.Date1904),
		styles: map[model.Style]uint32{},
	}
}

// Close releases the number formatter.
func (b *Book) Close() { b.nf.Close() }

func (b *Book) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Recalculate recalculates every formula (F9).
func (b *Book) Recalculate() calc.Result { return b.calc.Full(b.now()) }

// CanUndo and CanRedo report whether there is a change to take back or
// repeat.
func (b *Book) CanUndo() bool { return len(b.undo) > 0 }
func (b *Book) CanRedo() bool { return len(b.redo) > 0 }

// Undo takes back the last change.
func (b *Book) Undo() (Where, calc.Result, bool) {
	if len(b.undo) == 0 {
		return Where{}, calc.Result{}, false
	}
	ch := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.redo = append(b.redo, ch)
	var res calc.Result
	b.setAnnots(ch.annots, false)
	switch {
	case ch.dims != nil:
		ch.dims.set(b.WB, false)
	case ch.st != nil:
		res = b.revertStructural(ch.st)
	case ch.sheet != nil:
		ch.sheet.set(b.WB, false)
		res = b.setCells(ch.cells, false)
	default:
		res = b.setCells(ch.cells, false)
	}
	return ch.where, res, true
}

// Redo repeats the last change taken back.
func (b *Book) Redo() (Where, calc.Result, bool) {
	if len(b.redo) == 0 {
		return Where{}, calc.Result{}, false
	}
	ch := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.undo = append(b.undo, ch)
	var res calc.Result
	b.setAnnots(ch.annots, true)
	switch {
	case ch.dims != nil:
		ch.dims.set(b.WB, true)
	case ch.st != nil:
		res, _ = b.applyStructural(ch.st)
	case ch.sheet != nil:
		ch.sheet.set(b.WB, true)
		res = b.setCells(ch.cells, true)
	default:
		res = b.setCells(ch.cells, true)
	}
	return ch.where, res, true
}

// record adds a change that has been applied to the undo stack.
func (b *Book) record(ch *change) {
	b.undo = append(b.undo, ch)
	if len(b.undo) > maxUndo {
		b.undo = b.undo[len(b.undo)-maxUndo:]
	}
	b.redo = nil
}

// setCells writes the after (or before) side of changes and recalculates.
func (b *Book) setCells(cells []cellChange, after bool) calc.Result {
	if len(cells) == 0 {
		return calc.Result{}
	}
	bounds := map[int]model.Range{}
	for _, c := range cells {
		v := c.before
		if after {
			v = c.after
		}
		b.WB.Sheets[c.sheet].Set(c.row, c.col, v)
		r, ok := bounds[c.sheet]
		if !ok {
			r = model.Range{R1: c.row, C1: c.col, R2: c.row, C2: c.col}
		}
		bounds[c.sheet] = model.Range{R1: min(r.R1, c.row), C1: min(r.C1, c.col), R2: max(r.R2, c.row), C2: max(r.C2, c.col)}
	}
	var areas []calc.Area
	for s, r := range bounds {
		areas = append(areas, calc.Area{Sheet: s, Range: r})
	}
	return b.calc.Update(areas, b.now())
}

// apply makes cell changes, records them and recalculates.
func (b *Book) apply(where Where, cells []cellChange) calc.Result {
	if len(cells) == 0 {
		return calc.Result{}
	}
	res := b.setCells(cells, true)
	b.record(&change{where: where, cells: cells})
	return res
}

// spillChildren reports which cells of sh hold values spilled from a
// formula in another cell. Undo snapshots store those cells empty: the
// formula refills them.
func spillChildren(sh *model.Sheet) func(r, c int) bool {
	if len(sh.Spills) == 0 {
		return func(int, int) bool { return false }
	}
	return func(r, c int) bool {
		_, _, ok := sh.SpillAnchor(r, c)
		return ok
	}
}

// snapshot is a cell's content for undo.
func snapshot(sh *model.Sheet, child func(r, c int) bool, r, c int) model.Cell {
	cell := *sh.Get(r, c)
	if cell.Formula == "" && child(r, c) {
		return model.Cell{Style: cell.Style}
	}
	return cell
}

// Enter stores typed text in a cell, as pressing Enter does, keeping the
// cell's style (but giving a General cell the number format the input
// implies). A formula that doesn't parse returns a *FormulaError and
// changes nothing.
func (b *Book) Enter(sheet, row, col int, text string) (calc.Result, error) {
	sh := b.WB.Sheets[sheet]
	old := sh.Get(row, col)
	cell, err := b.inputCell(old.Style, text)
	if err != nil {
		return calc.Result{}, err
	}
	before := snapshot(sh, spillChildren(sh), row, col)
	where := Where{sheet, model.Range{R1: row, C1: col, R2: row, C2: col}}
	return b.apply(where, []cellChange{{sheet, row, col, before, cell}}), nil
}

// inputCell builds the cell for typed text in a cell with style.
func (b *Book) inputCell(style uint32, text string) (model.Cell, error) {
	st := b.style(style)
	if st.NumFmtCode == "@" || (st.NumFmtCode == "" && st.NumFmtID == 49) {
		// Text format: everything typed is text.
		if text == "" {
			return model.Cell{Style: style}, nil
		}
		return model.Cell{Style: style, Kind: model.KindText, Text: text}, nil
	}
	in, err := ParseInput(text, b.WB.Date1904)
	if err != nil {
		return model.Cell{}, err
	}
	return b.makeCell(style, in), nil
}

func (b *Book) style(i uint32) model.Style {
	if int(i) < len(b.WB.Styles) {
		return b.WB.Styles[i]
	}
	return b.WB.Styles[0]
}

// makeCell turns parsed input into a cell with the given style.
func (b *Book) makeCell(style uint32, in Input) model.Cell {
	c := model.Cell{Style: style, Kind: in.Kind, Num: in.Num, Text: in.Text, Formula: in.Formula}
	if in.Formula != "" {
		// Typed formulas are dynamic-array formulas, as in Excel 365.
		c.Dynamic, c.Kind, c.Text = true, model.KindEmpty, ""
		return c
	}
	st := b.style(style)
	if in.Format != "" && numfmt.IsGeneral(st.NumFmtID, st.NumFmtCode) {
		st.NumFmtID, st.NumFmtCode = 0, in.Format
		c.Style = b.addStyle(st)
	}
	if c.Kind == model.KindNumber {
		c.Text = b.nf.Format(c.Num, st.NumFmtID, st.NumFmtCode)
	}
	return c
}

func (b *Book) addStyle(st model.Style) uint32 {
	if len(b.styles) == 0 {
		for i, s := range b.WB.Styles {
			if _, ok := b.styles[s]; !ok {
				b.styles[s] = uint32(i)
			}
		}
	}
	return b.WB.AddStyle(st, b.styles)
}

// usedPart limits r to the sheet's used area (a selected column must not
// mean 262,144 cells). ok is false when nothing of r is used.
func usedPart(sh *model.Sheet, r model.Range) (model.Range, bool) {
	r.R2 = min(r.R2, sh.UsedRows()-1)
	r.C2 = min(r.C2, sh.UsedCols()-1)
	return r, r.R1 <= r.R2 && r.C1 <= r.C2
}

// Clear empties the cells of r, keeping their formats (Delete).
func (b *Book) Clear(sheet int, r model.Range) calc.Result {
	sh := b.WB.Sheets[sheet]
	where := Where{sheet, r}
	u, ok := usedPart(sh, r)
	if !ok {
		return calc.Result{}
	}
	child := spillChildren(sh)
	var cells []cellChange
	for row := u.R1; row <= u.R2; row++ {
		for col := u.C1; col <= min(u.C2, sh.RowLen(row)-1); col++ {
			before := snapshot(sh, child, row, col)
			if before.Formula == "" && before.Kind == model.KindEmpty {
				continue
			}
			cells = append(cells, cellChange{sheet, row, col, before, model.Cell{Style: before.Style}})
		}
	}
	return b.apply(where, cells)
}

// FormulaText is the formula of the dynamic array spilling into row, col
// on sheet, for the formula bar; "" if the cell isn't spilled into.
func FormulaText(sh *model.Sheet, row, col int) string {
	if r, c, ok := sh.SpillAnchor(row, col); ok {
		return "=" + sh.Get(r, c).Formula
	}
	return ""
}
