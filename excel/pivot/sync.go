package pivot

import (
	"slices"
	"strings"

	"officelite/excel/model"
)

// Sync matches a report's fields with the source's columns by
// name (a renamed or removed column drops out of the areas; new columns
// become fields) and keeps Σ Values where Excel keeps it: on an axis
// when there are two or more value fields.
func Sync(pt *model.PivotTable, src *Source) {
	old := pt.Fields
	idx := make([]int, len(old)) // old index -> new, -1 gone
	fields := make([]model.PivotField, len(src.Fields))
	for i := range idx {
		idx[i] = -1
	}
	for i, f := range src.Fields {
		fields[i] = model.PivotField{Name: f.Name}
		for j := range old {
			if idx[j] < 0 && strings.EqualFold(old[j].Name, f.Name) {
				fields[i] = old[j]
				fields[i].Name = f.Name
				idx[j] = i
				break
			}
		}
	}
	remap := func(fs []int) []int {
		var out []int
		for _, f := range fs {
			if f == model.DataField {
				out = append(out, f)
			} else if f >= 0 && f < len(idx) && idx[f] >= 0 && !slices.Contains(out, idx[f]) {
				out = append(out, idx[f])
			}
		}
		return out
	}
	pt.Fields = fields
	pt.Rows, pt.Cols, pt.Pages = remap(pt.Rows), remap(pt.Cols), remap(pt.Pages)
	var vals []model.PivotValue
	for _, v := range pt.Values {
		if v.Field >= 0 && v.Field < len(idx) && idx[v.Field] >= 0 {
			v.Field = idx[v.Field]
			vals = append(vals, v)
		}
	}
	pt.Values = vals
	Normalize(pt)
}

// Normalize keeps Σ Values on an axis exactly when there are two or
// more value fields (in Columns when it comes back), and a field on at
// most one axis.
func Normalize(pt *model.PivotTable) {
	seen := map[int]bool{}
	clean := func(fs []int) []int {
		var out []int
		for _, f := range fs {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
		return out
	}
	pt.Rows, pt.Cols, pt.Pages = clean(pt.Rows), clean(pt.Cols), clean(pt.Pages)
	pt.Pages = slices.DeleteFunc(pt.Pages, func(f int) bool { return f == model.DataField })
	if len(pt.Values) < 2 {
		isData := func(f int) bool { return f == model.DataField }
		pt.Rows, pt.Cols = slices.DeleteFunc(pt.Rows, isData), slices.DeleteFunc(pt.Cols, isData)
	} else if !seen[model.DataField] {
		pt.Cols = append(pt.Cols, model.DataField)
	}
}
