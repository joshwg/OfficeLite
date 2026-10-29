// legacy_test.go tests implicit intersection, whole columns and rows across
// sheets, 15-digit comparison and zero snapping, and ranges of errors.

package formula

import "testing"

// evalLegacy evaluates src as an ordinary (pre-dynamic-array) formula in
// the cell ref.
func evalLegacy(ctx *TestContext, ref, src string) Value {
	k := ctx.key(ref)
	n, err := Parse(src)
	if err != nil {
		return ErrName
	}
	env := NewEnv(ctx, ctx.Funcs, k[0], k[1], k[2])
	env.Legacy = true
	return env.Evaluate(n).Value
}

// TestLegacyImplicitIntersection checks that a legacy formula intersects a
// range with its row or column while a dynamic-array formula keeps the array.
func TestLegacyImplicitIntersection(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.SetRange("B1", [][]any{{1}, {2}, {3}})
	ctx.SetRange("D5", [][]any{{10, 20, 30}})
	for _, c := range []struct {
		at, src string
		want    Value
	}{
		{"A2", "=B1:B3*2", Number(4)},
		{"A3", "=-B1:B3", Number(-3)},
		{"A2", "=B1:B3", Number(2)},
		{"A2", "=B:B+1", Number(3)},
		{"A9", "=B1:B3*2", ErrValue}, // outside the range's rows
		{"E1", "=D5:F5+1", Number(21)},
		{"A2", "={1,2,3}*2", Number(2)}, // an array constant keeps its first element
	} {
		got := evalLegacy(ctx, c.at, c.src)
		if a, ok := got.(*Array); ok {
			got = a.V[0]
		}
		if !sameValue(got, c.want) {
			t.Errorf("legacy %s in %s = %#v, want %#v", c.src, c.at, got, c.want)
		}
	}
	// A dynamic-array formula keeps the whole array.
	if a, ok := ctx.EvalAt("A2", "=B1:B3*2").(*Array); !ok || a.Len() != 3 {
		t.Errorf("dynamic B1:B3*2 = %#v", ctx.EvalAt("A2", "=B1:B3*2"))
	}
}

// TestIntersectOtherSheet checks '@' on ranges of another sheet.
func TestIntersectOtherSheet(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.SetRange("Sheet2!A1", [][]any{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}})
	for _, c := range []struct {
		at, src string
		want    Value
	}{
		{"B3", "=@Sheet2!A:A", Number(7)},
		{"B3", "=@Sheet2!A1:A3", Number(7)},
		{"C1", "=@Sheet2!A2:C2", Number(6)},
		{"B2", "=@Sheet2!A1:C3", Number(5)}, // 2-D: row and column
		{"D2", "=@Sheet2!A1:C3", ErrValue},
		{"B9", "=@Sheet2!A1:A3", ErrValue},
	} {
		if got := ctx.EvalAt(c.at, c.src); !sameValue(got, c.want) {
			t.Errorf("%s in %s = %#v, want %#v", c.src, c.at, got, c.want)
		}
	}
}

// TestWholeColumnsAcrossSheets checks that whole columns (rows) on different
// sheets are trimmed to a common size and combine element by element.
func TestWholeColumnsAcrossSheets(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.SetRange("Sheet2!A1", [][]any{{1}, {2}, {3}, {4}, {5}})
	ctx.SetRange("B1", [][]any{{10}, {20}, {30}, {40}, {50}})
	ctx.Set("Z10", 1)
	a, ok := ctx.Eval("=(Sheet2!A:A>1)*B:B").(*Array)
	if !ok || a.Rows != 10 || a.Cols != 1 {
		t.Fatalf("(Sheet2!A:A>1)*B:B = %#v", ctx.Eval("=(Sheet2!A:A>1)*B:B"))
	}
	sum := 0.0
	for _, v := range a.V {
		n, ok := v.(Number)
		if !ok {
			t.Fatalf("element %#v", v)
		}
		sum += float64(n)
	}
	if sum != 140 {
		t.Errorf("sum = %v, want 140", sum)
	}
	// Whole rows likewise.
	ctx.SetRange("Sheet2!A20", [][]any{{1, 2}})
	ctx.Set("A21", 3)
	if b, ok := ctx.Eval("=Sheet2!20:20+21:21").(*Array); !ok || b.Cols != 26 || !sameValue(b.V[0], Number(4)) {
		t.Errorf("Sheet2!20:20+21:21 = %#v", ctx.Eval("=Sheet2!20:20+21:21"))
	}
}

// TestFifteenDigits checks 15-digit comparisons and the snapping of a final
// sum or difference to zero.
func TestFifteenDigits(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	for _, c := range []struct {
		src  string
		want Value
	}{
		{"=0.1+0.2=0.3", Bool(true)},
		{"=0.1+0.2<>0.3", Bool(false)},
		{"=0.1+0.2>0.3", Bool(false)},
		{"=0.3=0.30000000000001", Bool(false)}, // differs at the 14th digit
		{"=1-0.9-0.1", Number(0)},
		{"=(0.1+0.2)-0.3", Number(0)},
		{"=0.1+0.2-0.3", Number(0)},
		{"=1E-300-0", Number(1e-300)},
	} {
		if got := ctx.Eval(c.src); got != c.want { // exactly
			t.Errorf("%s = %#v, want %#v", c.src, got, c.want)
		}
	}
	// Only the final operation is rounded, as in Excel.
	if got := ctx.Eval("=1*(1-0.9-0.1)"); got == Number(0) {
		t.Errorf("1*(1-0.9-0.1) = 0, want the unrounded -2.8E-17")
	}
	if c, _ := Compare(Number(0.1+0.2), Number(0.3)); c != 0 {
		t.Errorf("Compare(0.1+0.2, 0.3) = %d", c)
	}
}

// TestRangeOfError checks that a range with an error endpoint is #REF!.
func TestRangeOfError(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	for _, src := range []string{"=#REF!:B1", "=B1:#REF!", "=" + Shift("A1:B2", -1, 0)} {
		if got := ctx.Eval(src); !sameValue(got, ErrRef) {
			t.Errorf("%s = %#v, want #REF!", src, got)
		}
	}
}
