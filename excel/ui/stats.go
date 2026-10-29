// stats.go computes the status bar's Average / Count / Sum for the selection.

package ui

import (
	"fmt"

	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// statsCache holds the status bar's Average/Count/Sum for the current
// selection, recomputed only when the selection changes.
type statsCache struct {
	sheet *model.Sheet
	rng   model.Range
	value string
	valid bool
}

// text returns the stats for range r of sheet sh, recomputing them only when
// the sheet or range changed since the last call.
func (s *statsCache) text(sh *model.Sheet, r model.Range) string {
	if !s.valid || s.sheet != sh || s.rng != r {
		s.sheet, s.rng, s.valid = sh, r, true
		s.value = selectionStats(sh, r)
	}
	return s.value
}

// selectionStats mirrors Excel's status bar: nothing for a single cell,
// otherwise Count of non-empty cells and, if any are numbers, Average and Sum.
func selectionStats(sh *model.Sheet, r model.Range) string {
	if r.R1 == r.R2 && r.C1 == r.C2 {
		return ""
	}
	var count, nums int
	var sum float64
	r2 := min(r.R2, sh.UsedRows()-1)
	c2 := min(r.C2, sh.UsedCols()-1)
	for row := r.R1; row <= r2; row++ {
		for col := r.C1; col <= c2; col++ {
			c := sh.Get(row, col)
			if c.Kind == model.KindEmpty {
				continue
			}
			count++
			if c.Kind == model.KindNumber {
				nums++
				sum += c.Num
			}
		}
	}
	switch {
	case count == 0:
		return ""
	case nums == 0:
		return fmt.Sprintf("Count: %d", count)
	default:
		return fmt.Sprintf("Average: %s    Count: %d    Sum: %s",
			numfmt.General(sum/float64(nums)), count, numfmt.General(sum))
	}
}
