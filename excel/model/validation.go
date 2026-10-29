package model

import "slices"

// Data validation: rules that restrict what may be typed into cells,
// stored as Excel stores them, a list of rules each covering a set of
// ranges (the sqref). The rules are checked by excel/edit; this file
// keeps them in place as rows, columns and cells move.

// ValidationType is what a rule allows, numbered as BIFF8's valType
// (OOXML names them none, whole, decimal, list, date, time, textLength
// and custom).
type ValidationType uint8

const (
	ValidAny ValidationType = iota
	ValidWhole
	ValidDecimal
	ValidList
	ValidDate
	ValidTime
	ValidTextLength
	ValidCustom
)

// ValidationOp is a rule's comparison, numbered as BIFF8's typOperator
// and in OOXML's ST_DataValidationOperator order.
type ValidationOp uint8

const (
	ValidBetween ValidationOp = iota
	ValidNotBetween
	ValidEqual
	ValidNotEqual
	ValidGreater
	ValidLess
	ValidGreaterEqual
	ValidLessEqual
)

// ErrorStyle is how a rule's error alert treats an invalid entry.
type ErrorStyle uint8

const (
	ErrorStop        ErrorStyle = iota // rejects it (Retry / Cancel)
	ErrorWarning                       // asks whether to keep it
	ErrorInformation                   // tells, and keeps it unless cancelled
)

// Validation is one data validation rule.
type Validation struct {
	Ranges []Range // the cells it applies to
	Type   ValidationType
	Op     ValidationOp // Whole, Decimal, Date, Time and TextLength
	// Formula1 and Formula2 (without '=') are the bounds (Formula2 only
	// for between and not between), the List source ("\"a,b,c\"" or a
	// reference) or the Custom formula. Relative references are relative
	// to the first range's top-left cell.
	Formula1, Formula2 string
	AllowBlank         bool // "Ignore blank"
	// NoDropDown hides a List rule's in-cell drop-down (OOXML's
	// confusingly named showDropDown="1").
	NoDropDown           bool
	ShowInput, ShowError bool
	ErrorStyle           ErrorStyle
	PromptTitle, Prompt  string // the input message
	ErrorTitle, Error    string // the error alert ("" for Excel's)
}

// Clone copies the rule.
func (v Validation) Clone() Validation {
	v.Ranges = slices.Clone(v.Ranges)
	return v
}

// Contains reports whether the rule covers row, col.
func (v *Validation) Contains(row, col int) bool {
	for _, r := range v.Ranges {
		if r.Contains(row, col) {
			return true
		}
	}
	return false
}

// TopLeft is the cell the rule's relative references are relative to.
func (v *Validation) TopLeft() (row, col int) {
	if len(v.Ranges) == 0 {
		return 0, 0
	}
	return v.Ranges[0].R1, v.Ranges[0].C1
}

// CloneValidations deep-copies a rule list.
func CloneValidations(vs []Validation) []Validation {
	if vs == nil {
		return nil
	}
	out := make([]Validation, len(vs))
	for i, v := range vs {
		out[i] = v.Clone()
	}
	return out
}

// ValidationAt returns the rule covering row, col, if any.
func (s *Sheet) ValidationAt(row, col int) (*Validation, bool) {
	for i := range s.Validations {
		if s.Validations[i].Contains(row, col) {
			return &s.Validations[i], true
		}
	}
	return nil, false
}

// SetValidation applies v to the ranges rs (nil v removes validation
// from them, Clear All): the cells leave whatever rule they had.
func (s *Sheet) SetValidation(rs []Range, v *Validation) {
	s.Validations = removeValidation(s.Validations, rs)
	if v != nil {
		nv := v.Clone()
		nv.Ranges = slices.Clone(rs)
		s.Validations = append(s.Validations, nv)
	}
}

// removeValidation takes the ranges rs out of every rule, dropping rules
// left with no cells.
func removeValidation(vs []Validation, rs []Range) []Validation {
	out := vs[:0:0]
	for _, v := range vs {
		ranges := v.Ranges
		for _, cut := range rs {
			ranges = subtractAll(ranges, cut)
		}
		if len(ranges) > 0 {
			v.Ranges = ranges
			out = append(out, v)
		}
	}
	return out
}

// subtractAll is the ranges rs without the cells of cut.
func subtractAll(rs []Range, cut Range) []Range {
	var out []Range
	for _, r := range rs {
		out = append(out, Subtract(r, cut)...)
	}
	return out
}

// Subtract is r without the cells of cut, as up to four ranges: the
// rows above and below cut, then the parts left and right of it.
func Subtract(r, cut Range) []Range {
	if cut.R1 > r.R2 || cut.R2 < r.R1 || cut.C1 > r.C2 || cut.C2 < r.C1 {
		return []Range{r}
	}
	var out []Range
	if cut.R1 > r.R1 {
		out = append(out, Range{R1: r.R1, C1: r.C1, R2: cut.R1 - 1, C2: r.C2})
	}
	if cut.R2 < r.R2 {
		out = append(out, Range{R1: cut.R2 + 1, C1: r.C1, R2: r.R2, C2: r.C2})
	}
	r1, r2 := max(r.R1, cut.R1), min(r.R2, cut.R2)
	if cut.C1 > r.C1 {
		out = append(out, Range{R1: r1, C1: r.C1, R2: r2, C2: cut.C1 - 1})
	}
	if cut.C2 < r.C2 {
		out = append(out, Range{R1: r1, C1: cut.C2 + 1, R2: r2, C2: r.C2})
	}
	return out
}

