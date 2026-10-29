// Looking up a report's records and cells: drill-down records and GETPIVOTDATA.

package pivot

import (
	"strings"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// Records returns the records (0 = the first row below the header)
// behind the cell where row line rl and column line cl cross: those the
// filters let through with the lines' items.
func Records(pt *model.PivotTable, src *Source, rl, cl model.PivotLine) []int {
	c := &computer{pt: pt, src: src}
	c.filter()
	rf, cf := c.valid(sourceFields(pt.Rows)), c.valid(sourceFields(pt.Cols))
	match := func(i int, fs []int, keys []string) bool {
		for k, key := range keys {
			if k >= len(fs) {
				return false
			}
			f := src.Fields[fs[k]]
			if f.Items[f.Rec[i]].Key != key {
				return false
			}
		}
		return true
	}
	var out []int
	for i := range src.N {
		if c.keep[i] && match(i, rf, rl.Items) && match(i, cf, cl.Items) {
			out = append(out, i)
		}
	}
	return out
}

// valueKey is how GETPIVOTDATA's item arguments are compared with item
// keys: numbers by value, text ignoring case.
func valueKey(v formula.Value) string {
	switch x := v.(type) {
	case formula.Number:
		return NumKey(float64(x))
	case formula.String:
		if n, ok := formula.ParseNumber(string(x)); ok {
			return NumKey(n)
		}
		return TextKey(string(x))
	case formula.Bool:
		if x {
			return "b:TRUE"
		}
		return "b:FALSE"
	}
	return ""
}

// GetData finds the cell of report pt holding value field dataField
// (its name, "Sum of Sales", or its source field's name) for the
// field/item pairs, as GETPIVOTDATA does: fields not named are totals.
// It reports the cell's position.
func GetData(pt *model.PivotTable, dataField string, pairs []formula.Value) (row, col int, ok bool) {
	data := -1
	for i, v := range pt.Values {
		if strings.EqualFold(v.Name, dataField) ||
			v.Field >= 0 && v.Field < len(pt.Fields) && strings.EqualFold(pt.Fields[v.Field].Name, dataField) {
			data = i
			break
		}
	}
	if data < 0 || len(pairs)%2 != 0 {
		return 0, 0, false
	}
	want := map[int]formula.Value{} // field -> item
	for i := 0; i < len(pairs); i += 2 {
		name, ok := pairs[i].(formula.String)
		if !ok {
			return 0, 0, false
		}
		f := -1
		for j := range pt.Fields {
			if strings.EqualFold(pt.Fields[j].Name, string(name)) || strings.EqualFold(pt.Fields[j].Label(), string(name)) {
				f = j
				break
			}
		}
		if f < 0 {
			return 0, 0, false
		}
		want[f] = pairs[i+1]
	}
	// Pairs naming a report filter are dropped, not checked against its
	// selection: the filtered report holds just those records anyway.
	for _, f := range pt.Pages {
		delete(want, f)
	}
	find := func(axis []int, lines []model.PivotLine) int {
		fs := sourceFields(axis)
		// The deepest field asked for decides the line's depth.
		depth := 0
		for k, f := range fs {
			if _, ok := want[f]; ok {
				depth = k + 1
			}
		}
		for i, l := range lines {
			if l.Kind == model.LineLabel || l.Kind == model.LineBlank {
				continue
			}
			if l.Data >= 0 && l.Data != data {
				continue
			}
			if depth == 0 {
				if len(fs) == 0 || l.Kind == model.LineGrand {
					return i
				}
				continue
			}
			if l.Kind == model.LineGrand || len(l.Items) != depth {
				continue
			}
			good := true
			for k := range depth {
				v, ok := want[fs[k]]
				if !ok || valueKey(v) != l.Items[k] && !(l.Items[k] == "" && isBlankArg(v)) {
					good = false
					break
				}
			}
			if good {
				return i
			}
		}
		return -1
	}
	for f := range want {
		if pt.AxisOf(f) != model.AxisRows && pt.AxisOf(f) != model.AxisCols {
			return 0, 0, false
		}
	}
	o := &pt.Out
	ri := find(pt.Rows, o.RowLines)
	ci := 0
	if len(o.ColLines) > 0 {
		ci = find(pt.Cols, o.ColLines)
	}
	if ri < 0 || ci < 0 {
		return 0, 0, false
	}
	return o.Body.R1 + o.FirstDataRow + ri, o.Body.C1 + o.FirstDataCol + ci, true
}

// isBlankArg reports whether a GETPIVOTDATA item argument names the blank
// item: empty text or "(blank)".
func isBlankArg(v formula.Value) bool {
	s, ok := v.(formula.String)
	return ok && (s == "" || strings.EqualFold(string(s), Blank))
}
