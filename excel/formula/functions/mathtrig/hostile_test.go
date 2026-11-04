// Tests that huge or hostile arguments give errors (or answers) promptly
// instead of overflowing, hanging or exhausting memory.

package mathtrig

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// TestHostileSizes checks array sizes whose product overflows int and
// conversions of huge floats to int.
func TestHostileSizes(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=SEQUENCE(4294967296,4294967296)", f.ErrNum}, // product wraps to 0
		{"=SEQUENCE(3037000500,3037000500)", f.ErrNum}, // product wraps negative
		{"=RANDARRAY(4294967296,4294967296)", f.ErrNum},
		{"=SEQUENCE(262144,2048)", f.ErrNum}, // 536M elements: ~9 GB
		{"=SUM(SEQUENCE(3,2))", 21},
		{"=MUNIT(1E+300)", f.ErrValue}, // int(1e300) is negative on amd64
		{"=SUM(MUNIT(3))", 3},
	})
}

// TestHostileCounts checks discrete distributions and PERMUT with counts
// so large that stepping one by one would never finish (past 2^53 x++
// leaves x unchanged).
func TestHostileCounts(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=PERMUT(1E+300,0)", 1},
		{"=PERMUT(1E+300,2)", f.ErrNum}, // overflows
		{"=PERMUT(5,2)", 20},
		{"=BINOM.DIST.RANGE(1E+300,0.5,1E+300)", ftest.Approx{V: 0, Tol: 1e-12}},
		{"=BINOM.DIST.RANGE(1E+10,0.5,0,1E+10)", ftest.Approx{V: 1, Tol: 1e-9}},
		{"=BINOM.DIST.RANGE(1E+10,0.5,0,5E+9)", ftest.Approx{V: 0.5, Tol: 1e-3}},
		{"=BINOM.DIST.RANGE(60,0.75,45,50)", near(0.523629)}, // Microsoft's example
		{"=CRITBINOM(1E+10,0.5,0.5)", ftest.Approx{V: 5e9, Tol: 1}},
		{"=BINOM.INV(6,0.5,0.75)", 4}, // Microsoft's example
		{"=HYPGEOM.DIST(1E+9,2E+9,1E+9,3E+9,TRUE)", f.ErrNum},
	})
}
