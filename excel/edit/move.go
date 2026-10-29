package edit

import (
	"slices"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Dragging a selection's border: the cells move (or with Ctrl, are
// copied) by whole rows and columns, with their formats, notes, links and
// merged areas, as one undoable change.

// MoveTarget is where src lands when dragged by dRow, dCol; ok is false
// when that would go past the sheet's edge.
func MoveTarget(src model.Range, dRow, dCol int) (model.Range, bool) {
	d := model.Range{R1: src.R1 + dRow, C1: src.C1 + dCol, R2: src.R2 + dRow, C2: src.C2 + dCol}
	return d, d.R1 >= 0 && d.C1 >= 0 && d.R2 < model.MaxRows && d.C2 < model.MaxCols
}

// MoveOverwrites reports whether moving (or copying) src by dRow, dCol
// would replace cells that have content, for Excel's "There's already
// data here" question. Cells of src that a move empties don't count.
func (b *Book) MoveOverwrites(sheet int, src model.Range, dRow, dCol int, copy bool) bool {
	dest, ok := MoveTarget(src, dRow, dCol)
	if !ok {
		return false
	}
	sh := b.WB.Sheets[sheet]
	u, ok := usedPart(sh, dest)
	if !ok {
		return false
	}
	for r := u.R1; r <= u.R2; r++ {
		for c := u.C1; c <= min(u.C2, sh.RowLen(r)-1); c++ {
			if !copy && src.Contains(r, c) {
				continue
			}
			if !sh.Get(r, c).IsBlank() {
				return true
			}
		}
	}
	return false
}

// MoveCells moves the cells of src by dRow, dCol (copy: copies them) on
// sheet: contents, formats, notes, hyperlinks, data validation and the merged
// areas inside src. Moved formulas keep their references, and formulas anywhere that
// point into src follow the cells; copied formulas shift their relative
// references, as a paste does. It returns the destination.
func (b *Book) MoveCells(sheet int, src model.Range, dRow, dCol int, copy bool) (model.Range, calc.Result, error) {
	defer b.Op("Drag and Drop")()
	dest, ok := MoveTarget(src, dRow, dCol)
	if !ok {
		return dest, calc.Result{}, ErrPasteSize
	}
	if dRow == 0 && dCol == 0 {
		return dest, calc.Result{}, nil
	}
	sh := b.WB.Sheets[sheet]
	child := spillChildren(sh)
	cells := map[[3]int]*cellChange{}
	var order [][3]int
	set := func(s, r, c int, v model.Cell) {
		k := [3]int{s, r, c}
		if ch, ok := cells[k]; ok {
			ch.after = v
			return
		}
		ws := b.WB.Sheets[s]
		cells[k] = &cellChange{s, r, c, snapshot(ws, spillChildren(ws), r, c), v}
		order = append(order, k)
	}

	// Read the source first: the destination may overlap it. Past the
	// used area every cell is empty, so only the used part (and the
	// destination cells it lands on) needs visiting.
	used, has := usedPart(sh, src)
	type moved struct {
		r, c int // destination
		v    model.Cell
	}
	var block []moved
	if has {
		for r := used.R1; r <= used.R2; r++ {
			for c := used.C1; c <= min(used.C2, sh.RowLen(r)-1); c++ {
				v := *sh.Get(r, c)
				if ar, ac, ok := sh.SpillAnchor(r, c); ok && v.Formula == "" && child(r, c) && src.Contains(ar, ac) {
					v = model.Cell{Style: v.Style} // the moved formula spills again
				}
				if v.Formula != "" && copy {
					v.Formula = formula.Shift(v.Formula, dRow, dCol)
				}
				block = append(block, moved{r + dRow, c + dCol, v})
			}
		}
	}
	if !copy && has {
		for r := used.R1; r <= used.R2; r++ {
			for c := used.C1; c <= min(used.C2, sh.RowLen(r)-1); c++ {
				set(sheet, r, c, model.Cell{})
			}
		}
	}
	// Destination cells the block doesn't cover become empty.
	if du, ok := usedPart(sh, dest); ok {
		for r := du.R1; r <= du.R2; r++ {
			for c := du.C1; c <= min(du.C2, sh.RowLen(r)-1); c++ {
				if !sh.Get(r, c).IsBlank() || sh.Get(r, c).Style != 0 {
					set(sheet, r, c, model.Cell{})
				}
			}
		}
	}
	for _, m := range block {
		set(sheet, m.r, m.c, m.v)
	}
	if !copy {
		// Formulas anywhere that point into the moved block follow it:
		// the cells already changed, then the rest.
		move := func(f, home string) string { return formula.MoveRefs(f, home, sh.Name, src, dRow, dCol) }
		for _, k := range order {
			ch := cells[k]
			if ch.after.Formula != "" {
				ch.after.Formula = move(ch.after.Formula, b.WB.Sheets[k[0]].Name)
			}
		}
		for si, s := range b.WB.Sheets {
			for r := 0; r < s.UsedRows(); r++ {
				for c := 0; c < s.RowLen(r); c++ {
					f := s.Get(r, c).Formula
					if _, done := cells[[3]int{si, r, c}]; done || f == "" {
						continue
					}
					if g := move(f, s.Name); g != f {
						nv := *s.Get(r, c)
						nv.Formula = g
						set(si, r, c, nv)
					}
				}
			}
		}
	}

	annots := b.moveAnnots(sheet, src, dest, dRow, dCol, copy)
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = ch.before
	ch.after.merges = moveMerges(sh.Merges, src, dest, dRow, dCol, copy)
	ch.after.valids = model.MoveValidations(sh.Validations, src, dRow, dCol, copy)

	list := make([]cellChange, 0, len(order))
	for _, k := range order {
		list = append(list, *cells[k])
	}
	var names *namesChange
	if !copy {
		names = b.moveNames(sh.Name, src, dRow, dCol)
	}
	b.setAnnots(annots, true)
	ch.set(b.WB, true)
	res := b.setCells(list, true)
	if names != nil {
		names.set(b.WB, true)
		res = b.recalculateAll()
	}
	b.record(&change{where: Where{sheet, dest}, cells: list, annots: annots, sheet: ch, names: names})
	return dest, res, nil
}

// namesChange is a change of the workbook's defined names.
type namesChange struct {
	before, after []model.DefinedName
}

func (c *namesChange) set(wb *model.Workbook, after bool) {
	if after {
		wb.Names = slices.Clone(c.after)
	} else {
		wb.Names = slices.Clone(c.before)
	}
}

// moveNames is the change that re-points the defined names referring
// into src on sheet target to where its cells moved (dRow, dCol), as
// Excel does for cut and pasted (or dragged) cells; nil when no name
// points there.
func (b *Book) moveNames(target string, src model.Range, dRow, dCol int) *namesChange {
	var after []model.DefinedName
	for i, n := range b.WB.Names {
		home := ""
		if n.Sheet >= 0 && n.Sheet < len(b.WB.Sheets) {
			home = b.WB.Sheets[n.Sheet].Name
		}
		f := formula.MoveRefs(n.Formula, home, target, src, dRow, dCol)
		if f == n.Formula {
			continue
		}
		if after == nil {
			after = slices.Clone(b.WB.Names)
		}
		after[i].Formula = f
	}
	if after == nil {
		return nil
	}
	return &namesChange{before: slices.Clone(b.WB.Names), after: after}
}

// moveMerges is merges after src's merged areas move (or are copied) to
// dest; areas the destination overlaps are unmerged.
func moveMerges(merges []model.Range, src, dest model.Range, dRow, dCol int, copy bool) []model.Range {
	var out, add []model.Range
	for _, m := range merges {
		inside := src.Contains(m.R1, m.C1) && src.Contains(m.R2, m.C2)
		if inside {
			add = append(add, model.Range{R1: m.R1 + dRow, C1: m.C1 + dCol, R2: m.R2 + dRow, C2: m.C2 + dCol})
			if !copy {
				continue
			}
		}
		if overlaps(m, dest) {
			continue
		}
		out = append(out, m)
	}
	return append(out, add...)
}

// moveAnnots moves (or copies) the notes and hyperlinks of src to dest;
// those already in dest are replaced.
func (b *Book) moveAnnots(sheet int, src, dest model.Range, dRow, dCol int, copy bool) []annotChange {
	sh := b.WB.Sheets[sheet]
	type key struct {
		r, c int
		link bool
	}
	changes := map[key]*annotChange{}
	var order []key
	get := func(r, c int, link bool) *annotChange {
		k := key{r, c, link}
		if ch, ok := changes[k]; ok {
			return ch
		}
		ch := &annotChange{sheet: sheet, row: r, col: c, link: link}
		if link {
			ch.beforeLink = linkPtr(sh, r, c)
			ch.afterLink = ch.beforeLink
		} else {
			ch.beforeNote = notePtr(sh, r, c)
			ch.afterNote = ch.beforeNote
		}
		changes[k] = ch
		order = append(order, k)
		return ch
	}
	// Notes and links: those in dest go; src's go to dest (and leave src
	// when moving).
	var notes []struct {
		r, c int
		n    model.Note
	}
	var links []struct {
		r, c int
		h    model.Hyperlink
	}
	for k, n := range sh.Notes {
		if src.Contains(k[0], k[1]) {
			notes = append(notes, struct {
				r, c int
				n    model.Note
			}{k[0], k[1], n})
		}
		if dest.Contains(k[0], k[1]) || !copy && src.Contains(k[0], k[1]) {
			get(k[0], k[1], false).afterNote = nil
		}
	}
	for k, h := range sh.Links {
		if src.Contains(k[0], k[1]) {
			links = append(links, struct {
				r, c int
				h    model.Hyperlink
			}{k[0], k[1], h})
		}
		if dest.Contains(k[0], k[1]) || !copy && src.Contains(k[0], k[1]) {
			get(k[0], k[1], true).afterLink = nil
		}
	}
	for _, n := range notes {
		v := n.n
		get(n.r+dRow, n.c+dCol, false).afterNote = &v
	}
	for _, l := range links {
		v := l.h
		get(l.r+dRow, l.c+dCol, true).afterLink = &v
	}
	// Map order is random; keep undo deterministic.
	slices.SortFunc(order, func(a, b key) int {
		if a.r != b.r {
			return a.r - b.r
		}
		if a.c != b.c {
			return a.c - b.c
		}
		if a.link == b.link {
			return 0
		}
		if a.link {
			return 1
		}
		return -1
	})
	var list []annotChange
	for _, k := range order {
		ch := changes[k]
		if ch.link && ch.beforeLink == ch.afterLink || !ch.link && ch.beforeNote == ch.afterNote {
			continue // pointer equality: untouched
		}
		list = append(list, *ch)
	}
	return list
}
