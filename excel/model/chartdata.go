// Chart data: a series' references are read from the cells each time the
// chart is drawn, so it follows edits. References are formula text such
// as Sheet1!$B$2:$B$10, 'My Sheet'!$A$1, (Sheet1!$A$1,Sheet1!$A$3:$A$4)
// or a defined name standing for one of those.

package model

import (
	"strconv"
	"strings"
)

// SheetRange is a range on a sheet named in a reference.
type SheetRange struct {
	Sheet string
	Range Range
}

// ParseRefList parses chart reference text: one reference or a
// parenthesized union of them. Unqualified references are on home.
func ParseRefList(f, home string) ([]SheetRange, bool) {
	f = strings.TrimSpace(strings.TrimPrefix(f, "="))
	if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
		f = f[1 : len(f)-1]
	}
	var out []SheetRange
	for _, part := range splitTopLevel(f) {
		sr, ok := parseSheetRange(strings.TrimSpace(part), home)
		if !ok {
			return nil, false
		}
		out = append(out, sr)
	}
	return out, len(out) > 0
}

// splitTopLevel splits s at commas outside quoted sheet names.
func splitTopLevel(s string) []string {
	var out []string
	start, quoted := 0, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			quoted = !quoted
		case ',':
			if !quoted {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// parseSheetRange parses one reference (Sheet1!$A$1:$B$2, 'My Sheet'!A:A,
// 1:1, [0]Sheet!A1; other workbooks' refs fail), on home when unqualified; whole rows and columns
// span the sheet's limits.
func parseSheetRange(s, home string) (SheetRange, bool) {
	sheet, ref := home, s
	if i := strings.LastIndexByte(s, '!'); i >= 0 {
		sheet, ref = s[:i], s[i+1:]
		if strings.HasPrefix(sheet, "'") && strings.HasSuffix(sheet, "'") && len(sheet) >= 2 {
			sheet = strings.ReplaceAll(sheet[1:len(sheet)-1], "''", "'")
		}
		if j := strings.LastIndexByte(sheet, ']'); j >= 0 {
			if sheet[:j+1] != "[0]" {
				return SheetRange{}, false // another workbook: use the cached values
			}
			sheet = sheet[j+1:] // [0]Sheet1: this workbook
		}
	}
	ref = strings.ReplaceAll(ref, "$", "")
	a, b, two := strings.Cut(ref, ":")
	if !two {
		b = a
	}
	r1, c1, ok1 := parseRefPart(a)
	r2, c2, ok2 := parseRefPart(b)
	if !ok1 || !ok2 {
		return SheetRange{}, false
	}
	// Whole columns (A:A) and rows (1:1).
	if r1 < 0 && r2 < 0 {
		r1, r2 = 0, MaxRows-1
	}
	if c1 < 0 && c2 < 0 {
		c1, c2 = 0, MaxCols-1
	}
	if r1 < 0 || r2 < 0 || c1 < 0 || c2 < 0 {
		return SheetRange{}, false
	}
	return SheetRange{sheet, Range{R1: min(r1, r2), C1: min(c1, c2), R2: max(r1, r2), C2: max(c1, c2)}}, true
}

// parseRefPart parses A1, A or 1 (row or column -1 when missing).
func parseRefPart(s string) (row, col int, ok bool) {
	i := 0
	for i < len(s) && (s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z') {
		i++
	}
	row, col = -1, -1
	if i > 0 {
		if i > 3 {
			return 0, 0, false
		}
		col = ParseColName(s[:i])
		if col < 0 {
			return 0, 0, false
		}
	}
	if i < len(s) {
		n, err := strconv.Atoi(s[i:])
		if err != nil || n < 1 {
			return 0, 0, false
		}
		row = n - 1
	}
	if row < 0 && col < 0 {
		return 0, 0, false
	}
	return row, col, true
}

// SheetByName returns the sheet named name (ignoring case), or nil.
func (wb *Workbook) SheetByName(name string) *Sheet {
	for _, sh := range wb.Sheets {
		if strings.EqualFold(sh.Name, name) {
			return sh
		}
	}
	return nil
}

// resolveRef parses reference text, following a defined name.
func (wb *Workbook) resolveRef(ref, home string) ([]SheetRange, bool) {
	if rs, ok := ParseRefList(ref, home); ok {
		return rs, true
	}
	// A defined name: Name, Sheet1!Name or Book.xlsx!Name.
	name, scope := ref, ""
	if i := strings.LastIndexByte(ref, '!'); i >= 0 {
		name, scope = ref[i+1:], strings.Trim(ref[:i], "'")
	}
	for pass := 0; pass < 2; pass++ {
		for _, dn := range wb.Names {
			if !strings.EqualFold(dn.Name, name) {
				continue
			}
			local := dn.Sheet >= 0 && dn.Sheet < len(wb.Sheets)
			if pass == 0 && !(local && strings.EqualFold(wb.Sheets[dn.Sheet].Name, scope)) {
				continue // a sheet's own name first
			}
			if pass == 1 && local {
				continue
			}
			if local {
				home = wb.Sheets[dn.Sheet].Name // unqualified cells are on the name's own sheet
			}
			return ParseRefList(dn.Formula, home)
		}
	}
	return nil, false
}

// ChartValues reads the cells reference ref points at, in order (down a
// column or along a row; a block of several rows and columns is read
// along its longer side, its last column or row being the values and the
// others the levels of multi-level labels, outer first). Hidden rows and
// columns are left out when visibleOnly. ok is false when ref can't be
// read (an error, a sheet that doesn't exist, a name for a formula).
func (wb *Workbook) ChartValues(ref, home string, visibleOnly bool) (vals []ChartValue, levels [][]ChartValue, fmtStyle uint32, ok bool) {
	rs, ok := wb.resolveRef(ref, home)
	if !ok {
		return nil, nil, 0, false
	}
	first := true
	for _, sr := range rs {
		sh := wb.SheetByName(sr.Sheet)
		if sh == nil || sh.IsChartSheet() {
			return nil, nil, 0, false
		}
		rg := sr.Range
		// Past the stored cells everything is empty: don't read a whole
		// column a million cells long.
		rg.R2 = min(rg.R2, max(sh.UsedRows()-1, rg.R1))
		rg.C2 = min(rg.C2, max(sh.UsedCols()-1, rg.C1))
		down := rg.R2-rg.R1 >= rg.C2-rg.C1
		n, depth := rg.R2-rg.R1+1, rg.C2-rg.C1+1
		if !down {
			n, depth = depth, n
		}
		at := func(i, d int) (*Cell, bool) {
			r, c := rg.R1+i, rg.C1+d
			if !down {
				r, c = rg.R1+d, rg.C1+i
			}
			hidden := false
			if visibleOnly {
				// A hidden row or a hidden column hides the cell, whichever
				// way the series runs.
				w, ok := sh.ColWidth[c]
				hidden = sh.RowHeightPx(r) == 0 || ok && w == 0
			}
			return sh.Get(r, c), hidden
		}
		if depth > 1 && levels == nil {
			levels = make([][]ChartValue, depth-1)
		}
		for i := 0; i < n; i++ {
			cell, hidden := at(i, depth-1)
			if hidden {
				continue
			}
			v := cellValue(cell)
			if first && v.Kind == ValueNumber {
				fmtStyle, first = cell.Style, false
			}
			vals = append(vals, v)
			for d := 0; d < depth-1 && d < len(levels); d++ {
				c, _ := at(i, d)
				levels[d] = append(levels[d], cellValue(c))
			}
		}
	}
	return vals, levels, fmtStyle, true
}

// cellValue is c as a chart value: numbers as numbers, text and booleans
// as text, and empty cells and errors blank.
func cellValue(c *Cell) ChartValue {
	switch c.Kind {
	case KindNumber:
		return ChartValue{Kind: ValueNumber, Num: c.Num, Text: c.Text}
	case KindText, KindBool:
		return ChartValue{Kind: ValueText, Text: c.Text}
	}
	return ChartValue{} // empty, or an error such as #N/A: nothing to plot
}

// SeriesData returns d's current values: read from the cells when d is a
// reference that can be read, else the cached (or literal) values.
func (wb *Workbook) SeriesData(d *ChartData, home string, visibleOnly bool) []ChartValue {
	if d.Ref != "" && wb != nil {
		if vals, _, _, ok := wb.ChartValues(d.Ref, home, visibleOnly); ok {
			return vals
		}
	}
	return d.Cache
}

// SeriesName returns a series' name: the literal, else its referenced
// cells' (or cached) text joined by spaces, else "Series<n+1>".
func (wb *Workbook) SeriesName(s *ChartSeries, home string, n int) string {
	if s.Name.Literal != "" {
		return s.Name.Literal
	}
	vals := wb.SeriesData(&s.Name, home, false)
	var parts []string
	for _, v := range vals {
		if v.Text != "" {
			parts = append(parts, v.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return "Series" + strconv.Itoa(n+1)
}
