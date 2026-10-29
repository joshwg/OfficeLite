// Tests for calccheck's function scan.

package main

import (
	"slices"
	"testing"
)

// TestUnknownFunctionsSkipsQuoted checks that parentheses inside string
// literals and quoted sheet names aren't taken for function calls.
func TestUnknownFunctionsSkipsQuoted(t *testing.T) {
	for src, want := range map[string][]string{
		`SUM(1)`:                 nil,
		`"foo(bar)"`:             nil,
		`"a""b(c)"&NOPE(1)`:      {"NOPE"},
		`'a(b)'!A1+SUM(2)`:       nil,
		`IF(A1,"x(y)",NOPE2(3))`: {"NOPE2"},
	} {
		if got := unknownFunctions(src); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", src, got, want)
		}
	}
}
