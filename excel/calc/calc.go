// Package calc recalculates a workbook's formulas with the formula engine
// and writes the results (formatted with each cell's number format) back
// into the model.
package calc

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
	"officelite/excel/pivot"
)

// Mode selects which formulas are calculated.
type Mode int

const (
	// Missing calculates only formulas without a stored value (CSV
	// formulas, files written by tools that don't calculate). Values Excel
	// saved are kept, which is what Excel shows on open.
	Missing Mode = iota
	// Full recalculates every formula (F9).
	Full
)

// Result summarizes a recalculation.
type Result struct {
	Calculated int      // formulas evaluated
	Circular   []string // cells in circular references, e.g. "Sheet1!B2"
}

type key struct{ sheet, row, col int }

const (
	pending = iota + 1
	computing
	done
)

type engine struct {
	wb     *model.Workbook
	funcs  *formula.Registry
	nf     *numfmt.Formatter
	now    time.Time
	rnd    *rand.Rand
	state  map[key]uint8
	parsed map[key]formula.Node
	used   map[int][2]int
	res    Result
	// dynamic lists each sheet's dynamic-array formulas still to be
	// calculated, which may spill into cells read before them.
	dynamic map[int][]key
}

// Recalculate evaluates the workbook's formulas according to mode.
func Recalculate(wb *model.Workbook, funcs *formula.Registry, mode Mode, now time.Time) Result {
	e := newEngine(wb, funcs, now)
	defer e.nf.Close()
	return e.recalculate(mode)
}

// recalculate evaluates the workbook's formulas according to mode, using
// any formulas already in e.parsed.
func (e *engine) recalculate(mode Mode) Result {
	wb := e.wb
	var todo []key
	for si, sh := range wb.Sheets {
		if mode == Full {
			// Spilled values are recomputed from their formulas.
			for anchor, rng := range sh.Spills {
				clearSpill(sh, anchor, rng)
			}
		}
		rows := sh.UsedRows()
		for r := 0; r < rows; r++ {
			cols := sh.RowLen(r)
			for c := 0; c < cols; c++ {
				cell := sh.Get(r, c)
				if cell.Formula == "" {
					continue
				}
				if mode == Missing && hasValue(cell) {
					continue
				}
				k := key{si, r, c}
				e.state[k] = pending
				todo = append(todo, k)
				if cell.Dynamic {
					e.dynamic[si] = append(e.dynamic[si], k)
				}
			}
		}
	}
	for _, k := range todo {
		if e.state[k] == pending {
			e.compute(k)
		}
	}
	return e.res
}

// newEngine returns an engine for wb with no formula scheduled, its clock
// at now and its random numbers seeded from it. The caller closes e.nf.
func newEngine(wb *model.Workbook, funcs *formula.Registry, now time.Time) *engine {
	return &engine{
		wb:      wb,
		funcs:   funcs,
		nf:      numfmt.New(wb.Date1904),
		now:     now,
		rnd:     rand.New(rand.NewSource(now.UnixNano())),
		state:   map[key]uint8{},
		parsed:  map[key]formula.Node{},
		used:    map[int][2]int{},
		dynamic: map[int][]key{},
	}
}

// hasValue reports whether a formula cell already holds a calculated
// value (an empty string result counts).
func hasValue(c *model.Cell) bool {
	return c.Kind != model.KindEmpty
}

// clearSpill empties the cells a previous result spilled into (not the
// anchor, which holds the formula).
func clearSpill(sh *model.Sheet, anchor [2]int, rng model.Range) {
	for r := rng.R1; r <= rng.R2; r++ {
		for c := rng.C1; c <= rng.C2; c++ {
			if r == anchor[0] && c == anchor[1] {
				continue
			}
			cell := sh.Get(r, c)
			if cell.Formula == "" && cell.Kind != model.KindEmpty {
				sh.Set(r, c, model.Cell{Style: cell.Style})
			}
		}
	}
	delete(sh.Spills, anchor)
}

