// core_test.go tests the evaluator core: operators, array broadcasting,
// parse errors, number text, date serials and criteria matching.

package formula

import (
	"math"
	"testing"
)

// evalIn evaluates src in ctx from its default cell.
func evalIn(t *testing.T, ctx *TestContext, src string) Value {
	t.Helper()
	return ctx.Eval(src)
}

// TestOperators checks arithmetic, text, comparison and reference operators,
// coercions, names and implicit intersection.
func TestOperators(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.Set("A1", 2)
	ctx.Set("A2", "3")
	ctx.Set("A3", true)
	ctx.Set("B1", "=A1*10")
	ctx.Set("Sheet2!C5", 7)
	ctx.Names["RATE"] = "0.5"
	for _, c := range []struct {
		src  string
		want Value
	}{
		{"=1+2*3", Number(7)},
		{"=(1+2)*3", Number(9)},
		{"=-2^2", Number(4)},
		{"=2^3^2", Number(64)},
		{"=10/4", Number(2.5)},
		{"=1/0", ErrDiv0},
		{"=0^0", ErrNum},
		{"=50%", Number(0.5)},
		{"=A1+A2", Number(5)},
		{"=A1+A3", Number(3)},
		{"=A1+\"x\"", ErrValue},
		{"=\"1/15/2026\"+0", Number(46037)},
		{"=\"$1,234.50\"*2", Number(2469)},
		{"=\"a\"&1&TRUE", String("a1TRUE")},
		{"=1/3&\"\"", String("0.333333333333333")},
		{"=1=1", Bool(true)},
		{"=\"abc\"=\"ABC\"", Bool(true)},
		{"=\"1\"=1", Bool(false)},
		{"=\"a\"<\"b\"", Bool(true)},
		{"=1<\"a\"", Bool(true)},
		{"=\"z\"<TRUE", Bool(true)},
		{"=C9=0", Bool(true)},
		{"=C9=\"\"", Bool(true)},
		{"=B1", Number(20)},
		{"=Sheet2!C5", Number(7)},
		{"='Sheet2'!C5+1", Number(8)},
		{"=NoSheet!A1", ErrRef},
		{"=RATE*4", Number(2)},
		{"=UNDEFINED", ErrName},
		{"=NOSUCHFUNC(1)", ErrName},
		{"=#N/A", ErrNA},
		{"=@A1:A3", Number(2)}, // skipped here: Eval runs from A1000, so it is checked below
	} {
		got := evalIn(t, ctx, c.src)
		if c.src == "=@A1:A3" {
			continue // implicit intersection from row 1000 is #VALUE!, checked below
		}
		if !sameValue(got, c.want) {
			t.Errorf("%s = %#v, want %#v", c.src, got, c.want)
		}
	}
	if got := ctx.EvalAt("C2", "=@A1:A3"); !sameValue(got, String("3")) {
		t.Errorf("@A1:A3 from row 2 = %#v", got)
	}
	if got := ctx.EvalAt("C9", "=@A1:A3"); !sameValue(got, ErrValue) {
		t.Errorf("@A1:A3 from row 9 = %#v", got)
	}
}

// TestArrays checks element-wise array operators and broadcasting.
func TestArrays(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.SetRange("A1", [][]any{{1, 2}, {3, 4}})
	got := ctx.Eval("=A1:B2*10")
	a, ok := got.(*Array)
	if !ok || a.Rows != 2 || a.Cols != 2 || !sameValue(a.At(1, 1), Number(40)) {
		t.Fatalf("A1:B2*10 = %#v", got)
	}
	got = ctx.Eval("={1,2,3}+{10;20}") // broadcast 1×3 with 2×1
	a, ok = got.(*Array)
	if !ok || a.Rows != 2 || a.Cols != 3 || !sameValue(a.At(1, 2), Number(23)) {
		t.Fatalf("broadcast = %#v", got)
	}
	got = ctx.Eval("={1,2}+{1,2,3}") // mismatched lengths give #N/A
	a = got.(*Array)
	if !sameValue(a.At(0, 2), ErrNA) {
		t.Fatalf("mismatch = %#v", a.V)
	}
	if got := ctx.Eval("={1}+1"); !sameValue(got, Number(2)) {
		t.Fatalf("1x1 array = %#v", got)
	}
}

// TestParseErrors checks that malformed formulas fail to parse and valid
// edge cases parse.
func TestParseErrors(t *testing.T) {
	for _, src := range []string{"=1+", "=SUM(1", "=(1", "={1,2;3}", "=\"abc", "=A1:"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded", src)
		}
	}
	for _, src := range []string{"=_xlfn.XLOOKUP(1,A:A,B:B)", "=SUM(Sheet1!$A$1:$B$2)", "=LOG10(100)",
		"=A1#", "=IF(A1,,)", "='It''s'!A1", "=LAMBDA(_xlpm.x,_xlpm.x+1)(2)", "={1,-2;\"a\",TRUE}", "=$A:$A", "=1:3"} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// TestNumberText checks the General-format text of numbers.
func TestNumberText(t *testing.T) {
	for v, want := range map[float64]string{
		0: "0", 1: "1", -2.5: "-2.5", 1.0 / 3: "0.333333333333333", 1e20: "1E+20",
		123456789012345: "123456789012345", 0.00001: "0.00001", 1e-10: "1E-10",
		0.1 + 0.2: "0.3",
	} {
		if got := NumberText(v); got != want {
			t.Errorf("NumberText(%v) = %q, want %q", v, got, want)
		}
	}
}

