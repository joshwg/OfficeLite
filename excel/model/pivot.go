package model

import "slices"

// PivotTable is a PivotTable report on a sheet: its definition (the
// source data, the fields in the Filters, Columns, Rows and Values areas
// and the report options) and, in Out, where its last update put things.
// The report's cells are ordinary cells of the sheet, written by an
// update (excel/pivot computes them, excel/edit writes them).
type PivotTable struct {
	Name string // PivotTable1, ...
	// Source is the source data as formula text without '=': a reference
	// ('Sales Data'!$A$1:$D$6) or a defined name. Its first row holds the
	// field names.
	Source string
	// Row, Col is the top-left cell of the report body; report filters go
	// above it.
	Row, Col int

	Fields []PivotField // the source's columns, by position
	// Rows and Cols are the fields on the row and column axes, outer
	// first; DataField stands for Σ Values (the value fields as items)
	// when there are two or more value fields.
	Rows, Cols []int
	Pages      []int // report filter fields, top to bottom
	Values     []PivotValue

	Layout       PivotLayout
	RepeatLabels bool // repeat outer item labels on every row (outline, tabular)
	// SubtotalsBottom puts row subtotals in "X Total" rows below their
	// group; Excel's compact and outline forms put them at the top (the
	// tabular form always at the bottom).
	SubtotalsBottom bool
	// NoRowGrand turns off the grand totals for rows (the Grand Total
	// column at the right), NoColGrand those for columns (the Grand Total
	// row at the bottom), as Excel's rowGrandTotals and colGrandTotals.
	NoRowGrand, NoColGrand bool
	// BlankRows inserts an empty row after each outer item.
	BlankRows bool
	Style     string // PivotStyleLight16 when empty

	Out PivotOut
}

// DataField is the field index of Σ Values in PivotTable.Rows or Cols.
const DataField = -2

// PivotLayout is the report layout (Design > Report Layout).
type PivotLayout uint8

const (
	LayoutCompact PivotLayout = iota // row fields in one column, indented
	LayoutOutline                    // a column per row field, outer items on their own rows
	LayoutTabular                    // a column per row field, outer items beside their first inner item
)

// PivotSort orders a field's items.
type PivotSort uint8

const (
	SortAscending PivotSort = iota // Excel's default for new reports
	SortDescending
	SortManual // PivotField.Order, then new items in source order
)

// PivotField is a source column as a PivotTable field.
type PivotField struct {
	Name    string // the source column's header
	Caption string // custom name (Field Settings), "" for Name
	// Hidden lists the keys of the items filtered out (the drop-down's
	// checklist; a report filter's selection is the items not hidden).
	Hidden     []string
	Sort       PivotSort
	Order      []string // SortManual: item keys
	NoSubtotal bool     // Subtotals: None
}

// Label is the field's caption.
func (f *PivotField) Label() string {
	if f.Caption != "" {
		return f.Caption
	}
	return f.Name
}

// IsHidden reports whether the item with key k is filtered out.
func (f *PivotField) IsHidden(k string) bool { return slices.Contains(f.Hidden, k) }

// PivotFunc summarizes a value field (Summarize Values By).
type PivotFunc uint8

const (
	FuncSum PivotFunc = iota
	FuncCount
	FuncAverage
	FuncMax
	FuncMin
	FuncProduct
	FuncCountNums
	FuncStdDev
	FuncStdDevp
	FuncVar
	FuncVarp
)

// FuncNames are the functions' names as Excel shows them, by PivotFunc.
var FuncNames = []string{"Sum", "Count", "Average", "Max", "Min", "Product", "Count Numbers", "StdDev", "StdDevp", "Var", "Varp"}

// PivotShowAs is a value field's Show Values As calculation.
type PivotShowAs uint8

const (
	ShowNormal         PivotShowAs = iota // No Calculation
	ShowPercentOfTotal                    // % of Grand Total
	ShowPercentOfCol                      // % of Column Total
	ShowPercentOfRow                      // % of Row Total
)

// ShowAsNames are the Show Values As names, by PivotShowAs.
var ShowAsNames = []string{"No Calculation", "% of Grand Total", "% of Column Total", "% of Row Total"}

// PivotValue is a field in the Values area.
type PivotValue struct {
	Field      int // source field
	Func       PivotFunc
	Name       string // "Sum of Revenue"
	NumFmtID   int    // number format of its cells; 0 and "" is General
	NumFmtCode string
	ShowAs     PivotShowAs
}

// PivotOut is where the last update put the report: written by the
// engine, not saved (a reader rebuilds it).
type PivotOut struct {
	// Area is every cell of the report, its filters included; Body the
	// table below them (Excel's location).
	Area, Body Range
	// FirstHeaderRow, FirstDataRow and FirstDataCol are offsets in Body,
	// as Excel's <location> gives them.
	FirstHeaderRow, FirstDataRow, FirstDataCol int
	Buttons                                    []PivotButton
	// RowLines are the body's rows from FirstDataRow down, ColLines its
	// columns from FirstDataCol right.
	RowLines, ColLines []PivotLine
	// Headers are the cells showing a value field's name, for renaming
	// it by typing over the name.
	Headers []PivotHeader
}

