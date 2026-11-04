// panic_test.go checks that a worksheet function that panics fails only its
// own cell.

package calc

import (
	"io"
	"log"
	"testing"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// TestFunctionPanicIsContained checks that a panicking function gives
// #VALUE! in its cell while the rest of the workbook still calculates.
func TestFunctionPanicIsContained(t *testing.T) {
	reg := formula.NewRegistry()
	reg.Add(formula.Spec{Name: "BOOM", Fn: func(*formula.Env, []formula.Value) formula.Value {
		var a []formula.Value
		return a[1] // index out of range
	}})
	defer log.SetOutput(log.Writer())
	log.SetOutput(io.Discard)
	wb := book(map[string]model.Cell{
		"A1": fx("BOOM()"),
		"A3": fx("1+1"),
	})
	Recalculate(wb, reg, Full, now)
	if c := get(wb, "A1"); c.Kind != model.KindError || c.Text != string(formula.ErrValue) {
		t.Errorf("A1 = %v %q, want #VALUE!", c.Kind, c.Text)
	}
	if c := get(wb, "A3"); c.Num != 2 {
		t.Errorf("A3 = %v, want 2", c.Num)
	}
}
