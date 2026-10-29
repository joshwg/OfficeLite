package edit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Subtotals (Data > Subtotal): a row after (or before) each group of rows
// with equal values in one column, holding SUBTOTAL formulas, a Grand
// row over the whole list, and the row outline: level 1 shows the grand
// total, level 2 the subtotals, level 3 the detail. Remove All takes the
// rows out again. Each is one undo step.

// SubtotalFunc is a function of the Subtotal dialog's "Use function".
type SubtotalFunc struct {
	Name  string // in the dialog
	Num   int    // SUBTOTAL's function_num
	Label string // after the group's value: "<value> Total"
}

// SubtotalFuncs are the dialog's functions, in its order.
var SubtotalFuncs = []SubtotalFunc{
	{"Sum", 9, "Total"},
	{"Count", 3, "Count"},
	{"Average", 1, "Average"},
	{"Max", 4, "Max"},
	{"Min", 5, "Min"},
	{"Product", 6, "Product"},
	{"Count Numbers", 2, "Count"},
	{"StdDev", 7, "StdDev"},
	{"StdDevp", 8, "StdDevp"},
	{"Var", 10, "Var"},
	{"Varp", 11, "Varp"},
}

// SubtotalSpec is what the Subtotal dialog asks for.
type SubtotalSpec struct {
	Range   model.Range // the list, its first row the headers
	GroupBy int         // "At each change in": a sheet column
	Func    int         // index into SubtotalFuncs
	Cols    []int       // "Add subtotal to": sheet columns
	Replace bool        // replace current subtotals
	Above   bool        // summary rows above the data (else below)
}

// ErrSubtotalRange is the refusal to subtotal a list without data rows.
var ErrSubtotalRange = errors.New("Microsoft Excel cannot determine which row in your list or selection contains column labels, which are required for this command.")

// SubtotalRange is the list Subtotal works on: the selection, or for a
// single cell the data around it.
func SubtotalRange(sh *model.Sheet, sel model.Range) model.Range {
	if sel.R1 == sel.R2 && sel.C1 == sel.C2 {
		return CurrentRegion(sh, sel.R1, sel.C1)
	}
	sel.R2 = min(sel.R2, max(sh.UsedRows()-1, sel.R1))
	sel.C2 = min(sel.C2, max(sh.UsedCols()-1, sel.C1))
	return sel
}

// isSubtotalRow reports whether row r holds a SUBTOTAL formula in
// columns c1..c2: a row Subtotal added.
func isSubtotalRow(sh *model.Sheet, r, c1, c2 int) bool {
	for c := c1; c <= min(c2, sh.RowLen(r)-1); c++ {
		if f := sh.Get(r, c).Formula; f != "" && strings.Contains(strings.ToUpper(f), "SUBTOTAL(") {
			return true
		}
	}
	return false
}

// applyRowSet inserts op.n rows before each of op.rows (n > 0) or
// deletes each of op.rows (n < 0), as one structural change: formulas
// are adjusted once for all of them.
func (b *Book) applyRowSet(op *structural) (calc.Result, error) {
	sh := b.WB.Sheets[op.sheet]
	rows := slices.Clone(op.rows)
	slices.Sort(rows)
	if op.n > 0 && !fits(sh, true, len(rows)) {
		return calc.Result{}, ErrShiftOffSheet
	}
	op.saved = sh.Clone()
	// Runs of (at, count), bottom first so that the rows above keep
	// their numbers.
	type run struct{ at, n int }
	var runs []run
	for i := len(rows) - 1; i >= 0; i-- {
		at := rows[i]
		switch {
		case len(runs) > 0 && op.n > 0 && runs[len(runs)-1].at == at:
			runs[len(runs)-1].n++
		case len(runs) > 0 && op.n < 0 && runs[len(runs)-1].at == at+1:
			runs[len(runs)-1].at, runs[len(runs)-1].n = at, runs[len(runs)-1].n+1
		default:
			runs = append(runs, run{at, 1})
		}
	}
	adjust := func(f, home string) string {
		for _, r := range runs {
			n := r.n
			if op.n < 0 {
				n = -n
			}
			f = formula.AdjustRows(f, home, sh.Name, r.at, n)
		}
		return f
	}
	b.rewriteFormulas(op, op.sheet, adjust)
	adjustSheet(sh, adjust)
	for _, r := range runs {
		if op.n > 0 {
			sh.InsertRows(r.at, r.n)
		} else {
			sh.DeleteRows(r.at, r.n)
		}
	}
	return b.recalculateAll(), nil
}

