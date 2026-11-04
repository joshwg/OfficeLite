// Tests that MAKEARRAY rejects sizes whose product overflows.

package logical

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// TestMakeArrayOverflow checks that rows×columns overflowing int (2^32 ×
// 2^32 wraps to 0) is #VALUE! instead of a loop of 2^64 lambda calls.
func TestMakeArrayOverflow(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, nil, []ftest.Case{
		{"=MAKEARRAY(4294967296,4294967296,LAMBDA(r,c,r))", f.ErrValue},
		{"=MAKEARRAY(3037000500,3037000500,LAMBDA(r,c,r))", f.ErrValue},
		{"=MAKEARRAY(2,2,LAMBDA(r,c,r*c))", ftest.Arr{{1, 2}, {2, 4}}},
	})
}
