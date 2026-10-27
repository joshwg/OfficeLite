package formula

import (
	"sort"
	"strings"
)

// Variadic is Spec.Max for functions taking any number of arguments.
const Variadic = -1

// Spec describes a worksheet function.
//
// By default every argument is scalar: the evaluator dereferences
// references, and when a multi-cell range or array is passed where a scalar
// is expected it calls Fn once per element and returns an array of the
// results (Excel's "lifting", e.g. =LEN(A1:A3) gives three lengths).
// Arrays/Refs opt arguments out of that.
type Spec struct {
	Name     string // upper-case, e.g. "NETWORKDAYS.INTL"
	Min, Max int    // argument count; Max may be Variadic

	// Fn receives evaluated arguments: scalars, *Array for arguments listed
	// in Arrays, Ref for arguments listed in Refs that were references, and
	// Missing for omitted arguments.
	Fn func(e *Env, args []Value) Value

	// Lazy, if set, is used instead of Fn and receives the unevaluated
	// argument expressions (for IF, IFERROR, SWITCH ... which must not
	// evaluate every branch). Use e.Eval / e.EvalScalar on them.
	Lazy func(e *Env, args []Node) Value

	// Arrays reports whether argument i is passed whole as *Array (ranges
	// dereferenced) instead of being lifted. nil means no argument is.
	Arrays func(i int) bool
	// Refs reports whether argument i, when it is a reference, is passed
	// as a Ref without being dereferenced (ROW, ISREF, SUMIF ranges ...).
	// Takes precedence over Arrays.
	Refs func(i int) bool

	Volatile bool   // recalculated on every calculation (NOW, RAND ...)
	Format   string // number format for the result in a General cell (dates)
}

// All, From and Only build Spec.Arrays / Spec.Refs predicates.
func All(int) bool { return true }

// From matches arguments k, k+1, ...
func From(k int) func(int) bool { return func(i int) bool { return i >= k } }

// Only matches the listed argument positions.
func Only(idx ...int) func(int) bool {
	return func(i int) bool {
		for _, j := range idx {
			if i == j {
				return true
			}
		}
		return false
	}
}

// Registry holds the worksheet functions.
type Registry struct {
	funcs map[string]*Spec
}

func NewRegistry() *Registry { return &Registry{funcs: map[string]*Spec{}} }

// Add registers functions, replacing any with the same name.
func (r *Registry) Add(specs ...Spec) {
	for i := range specs {
		s := specs[i]
		s.Name = strings.ToUpper(s.Name)
		r.funcs[s.Name] = &s
	}
}

// Lookup returns the function named name (case-insensitive), or nil.
func (r *Registry) Lookup(name string) *Spec {
	if r == nil {
		return nil
	}
	return r.funcs[strings.ToUpper(name)]
}

// Names lists the registered functions, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.funcs))
	for n := range r.funcs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
