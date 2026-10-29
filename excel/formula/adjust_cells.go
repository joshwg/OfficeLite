// adjust_cells.go rewrites references after Insert/Delete Cells shifts that
// cover only part of the sheet's rows or columns.

package formula

// AdjustRowsIn is AdjustRows for cells inserted or deleted with a shift
// down or up in columns c1..c2 only (Excel's Insert Cells, Shift cells
// down): a reference moves when its columns lie within c1..c2; one that
// also covers other columns, or whole columns or rows, is left alone, as
// Excel leaves it.
func AdjustRowsIn(f, home, target string, at, n, c1, c2 int) string {
	return rewriteRefs(f, func(t *refTok) string {
		r := t.r
		if t.name != "" || !t.on(home, target) || r.kind != refCell ||
			min(r.c1, r.c2) < c1 || max(r.c1, r.c2) > c2 {
			return t.text
		}
		var ok bool
		r.r1, r.r2, ok = shiftSpan(r.r1, r.r2, at, n, excelRows)
		return t.result(r, ok)
	})
}

// AdjustColsIn is AdjustRowsIn for a shift right or left in rows r1..r2.
func AdjustColsIn(f, home, target string, at, n, r1, r2 int) string {
	return rewriteRefs(f, func(t *refTok) string {
		r := t.r
		if t.name != "" || !t.on(home, target) || r.kind != refCell ||
			min(r.r1, r.r2) < r1 || max(r.r1, r.r2) > r2 {
			return t.text
		}
		var ok bool
		r.c1, r.c2, ok = shiftSpan(r.c1, r.c2, at, n, excelCols)
		return t.result(r, ok)
	})
}
