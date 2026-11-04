// Tests for structural edits: span shifting, spills under row and column
// edits, and pixel-to-width round trips.

package model

import (
	"reflect"
	"testing"

	"officelite/excel"
)

// TestShiftSpan checks ShiftSpan for insertions and deletions before,
// inside and after a span.
func TestShiftSpan(t *testing.T) {
	for _, tc := range []struct {
		lo, hi, at, n int
		wlo, whi      int
		wok           bool
	}{
		{2, 5, 3, 2, 2, 7, true},   // insert inside: grows
		{2, 5, 2, 2, 4, 7, true},   // insert at the start: moves
		{2, 5, 6, 2, 2, 5, true},   // insert after: unchanged
		{2, 5, 3, -2, 2, 3, true},  // delete 3..4: shrinks
		{2, 5, 0, -3, 0, 2, true},  // delete 0..2: clipped start
		{2, 5, 2, -4, 2, 1, false}, // delete all of it
	} {
		lo, hi, ok := ShiftSpan(tc.lo, tc.hi, tc.at, tc.n)
		if lo != tc.wlo || hi != tc.whi || ok != tc.wok {
			t.Errorf("ShiftSpan(%d,%d,%d,%d) = %d,%d,%v; want %d,%d,%v",
				tc.lo, tc.hi, tc.at, tc.n, lo, hi, ok, tc.wlo, tc.whi, tc.wok)
		}
	}
}

// TestDeleteRowsSpills checks that deleting a spill's anchor row empties
// its spilled values and that other spills move with rows and columns.
func TestDeleteRowsSpills(t *testing.T) {
	s := NewSheet("S")
	num := func(v float64) Cell { return Cell{Kind: KindNumber, Num: v, Text: "x"} }
	s.Set(1, 0, Cell{Formula: "SEQUENCE(3)", Dynamic: true, Kind: KindNumber})
	s.Set(2, 0, num(2))
	s.Set(3, 0, num(3))
	s.Spills[[2]int{1, 0}] = Range{R1: 1, R2: 3}
	s.Set(5, 1, Cell{Formula: "{1,2}", Dynamic: true, Kind: KindNumber})
	s.Set(5, 2, num(2))
	s.Spills[[2]int{5, 1}] = Range{R1: 5, C1: 1, R2: 5, C2: 2}

	// Deleting the anchor's row empties what it spilled.
	s.DeleteRows(1, 1)
	if s.Get(1, 0).Kind != KindEmpty || s.Get(2, 0).Kind != KindEmpty {
		t.Errorf("spilled values left behind: %+v %+v", s.Get(1, 0), s.Get(2, 0))
	}
	want := map[[2]int]Range{{4, 1}: {R1: 4, C1: 1, R2: 4, C2: 2}}
	if !reflect.DeepEqual(s.Spills, want) {
		t.Errorf("spills %v, want %v", s.Spills, want)
	}
	s.InsertCols(0, 2)
	if c := s.Get(4, 3); c.Formula != "{1,2}" {
		t.Errorf("D5 = %+v after inserting columns", c)
	}
	if rng := s.Spills[[2]int{4, 3}]; rng != (Range{R1: 4, C1: 3, R2: 4, C2: 4}) {
		t.Errorf("spill after inserting columns: %v", s.Spills)
	}
}

// TestPxToColWidth checks that PxToColWidth round-trips through
// ColWidthToPx for every pixel width, and that 0 px hides.
func TestPxToColWidth(t *testing.T) {
	for _, mdw := range []float64{6, 7, 8, 9} {
		for px := 6.0; px < 400; px++ {
			if got := ColWidthToPx(float64(PxToColWidth(px, mdw)), mdw); float64(got) != px {
				t.Fatalf("mdw %v: %vpx -> %v chars -> %vpx", mdw, px, PxToColWidth(px, mdw), got)
			}
		}
	}
	if PxToColWidth(0, 7) != 0 {
		t.Error("0px should hide")
	}
}

// TestInsertRowsClipsAtSheetEnd checks that inserting rows clips merged
// areas and validation ranges at the sheet's last row (a whole-column
// rule stays whole-column) and drops those pushed entirely off it.
func TestInsertRowsClipsAtSheetEnd(t *testing.T) {
	s := NewSheet("S")
	s.Merges = []Range{
		{R1: excel.MaxRows - 3, C1: 0, R2: excel.MaxRows - 2, C2: 1}, // pushed off
		{R1: excel.MaxRows - 6, C1: 2, R2: excel.MaxRows - 4, C2: 3}, // clipped
	}
	s.Validations = []Validation{{Ranges: []Range{{R1: 0, C1: 0, R2: excel.MaxRows - 1, C2: 0}}}}
	s.Spills = map[[2]int]Range{{excel.MaxRows - 2, 4}: {R1: excel.MaxRows - 2, C1: 4, R2: excel.MaxRows - 1, C2: 4}}
	s.InsertRows(0, 5)
	if want := []Range{{R1: excel.MaxRows - 1, C1: 2, R2: excel.MaxRows - 1, C2: 3}}; !reflect.DeepEqual(s.Merges, want) {
		t.Errorf("merges %v, want %v", s.Merges, want)
	}
	if got := s.Validations[0].Ranges; !reflect.DeepEqual(got, []Range{{R1: 5, C1: 0, R2: excel.MaxRows - 1, C2: 0}}) {
		t.Errorf("validation ranges %v, want A6 to the last row", got)
	}
	if len(s.Spills) != 0 {
		t.Errorf("spills %v, want none (anchor pushed off the sheet)", s.Spills)
	}
	s.InsertCols(0, 1)
	if got := s.Validations[0].Ranges; got[0].C2 != 1 {
		t.Errorf("validation after inserting a column: %v", got)
	}
	s.Validations = []Validation{{Ranges: []Range{{R1: 0, C1: excel.MaxCols - 2, R2: 0, C2: excel.MaxCols - 1}}}}
	s.InsertCols(0, 1)
	if got := s.Validations[0].Ranges; !reflect.DeepEqual(got, []Range{{R1: 0, C1: excel.MaxCols - 1, R2: 0, C2: excel.MaxCols - 1}}) {
		t.Errorf("validation ranges %v, want clipped to the last column", got)
	}
}
