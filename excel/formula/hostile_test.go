// hostile_test.go checks that malformed or hostile formulas fail cleanly:
// deep nesting, references past the sheet, over-long text and huge
// ranges give errors instead of crashing or exhausting memory.

package formula

import (
	"fmt"
	"strings"
	"testing"

	"officelite/excel"
)

// TestParseDepthLimit checks that pathologically deep formulas are parse
// errors (not a fatal stack overflow) while ordinary nesting still parses.
func TestParseDepthLimit(t *testing.T) {
	const n = 200000
	for _, src := range []string{
		"=" + strings.Repeat("(", n) + "1" + strings.Repeat(")", n),
		"=" + strings.Repeat("-", n) + "1",
		"=1" + strings.Repeat("+1", n),
		"=1" + strings.Repeat("%", n),
		"=A1" + strings.Repeat(":A1", n),
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%.20s...) of %d bytes: no error", src, len(src))
		}
	}
	for _, src := range []string{
		"=" + strings.Repeat("(", 500) + "1" + strings.Repeat(")", 500),
		"=1" + strings.Repeat("+1", 2000),
		"=" + strings.Repeat("-", 500) + "1",
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%.20s...): %v", src, err)
		}
	}
}

// TestHostileFormulas checks rows past Excel's last row, concatenation past
// a cell's text limit and ranges too big to materialize.
func TestHostileFormulas(t *testing.T) {
	ctx := NewTestContext(NewRegistry())
	ctx.Set("A1", 1)
	ctx.Set("B2", 2)
	long := `"` + strings.Repeat("a", 20000) + `"`
	for _, c := range []struct {
		src  string
		want Value
	}{
		{"=A1048576", Blank{}},
		{"=A1048577", ErrName},              // a name, as in Excel
		{"=A99999999999999999999", ErrName}, // not a row number that overflows
		{"=" + long + "&" + long, ErrValue},
	} {
		if got := ctx.Eval(c.src); got != c.want {
			t.Errorf("%.40s: got %v, want %v", c.src, got, c.want)
		}
	}
	if got, ok := ctx.Eval(`="` + strings.Repeat("a", 32766) + `"&"b"`).(String); !ok || len(got) != excel.FormulaMaxText {
		t.Errorf("32767-character concatenation: got %T of length %d", got, len(got))
	}
	// 17 billion cells: trimmed to the used area instead of allocated.
	a, _ := ctx.Eval("=B1:XFD1048576").(*Array)
	if a == nil || a.Rows != 2 || a.At(1, 0) != Number(2) {
		t.Errorf("=B1:XFD1048576: got %s", ftestShow(a))
	}
	if got := ctx.Eval("=B2:XFD1048576"); got != Number(2) { // 1×1 once trimmed
		t.Errorf("=B2:XFD1048576: got %v", got)
	}
	// An ordinary range keeps its shape past the used area.
	if a, ok := ctx.Eval("=B1:B5").(*Array); !ok || a.Rows != 5 {
		t.Errorf("=B1:B5: got %v", a)
	}
}

// ftestShow describes an array's shape for test messages.
func ftestShow(a *Array) string {
	if a == nil {
		return "no array"
	}
	return fmt.Sprintf("%d×%d array", a.Rows, a.Cols)
}

// TestDateSerialRange checks that DateSerial rejects dates past
// 9999-12-31 reached by rolling the day or month over.
func TestDateSerialRange(t *testing.T) {
	if s, ok := DateSerial(9999, 12, 31, false); !ok || s != MaxSerial {
		t.Errorf("DateSerial(9999,12,31) = %v, %v", s, ok)
	}
	if s, ok := DateSerial(9999, 12, 31, true); !ok || s != MaxSerial-Offset1904 {
		t.Errorf("DateSerial(9999,12,31) 1904 = %v, %v", s, ok)
	}
	for _, d := range [][3]int{{9999, 12, 32}, {9999, 13, 1}, {9999, 1, 1000000}} {
		if s, ok := DateSerial(d[0], d[1], d[2], false); ok {
			t.Errorf("DateSerial%v = %v, want failure", d, s)
		}
	}
}

// TestWildcardLongText checks wildcard matching of many stars against
// long text (the memoized matcher this replaced kept a map entry per
// pattern × text position: millions for one cell).
func TestWildcardLongText(t *testing.T) {
	text := strings.Repeat("a", 30000)
	pattern := strings.Repeat("*a", 100)
	if !WildcardMatch(pattern, text) || WildcardMatch(pattern+"b", text) {
		t.Error("wrong match of many stars against long text")
	}
	for _, c := range []struct {
		p, s string
		want bool
	}{
		{"a~*c", "a*c", true}, {"a~*c", "abc", false}, {"a~", "a~", true},
		{"?b*", "ABCD", true}, {"*~?", "what?", true}, {"*~?", "what", false},
	} {
		if got := WildcardMatch(c.p, c.s); got != c.want {
			t.Errorf("WildcardMatch(%q, %q) = %v", c.p, c.s, got)
		}
	}
}