// TestDates checks DateSerial, SerialToYMD and ParseDateTime, including the
// 1900 leap-year bug.
func TestDates(t *testing.T) {
	for _, c := range []struct {
		y, m, d int
		want    float64
	}{
		{1900, 1, 1, 1}, {1900, 2, 28, 59}, {1900, 2, 29, 60}, {1900, 3, 1, 61},
		{2026, 10, 15, 46310}, {2026, 14, 1, 46419}, {2026, 1, 0, 46022}, {126, 1, 1, 46023},
		{9999, 12, 31, 2958465}, {2300, 1, 1, 146099},
	} {
		got, ok := DateSerial(c.y, c.m, c.d, false)
		if !ok || got != c.want {
			t.Errorf("DateSerial(%d,%d,%d) = %v, want %v", c.y, c.m, c.d, got, c.want)
		}
	}
	if y, m, d := SerialToYMD(60, false); y != 1900 || m != 2 || d != 29 {
		t.Errorf("serial 60 = %d-%d-%d", y, m, d)
	}
	if y, m, d := SerialToYMD(46310, false); y != 2026 || m != 10 || d != 15 {
		t.Errorf("serial 46310 = %d-%d-%d", y, m, d)
	}
	for s, want := range map[string]float64{
		"10/15/2026": 46310, "2026-10-15": 46310, "15-Oct-2026": 46310, "October 15, 2026": 46310,
		"Oct 15 2026": 46310, "3:00 PM": 0.625, "15:00": 0.625, "10/15/2026 6:00": 46310.25,
		"12:00 AM": 0, "1/1/30": 10959,
	} {
		got, ok := ParseDateTime(s, false)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("ParseDateTime(%q) = %v, %v; want %v", s, got, ok, want)
		}
	}
	for _, s := range []string{"2/30/2026", "13/1/2026", "hello", "25:00 PM", "42"} {
		if _, ok := ParseDateTime(s, false); ok {
			t.Errorf("ParseDateTime(%q) accepted", s)
		}
	}
}

// TestCriteria checks COUNTIF-style criteria: comparisons, wildcards, blanks
// and errors.
func TestCriteria(t *testing.T) {
	for _, c := range []struct {
		crit Value
		v    Value
		want bool
	}{
		{Number(5), Number(5), true},
		{Number(5), String("5"), true},
		{String(">=5"), Number(7), true},
		{String(">=5"), Number(4), false},
		{String(">5"), String("9"), false},
		{String("<>5"), Number(4), true},
		{String("<>5"), Blank{}, true},
		{String("apple"), String("APPLE"), true},
		{String("a*"), String("avocado"), true},
		{String("a?c"), String("abc"), true},
		{String("a~*"), String("a*"), true},
		{String("a~*"), String("ab"), false},
		{String("<>a*"), String("banana"), true},
		{String(""), Blank{}, true},
		{String(""), String(""), true},
		{String(""), Number(0), false},
		{String("<>"), Blank{}, false},
		{String("<>"), Number(0), true},
		{String("TRUE"), Bool(true), true},
		{String(">b"), String("c"), true},
		{String(">b"), Number(5), false},
		{String("#N/A"), ErrNA, true},
		{String("1/15/2026"), Number(46037), true},
	} {
		if got := ParseCriteria(c.crit).Match(c.v); got != c.want {
			t.Errorf("criteria %#v on %#v = %v, want %v", c.crit, c.v, got, c.want)
		}
	}
}

// sameValue reports whether a and b are equal, numbers within 1e-9.
func sameValue(a, b Value) bool {
	if x, ok := a.(Number); ok {
		y, ok := b.(Number)
		return ok && math.Abs(float64(x-y)) < 1e-9
	}
	return a == b
}

// TestLiftedResultsAreScalars checks that a function lifted over an array
// argument has each reference or array result reduced to a single value.
func TestLiftedResultsAreScalars(t *testing.T) {
	reg := NewRegistry()
	reg.Add(
		// Returns a reference to the cell n rows below A1.
		Spec{Name: "REFAT", Min: 1, Max: 1, Fn: func(e *Env, a []Value) Value {
			n, _ := Int(a[0])
			return Ref{Sheet: e.Sheet, R1: n, C1: 0, R2: n, C2: 0}
		}},
		// Returns an n×1 array 1..n.
		Spec{Name: "SEQ", Min: 1, Max: 1, Fn: func(e *Env, a []Value) Value {
			n, _ := Int(a[0])
			out := NewArray(n, 1)
			for i := range out.V {
				out.V[i] = Number(i + 1)
			}
			return out
		}},
	)
	ctx := NewTestContext(reg)
	ctx.SetRange("A1", [][]any{{10}, {20}, {30}})
	a, ok := ctx.Eval("=REFAT({1,2})").(*Array)
	if !ok || !sameValue(a.At(0, 0), Number(20)) || !sameValue(a.At(0, 1), Number(30)) {
		t.Errorf("lifted refs = %#v", a)
	}
	a, ok = ctx.Eval("=SEQ({2,3})").(*Array)
	if !ok || !sameValue(a.At(0, 0), Number(1)) || !sameValue(a.At(0, 1), Number(1)) {
		t.Errorf("lifted nested arrays = %#v", a)
	}
}