// PivotAxis is an area of the field list.
type PivotAxis uint8

const (
	AxisNone PivotAxis = iota
	AxisRows
	AxisCols
	AxisPages
	AxisValues
)

// PivotButton is a drop-down button on a report cell: a row or column
// field's header (Field -1: the compact form's Row Labels or Column
// Labels, for every field of the axis), or a report filter's value.
type PivotButton struct {
	Row, Col int
	Axis     PivotAxis
	Field    int
	Filtered bool // some item is hidden (the button shows a funnel)
}

// PivotHeader is a value field's name in a header cell.
type PivotHeader struct {
	Row, Col int
	Value    int
}

// PivotLineKind is the kind of a row or column of the report.
type PivotLineKind uint8

const (
	LineItem     PivotLineKind = iota // an item (with values when it is innermost, or subtotals at the top)
	LineLabel                         // an outer item's label without values
	LineSubtotal                      // "X Total"
	LineGrand                         // Grand Total
	LineBlank                         // an empty row after an outer item (Blank Rows)
)

// PivotLine is a row (or column) of the report's data area: the items of
// the axis fields it shows, outer first (Σ Values left out; shorter for
// subtotals and outer items, none for the grand total), and the value
// field when Σ Values is on its axis (else -1).
type PivotLine struct {
	Kind  PivotLineKind
	Items []string
	Data  int
}

// Clone copies the pivot table.
func (p *PivotTable) Clone() *PivotTable {
	c := *p
	c.Fields = make([]PivotField, len(p.Fields))
	for i, f := range p.Fields {
		f.Hidden = slices.Clone(f.Hidden)
		f.Order = slices.Clone(f.Order)
		c.Fields[i] = f
	}
	c.Rows, c.Cols, c.Pages = slices.Clone(p.Rows), slices.Clone(p.Cols), slices.Clone(p.Pages)
	c.Values = slices.Clone(p.Values)
	c.Out.Buttons = slices.Clone(p.Out.Buttons)
	c.Out.RowLines, c.Out.ColLines = slices.Clone(p.Out.RowLines), slices.Clone(p.Out.ColLines)
	c.Out.Headers = slices.Clone(p.Out.Headers)
	return &c
}

// ClonePivots copies a sheet's pivot tables.
func ClonePivots(ps []*PivotTable) []*PivotTable {
	if ps == nil {
		return nil
	}
	out := make([]*PivotTable, len(ps))
	for i, p := range ps {
		out[i] = p.Clone()
	}
	return out
}

// AxisOf reports the area field f is in (AxisValues when it is only a
// value field).
func (p *PivotTable) AxisOf(f int) PivotAxis {
	switch {
	case slices.Contains(p.Rows, f):
		return AxisRows
	case slices.Contains(p.Cols, f):
		return AxisCols
	case slices.Contains(p.Pages, f):
		return AxisPages
	}
	for _, v := range p.Values {
		if v.Field == f {
			return AxisValues
		}
	}
	return AxisNone
}

// PivotAt returns the pivot table whose report covers row, col, and its
// index in Sheet.Pivots.
func (s *Sheet) PivotAt(row, col int) (*PivotTable, int) {
	for i, p := range s.Pivots {
		if p.Out.Area.Contains(row, col) {
			return p, i
		}
	}
	return nil, -1
}

// PivotButtonAt returns the drop-down button on cell row, col, if any.
func (s *Sheet) PivotButtonAt(row, col int) (*PivotTable, PivotButton, bool) {
	for _, p := range s.Pivots {
		if !p.Out.Area.Contains(row, col) {
			continue
		}
		for _, b := range p.Out.Buttons {
			if b.Row == row && b.Col == col {
				return p, b, true
			}
		}
	}
	return nil, PivotButton{}, false
}

// shiftPivots moves the pivot tables with inserted or deleted rows (or
// columns); a report whose cells are all deleted goes away.
func (s *Sheet) shiftPivots(rows bool, at, n int) {
	out := s.Pivots[:0]
	for _, p := range s.Pivots {
		area, ok := shiftRange(p.Out.Area, rows, at, n)
		if !ok {
			continue
		}
		p.Out.Area = area
		p.Out.Body, _ = shiftRange(p.Out.Body, rows, at, n)
		if rows {
			p.Row, _ = ShiftIndex(p.Row, at, n)
		} else {
			p.Col, _ = ShiftIndex(p.Col, at, n)
		}
		for i := range p.Out.Buttons {
			b := &p.Out.Buttons[i]
			if rows {
				b.Row, _ = ShiftIndex(b.Row, at, n)
			} else {
				b.Col, _ = ShiftIndex(b.Col, at, n)
			}
		}
		for i := range p.Out.Headers {
			h := &p.Out.Headers[i]
			if rows {
				h.Row, _ = ShiftIndex(h.Row, at, n)
			} else {
				h.Col, _ = ShiftIndex(h.Col, at, n)
			}
		}
		out = append(out, p)
	}
	s.Pivots = out
}