// compute evaluates the formula of cell k (parsing it once; #NAME? when it
// doesn't parse) and stores the result.
func (e *engine) compute(k key) {
	sh := e.wb.Sheets[k.sheet]
	cell := sh.Get(k.row, k.col)
	e.state[k] = computing
	node, ok := e.parsed[k]
	if !ok {
		var err error
		node, err = formula.Parse(cell.Formula)
		if err != nil {
			node = nil
		}
		e.parsed[k] = node
	}
	var res formula.Result
	if node == nil {
		res.Value = formula.ErrName
	} else {
		env := formula.NewEnv(e, e.funcs, k.sheet, k.row, k.col)
		env.Legacy = !cell.Dynamic
		res = env.Evaluate(node)
	}
	e.state[k] = done
	e.res.Calculated++
	e.store(k, res)
}

// store writes a result into the formula's cell, spilling arrays.
func (e *engine) store(k key, res formula.Result) {
	sh := e.wb.Sheets[k.sheet]
	cell := sh.Get(k.row, k.col)
	arr, isArr := res.Value.(*formula.Array)
	if !isArr {
		e.setValue(sh, k.row, k.col, res.Value, res.Format)
		return
	}
	if !cell.Dynamic {
		// A legacy (non-array) formula shows only the first element (its
		// ranges were implicitly intersected; this is an array constant
		// or a function's array result).
		e.setValue(sh, k.row, k.col, arr.V[0], res.Format)
		return
	}
	anchor := [2]int{k.row, k.col}
	if old, ok := sh.Spills[anchor]; ok {
		clearSpill(sh, anchor, old)
	}
	rng := model.Range{R1: k.row, C1: k.col, R2: k.row + arr.Rows - 1, C2: k.col + arr.Cols - 1}
	if rng.R2 >= model.MaxRows || rng.C2 >= model.MaxCols || e.blocked(sh, k, rng) {
		e.setValue(sh, k.row, k.col, formula.ErrSpill, "")
		return
	}
	sh.Spills[anchor] = rng
	if u, ok := e.used[k.sheet]; ok {
		// Whole columns read from now on reach the new values.
		e.used[k.sheet] = [2]int{max(u[0], rng.R2+1), max(u[1], rng.C2+1)}
	}
	for r := 0; r < arr.Rows; r++ {
		for c := 0; c < arr.Cols; c++ {
			v := arr.At(r, c)
			if _, ok := v.(formula.Blank); ok && (r > 0 || c > 0) {
				v = formula.Number(0)
			}
			e.setValue(sh, k.row+r, k.col+c, v, res.Format)
		}
	}
}

// blocked reports whether a spill range overlaps data or another spill.
func (e *engine) blocked(sh *model.Sheet, k key, rng model.Range) bool {
	for r := rng.R1; r <= rng.R2; r++ {
		for c := rng.C1; c <= rng.C2; c++ {
			if r == k.row && c == k.col {
				continue
			}
			cell := sh.Get(r, c)
			if cell.Formula != "" || cell.Kind != model.KindEmpty {
				return true
			}
			for anchor, other := range sh.Spills {
				if anchor != [2]int{k.row, k.col} && other.Contains(r, c) {
					return true
				}
			}
		}
	}
	return false
}

// setValue stores v in a cell, keeping its formula and style.
func (e *engine) setValue(sh *model.Sheet, row, col int, v formula.Value, hint string) {
	cell := *sh.Get(row, col)
	cell.Num, cell.Text = 0, ""
	switch x := v.(type) {
	case formula.Number:
		cell.Kind, cell.Num = model.KindNumber, float64(x)
		cell.Text = e.format(cell.Style, float64(x), hint)
	case formula.String:
		cell.Kind, cell.Text = model.KindText, string(x)
	case formula.Bool:
		cell.Kind, cell.Text = model.KindBool, "FALSE"
		if x {
			cell.Num, cell.Text = 1, "TRUE"
		}
	case formula.Error:
		cell.Kind, cell.Text = model.KindError, string(x)
	case formula.Blank, formula.Missing:
		// A formula referring to an empty cell shows 0.
		cell.Kind, cell.Text = model.KindNumber, "0"
	default:
		cell.Kind, cell.Text = model.KindError, string(formula.ErrValue)
	}
	sh.Set(row, col, cell)
}

// format renders a number with the cell's number format, or with the
// function's suggested format (dates) when the cell is General.
func (e *engine) format(style uint32, v float64, hint string) string {
	var st model.Style
	if int(style) < len(e.wb.Styles) {
		st = e.wb.Styles[style]
	}
	id, code := st.NumFmtID, st.NumFmtCode
	if numfmt.IsGeneral(id, code) && hint != "" {
		id, code = 0, hint
	}
	return e.nf.Format(v, id, code)
}

