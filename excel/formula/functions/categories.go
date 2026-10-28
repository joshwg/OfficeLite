package functions

import (
	"slices"

	"officelite/excel/formula"
	"officelite/excel/formula/functions/database"
	"officelite/excel/formula/functions/datetime"
	"officelite/excel/formula/functions/financial"
	"officelite/excel/formula/functions/info"
	"officelite/excel/formula/functions/logical"
	"officelite/excel/formula/functions/lookup"
	"officelite/excel/formula/functions/mathtrig"
	"officelite/excel/formula/functions/text"
)

// Category is a group of functions as the Insert Function dialog lists
// them.
type Category struct {
	Name  string
	Funcs []string // sorted
}

// Categories lists the implemented functions by category. LET and
// LAMBDA, built into the evaluator, are listed under Logical.
func Categories() []Category {
	var out []Category
	for _, c := range []struct {
		name     string
		register func(*formula.Registry)
		extra    []string
	}{
		{"Database", database.Register, nil},
		{"Date & Time", datetime.Register, nil},
		{"Financial", financial.Register, nil},
		{"Information", info.Register, nil},
		{"Logical", logical.Register, []string{"LAMBDA", "LET"}},
		{"Lookup & Reference", lookup.Register, nil},
		{"Math & Statistics", mathtrig.Register, nil},
		{"Text", text.Register, nil},
	} {
		r := formula.NewRegistry()
		c.register(r)
		names := append(r.Names(), c.extra...)
		slices.Sort(names)
		out = append(out, Category{Name: c.name, Funcs: slices.Compact(names)})
	}
	return out
}
