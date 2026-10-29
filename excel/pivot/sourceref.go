// Parsing and resolving a PivotTable's source reference (sheet!range or a defined name).

package pivot

import (
	"strconv"
	"strings"

	"officelite/excel/model"
)

// SplitSource splits a source reference's formula text into its sheet
// name ("" when it has none) and range, as file formats store it
// (sheet="Sales Data" ref="A1:D6"). ok is false for anything else (a
// defined name, an expression).
func SplitSource(src string) (sheet string, r model.Range, ok bool) {
	src = strings.TrimPrefix(strings.TrimSpace(src), "=")
	ref := src
	if i := strings.LastIndexByte(src, '!'); i >= 0 {
		sheet, ref = src[:i], src[i+1:]
		if strings.HasPrefix(sheet, "'") && strings.HasSuffix(sheet, "'") && len(sheet) >= 2 {
			sheet = strings.ReplaceAll(sheet[1:len(sheet)-1], "''", "'")
		}
	}
	r, ok = ParseRange(ref)
	return sheet, r, ok
}

// ParseRange parses A1:D6 (or $A$1:$D$6, or a single cell).
func ParseRange(s string) (model.Range, bool) {
	s = strings.ReplaceAll(s, "$", "")
	a, b, found := strings.Cut(s, ":")
	if !found {
		b = a
	}
	r1, c1, ok1 := parseCell(a)
	r2, c2, ok2 := parseCell(b)
	if !ok1 || !ok2 {
		return model.Range{}, false
	}
	return model.Range{R1: min(r1, r2), C1: min(c1, c2), R2: max(r1, r2), C2: max(c1, c2)}, true
}

// parseCell parses an A1-style cell (no $) into 0-based row and column,
// rejecting more than three column letters and anything off the sheet.
func parseCell(s string) (row, col int, ok bool) {
	i := 0
	for i < len(s) && (s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z') {
		i++
	}
	if i == 0 || i == len(s) || i > 3 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil || n < 1 || n > model.MaxRows {
		return 0, 0, false
	}
	c := model.ParseColName(s[:i])
	if c < 0 || c >= model.MaxCols {
		return 0, 0, false
	}
	return n - 1, c, true
}

// ResolveSource finds the sheet and range a report's source names, for
// readers and writers that have no calculator: a reference, or a defined
// name referring to one. home is the report's sheet.
func ResolveSource(wb *model.Workbook, home int, src string) (int, model.Range, bool) {
	for depth := 0; depth < 4; depth++ {
		if name, r, ok := SplitSource(src); ok {
			if name == "" {
				return home, r, home >= 0 && home < len(wb.Sheets)
			}
			for i, sh := range wb.Sheets {
				if strings.EqualFold(sh.Name, name) {
					return i, r, true
				}
			}
			return 0, model.Range{}, false
		}
		next := ""
		name := strings.TrimPrefix(strings.TrimSpace(src), "=")
		for _, dn := range wb.Names {
			if strings.EqualFold(dn.Name, name) && (dn.Sheet == home || dn.Sheet < 0 && next == "") {
				next = dn.Formula
			}
		}
		if next == "" {
			break
		}
		src = next
	}
	return 0, model.Range{}, false
}