// --- formula.Context ------------------------------------------------------

// CellValue returns a cell's value, calculating its formula first when it
// is pending (0 for a circular reference, which is recorded) and any
// dynamic array that may spill into an empty cell.
func (e *engine) CellValue(sheet, row, col int) formula.Value {
	if sheet < 0 || sheet >= len(e.wb.Sheets) {
		return formula.ErrRef
	}
	k := key{sheet, row, col}
	switch e.state[k] {
	case pending:
		e.compute(k)
	case computing:
		e.res.Circular = append(e.res.Circular, fmt.Sprintf("%s!%s", e.wb.Sheets[sheet].Name, model.CellName(row, col)))
		return formula.Number(0)
	}
	c := e.wb.Sheets[sheet].Get(row, col)
	if c.IsBlank() && e.spillPending(sheet, row, col) {
		c = e.wb.Sheets[sheet].Get(row, col)
	}
	return cellValue(c)
}

// spillPending calculates uncalculated dynamic-array formulas above and to
// the left of an empty cell, since one of them may spill into it. It
// reports whether any were calculated.
func (e *engine) spillPending(sheet, row, col int) bool {
	ran := false
	for _, k := range e.dynamic[sheet] {
		if e.state[k] == pending && k.row <= row && k.col <= col {
			e.compute(k)
			ran = true
		}
	}
	return ran
}

// CellValue is a stored cell's value as formulas see it.
func CellValue(c *model.Cell) formula.Value { return cellValue(c) }

// cellValue converts a stored cell to a formula value; empty cells are
// Blank.
func cellValue(c *model.Cell) formula.Value {
	switch c.Kind {
	case model.KindNumber:
		return formula.Number(c.Num)
	case model.KindText:
		return formula.String(c.Text)
	case model.KindBool:
		return formula.Bool(c.Num != 0)
	case model.KindError:
		return formula.Error(c.Text)
	}
	return formula.Blank{}
}

// CellFormula returns a cell's formula text, "" for a constant or a
// sheet that doesn't exist.
func (e *engine) CellFormula(sheet, row, col int) string {
	if sheet < 0 || sheet >= len(e.wb.Sheets) {
		return ""
	}
	return e.wb.Sheets[sheet].Get(row, col).Formula
}

// SheetIndex returns the index of the sheet named name (ignoring case),
// or -1.
func (e *engine) SheetIndex(name string) int {
	for i, sh := range e.wb.Sheets {
		if strings.EqualFold(sh.Name, name) {
			return i
		}
	}
	return -1
}

// SheetName returns the name of sheet, or "" when it doesn't exist.
func (e *engine) SheetName(sheet int) string {
	if sheet < 0 || sheet >= len(e.wb.Sheets) {
		return ""
	}
	return e.wb.Sheets[sheet].Name
}

// SheetCount returns the number of sheets.
func (e *engine) SheetCount() int { return len(e.wb.Sheets) }

// UsedSize returns sheet's stored extent (at least 1×1), cached for the
// recalculation and grown as dynamic arrays spill.
func (e *engine) UsedSize(sheet int) (rows, cols int) {
	if u, ok := e.used[sheet]; ok {
		return u[0], u[1]
	}
	sh := e.wb.Sheets[sheet]
	rows, cols = max(sh.UsedRows(), 1), max(sh.UsedCols(), 1)
	e.used[sheet] = [2]int{rows, cols}
	return rows, cols
}

// UsedSizeFor is UsedSize after calculating the dynamic arrays that could
// spill into r (a whole-column or whole-row reference about to be
// trimmed to the used area): =SUM(B:B) above B1's =SEQUENCE(10).
func (e *engine) UsedSizeFor(r formula.Ref) (rows, cols int) {
	if r.Sheet < 0 || r.Sheet >= len(e.wb.Sheets) {
		return 1, 1
	}
	list := e.dynamic[r.Sheet]
	for _, k := range list {
		if e.state[k] == pending && k.row <= r.R2 && k.col <= r.C2 {
			e.compute(k)
		}
	}
	// Drop the calculated ones so whole columns read later skip them (a new
	// slice: a nested call may be iterating the old one).
	var still []key
	for _, k := range list {
		if e.state[k] == pending {
			still = append(still, k)
		}
	}
	e.dynamic[r.Sheet] = still
	return e.UsedSize(r.Sheet)
}

