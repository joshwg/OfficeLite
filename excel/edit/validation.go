package edit

import (
	"math"
	"strconv"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

// Data validation: setting rules (Data > Data Validation), checking typed
// entries against them, the List rule's drop-down items and Circle
// Invalid Data. The rules themselves live in model/validation.go.

// DefaultValidationError is Excel's error alert for a rule without a
// message of its own.
const DefaultValidationError = "This value doesn't match the data validation restrictions defined for this cell."

// maxCircles is how many invalid cells Circle Invalid Data marks, as in
// Excel.
const maxCircles = 255

// SetValidation applies rule v to the ranges rs of sheet, replacing the
// rules their cells had; nil v removes validation from them (Clear All).
// Undoable as one step.
func (b *Book) SetValidation(sheet int, rs []model.Range, v *model.Validation) {
	defer b.Op("Data Validation")()
	if len(rs) == 0 {
		return
	}
	sh := b.WB.Sheets[sheet]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	sh.SetValidation(rs, v)
	ch.after = propsOf(sh)
	b.record(&change{where: Where{sheet, rs[len(rs)-1]}, sheet: ch})
}

// CheckEntry checks text typed into row, col against the cell's rule. It
// returns the rule the entry breaks, or nil when the entry is allowed (or
// the cell has no rule). An entry that doesn't parse is allowed here: the
// entry itself reports the error.
func (b *Book) CheckEntry(sheet, row, col int, text string) *model.Validation {
	sh := b.WB.Sheets[sheet]
	v, ok := sh.ValidationAt(row, col)
	if !ok || v.Type == model.ValidAny {
		return nil
	}
	cell, err := b.inputCell(sh.Get(row, col).Style, text)
	if err != nil {
		return nil
	}
	val := calc.CellValue(&cell)
	if cell.Formula != "" {
		val, _ = calc.Evaluate(b.WB, functions.Default(), cell.Formula, sheet, row, col, b.now(), nil)
		if a, ok := val.(*formula.Array); ok && len(a.V) > 0 {
			val = a.V[0]
		}
	}
	if b.valid(sheet, row, col, v, val, cell.Text) {
		return nil
	}
	return v
}

// CellValid reports whether the value row, col holds meets its rule (a
// cell without a rule is valid).
func (b *Book) CellValid(sheet, row, col int) bool {
	sh := b.WB.Sheets[sheet]
	v, ok := sh.ValidationAt(row, col)
	if !ok || v.Type == model.ValidAny {
		return true
	}
	c := sh.Get(row, col)
	return b.valid(sheet, row, col, v, calc.CellValue(c), c.Text)
}

// InvalidCells lists the nonblank cells of sheet that break their rules
// (at most 255, as Excel circles), for Circle Invalid Data.
func (b *Book) InvalidCells(sheet int) [][2]int {
	sh := b.WB.Sheets[sheet]
	var out [][2]int
	seen := map[[2]int]bool{}
	for i := range sh.Validations {
		v := &sh.Validations[i]
		if v.Type == model.ValidAny {
			continue
		}
		for _, r := range v.Ranges {
			u, ok := usedPart(sh, r)
			if !ok {
				continue
			}
			for row := u.R1; row <= u.R2; row++ {
				for col := u.C1; col <= min(u.C2, sh.RowLen(row)-1); col++ {
					k := [2]int{row, col}
					c := sh.Get(row, col)
					if seen[k] || c.IsBlank() {
						continue
					}
					seen[k] = true
					// The first rule covering a cell is its rule.
					if w, _ := sh.ValidationAt(row, col); w != v {
						continue
					}
					if !b.valid(sheet, row, col, v, calc.CellValue(c), c.Text) {
						out = append(out, k)
						if len(out) == maxCircles {
							return out
						}
					}
				}
			}
		}
	}
	return out
}

// ruleFormula evaluates one of rule v's formulas for the cell row, col:
// relative references move from the rule's top-left cell to it. o stands
// in the entry's value.
func (b *Book) ruleFormula(sheet, row, col int, v *model.Validation, f string, o *calc.Override) (formula.Value, *formula.Ref) {
	r0, c0 := v.TopLeft()
	f = formula.Shift(f, row-r0, col-c0)
	return calc.Evaluate(b.WB, functions.Default(), f, sheet, row, col, b.now(), o)
}

// valid checks value val (displayed as text) for row, col against v.
func (b *Book) valid(sheet, row, col int, v *model.Validation, val formula.Value, text string) bool {
	blank := false
	switch x := val.(type) {
	case formula.Blank:
		blank = true
	case formula.String:
		blank = x == ""
	}
	if blank {
		return v.AllowBlank
	}
	if _, ok := val.(formula.Error); ok && v.Type != model.ValidCustom && v.Type != model.ValidList {
		return false
	}
	switch v.Type {
	case model.ValidAny:
		return true
	case model.ValidCustom:
		o := &calc.Override{Sheet: sheet, Row: row, Col: col, Value: val}
		res, _ := b.ruleFormula(sheet, row, col, v, v.Formula1, o)
		if a, ok := res.(*formula.Array); ok && len(a.V) > 0 {
			res = a.V[0]
		}
		ok, e := formula.Truth(res)
		return e == nil && ok
	case model.ValidList:
		for _, it := range b.listValues(sheet, row, col, v) {
			if listMatch(it, val, b.WB.Date1904) {
				return true
			}
		}
		return false
	}
	// A number (whole, decimal, date or time) or a text length, compared
	// with the bounds.
	var x float64
	switch v.Type {
	case model.ValidTextLength:
		s := text
		if s == "" {
			s, _ = formula.Str(val)
		}
		x = float64(len([]rune(s)))
	default:
		n, ok := val.(formula.Number)
		if !ok {
			return false
		}
		x = float64(n)
		if v.Type == model.ValidWhole && x != math.Trunc(x) {
			return false
		}
	}
	bound := func(f string) (float64, bool, bool) { // value, blank, ok
		res, _ := b.ruleFormula(sheet, row, col, v, f, nil)
		if a, ok := res.(*formula.Array); ok && len(a.V) > 0 {
			res = a.V[0]
		}
		if _, ok := res.(formula.Blank); ok {
			return 0, true, true
		}
		n, e := formula.Num(res)
		return n, false, e == nil
	}
	lo, blank1, ok := bound(v.Formula1)
	if !ok {
		return false
	}
	hi := lo
	blank2 := false
	if v.Op == model.ValidBetween || v.Op == model.ValidNotBetween {
		if hi, blank2, ok = bound(v.Formula2); !ok {
			return false
		}
	}
	// With Ignore blank, a bound read from an empty cell allows anything.
	if v.AllowBlank && (blank1 || blank2) {
		return true
	}
	return validCompare(v.Op, x, lo, hi)
}

// validCompare applies a rule's comparison to x with the bounds lo (and hi).
func validCompare(op model.ValidationOp, x, lo, hi float64) bool {
	switch op {
	case model.ValidBetween:
		return x >= lo && x <= hi
	case model.ValidNotBetween:
		return x < lo || x > hi
	case model.ValidEqual:
		return x == lo
	case model.ValidNotEqual:
		return x != lo
	case model.ValidGreater:
		return x > lo
	case model.ValidLess:
		return x < lo
	case model.ValidGreaterEqual:
		return x >= lo
	case model.ValidLessEqual:
		return x <= lo
	}
	return false
}

// listMatch compares a list entry with a value as Excel does: text
// ignoring case, numbers (and text that reads as one) by value.
func listMatch(item, val formula.Value, date1904 bool) bool {
	num := func(x formula.Value) (float64, bool) {
		switch x := x.(type) {
		case formula.Number:
			return float64(x), true
		case formula.String:
			if in, err := ParseInput(strings.TrimSpace(string(x)), date1904); err == nil && in.Kind == model.KindNumber {
				return in.Num, true
			}
		}
		return 0, false
	}
	if a, ok := num(item); ok {
		if b, ok := num(val); ok {
			return a == b
		}
	}
	as, e1 := formula.Str(item)
	bs, e2 := formula.Str(val)
	return e1 == nil && e2 == nil && strings.EqualFold(as, bs)
}

// listValues are the entries of a List rule for row, col: the
// comma-separated values of a text source, or a range's values.
func (b *Book) listValues(sheet, row, col int, v *model.Validation) []formula.Value {
	res, _ := b.ruleFormula(sheet, row, col, v, v.Formula1, nil)
	var out []formula.Value
	switch x := res.(type) {
	case formula.String:
		for _, s := range splitList(string(x)) {
			out = append(out, formula.String(s))
		}
	case *formula.Array:
		for _, el := range x.V {
			if _, ok := el.(formula.Blank); !ok {
				out = append(out, el)
			}
		}
	case formula.Blank:
	default:
		out = append(out, x)
	}
	return out
}

// splitList splits a List rule's text source at its commas.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ListItems are the entries the in-cell drop-down of row, col offers, as
// displayed (a range's cells with their number formats); nil when the
// cell has no List rule with a drop-down.
func (b *Book) ListItems(sheet, row, col int) []string {
	sh := b.WB.Sheets[sheet]
	v, ok := sh.ValidationAt(row, col)
	if !ok || v.Type != model.ValidList || v.NoDropDown {
		return nil
	}
	res, ref := b.ruleFormula(sheet, row, col, v, v.Formula1, nil)
	if ref != nil && ref.Sheet >= 0 && ref.Sheet < len(b.WB.Sheets) {
		src := b.WB.Sheets[ref.Sheet]
		var out []string
		for r := ref.R1; r <= min(ref.R2, src.UsedRows()-1); r++ {
			for c := ref.C1; c <= ref.C2; c++ {
				if t := src.Get(r, c).Text; t != "" {
					out = append(out, t)
				}
			}
		}
		return out
	}
	if s, ok := res.(formula.String); ok {
		return splitList(string(s))
	}
	var out []string
	for _, it := range b.listValues(sheet, row, col, v) {
		if n, ok := it.(formula.Number); ok {
			out = append(out, formula.NumberText(float64(n)))
			continue
		}
		if s, e := formula.Str(it); e == nil {
			out = append(out, s)
		}
	}
	return out
}

// ValidationBound turns what is typed in the Data Validation dialog's
// Minimum / Maximum / Value / Source boxes into a rule formula: "=..." is
// a formula or reference, a number, date or time is its value, anything
// else (a List source) is a text constant. ok is false for a box left
// empty or a formula that doesn't parse.
func ValidationBound(s string, list, date1904 bool) (f string, ok bool) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", false
	case strings.HasPrefix(s, "="):
		g, err := formula.Normalize(s[1:])
		return g, err == nil && g != ""
	case list:
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`, true
	}
	in, err := ParseInput(s, date1904)
	if err != nil || in.Kind != model.KindNumber {
		return "", false
	}
	return strconv.FormatFloat(in.Num, 'g', -1, 64), true
}

// ValidationBoundText shows a rule formula in the dialog's boxes: a text
// constant without its quotes, a number as typed for a date or time rule,
// a reference or formula with '='.
func ValidationBoundText(f string, t model.ValidationType, date1904 bool) string {
	if f == "" {
		return ""
	}
	if len(f) >= 2 && f[0] == '"' && f[len(f)-1] == '"' {
		return strings.ReplaceAll(f[1:len(f)-1], `""`, `"`)
	}
	n, err := strconv.ParseFloat(f, 64)
	if err != nil {
		return "=" + f
	}
	switch t {
	case model.ValidDate:
		y, m, d := formula.SerialToYMD(n, date1904)
		return strconv.Itoa(m) + "/" + strconv.Itoa(d) + "/" + strconv.Itoa(y)
	case model.ValidTime:
		secs := int(math.Round((n - math.Floor(n)) * 86400))
		h, mi, s := secs/3600, secs/60%60, secs%60
		out := strconv.Itoa(h) + ":" + pad2(mi)
		if s != 0 {
			out += ":" + pad2(s)
		}
		return out
	}
	return f
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