// Intersect is the cells r and q share; ok is false when none.
func Intersect(r, q Range) (Range, bool) {
	x := Range{R1: max(r.R1, q.R1), C1: max(r.C1, q.C1), R2: min(r.R2, q.R2), C2: min(r.C2, q.C2)}
	return x, x.R1 <= x.R2 && x.C1 <= x.C2
}

// shiftValidations moves the rules with inserted or deleted rows
// (columns): ranges grow, shrink or go, as merged areas do.
func (s *Sheet) shiftValidations(rows bool, at, n int) {
	out := s.Validations[:0]
	for _, v := range s.Validations {
		ranges := v.Ranges[:0]
		for _, r := range v.Ranges {
			if r, ok := shiftRange(r, rows, at, n); ok {
				ranges = append(ranges, r)
			}
		}
		if len(ranges) > 0 {
			v.Ranges = ranges
			out = append(out, v)
		}
	}
	s.Validations = out
}

// shiftStripValidations moves the parts of the rules inside strip for
// Insert / Delete Cells; at, n as for ShiftIndex along the shift. A rule
// spanning the insertion grows, as with whole rows.
func (s *Sheet) shiftStripValidations(strip Range, vertical bool, at, n int) {
	out := s.Validations[:0]
	for _, v := range s.Validations {
		var ranges []Range
		for _, r := range v.Ranges {
			// The part across the strip's columns (rows) shifts; the
			// parts beside it stay.
			band := Range{R1: 0, C1: strip.C1, R2: MaxRows - 1, C2: strip.C2}
			if !vertical {
				band = Range{R1: strip.R1, C1: 0, R2: strip.R2, C2: MaxCols - 1}
			}
			in, ok := Intersect(r, band)
			if !ok {
				ranges = append(ranges, r)
				continue
			}
			ranges = append(ranges, Subtract(r, band)...)
			if moved, ok := shiftRange(in, vertical, at, n); ok {
				moved.R2, moved.C2 = min(moved.R2, MaxRows-1), min(moved.C2, MaxCols-1)
				if moved.R1 <= moved.R2 && moved.C1 <= moved.C2 {
					ranges = append(ranges, moved)
				}
			}
		}
		if len(ranges) > 0 {
			v.Ranges = ranges
			out = append(out, v)
		}
	}
	s.Validations = out
}

// MoveValidations is the rules after the cells of src move (or with
// copy, are copied) by dRow, dCol: the rules of dest's cells give way to
// those coming from src.
func MoveValidations(vs []Validation, src Range, dRow, dCol int, copy bool) []Validation {
	dest := Range{R1: src.R1 + dRow, C1: src.C1 + dCol, R2: src.R2 + dRow, C2: src.C2 + dCol}
	out := make([]Validation, 0, len(vs))
	for _, v := range vs {
		var moved []Range
		for _, r := range v.Ranges {
			if in, ok := Intersect(r, src); ok {
				moved = append(moved, Range{R1: in.R1 + dRow, C1: in.C1 + dCol, R2: in.R2 + dRow, C2: in.C2 + dCol})
			}
		}
		ranges := subtractAll(slices.Clone(v.Ranges), dest)
		if !copy {
			ranges = subtractAll(ranges, src)
		}
		ranges = append(ranges, moved...)
		if len(ranges) > 0 {
			v = v.Clone()
			v.Ranges = ranges
			out = append(out, v)
		}
	}
	return out
}

// PermuteValidationRows moves the rules of data's cells with a sort:
// row data.R1+i takes the cells of row from[i].
func (s *Sheet) PermuteValidationRows(data Range, from []int) {
	out := s.Validations[:0]
	for _, v := range s.Validations {
		// The rule's column spans in each source row of data.
		spans := map[int][][2]int{}
		touched := false
		for _, r := range v.Ranges {
			in, ok := Intersect(r, data)
			if !ok {
				continue
			}
			touched = true
			for row := in.R1; row <= in.R2; row++ {
				spans[row] = append(spans[row], [2]int{in.C1, in.C2})
			}
		}
		if !touched {
			out = append(out, v)
			continue
		}
		ranges := subtractAll(v.Ranges, data)
		var cells []Range
		for i, f := range from {
			for _, sp := range spans[f] {
				cells = append(cells, Range{R1: data.R1 + i, C1: sp[0], R2: data.R1 + i, C2: sp[1]})
			}
		}
		ranges = append(ranges, mergeRows(cells)...)
		if len(ranges) > 0 {
			v.Ranges = ranges
			out = append(out, v)
		}
	}
	s.Validations = out
}

// mergeRows joins one-row ranges in row order with the same columns as
// the row above into taller ranges.
func mergeRows(cells []Range) []Range {
	slices.SortFunc(cells, func(a, b Range) int {
		if a.C1 != b.C1 {
			return a.C1 - b.C1
		}
		if a.C2 != b.C2 {
			return a.C2 - b.C2
		}
		return a.R1 - b.R1
	})
	var out []Range
	for _, c := range cells {
		if n := len(out); n > 0 && out[n-1].C1 == c.C1 && out[n-1].C2 == c.C2 && out[n-1].R2+1 == c.R1 {
			out[n-1].R2 = c.R2
			continue
		}
		out = append(out, c)
	}
	return out
}