// rowSet records the insertion (or deletion) of rows.
func (b *Book) rowSet(sheet int, rows []int, insert bool) (calc.Result, error) {
	n := 1
	if !insert {
		n = -1
	}
	lo, hi := slices.Min(rows), slices.Max(rows)
	return b.structural(&structural{kind: rowSet, sheet: sheet, n: n, rows: rows},
		Where{sheet, model.Range{R1: lo, R2: hi, C2: model.MaxCols - 1}})
}

// RemoveSubtotals deletes the subtotal rows of list r and its row outline
// (Subtotal > Remove All), showing the rows it hid. It returns the list
// as it is afterwards.
func (b *Book) RemoveSubtotals(sheet int, r model.Range) (model.Range, calc.Result, error) {
	defer b.Op("Remove Subtotals")()
	b.BeginGroup()
	defer b.EndGroup()
	return b.removeSubtotals(sheet, r)
}

func (b *Book) removeSubtotals(sheet int, r model.Range) (model.Range, calc.Result, error) {
	sh := b.WB.Sheets[sheet]
	var del []int
	for row := r.R1 + 1; row <= r.R2; row++ {
		if isSubtotalRow(sh, row, r.C1, r.C2) {
			del = append(del, row)
		}
	}
	var res calc.Result
	if len(del) > 0 {
		var err error
		if res, err = b.rowSet(sheet, del, false); err != nil {
			return r, res, err
		}
		r.R2 -= len(del)
	}
	b.changeOutline(sheet, r, func(sh *model.Sheet) bool {
		changed := false
		for row := r.R1; row <= r.R2; row++ {
			if sh.Level(true, row) > 0 || sh.IsCollapsed(true, row) {
				sh.SetIndexHidden(true, row, false)
				sh.SetLevel(true, row, 0)
				sh.SetCollapsedFlag(true, row, false)
				changed = true
			}
		}
		return changed
	})
	return r, res, nil
}

