package edit

import (
	"errors"
	"slices"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// SortKey is one level of a sort: a sheet column and its order.
type SortKey struct {
	Col        int
	Descending bool
}

// SortSpec is a sort of Range's rows by up to three keys. With Header the
// first row stays where it is.
type SortSpec struct {
	Range     model.Range
	Keys      []SortKey
	Header    bool
	MatchCase bool
}

// ErrSortMerged is Excel's refusal to sort a range with merged cells.
var ErrSortMerged = errors.New("To do this, all the merged cells need to be the same size")

// Sort sorts the rows of spec.Range as Excel does: whole rows of the
// range move with their styles, notes and links; relative references in
// the moved formulas shift with them; blanks sort last in either order.
// Undoable as one step.
func (b *Book) Sort(sheet int, spec SortSpec) (calc.Result, error) {
	if len(spec.Keys) == 0 {
		return calc.Result{}, nil
	}
	sh := b.WB.Sheets[sheet]
	r := spec.Range
	r.R2 = min(r.R2, max(sh.UsedRows()-1, r.R1))
	r.C2 = min(r.C2, max(sh.UsedCols()-1, r.C1))
	spec.Range = r
	op := &structural{kind: sortRows, sheet: sheet, sort: &spec}
	return b.structural(op, Where{sheet, r})
}

func (b *Book) applySort(op *structural) (calc.Result, error) {
	sh := b.WB.Sheets[op.sheet]
	spec := op.sort
	r := spec.Range
	first := r.R1
	if spec.Header {
		first++
	}
	if first >= r.R2 {
		return b.recalculateAll(), nil
	}
	data := model.Range{R1: first, C1: r.C1, R2: r.R2, C2: r.C2}
	for _, m := range sh.Merges {
		if m.R2 >= data.R1 && m.R1 <= data.R2 && m.C2 >= data.C1 && m.C1 <= data.C2 {
			return calc.Result{}, ErrSortMerged
		}
	}
	op.saved = sh.Clone()
	old := op.saved
	perm := make([]int, data.R2-data.R1+1)
	for i := range perm {
		perm[i] = data.R1 + i
	}
	slices.SortStableFunc(perm, func(x, y int) int {
		for _, k := range spec.Keys {
			if c := compareSort(old.Get(x, k.Col), old.Get(y, k.Col), k.Descending, spec.MatchCase); c != 0 {
				return c
			}
		}
		return 0
	})
	for i, from := range perm {
		to := data.R1 + i
		if from == to {
			continue
		}
		for c := data.C1; c <= data.C2; c++ {
			cell := *old.Get(from, c)
			if cell.Formula != "" {
				cell.Formula = formula.Shift(cell.Formula, to-from, 0)
			}
			sh.SetNote(to, c, notePtr(old, from, c))
			sh.SetLink(to, c, linkPtr(old, from, c))
			if cell == (model.Cell{}) && (to >= sh.UsedRows() || c >= sh.RowLen(to)) {
				continue
			}
			sh.Set(to, c, cell)
		}
	}
	// Spill areas of moved formulas move with them.
	for a := range old.Spills {
		if !data.Contains(a[0], a[1]) {
			continue
		}
		delete(sh.Spills, a)
	}
	for i, from := range perm {
		for c := data.C1; c <= data.C2; c++ {
			if rng, ok := old.Spills[[2]int{from, c}]; ok {
				d := data.R1 + i - from
				rng.R1, rng.R2 = rng.R1+d, rng.R2+d
				sh.Spills[[2]int{data.R1 + i, c}] = rng
			}
		}
	}
	return b.recalculateAll(), nil
}

// sortRank orders kinds as Excel sorts them ascending: numbers, text,
// logical values, errors; blanks always come last.
func sortRank(c *model.Cell) int {
	switch c.Kind {
	case model.KindNumber:
		return 0
	case model.KindText:
		if c.Text == "" {
			return 4
		}
		return 1
	case model.KindBool:
		return 2
	case model.KindError:
		return 3
	}
	return 4
}

func compareSort(a, b *model.Cell, desc, matchCase bool) int {
	ra, rb := sortRank(a), sortRank(b)
	if ra == 4 || rb == 4 {
		return ra - rb // blanks last either way
	}
	c := 0
	switch {
	case ra != rb:
		c = ra - rb
	case ra == 0 || ra == 2:
		switch {
		case a.Num < b.Num:
			c = -1
		case a.Num > b.Num:
			c = 1
		}
		if ra == 2 && c == 0 {
			c = strings.Compare(a.Text, b.Text) // FALSE < TRUE
		}
	default:
		x, y := a.Text, b.Text
		if !matchCase {
			x, y = strings.ToLower(x), strings.ToLower(y)
		}
		c = strings.Compare(x, y)
	}
	if desc {
		return -c
	}
	return c
}

// CurrentRegion is the block of data around row, col bounded by blank
// rows and columns (Excel's Ctrl+Shift+*).
func CurrentRegion(sh *model.Sheet, row, col int) model.Range {
	r := model.Range{R1: row, C1: col, R2: row, C2: col}
	filled := func(rr, cc int) bool {
		return rr >= 0 && cc >= 0 && !isBlank(sh.Get(rr, cc))
	}
	lineFilled := func(r1, c1, r2, c2 int) bool {
		for rr := r1; rr <= r2; rr++ {
			for cc := c1; cc <= c2; cc++ {
				if filled(rr, cc) {
					return true
				}
			}
		}
		return false
	}
	for grew := true; grew; {
		grew = false
		if r.R1 > 0 && lineFilled(r.R1-1, r.C1-1, r.R1-1, r.C2+1) {
			r.R1--
			grew = true
		}
		if r.R2 < sh.UsedRows()-1 && lineFilled(r.R2+1, r.C1-1, r.R2+1, r.C2+1) {
			r.R2++
			grew = true
		}
		if r.C1 > 0 && lineFilled(r.R1-1, r.C1-1, r.R2+1, r.C1-1) {
			r.C1--
			grew = true
		}
		if r.C2 < sh.UsedCols()-1 && lineFilled(r.R1-1, r.C2+1, r.R2+1, r.C2+1) {
			r.C2++
			grew = true
		}
	}
	return r
}

// GuessHeader reports whether r's first row looks like a header row, as
// Excel guesses for Sort: every cell of it is text, and below it some
// column holds a number, or the header's font differs (bold, say).
func (b *Book) GuessHeader(sheet int, r model.Range) bool {
	sh := b.WB.Sheets[sheet]
	if r.R2 <= r.R1 {
		return false
	}
	differs := false
	for c := r.C1; c <= min(r.C2, sh.UsedCols()-1); c++ {
		h, d := sh.Get(r.R1, c), sh.Get(r.R1+1, c)
		if h.Kind != model.KindText {
			if isBlank(h) {
				continue
			}
			return false
		}
		if d.Kind != model.KindText && !isBlank(d) {
			differs = true
		}
		if b.style(h.Style).Font != b.style(d.Style).Font {
			differs = true
		}
	}
	return differs
}
