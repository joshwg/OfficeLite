package model

import (
	"maps"
	"slices"
)

// AutoFilter is a sheet's AutoFilter (Excel allows one per sheet): the
// filtered range, whose first row holds the headers with the drop-down
// buttons, and the criteria of its filtered columns. The rows a filter
// hides are listed in Sheet.Filtered, apart from manually hidden rows.
type AutoFilter struct {
	Range   Range
	Columns []FilterColumn // in column order, only columns with criteria
}

// FilterKind is the kind of a column's criteria.
type FilterKind uint8

const (
	// FilterValues shows the rows whose displayed value is one of Values
	// (or a date in one of Dates), and blank rows when Blanks is set:
	// the drop-down's checklist.
	FilterValues FilterKind = iota
	// FilterCustom compares with one or two conditions joined by And or
	// Or (Custom AutoFilter; Equals, Begins With, Between, ...).
	FilterCustom
	// FilterTop10 shows the Top10.N largest (or smallest) items, or
	// percent of the items.
	FilterTop10
	// FilterDynamic is a criterion computed from the data: Above Average
	// and Below Average (Dynamic is the OOXML type name). Other types
	// read from a file (dates relative to today) are kept for saving but
	// show every row when the filter is applied again.
	FilterDynamic
)

// FilterOp is a comparison of a custom filter condition, numbered as in
// OOXML's ST_FilterOperator order used here and mapped to BIFF's codes by
// the .xls reader and writer.
type FilterOp uint8

const (
	OpEqual FilterOp = iota
	OpNotEqual
	OpGreater
	OpGreaterEqual
	OpLess
	OpLessEqual
)

// FilterCond is one condition of a custom filter. Val is compared as a
// number when it is one and the cell holds a number; otherwise as text,
// where Equal and NotEqual take * and ? wildcards (Begins With is
// "abc*").
type FilterCond struct {
	Op  FilterOp
	Val string
}

// DateGroup selects dates in a values filter as Excel's date tree does:
// the year, or the month of a year, or a day, ... down to Grouping.
type DateGroup struct {
	Year, Month, Day, Hour, Minute, Second int
	Grouping                               string // year, month, day, hour, minute, second
}

// Top10 is a Top 10 filter.
type Top10 struct {
	Top     bool    // largest (else smallest)
	Percent bool    // N percent of the items (else N items)
	N       float64 // 1..500 items or 1..100 percent
	Val     float64 // the threshold found when it was applied
}

// FilterColumn is the criteria of one column of an AutoFilter.
type FilterColumn struct {
	Col  int // sheet column
	Kind FilterKind

	Values []string // FilterValues: displayed texts shown
	Dates  []DateGroup
	Blanks bool

	Conds []FilterCond // FilterCustom: one or two
	And   bool         // both conditions must hold (else either)

	Top10 Top10 // FilterTop10

	Dynamic    string  // FilterDynamic: aboveAverage, belowAverage, ...
	DynamicVal float64 // the average when it was applied

	// HideButton hides the column's drop-down button (an AutoFilter set
	// up by a macro or another program; Excel's UI never sets it).
	HideButton bool
}

// Clone copies the filter.
func (f *AutoFilter) Clone() *AutoFilter {
	if f == nil {
		return nil
	}
	c := *f
	c.Columns = make([]FilterColumn, len(f.Columns))
	for i, fc := range f.Columns {
		fc.Values = slices.Clone(fc.Values)
		fc.Dates = slices.Clone(fc.Dates)
		fc.Conds = slices.Clone(fc.Conds)
		c.Columns[i] = fc
	}
	return &c
}

// Column returns the criteria of sheet column col, if it has any.
func (f *AutoFilter) Column(col int) (*FilterColumn, bool) {
	if f == nil {
		return nil, false
	}
	for i := range f.Columns {
		if f.Columns[i].Col == col {
			return &f.Columns[i], true
		}
	}
	return nil, false
}

// SetColumn replaces the criteria of fc.Col, keeping the columns in
// order; a nil fc (with col) removes them.
func (f *AutoFilter) SetColumn(col int, fc *FilterColumn) {
	f.Columns = slices.DeleteFunc(f.Columns, func(c FilterColumn) bool { return c.Col == col })
	if fc == nil {
		return
	}
	c := *fc
	c.Col = col
	i, _ := slices.BinarySearchFunc(f.Columns, col, func(c FilterColumn, col int) int { return c.Col - col })
	f.Columns = slices.Insert(f.Columns, i, c)
}

// Data is the range's rows below the header row.
func (f *AutoFilter) Data() Range {
	r := f.Range
	r.R1++
	return r
}

// Active reports whether some column has criteria.
func (f *AutoFilter) Active() bool { return f != nil && len(f.Columns) > 0 }

// RowFiltered reports whether row is hidden by the AutoFilter.
func (s *Sheet) RowFiltered(row int) bool { return s.Filtered[row] }

// RowHidden reports whether row is hidden, by its height or a filter.
func (s *Sheet) RowHidden(row int) bool {
	if s.Filtered[row] {
		return true
	}
	h, ok := s.RowHeight[row]
	return ok && h == 0
}

// AdoptFilteredRows is for readers: a file doesn't say which hidden rows
// an AutoFilter hid, so when it has criteria (the caller checks) every
// hidden row of the filter's data is taken as filtered (shown again by
// Clear). heights holds the stored heights of those rows, which they get
// back.
func (s *Sheet) AdoptFilteredRows(heights map[int]float32) {
	f := s.AutoFilter
	if f == nil {
		return
	}
	d := f.Data()
	for r, h := range s.RowHeight {
		if h != 0 || r < d.R1 || r > d.R2 {
			continue
		}
		if s.Filtered == nil {
			s.Filtered = map[int]bool{}
		}
		s.Filtered[r] = true
		if ht, ok := heights[r]; ok && ht > 0 {
			s.RowHeight[r] = ht
		} else {
			delete(s.RowHeight, r)
		}
	}
}

// shiftFilter moves the AutoFilter and the filtered rows with inserted
// or deleted rows (or columns). A filter whose header row or every
// column is deleted goes away.
func (s *Sheet) shiftFilter(rows bool, at, n int) {
	if rows && len(s.Filtered) > 0 {
		out := make(map[int]bool, len(s.Filtered))
		for r := range s.Filtered {
			if r2, ok := ShiftIndex(r, at, n); ok {
				out[r2] = true
			}
		}
		s.Filtered = out
	}
	f := s.AutoFilter
	if f == nil {
		return
	}
	if rows {
		if _, ok := ShiftIndex(f.Range.R1, at, n); !ok {
			s.AutoFilter, s.Filtered = nil, nil
			return
		}
		f.Range, _ = shiftRange(f.Range, true, at, n)
		return
	}
	r, ok := shiftRange(f.Range, false, at, n)
	if !ok {
		s.AutoFilter, s.Filtered = nil, nil
		return
	}
	f.Range = r
	cols := f.Columns[:0]
	for _, c := range f.Columns {
		if c2, ok := ShiftIndex(c.Col, at, n); ok {
			c.Col = c2
			cols = append(cols, c)
		}
	}
	f.Columns = cols
}

func cloneFiltered(m map[int]bool) map[int]bool {
	if m == nil {
		return nil
	}
	return maps.Clone(m)
}
