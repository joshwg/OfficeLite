// deps.go extracts a parsed formula's static dependencies (references,
// names and functions) for recalculation ordering.

package formula

import "officelite/excel/model"

// DepRef is a rectangle of cells a formula reads. Sheet is "" for the
// formula's own sheet.
type DepRef struct {
	Sheet          string
	R1, C1, R2, C2 int
}

// Deps is what a parsed formula depends on, found without evaluating it.
type Deps struct {
	Refs  []DepRef
	Names []NameRef // defined names (and LET/LAMBDA variables, which can't be told apart)
	Funcs []string  // functions called, upper-case
	// Dynamic is set when the formula builds references at run time in a
	// way the static references don't cover (A1:INDEX(...)).
	Dynamic bool
}

// NameRef is a name used in a formula, with its sheet qualifier if any.
type NameRef struct{ Name, Sheet string }

// Dependencies lists the references, names and functions in n.
func Dependencies(n Node) Deps {
	var d Deps
	d.walk(n)
	return d
}

// walk adds the dependencies of n and its children to d, setting Dynamic for
// ranges built from non-reference operands.
func (d *Deps) walk(n Node) {
	switch n := n.(type) {
	case refNode:
		d.Refs = append(d.Refs, depRef(n))
	case nameNode:
		d.Names = append(d.Names, NameRef{Name: n.name, Sheet: n.sheet})
	case callNode:
		d.Funcs = append(d.Funcs, n.name)
		for _, a := range n.args {
			d.walk(a)
		}
	case invokeNode:
		d.walk(n.fn)
		for _, a := range n.args {
			d.walk(a)
		}
	case unaryNode:
		d.walk(n.x)
	case binaryNode:
		if n.op == ":" {
			l, lok := n.l.(refNode)
			r, rok := n.r.(refNode)
			if lok && rok && l.sheet == r.sheet {
				a, b := depRef(l), depRef(r)
				d.Refs = append(d.Refs, DepRef{Sheet: a.Sheet,
					R1: min(a.R1, b.R1), C1: min(a.C1, b.C1), R2: max(a.R2, b.R2), C2: max(a.C2, b.C2)})
				return
			}
			d.Dynamic = true
		}
		d.walk(n.l)
		d.walk(n.r)
	case spillNode:
		// A1# depends on the anchor; its spill area is covered by the
		// caller tracking the anchor's result.
		if _, ok := n.x.(refNode); !ok {
			d.Dynamic = true
		}
		d.walk(n.x)
	case arrayNode:
		for _, row := range n.rows {
			for _, el := range row {
				d.walk(el)
			}
		}
	}
}

// depRef converts a reference node to a normalized DepRef, expanding whole
// columns and rows to the model's sheet limits.
func depRef(n refNode) DepRef {
	r := DepRef{Sheet: n.sheet, R1: n.r1, C1: n.c1, R2: n.r2, C2: n.c2}
	if n.wholeCols {
		r.R1, r.R2 = 0, model.MaxRows-1
	}
	if n.wholeRows {
		r.C1, r.C2 = 0, model.MaxCols-1
	}
	if r.R1 > r.R2 {
		r.R1, r.R2 = r.R2, r.R1
	}
	if r.C1 > r.C2 {
		r.C1, r.C2 = r.C2, r.C1
	}
	return r
}
