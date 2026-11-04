// Tests that huge sizes given to the array-shaping functions are errors
// rather than allocations that exhaust memory.

package lookup

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// TestHostileShapes checks EXPAND and WRAPROWS/WRAPCOLS with huge sizes.
func TestHostileShapes(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, nil, []ftest.Case{
		{"=EXPAND({1},262144,2048)", f.ErrNum},            // 536M elements
		{"=WRAPROWS({1,2,3},9000000000000000)", f.ErrNum}, // one huge padded row
		{"=WRAPCOLS({1,2,3},9000000000000000)", f.ErrNum},
		{"=WRAPROWS({1,2,3},4)", ftest.Arr{{1, 2, 3, f.ErrNA}}},
	})
}
