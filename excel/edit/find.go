package edit

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/model"
)

// FindOptions are the settings of Find and Replace.
type FindOptions struct {
	What        string
	Replacement string
	Workbook    bool // search every sheet (else the given sheet)
	Formulas    bool // look in formulas, i.e. the content as typed (else values as shown)
	MatchCase   bool
	WholeCell   bool // match entire cell contents
	// Within limits a sheet search to a selection (Excel searches the
	// selection when more than one cell is selected).
	Within *model.Range
}

// Found is a cell that matched.
type Found struct{ Sheet, Row, Col int }

func (f Found) before(g Found) bool {
	if f.Sheet != g.Sheet {
		return f.Sheet < g.Sheet
	}
	if f.Row != g.Row {
		return f.Row < g.Row
	}
	return f.Col < g.Col
}

// pattern compiles What: * and ? are wildcards and ~ escapes them, as in
// Excel.
func (o FindOptions) pattern() (*regexp.Regexp, error) {
	if o.What == "" {
		return nil, errors.New("Type what to find")
	}
	var b strings.Builder
	if !o.MatchCase {
		b.WriteString("(?i)")
	}
	b.WriteString("(?s)")
	if o.WholeCell {
		b.WriteString("^")
	}
	rs := []rune(o.What)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '~' && i+1 < len(rs):
			i++
			b.WriteString(regexp.QuoteMeta(string(rs[i])))
		case r == '*':
			b.WriteString(".*?")
		case r == '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if o.WholeCell {
		b.WriteString("$")
	}
	return regexp.Compile(b.String())
}

// ContentText is a cell's content as typed: a formula with its '=', a
// number in full precision (percentages as 12%), dates as shown, text.
func ContentText(sh *model.Sheet, r, c int) string {
	cell := sh.Get(r, c)
	switch {
	case cell.Formula != "":
		return "=" + cell.Formula
	case cell.Kind == model.KindNumber && plainNumber(cell.Text):
		if strings.HasSuffix(cell.Text, "%") {
			return strconv.FormatFloat(cell.Num*100, 'G', 15, 64) + "%"
		}
		return strconv.FormatFloat(cell.Num, 'G', 15, 64)
	}
	return cell.Text
}

