// Package info implements Excel's information functions: CELL, ERROR.TYPE,
// INFO, the IS* family (ISBLANK, ISERR, ISERROR, ISEVEN, ISFORMULA,
// ISLOGICAL, ISNA, ISNONTEXT, ISNUMBER, ISODD, ISOMITTED, ISREF, ISTEXT),
// N, NA, SHEET, SHEETS, STOCKHISTORY and TYPE.
package info

import (
	"math"
	"strconv"
	"strings"

	f "officelite/excel/formula"
	"officelite/excel/model"
)

// Register adds the information functions to r.
func Register(r *f.Registry) {
	r.Add(
		is("ISBLANK", func(v f.Value) bool {
			switch v.(type) {
			case f.Blank, f.Missing:
				return true // truly empty only: a formula's "" is text
			}
			return false
		}),
		is("ISERR", func(v f.Value) bool { e, ok := v.(f.Error); return ok && e != f.ErrNA }),
		is("ISERROR", func(v f.Value) bool { return f.IsError(v) }),
		is("ISNA", func(v f.Value) bool { e, ok := v.(f.Error); return ok && e == f.ErrNA }),
		is("ISLOGICAL", func(v f.Value) bool { _, ok := v.(f.Bool); return ok }),
		is("ISNUMBER", func(v f.Value) bool { _, ok := v.(f.Number); return ok }),
		is("ISTEXT", func(v f.Value) bool { _, ok := v.(f.String); return ok }),
		is("ISNONTEXT", func(v f.Value) bool { _, ok := v.(f.String); return !ok }),
		is("ISOMITTED", func(v f.Value) bool { _, ok := v.(f.Missing); return ok }),
		f.Spec{Name: "ISEVEN", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value { return parity(a[0], 0) }},
		f.Spec{Name: "ISODD", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value { return parity(a[0], 1) }},
		f.Spec{Name: "ISREF", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			_, ok := a[0].(f.Ref)
			return f.Bool(ok)
		}},
		f.Spec{Name: "ISFORMULA", Min: 1, Max: 1, Refs: f.All, Arrays: f.All, Fn: isFormula},
		f.Spec{Name: "N", Min: 1, Max: 1, Fn: nFn},
		f.Spec{Name: "NA", Min: 0, Max: 0, Fn: func(*f.Env, []f.Value) f.Value { return f.ErrNA }},
		f.Spec{Name: "TYPE", Min: 1, Max: 1, Arrays: f.All, Fn: typeFn},
		f.Spec{Name: "ERROR.TYPE", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			if err, ok := a[0].(f.Error); ok {
				if n, ok := f.ErrorCodes[err]; ok {
					return f.Number(n)
				}
			}
			return f.ErrNA
		}},
		f.Spec{Name: "SHEET", Min: 0, Max: 1, Refs: f.All, Arrays: f.All, Fn: sheet},
		f.Spec{Name: "SHEETS", Min: 0, Max: 1, Refs: f.All, Arrays: f.All, Fn: sheets},
		f.Spec{Name: "CELL", Min: 1, Max: 2, Refs: f.Only(1), Arrays: f.Only(1), Fn: cell},
		f.Spec{Name: "INFO", Min: 1, Max: 1, Fn: info},
		// STOCKHISTORY(stock, start_date, [end_date], [interval], [headers],
		// [property0] ... [property5]) needs Microsoft's online data service.
		f.Spec{Name: "STOCKHISTORY", Min: 2, Max: 11, Arrays: f.All, Fn: func(_ *f.Env, a []f.Value) f.Value {
			for _, v := range a {
				if err, ok := v.(f.Error); ok {
					return err
				}
			}
			return f.ErrConnect
		}},
	)
}

// is builds an IS* predicate: lifted over arrays, never an error.
func is(name string, pred func(f.Value) bool) f.Spec {
	return f.Spec{Name: name, Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
		return f.Bool(pred(a[0]))
	}}
}

// parity implements ISEVEN/ISODD: the number is truncated; logical values
// are #VALUE! (unlike other numeric arguments).
func parity(v f.Value, want float64) f.Value {
	if _, ok := v.(f.Bool); ok {
		return f.ErrValue
	}
	n, err := f.Num(v)
	if err != nil {
		return err
	}
	return f.Bool(math.Abs(math.Mod(math.Trunc(n), 2)) == want)
}

// ISFORMULA(reference): a multi-cell reference gives an array.
func isFormula(e *f.Env, a []f.Value) f.Value {
	r, ok := a[0].(f.Ref)
	if !ok {
		if err, isErr := a[0].(f.Error); isErr {
			return err
		}
		return f.ErrValue
	}
	has := func(row, col int) f.Value { return f.Bool(e.Ctx.CellFormula(r.Sheet, row, col) != "") }
	if r.Single() {
		return has(r.R1, r.C1)
	}
	out := &f.Array{Rows: r.Rows(), Cols: r.Cols(), V: make([]f.Value, 0, r.Rows()*r.Cols())}
	for row := r.R1; row <= r.R2; row++ {
		for col := r.C1; col <= r.C2; col++ {
			out.V = append(out.V, has(row, col))
		}
	}
	return out
}