// Name returns the formula of defined name name as seen from sheet: the
// sheet's own name first, else the workbook-level one.
func (e *engine) Name(name string, sheet int) (string, bool) {
	var global string
	found := false
	for _, n := range e.wb.Names {
		if !strings.EqualFold(n.Name, name) {
			continue
		}
		if n.Sheet == sheet {
			return n.Formula, true // a sheet-level name wins
		}
		if n.Sheet < 0 {
			global, found = n.Formula, true
		}
	}
	return global, found
}

// Spill returns the range the dynamic array at sheet, row, col spilled
// into, calculating it first when it is pending.
func (e *engine) Spill(sheet, row, col int) (formula.Ref, bool) {
	if sheet < 0 || sheet >= len(e.wb.Sheets) {
		return formula.Ref{}, false
	}
	k := key{sheet, row, col}
	if e.state[k] == pending {
		e.compute(k)
	}
	rng, ok := e.wb.Sheets[sheet].Spills[[2]int{row, col}]
	if !ok {
		return formula.Ref{}, false
	}
	return formula.Ref{Sheet: sheet, R1: rng.R1, C1: rng.C1, R2: rng.R2, C2: rng.C2}, true
}

// Now returns the recalculation's fixed current time.
func (e *engine) Now() time.Time { return e.now }

// RowHidden reports whether a row is hidden, by its height or by the
// AutoFilter: what SUBTOTAL and AGGREGATE need to skip hidden rows (not
// part of formula.Context; functions find it by a type assertion).
func (e *engine) RowHidden(sheet, row int) bool {
	return sheet >= 0 && sheet < len(e.wb.Sheets) && e.wb.Sheets[sheet].RowHidden(row)
}

// RowFiltered reports whether the AutoFilter hid a row (not part of
// formula.Context; functions find it by a type assertion).
func (e *engine) RowFiltered(sheet, row int) bool {
	return sheet >= 0 && sheet < len(e.wb.Sheets) && e.wb.Sheets[sheet].RowFiltered(row)
}

// PivotData is GETPIVOTDATA's lookup: the value of the report covering
// row, col for dataField and the field/item pairs (not part of
// formula.Context; the function finds it by a type assertion).
func (e *engine) PivotData(sheet, row, col int, dataField string, pairs []formula.Value) formula.Value {
	if sheet < 0 || sheet >= len(e.wb.Sheets) {
		return formula.ErrRef
	}
	pt, _ := e.wb.Sheets[sheet].PivotAt(row, col)
	if pt == nil {
		return formula.ErrRef
	}
	r, c, ok := pivot.GetData(pt, dataField, pairs)
	if !ok {
		return formula.ErrRef
	}
	return e.CellValue(sheet, r, c)
}

// Date1904 reports whether the workbook counts dates from 1904, and Rand
// returns the next random number in [0, 1).
func (e *engine) Date1904() bool { return e.wb.Date1904 }
func (e *engine) Rand() float64  { return e.rnd.Float64() }

// Override stands in a value for one cell while a formula is evaluated:
// the entry being checked by a data validation rule.
type Override struct {
	Sheet, Row, Col int
	Value           formula.Value
}

type overrideCtx struct {
	*engine
	o *Override
}

// CellValue returns the override's value for its cell, else the engine's.
func (c overrideCtx) CellValue(sheet, row, col int) formula.Value {
	if o := c.o; o != nil && o.Sheet == sheet && o.Row == row && o.Col == col {
		return o.Value
	}
	return c.engine.CellValue(sheet, row, col)
}

// Evaluate calculates formula text (without '=') as if it were in the cell
// at sheet, row, col, reading the workbook's current values without
// recalculating anything (data validation formulas); o, when set, stands
// in one cell's value. ref is the range the formula refers to when its
// result is a reference.
func Evaluate(wb *model.Workbook, funcs *formula.Registry, src string, sheet, row, col int, now time.Time, o *Override) (v formula.Value, ref *formula.Ref) {
	n, err := formula.Parse(src)
	if err != nil {
		return formula.ErrName, nil
	}
	e := newEngine(wb, funcs, now)
	defer e.nf.Close()
	env := formula.NewEnv(overrideCtx{e, o}, funcs, sheet, row, col)
	if r, ok := env.Eval(n).(formula.Ref); ok {
		ref = &r
	}
	return env.Evaluate(n).Value, ref
}
