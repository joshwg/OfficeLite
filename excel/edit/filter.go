package edit

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// AutoFilter: turning it on and off, setting a column's criteria, and
// working out which rows they hide. Rows hidden by the filter are kept in
// Sheet.Filtered, apart from rows hidden by their height, so that Clear
// shows only what the filter hid. Each change is one undo step.

// ErrFilterRange is Excel's refusal to filter an empty cell.
var ErrFilterRange = errors.New("This can't be applied to the selected range. Select a single cell in a range and try again.")

// filterChange is a change of a sheet's AutoFilter and filtered rows.
type filterChange struct {
	sheet         int
	before, after filterState
}

type filterState struct {
	af       *model.AutoFilter
	filtered map[int]bool
}

func filterStateOf(sh *model.Sheet) filterState {
	return filterState{sh.AutoFilter.Clone(), cloneRows(sh.Filtered)}
}

func cloneRows(m map[int]bool) map[int]bool {
	if len(m) == 0 {
		return nil
	}
	out := make(map[int]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func (c *filterChange) set(wb *model.Workbook, after bool) {
	s := c.before
	if after {
		s = c.after
	}
	sh := wb.Sheets[c.sheet]
	sh.AutoFilter, sh.Filtered = s.af.Clone(), cloneRows(s.filtered)
}

// changeFilter applies f to the sheet's filter state and records the
// change.
func (b *Book) changeFilter(sheet int, f func(sh *model.Sheet)) {
	sh := b.WB.Sheets[sheet]
	ch := &filterChange{sheet: sheet, before: filterStateOf(sh)}
	f(sh)
	ch.after = filterStateOf(sh)
	b.visibilityChanged()
	where := model.Range{R1: sh.ActiveRow, C1: sh.ActiveCol, R2: sh.ActiveRow, C2: sh.ActiveCol}
	if af := ch.after.af; af != nil {
		where = af.Range
	} else if af := ch.before.af; af != nil {
		where = af.Range
	}
	b.record(&change{where: Where{sheet, where}, filter: ch})
}

// FilterRange is the range Filter turns the AutoFilter on for: the
// selection, or for a single cell (or a single row of cells) the data
// around it down to its last row.
func FilterRange(sh *model.Sheet, sel model.Range) (model.Range, error) {
	sel.R2 = min(sel.R2, max(sh.UsedRows()-1, sel.R1))
	sel.C2 = min(sel.C2, max(sh.UsedCols()-1, sel.C1))
	if sel.R1 == sel.R2 {
		reg := CurrentRegion(sh, sel.R1, sel.C1)
		if sel.C1 == sel.C2 {
			sel.C1, sel.C2 = reg.C1, reg.C2
			sel.R1 = reg.R1
		}
		sel.R2 = max(reg.R2, sel.R1)
	}
	for r := sel.R1; r <= sel.R2; r++ {
		for c := sel.C1; c <= sel.C2; c++ {
			if !sh.Get(r, c).IsBlank() {
				return sel, nil
			}
		}
	}
	return sel, ErrFilterRange
}

// SetAutoFilter turns the AutoFilter on for r (Data > Filter), or with
// r nil turns it off, showing the rows it hid.
func (b *Book) SetAutoFilter(sheet int, r *model.Range) {
	defer b.Op("AutoFilter")()
	b.changeFilter(sheet, func(sh *model.Sheet) {
		sh.Filtered = nil
		sh.AutoFilter = nil
		if r != nil {
			sh.AutoFilter = &model.AutoFilter{Range: *r}
		}
	})
}

// FilterColumn sets the criteria of column col (nil clears them) and
// filters the rows again.
func (b *Book) FilterColumn(sheet, col int, fc *model.FilterColumn) {
	defer b.Op("Filter")()
	b.changeFilter(sheet, func(sh *model.Sheet) {
		if sh.AutoFilter == nil {
			return
		}
		sh.AutoFilter.SetColumn(col, fc)
		b.refilter(sh)
	})
}

// ClearFilters clears every column's criteria, showing all rows (Data >
// Clear).
func (b *Book) ClearFilters(sheet int) {
	defer b.Op("Clear Filter")()
	b.changeFilter(sheet, func(sh *model.Sheet) {
		if sh.AutoFilter != nil {
			sh.AutoFilter.Columns = nil
		}
		sh.Filtered = nil
	})
}

// ReapplyFilter filters the rows again after the data changed (Data >
// Reapply); the range grows over data added right below it.
func (b *Book) ReapplyFilter(sheet int) {
	defer b.Op("Reapply Filter")()
	b.changeFilter(sheet, func(sh *model.Sheet) {
		if sh.AutoFilter != nil {
			b.refilter(sh)
		}
	})
}

// SortFiltered sorts the AutoFilter's rows by column col (the drop-down's
// Sort A to Z) and filters them again, as one undo step.
func (b *Book) SortFiltered(sheet, col int, desc bool) (calc.Result, error) {
	defer b.Op("Sort")()
	sh := b.WB.Sheets[sheet]
	if sh.AutoFilter == nil {
		return calc.Result{}, nil
	}
	b.BeginGroup()
	defer b.EndGroup()
	res, err := b.Sort(sheet, SortSpec{Range: grownRange(sh), Keys: []SortKey{{Col: col, Descending: desc}}, Header: true})
	if err != nil {
		return res, err
	}
	b.ReapplyFilter(sheet)
	return res, nil
}

// grownRange is the filter's range extended over rows of data added
// right below it, as Excel extends it when it filters again.
func grownRange(sh *model.Sheet) model.Range {
	rng := sh.AutoFilter.Range
	for r := rng.R2 + 1; r < sh.UsedRows(); r++ {
		filled := false
		for c := rng.C1; c <= rng.C2; c++ {
			if !sh.Get(r, c).IsBlank() {
				filled = true
				break
			}
		}
		if !filled {
			break
		}
		rng.R2 = r
	}
	return rng
}

// refilter works out the rows the criteria hide.
func (b *Book) refilter(sh *model.Sheet) {
	f := sh.AutoFilter
	f.Range = grownRange(sh)
	sh.Filtered = nil
	if !f.Active() {
		return
	}
	ev := b.newFilterEval(sh, f)
	d := f.Data()
	for r := d.R1; r <= min(d.R2, sh.UsedRows()-1); r++ {
		if !ev.show(r, -1) {
			if sh.Filtered == nil {
				sh.Filtered = map[int]bool{}
			}
			sh.Filtered[r] = true
		}
	}
}

// FilterCount is how many of the filter's data rows are shown, of all
// of them ("N of M records found").
func FilterCount(sh *model.Sheet) (shown, total int) {
	f := sh.AutoFilter
	if f == nil {
		return 0, 0
	}
	d := f.Data()
	total = max(d.R2-d.R1+1, 0)
	hidden := 0
	for r := range sh.Filtered {
		if r >= d.R1 && r <= d.R2 {
			hidden++
		}
	}
	return total - hidden, total
}

// filterEval tests rows against a filter's columns. The thresholds of
// Top 10 and average filters are found over all the data rows.
type filterEval struct {
	b    *Book
	sh   *model.Sheet
	f    *model.AutoFilter
	cols []model.FilterColumn
}

func (b *Book) newFilterEval(sh *model.Sheet, f *model.AutoFilter) *filterEval {
	ev := &filterEval{b: b, sh: sh, f: f, cols: f.Columns}
	for i := range f.Columns {
		fc := &f.Columns[i]
		switch fc.Kind {
		case model.FilterTop10:
			fc.Top10.Val = ev.top10(fc)
		case model.FilterDynamic:
			if fc.Dynamic == "aboveAverage" || fc.Dynamic == "belowAverage" {
				fc.DynamicVal = ev.average(fc.Col)
			}
		}
	}
	return ev
}

// numbers lists the numbers of column col's data rows.
func (ev *filterEval) numbers(col int) []float64 {
	d := ev.f.Data()
	var xs []float64
	for r := d.R1; r <= min(d.R2, ev.sh.UsedRows()-1); r++ {
		if c := ev.sh.Get(r, col); c.Kind == model.KindNumber {
			xs = append(xs, c.Num)
		}
	}
	return xs
}

func (ev *filterEval) average(col int) float64 {
	xs := ev.numbers(col)
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// top10 is the threshold of a Top 10 filter: the smallest value shown
// (the largest, for Bottom).
func (ev *filterEval) top10(fc *model.FilterColumn) float64 {
	xs := ev.numbers(fc.Col)
	if len(xs) == 0 {
		return 0
	}
	slices.Sort(xs)
	if fc.Top10.Top {
		slices.Reverse(xs)
	}
	n := int(fc.Top10.N)
	if fc.Top10.Percent {
		n = int(math.Floor(float64(len(xs)) * fc.Top10.N / 100))
	}
	n = min(max(n, 1), len(xs))
	return xs[n-1]
}

// show reports whether row r passes every column's criteria but skip's.
func (ev *filterEval) show(r, skip int) bool {
	for i := range ev.cols {
		fc := &ev.cols[i]
		if fc.Col != skip && !ev.b.matchColumn(ev.sh, fc, ev.sh.Get(r, fc.Col)) {
			return false
		}
	}
	return true
}

// matchColumn reports whether a cell passes a column's criteria.
func (b *Book) matchColumn(sh *model.Sheet, fc *model.FilterColumn, c *model.Cell) bool {
	blank := c.Kind == model.KindEmpty || c.Kind == model.KindText && c.Text == ""
	switch fc.Kind {
	case model.FilterValues:
		if blank {
			return fc.Blanks
		}
		for _, v := range fc.Values {
			if strings.EqualFold(v, c.Text) {
				return true
			}
		}
		if len(fc.Dates) > 0 && c.Kind == model.KindNumber && b.isDate(c) {
			for _, g := range fc.Dates {
				if b.matchDate(g, c.Num) {
					return true
				}
			}
		}
		return false
	case model.FilterCustom:
		if len(fc.Conds) == 0 {
			return true
		}
		ok := matchCond(fc.Conds[0], c, blank)
		if len(fc.Conds) > 1 {
			ok2 := matchCond(fc.Conds[1], c, blank)
			if fc.And {
				ok = ok && ok2
			} else {
				ok = ok || ok2
			}
		}
		return ok
	case model.FilterTop10:
		if c.Kind != model.KindNumber {
			return false
		}
		if fc.Top10.Top {
			return c.Num >= fc.Top10.Val
		}
		return c.Num <= fc.Top10.Val
	case model.FilterDynamic:
		switch fc.Dynamic {
		case "aboveAverage":
			return c.Kind == model.KindNumber && c.Num > fc.DynamicVal
		case "belowAverage":
			return c.Kind == model.KindNumber && c.Num < fc.DynamicVal
		}
	}
	return true
}

func (b *Book) isDate(c *model.Cell) bool {
	st := b.style(c.Style)
	return numfmt.IsDate(st.NumFmtID, st.NumFmtCode)
}

// matchDate reports whether serial falls in a date group.
func (b *Book) matchDate(g model.DateGroup, serial float64) bool {
	t := formula.SerialToTime(serial, b.WB.Date1904)
	have := []int{t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()}
	want := []int{g.Year, g.Month, g.Day, g.Hour, g.Minute, g.Second}
	n := map[string]int{"year": 1, "month": 2, "day": 3, "hour": 4, "minute": 5, "second": 6}[g.Grouping]
	if n == 0 {
		n = 3
	}
	return slices.Equal(have[:n], want[:n])
}

// matchCond tests one custom condition: numbers against a number, text
// (the displayed text) against text, with wildcards for = and <>.
func matchCond(cd model.FilterCond, c *model.Cell, blank bool) bool {
	if cd.Val == "" {
		switch cd.Op {
		case model.OpEqual:
			return blank
		case model.OpNotEqual:
			return !blank
		}
		return false
	}
	if v, err := strconv.ParseFloat(cd.Val, 64); err == nil && c.Kind == model.KindNumber {
		return compareOp(cd.Op, cmp.Compare(c.Num, v))
	}
	switch cd.Op {
	case model.OpEqual:
		return formula.WildcardMatch(cd.Val, c.Text)
	case model.OpNotEqual:
		return !formula.WildcardMatch(cd.Val, c.Text)
	}
	if blank || c.Kind == model.KindNumber {
		return false
	}
	return compareOp(cd.Op, strings.Compare(strings.ToLower(c.Text), strings.ToLower(cd.Val)))
}

func compareOp(op model.FilterOp, c int) bool {
	switch op {
	case model.OpEqual:
		return c == 0
	case model.OpNotEqual:
		return c != 0
	case model.OpGreater:
		return c > 0
	case model.OpGreaterEqual:
		return c >= 0
	case model.OpLess:
		return c < 0
	case model.OpLessEqual:
		return c <= 0
	}
	return false
}

// FilterItem is one entry of a filter drop-down's checklist: a distinct
// displayed value of the column, and whether the column's criteria show
// it.
type FilterItem struct {
	Text    string // "" for (Blanks)
	Checked bool
}

// FilterItems lists the distinct displayed values of column col among the
// rows the other columns' criteria show: numbers (and dates) in order,
// then text, then (Blanks).
func (b *Book) FilterItems(sheet, col int) []FilterItem {
	sh := b.WB.Sheets[sheet]
	f := sh.AutoFilter
	if f == nil {
		return nil
	}
	ev := b.newFilterEval(sh, f.Clone())
	fc, filtered := f.Column(col)
	type entry struct {
		item FilterItem
		num  bool
		v    float64
	}
	seen := map[string]int{}
	var list []entry
	d := f.Data()
	for r := d.R1; r <= min(d.R2, sh.UsedRows()-1); r++ {
		if !ev.show(r, col) {
			continue
		}
		c := sh.Get(r, col)
		text := c.Text
		if c.Kind == model.KindEmpty {
			text = ""
		}
		key := strings.ToLower(text)
		checked := !filtered || b.matchColumn(sh, fc, c)
		if i, ok := seen[key]; ok {
			list[i].item.Checked = list[i].item.Checked || checked
			continue
		}
		seen[key] = len(list)
		list = append(list, entry{FilterItem{text, checked}, c.Kind == model.KindNumber, c.Num})
	}
	slices.SortStableFunc(list, func(x, y entry) int {
		switch {
		case (x.item.Text == "") != (y.item.Text == ""):
			if x.item.Text == "" {
				return 1
			}
			return -1
		case x.num != y.num:
			if x.num {
				return -1
			}
			return 1
		case x.num:
			return cmp.Compare(x.v, y.v)
		}
		return strings.Compare(strings.ToLower(x.item.Text), strings.ToLower(y.item.Text))
	})
	out := make([]FilterItem, len(list))
	for i, e := range list {
		out[i] = e.item
	}
	return out
}

// ColumnKind is what a filter column holds, which picks Excel's Text,
// Number or Date Filters menu: the most common kind among its data.
type ColumnKind int

const (
	ColumnText ColumnKind = iota
	ColumnNumber
	ColumnDate
)

// FilterColumnKind reports what column col of the AutoFilter mostly
// holds.
func (b *Book) FilterColumnKind(sheet, col int) ColumnKind {
	sh := b.WB.Sheets[sheet]
	f := sh.AutoFilter
	if f == nil {
		return ColumnText
	}
	var n [3]int
	d := f.Data()
	for r := d.R1; r <= min(d.R2, sh.UsedRows()-1); r++ {
		c := sh.Get(r, col)
		switch {
		case c.Kind == model.KindNumber && b.isDate(c):
			n[ColumnDate]++
		case c.Kind == model.KindNumber:
			n[ColumnNumber]++
		case c.Kind == model.KindText && c.Text != "":
			n[ColumnText]++
		}
	}
	switch {
	case n[ColumnDate] > n[ColumnText] && n[ColumnDate] >= n[ColumnNumber]:
		return ColumnDate
	case n[ColumnNumber] > n[ColumnText]:
		return ColumnNumber
	}
	return ColumnText
}