// N(value): numbers (and dates) as is, TRUE 1, everything else 0; errors
// propagate.
func nFn(e *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case f.Number:
		return v
	case f.Bool:
		if v {
			return f.Number(1)
		}
	case f.Error:
		return v
	}
	return f.Number(0)
}

// TYPE(value): 1 number (and blank), 2 text, 4 logical, 16 error, 64 array
// (including multi-cell ranges), 128 for anything else (lambdas).
func typeFn(e *f.Env, a []f.Value) f.Value {
	switch a[0].(type) {
	case f.Number, f.Blank, f.Missing:
		return f.Number(1)
	case f.String:
		return f.Number(2)
	case f.Bool:
		return f.Number(4)
	case f.Error:
		return f.Number(16)
	case *f.Array:
		return f.Number(64)
	}
	return f.Number(128)
}

// SHEET([value]): the 1-based number of the sheet of a reference or named
// by text; the formula's own sheet when omitted.
func sheet(e *f.Env, a []f.Value) f.Value {
	if len(a) == 0 {
		return f.Number(e.Sheet + 1)
	}
	switch v := a[0].(type) {
	case f.Missing:
		return f.Number(e.Sheet + 1)
	case f.Ref:
		return f.Number(v.Sheet + 1)
	case f.Error:
		return v
	case f.String:
		if i := e.Ctx.SheetIndex(string(v)); i >= 0 {
			return f.Number(i + 1)
		}
	}
	return f.ErrNA
}

// SHEETS([reference]): the number of sheets in a reference (always 1, as
// 3-D references are not supported), or in the workbook when omitted.
func sheets(e *f.Env, a []f.Value) f.Value {
	if len(a) == 0 {
		return f.Number(e.Ctx.SheetCount())
	}
	switch v := a[0].(type) {
	case f.Missing:
		return f.Number(e.Ctx.SheetCount())
	case f.Ref:
		return f.Number(1)
	case f.Error:
		return v
	}
	return f.ErrRef
}

// CELL(info_type, [reference]) for the top-left cell of reference, or the
// formula's own cell when omitted (Excel uses the last changed cell).
//
// The workbook context has no formatting information, so some types are
// approximations: "format" is always "G" (General), "color" and
// "parentheses" 0, "protect" 1 (cells are locked by default), "prefix" "'"
// for text (left-aligned by default) and "" otherwise, "width" the default
// column width {8, TRUE}, and "filename" "" (as for an unsaved workbook).
// "address" of a cell on another sheet is prefixed with the sheet name but
// no [Book] name.
func cell(e *f.Env, a []f.Value) f.Value {
	typ, err := f.Str(a[0])
	if err != nil {
		return err
	}
	r := f.Ref{Sheet: e.Sheet, R1: e.Row, C1: e.Col, R2: e.Row, C2: e.Col}
	if len(a) > 1 {
		switch v := a[1].(type) {
		case f.Ref:
			r = v
		case f.Missing:
		case f.Error:
			return v
		default:
			return f.ErrValue
		}
	}
	val := func() f.Value { return e.Ctx.CellValue(r.Sheet, r.R1, r.C1) }
	switch strings.ToLower(typ) {
	case "address":
		addr := "$" + model.ColName(r.C1) + "$" + strconv.Itoa(r.R1+1)
		if r.Sheet != e.Sheet {
			addr = f.QuoteSheet(e.Ctx.SheetName(r.Sheet)) + "!" + addr
		}
		return f.String(addr)
	case "col":
		return f.Number(r.C1 + 1)
	case "row":
		return f.Number(r.R1 + 1)
	case "contents":
		return val()
	case "type":
		switch val().(type) {
		case f.Blank:
			return f.String("b")
		case f.String:
			return f.String("l")
		}
		return f.String("v")
	case "filename":
		return f.String("")
	case "format":
		return f.String("G")
	case "color", "parentheses":
		return f.Number(0)
	case "protect":
		return f.Number(1)
	case "prefix":
		if _, ok := val().(f.String); ok {
			return f.String("'")
		}
		return f.String("")
	case "width":
		return &f.Array{Rows: 1, Cols: 2, V: []f.Value{f.Number(model.DefaultBaseColWidth), f.Bool(true)}}
	}
	return f.ErrValue
}

// INFO(type_text). "directory" is "" (no current folder is known),
// "osversion" and "release" report a 64-bit Windows Excel 2016+, and
// "memavail", "memused" and "totmem" are #N/A as in Excel 2007 and later.
func info(e *f.Env, a []f.Value) f.Value {
	typ, err := f.Str(a[0])
	if err != nil {
		return err
	}
	switch strings.ToLower(typ) {
	case "directory":
		return f.String("")
	case "numfile":
		return f.Number(e.Ctx.SheetCount())
	case "origin":
		return f.String("$A:$A$1")
	case "osversion":
		return f.String("Windows (64-bit) NT 10.00")
	case "recalc":
		return f.String("Automatic")
	case "release":
		return f.String("16.0")
	case "system":
		return f.String("pcdos")
	case "memavail", "memused", "totmem":
		return f.ErrNA
	}
	return f.ErrValue
}
