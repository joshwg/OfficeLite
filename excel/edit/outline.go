package edit

import (
	"maps"

	"officelite/excel/model"
)

// The outline (Data > Outline): grouping and ungrouping rows and columns,
// collapsing and expanding groups, the level buttons and Show / Hide
// Detail. Each command is one undo step that restores the outline and
// the sizes of the rows and columns it hid. Subtotals are in subtotal.go.

// outlineChange is a change of a sheet's outline and hidden rows and
// columns.
type outlineChange struct {
	sheet         int
	before, after outlineState
}

type outlineState struct {
	outline model.Outline
	heights map[int]float32
	widths  map[int]float32
}

func outlineStateOf(sh *model.Sheet) outlineState {
	return outlineState{sh.Outline.Clone(), maps.Clone(sh.RowHeight), maps.Clone(sh.ColWidth)}
}

func (c *outlineChange) set(wb *model.Workbook, after bool) {
	s := c.before
	if after {
		s = c.after
	}
	sh := wb.Sheets[c.sheet]
	sh.Outline = s.outline.Clone()
	sh.RowHeight, sh.ColWidth = maps.Clone(s.heights), maps.Clone(s.widths)
	if sh.RowHeight == nil {
		sh.RowHeight = map[int]float32{}
	}
	if sh.ColWidth == nil {
		sh.ColWidth = map[int]float32{}
	}
}

// changeOutline applies f to the sheet and records the change, unless f
// reports that nothing changed.
func (b *Book) changeOutline(sheet int, where model.Range, f func(sh *model.Sheet) bool) bool {
	sh := b.WB.Sheets[sheet]
	ch := &outlineChange{sheet: sheet, before: outlineStateOf(sh)}
	if !f(sh) {
		ch.set(b.WB, false)
		return false
	}
	sh.TidyOutline()
	ch.after = outlineStateOf(sh)
	b.visibilityChanged()
	b.record(&change{where: Where{sheet, where}, outline: ch})
	return true
}

// axisWhere is the whole rows (columns) lo..hi.
func axisWhere(rows bool, lo, hi int) model.Range {
	if rows {
		return model.Range{R1: lo, R2: hi, C2: model.MaxCols - 1}
	}
	return model.Range{C1: lo, C2: hi, R2: model.MaxRows - 1}
}

// Group puts rows (columns) lo..hi one outline level deeper (Data >
// Group). Rows already at the deepest level stay there; it reports
// false when nothing changed.
func (b *Book) Group(sheet int, rows bool, lo, hi int) bool {
	defer b.Op("Group")()
	return b.changeOutline(sheet, axisWhere(rows, lo, hi), func(sh *model.Sheet) bool {
		changed := false
		for i := lo; i <= hi; i++ {
			if l := sh.Level(rows, i); l < model.MaxOutlineLevel {
				sh.SetLevel(rows, i, l+1)
				changed = true
			}
		}
		return changed
	})
}

// Ungroup takes rows (columns) lo..hi one outline level up (Data >
// Ungroup), showing those that a collapsed group hid. It reports false
// when none of them was grouped.
func (b *Book) Ungroup(sheet int, rows bool, lo, hi int) bool {
	defer b.Op("Ungroup")()
	return b.changeOutline(sheet, axisWhere(rows, lo, hi), func(sh *model.Sheet) bool {
		changed := false
		for i := lo; i <= hi; i++ {
			if l := sh.Level(rows, i); l > 0 {
				if sh.IndexHidden(rows, i) {
					sh.SetIndexHidden(rows, i, false)
				}
				sh.SetLevel(rows, i, l-1)
				changed = true
			}
		}
		return changed
	})
}

// ClearOutline removes the sheet's whole outline (Ungroup > Clear
// Outline). As in Excel, rows and columns hidden by collapsed groups stay
// hidden.
func (b *Book) ClearOutline(sheet int) bool {
	defer b.Op("Clear Outline")()
	where := model.Range{R2: model.MaxRows - 1, C2: model.MaxCols - 1}
	return b.changeOutline(sheet, where, func(sh *model.Sheet) bool {
		o := &sh.Outline
		if len(o.RowLevel) == 0 && len(o.ColLevel) == 0 {
			return false
		}
		o.RowLevel, o.ColLevel, o.RowCollapsed, o.ColCollapsed = nil, nil, nil, nil
		return true
	})
}