// Subtotal adds subtotals to the list spec.Range (Data > Subtotal) and
// returns the list as it is afterwards. Without Replace, the subtotals
// already there stay, and the new ones nest inside their groups (with no
// second Grand row).
func (b *Book) Subtotal(sheet int, spec SubtotalSpec) (model.Range, calc.Result, error) {
	defer b.Op("Subtotal")()
	r := spec.Range
	if r.R2 <= r.R1 || spec.Func < 0 || spec.Func >= len(SubtotalFuncs) || len(spec.Cols) == 0 {
		return r, calc.Result{}, ErrSubtotalRange
	}
	fn := SubtotalFuncs[spec.Func]
	b.BeginGroup()
	defer b.EndGroup()
	var res calc.Result
	if spec.Replace {
		var err error
		if r, res, err = b.removeSubtotals(sheet, r); err != nil {
			return r, res, err
		}
	}
	sh := b.WB.Sheets[sheet]

	// The groups: runs of rows with equal values in the change column,
	// broken by the subtotal rows already there.
	type group struct{ lo, hi int }
	var groups []group
	nested := false
	key := func(row int) string { return strings.ToLower(sh.Get(row, spec.GroupBy).Text) }
	for row := r.R1 + 1; row <= r.R2; row++ {
		if isSubtotalRow(sh, row, r.C1, r.C2) {
			nested = true
			continue
		}
		if n := len(groups); n > 0 && groups[n-1].hi == row-1 && key(row) == key(groups[n-1].lo) {
			groups[n-1].hi = row
			continue
		}
		groups = append(groups, group{row, row})
	}
	if len(groups) == 0 {
		return r, res, ErrSubtotalRange
	}

	// Where rows go in (before the row numbered so now), in order: a
	// group's row below it or above it; the Grand row at the end, or at
	// the top below the headers.
	type insert struct {
		at    int
		group int // -1 for the Grand row
	}
	var ins []insert
	if spec.Above && !nested {
		ins = append(ins, insert{r.R1 + 1, -1})
	}
	for i, g := range groups {
		if spec.Above {
			ins = append(ins, insert{g.lo, i})
		} else {
			ins = append(ins, insert{g.hi + 1, i})
		}
	}
	if !spec.Above && !nested {
		ins = append(ins, insert{r.R2 + 1, -1})
	}
	ats := make([]int, len(ins))
	for i, x := range ins {
		ats[i] = x.at
	}
	sres, err := b.rowSet(sheet, ats, true)
	if err != nil {
		return r, res, err
	}
	res = joinResults(res, sres)
	// final is where an old row is now; ins[k] is at ins[k].at+k.
	final := func(row int) int {
		n := 0
		for _, x := range ins {
			if x.at <= row {
				n++
			}
		}
		return row + n
	}
	end := r.R2 + len(ins) // every row went in inside the list or right below it
	list := model.Range{R1: r.R1, C1: r.C1, R2: end, C2: r.C2}

	// The new rows' cells: the label in the change column, SUBTOTAL in
	// the chosen ones.
	var cells []cellChange
	bold := func(row, col int) uint32 {
		st := b.style(sh.Get(row, col).Style)
		st.Font.Bold = true
		return b.addStyle(st)
	}
	put := func(row, col int, c model.Cell) {
		cells = append(cells, cellChange{sheet, row, col, *sh.Get(row, col), c})
	}
	ref := func(col, r1, r2 int) string {
		return model.CellName(r1, col) + ":" + model.CellName(r2, col)
	}
	var subRows []int
	grandRow, first, last := -1, final(r.R1+1), end
	for k, x := range ins {
		row := x.at + k
		var lo, hi int
		var label string
		if x.group < 0 {
			grandRow = row
			label = "Grand " + fn.Label
			if spec.Above {
				lo, hi = row+1, end
			} else {
				lo, hi = first, row-1
			}
		} else {
			g := groups[x.group]
			subRows = append(subRows, row)
			lo, hi = final(g.lo), final(g.hi)
			label = sh.Get(lo, spec.GroupBy).Text + " " + fn.Label
		}
		near := lo // a detail row, whose formats the new cells take
		put(row, spec.GroupBy, model.Cell{Kind: model.KindText, Text: label, Style: bold(near, spec.GroupBy)})
		for _, c := range spec.Cols {
			if c == spec.GroupBy {
				continue
			}
			put(row, c, model.Cell{Formula: fmt.Sprintf("SUBTOTAL(%d,%s)", fn.Num, ref(c, lo, hi)), Style: sh.Get(near, c).Style})
		}
	}
	if spec.Above && grandRow >= 0 {
		first = grandRow + 1
	}
	res = joinResults(res, b.apply(Where{sheet, list}, cells))

	// The outline: one level for the Grand row's group, one more for
	// each group's detail; a subtotal row sits at its detail's old level.
	b.changeOutline(sheet, list, func(sh *model.Sheet) bool {
		sh.Outline.SummaryAbove = spec.Above
		isSub := map[int]bool{}
		for _, s := range subRows {
			isSub[s] = true
		}
		if grandRow >= 0 {
			for row := first; row <= last; row++ {
				if row != grandRow {
					sh.SetLevel(true, row, sh.Level(true, row)+1)
				}
			}
		}
		for k, x := range ins {
			if x.group < 0 {
				continue
			}
			g := groups[x.group]
			lo, hi := final(g.lo), final(g.hi)
			sh.SetLevel(true, x.at+k, sh.Level(true, lo))
			for row := lo; row <= hi; row++ {
				sh.SetLevel(true, row, sh.Level(true, row)+1)
			}
		}
		for row := list.R1; row <= list.R2; row++ {
			sh.SetCollapsedFlag(true, row, false)
		}
		return true
	})
	return list, res, nil
}
