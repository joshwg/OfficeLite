package model

import (
	"reflect"
	"testing"
)

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
