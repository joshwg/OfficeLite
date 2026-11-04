// Tests that functions stepping through periods one by one reject period
// counts that would hang the calculation.

package financial

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// TestHugePeriods checks CUMIPMT, CUMPRINC, DB and VDB with huge periods.
func TestHugePeriods(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, nil, []ftest.Case{
		{"=CUMIPMT(0.1,1E+300,1000,1,1E+300,0)", f.ErrNum},
		{"=CUMPRINC(0.1,1E+300,1000,1E+300,1E+300,0)", f.ErrNum}, // p++ stalls
		{"=DB(1000,1,1E+300,1E+300)", f.ErrNum},
		{"=VDB(1000,1,1E+300,0,1E+300)", f.ErrNum},
		{"=VDB(1000,1,1E+300,0,1E+300,2,TRUE)", f.ErrNum},
	})
}