// SetGroupCollapsed collapses (hides the detail of) or expands group g:
// the outline's - and + buttons.
func (b *Book) SetGroupCollapsed(sheet int, g model.OutlineGroup, collapsed bool) {
	defer b.Op(either(collapsed, "Hide Detail", "Show Detail"))()
	where := axisWhere(g.Rows, g.Lo, g.Hi)
	if g.HasSummary() {
		where = axisWhere(g.Rows, g.Summary, g.Summary)
	}
	b.changeOutline(sheet, where, func(sh *model.Sheet) bool {
		if collapsed {
			sh.CollapseGroup(g)
		} else {
			sh.ExpandGroup(g)
		}
		return true
	})
}

// ShowLevels shows the rows (columns) of outline levels below n: the
// level buttons 1, 2, 3...
func (b *Book) ShowLevels(sheet int, rows bool, n int) {
	defer b.Op("Outline Level")()
	sh := b.WB.Sheets[sheet]
	where := model.Range{R1: sh.ActiveRow, C1: sh.ActiveCol, R2: sh.ActiveRow, C2: sh.ActiveCol}
	b.changeOutline(sheet, where, func(sh *model.Sheet) bool {
		sh.ShowLevels(rows, n)
		return true
	})
}

// HideDetail collapses the groups the selection r is in (Data > Hide
// Detail): for each selected row (column), the innermost expanded group
// whose summary it is, or else that it belongs to. A selection of whole
// rows works on rows only, whole columns on columns only. It reports
// false when no group was found.
func (b *Book) HideDetail(sheet int, r model.Range) bool {
	defer b.Op("Hide Detail")()
	return b.detail(sheet, r, true)
}

// ShowDetail expands the collapsed groups whose summary row (column) is
// selected, or that a selected row is in (Data > Show Detail).
func (b *Book) ShowDetail(sheet int, r model.Range) bool {
	defer b.Op("Show Detail")()
	return b.detail(sheet, r, false)
}

func (b *Book) detail(sheet int, r model.Range, hide bool) bool {
	return b.changeOutline(sheet, r, func(sh *model.Sheet) bool {
		changed := false
		for _, rows := range []bool{true, false} {
			lo, hi := r.R1, r.R2
			if !rows {
				lo, hi = r.C1, r.C2
			}
			if rows && r.R1 == 0 && r.R2 == model.MaxRows-1 && (r.C1 > 0 || r.C2 < model.MaxCols-1) {
				continue // whole columns
			}
			if !rows && r.C1 == 0 && r.C2 == model.MaxCols-1 && (r.R1 > 0 || r.R2 < model.MaxRows-1) {
				continue // whole rows
			}
			for _, g := range detailTargets(sh.Groups(rows), lo, hi, hide) {
				if hide {
					sh.CollapseGroup(g)
				} else {
					sh.ExpandGroup(g)
				}
				changed = true
			}
		}
		return changed
	})
}

// detailTargets picks the groups Hide (or Show) Detail acts on for the
// selected rows lo..hi: for each, the deepest expanded (collapsed) group
// it is the summary of, or failing that, the deepest it is in.
func detailTargets(groups []model.OutlineGroup, lo, hi int, hide bool) []model.OutlineGroup {
	if len(groups) == 0 {
		return nil
	}
	first, last := groups[0].Lo, groups[0].Hi
	for _, g := range groups {
		first, last = min(first, g.Lo-1), max(last, g.Hi+1)
	}
	lo, hi = max(lo, first), min(hi, last)
	seen := map[int]bool{}
	var out []model.OutlineGroup
	for i := lo; i <= hi; i++ {
		best := -1
		for _, inside := range []bool{false, true} {
			for k, g := range groups {
				if g.Collapsed == hide {
					continue
				}
				hit := g.Summary == i
				if inside {
					hit = i >= g.Lo && i <= g.Hi
				}
				if hit && (best < 0 || g.Level > groups[best].Level) {
					best = k
				}
			}
			if best >= 0 {
				break
			}
		}
		if best >= 0 && !seen[best] {
			seen[best] = true
			out = append(out, groups[best])
		}
	}
	return out
}