func plainNumber(s string) bool {
	s = strings.NewReplacer(",", "", "$", "", "%", "", "(", "", ")", "", " ", "").Replace(s)
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// FindAll returns every matching cell, sheet by sheet, row by row.
func (b *Book) FindAll(sheet int, o FindOptions) ([]Found, error) {
	re, err := o.pattern()
	if err != nil {
		return nil, err
	}
	var out []Found
	for si, sh := range b.WB.Sheets {
		if !o.Workbook && si != sheet || sh.Hidden && si != sheet {
			continue
		}
		child := spillChildren(sh)
		r1, c1, r2, c2 := 0, 0, sh.UsedRows()-1, model.MaxCols-1
		if o.Within != nil && !o.Workbook {
			r1, c1, r2, c2 = o.Within.R1, o.Within.C1, min(o.Within.R2, r2), o.Within.C2
		}
		for r := r1; r <= r2; r++ {
			for c := c1; c <= min(c2, sh.RowLen(r)-1); c++ {
				var s string
				if o.Formulas {
					if child(r, c) && sh.Get(r, c).Formula == "" {
						continue // spilled values have no content of their own
					}
					s = ContentText(sh, r, c)
				} else {
					s = sh.Get(r, c).Text
				}
				if s != "" && re.MatchString(s) {
					out = append(out, Found{si, r, c})
				}
			}
		}
	}
	return out, nil
}

// FindNext returns the first match after from (before it, with back),
// wrapping around.
func (b *Book) FindNext(from Found, o FindOptions, back bool) (Found, error) {
	all, err := b.FindAll(from.Sheet, o)
	if err != nil {
		return Found{}, err
	}
	if len(all) == 0 {
		return Found{}, ErrNotFound
	}
	if back {
		for i := len(all) - 1; i >= 0; i-- {
			if all[i].before(from) {
				return all[i], nil
			}
		}
		return all[len(all)-1], nil
	}
	for _, f := range all {
		if from.before(f) {
			return f, nil
		}
	}
	return all[0], nil
}

// ErrNotFound is Excel's message when nothing matches.
var ErrNotFound = errors.New("We couldn't find what you were looking for")

// replaced is the cell at f after replacing; ok is false when nothing in
// it matches or the result isn't a valid entry.
func (b *Book) replaced(f Found, re *regexp.Regexp, o FindOptions) (before, after model.Cell, ok bool) {
	sh := b.WB.Sheets[f.Sheet]
	before = snapshot(sh, spillChildren(sh), f.Row, f.Col)
	if before.IsBlank() {
		return before, before, false
	}
	s := ContentText(sh, f.Row, f.Col)
	if !re.MatchString(s) {
		return before, before, false
	}
	t := re.ReplaceAllLiteralString(s, o.Replacement)
	if o.WholeCell {
		t = o.Replacement
	}
	if t == s {
		return before, before, false
	}
	after, err := b.inputCell(before.Style, t)
	if err != nil {
		return before, before, false
	}
	return before, after, true
}

// Replace replaces the matches in one cell. It reports false when the
// cell doesn't match.
func (b *Book) Replace(f Found, o FindOptions) (calc.Result, bool, error) {
	defer b.Op("Replace")()
	re, err := o.pattern()
	if err != nil {
		return calc.Result{}, false, err
	}
	before, after, ok := b.replaced(f, re, o)
	if !ok {
		return calc.Result{}, false, nil
	}
	where := Where{f.Sheet, model.Range{R1: f.Row, C1: f.Col, R2: f.Row, C2: f.Col}}
	return b.apply(where, []cellChange{{f.Sheet, f.Row, f.Col, before, after}}), true, nil
}

// ReplaceAll replaces every match as one undoable change and returns how
// many cells changed.
func (b *Book) ReplaceAll(sheet int, o FindOptions) (int, calc.Result, error) {
	defer b.Op("Replace All")()
	re, err := o.pattern()
	if err != nil {
		return 0, calc.Result{}, err
	}
	o.Formulas = true // replacing works on the content
	all, err := b.FindAll(sheet, o)
	if err != nil {
		return 0, calc.Result{}, err
	}
	var cells []cellChange
	bounds := model.Range{R1: model.MaxRows, C1: model.MaxCols, R2: -1, C2: -1}
	for _, f := range all {
		before, after, ok := b.replaced(f, re, o)
		if !ok {
			continue
		}
		cells = append(cells, cellChange{f.Sheet, f.Row, f.Col, before, after})
		if f.Sheet == sheet {
			bounds = model.Range{R1: min(bounds.R1, f.Row), C1: min(bounds.C1, f.Col), R2: max(bounds.R2, f.Row), C2: max(bounds.C2, f.Col)}
		}
	}
	if bounds.R2 < 0 {
		bounds = model.Range{}
	}
	return len(cells), b.apply(Where{sheet, bounds}, cells), nil
}

// ErrBadReference is Go To's message for text it can't read.
var ErrBadReference = errors.New("Reference isn't valid")

// ResolveRef reads a Go To reference: B12, A1:C5, C:D, 3:7,
// Sheet2!A1:C5, 'My Sheet'!B2, or a defined name. Unqualified references
// are on sheet.
func (b *Book) ResolveRef(text string, sheet int) (int, model.Range, error) {
	return b.resolveRef(strings.TrimSpace(text), sheet, 0)
}

func (b *Book) resolveRef(s string, sheet, depth int) (int, model.Range, error) {
	s = strings.TrimPrefix(s, "=")
	if s == "" || depth > 4 {
		return 0, model.Range{}, ErrBadReference
	}
	// Sheet prefix.
	if i := strings.LastIndex(s, "!"); i >= 0 {
		name := s[:i]
		if strings.HasPrefix(name, "'") && strings.HasSuffix(name, "'") && len(name) >= 2 {
			name = strings.ReplaceAll(name[1:len(name)-1], "''", "'")
		}
		sheet = b.sheetIndex(name)
		if sheet < 0 {
			return 0, model.Range{}, ErrBadReference
		}
		s = s[i+1:]
	}
	if r, ok := parseArea(s); ok {
		return sheet, r, nil
	}
	// A defined name: the sheet's own first, then the workbook's.
	for _, scope := range []int{sheet, -1} {
		for _, n := range b.WB.Names {
			if n.Sheet == scope && strings.EqualFold(n.Name, s) {
				return b.resolveRef(n.Formula, sheet, depth+1)
			}
		}
	}
	return 0, model.Range{}, ErrBadReference
}

// parseArea reads A1, A1:B2, A:C or 1:3 ($ signs allowed).
func parseArea(s string) (model.Range, bool) {
	s = strings.ReplaceAll(strings.ToUpper(s), "$", "")
	a, bs, isRange := strings.Cut(s, ":")
	if !isRange {
		bs = a
	}
	type part struct {
		row, col     int
		hasR, hasCol bool
	}
	parse := func(t string) (part, bool) {
		i := 0
		for i < len(t) && t[i] >= 'A' && t[i] <= 'Z' {
			i++
		}
		var p part
		if i > 0 {
			if i > 3 {
				return p, false
			}
			p.col, p.hasCol = model.ParseColName(t[:i]), true
		}
		if i < len(t) {
			n, err := strconv.Atoi(t[i:])
			if err != nil || n < 1 {
				return p, false
			}
			p.row, p.hasR = n-1, true
		}
		return p, p.hasR || p.hasCol
	}
	p, ok1 := parse(a)
	q, ok2 := parse(bs)
	if !ok1 || !ok2 || p.hasR != q.hasR || p.hasCol != q.hasCol {
		return model.Range{}, false
	}
	r := model.Range{R1: min(p.row, q.row), C1: min(p.col, q.col), R2: max(p.row, q.row), C2: max(p.col, q.col)}
	switch {
	case !p.hasR: // A:C
		if !isRange {
			return model.Range{}, false
		}
		r.R1, r.R2 = 0, model.MaxRows-1
	case !p.hasCol: // 1:3
		if !isRange {
			return model.Range{}, false
		}
		r.C1, r.C2 = 0, model.MaxCols-1
	}
	if r.R2 >= model.MaxRows || r.C2 >= model.MaxCols {
		return model.Range{}, false
	}
	return r, true
}
