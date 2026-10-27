// Package ftest is a table-driven test harness for worksheet functions.
package ftest

import (
	"fmt"
	"math"
	"testing"

	f "officelite/excel/formula"
)

// Case is one formula and its expected result. Want may be:
//
//	float64 / int        a Number (compared with a relative tolerance of 1e-9)
//	Approx{v, tol}       a Number within an absolute tolerance
//	string               a String
//	bool                 a Bool
//	f.Error              an error value
//	nil                  Blank
//	Arr                  an array of the above
//	func(f.Value) error  a custom check
//
// Case is an alias of an unnamed struct so that tables can use compact
// {"=SUM(1,2)", 3.0} literals without vet's unkeyed-fields warning.
type Case = struct {
	Src  string
	Want any
}

// Arr is an expected array, row by row.
type Arr [][]any

// Approx is a number with an absolute tolerance.
type Approx struct{ V, Tol float64 }

// Run evaluates each case in a fresh TestContext prepared by setup (which
// may be nil) and reports mismatches.
func Run(t *testing.T, reg *f.Registry, setup func(*f.TestContext), cases []Case) {
	t.Helper()
	for _, c := range cases {
		ctx := f.NewTestContext(reg)
		if setup != nil {
			setup(ctx)
		}
		got := ctx.Eval(c.Src)
		if err := Check(got, c.Want); err != nil {
			t.Errorf("%s: %v", c.Src, err)
		}
	}
}

// Check compares a value with an expectation (see Case).
func Check(got f.Value, want any) error {
	if fn, ok := want.(func(f.Value) error); ok {
		return fn(got)
	}
	if arr, ok := want.(Arr); ok {
		a, ok := got.(*f.Array)
		if !ok {
			return fmt.Errorf("got %s, want an array", Show(got))
		}
		if a.Rows != len(arr) || a.Cols != len(arr[0]) {
			return fmt.Errorf("got %d×%d array %s, want %d×%d", a.Rows, a.Cols, Show(a), len(arr), len(arr[0]))
		}
		for r, row := range arr {
			for c, w := range row {
				if err := Check(a.At(r, c), w); err != nil {
					return fmt.Errorf("[%d,%d]: %v", r, c, err)
				}
			}
		}
		return nil
	}
	switch w := want.(type) {
	case int:
		return Check(got, float64(w))
	case float64:
		n, ok := got.(f.Number)
		if !ok {
			return fmt.Errorf("got %s, want %v", Show(got), w)
		}
		if d := math.Abs(float64(n) - w); d > 1e-9*math.Max(1, math.Abs(w)) {
			return fmt.Errorf("got %v, want %v", float64(n), w)
		}
	case Approx:
		n, ok := got.(f.Number)
		if !ok || math.Abs(float64(n)-w.V) > w.Tol {
			return fmt.Errorf("got %s, want %v ± %v", Show(got), w.V, w.Tol)
		}
	case string:
		if s, ok := got.(f.String); !ok || string(s) != w {
			return fmt.Errorf("got %s, want %q", Show(got), w)
		}
	case bool:
		if b, ok := got.(f.Bool); !ok || bool(b) != w {
			return fmt.Errorf("got %s, want %v", Show(got), w)
		}
	case f.Error:
		if e, ok := got.(f.Error); !ok || e != w {
			return fmt.Errorf("got %s, want %s", Show(got), w)
		}
	case nil:
		if _, ok := got.(f.Blank); !ok {
			return fmt.Errorf("got %s, want blank", Show(got))
		}
	default:
		return fmt.Errorf("unsupported expectation %T", want)
	}
	return nil
}

// Show formats a value for test messages.
func Show(v f.Value) string {
	switch x := v.(type) {
	case f.Number:
		return fmt.Sprintf("%v", float64(x))
	case f.String:
		return fmt.Sprintf("%q", string(x))
	case f.Bool:
		return fmt.Sprintf("%v", bool(x))
	case f.Error:
		return string(x)
	case f.Blank:
		return "<blank>"
	case f.Missing:
		return "<missing>"
	case *f.Array:
		s := "{"
		for r := 0; r < x.Rows; r++ {
			if r > 0 {
				s += ";"
			}
			for c := 0; c < x.Cols; c++ {
				if c > 0 {
					s += ","
				}
				s += Show(x.At(r, c))
			}
		}
		return s + "}"
	case f.Ref:
		return fmt.Sprintf("Ref%+v", x)
	case *f.Lambda:
		return "<lambda>"
	}
	return fmt.Sprintf("%T", v)
}
