package edit

import (
	"errors"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Clip is a block of copied (or cut) cells.
type Clip struct {
	WB    *model.Workbook // the workbook copied from
	Sheet int
	Src   model.Range    // the copied cells, limited to the used area
	Cells [][]model.Cell // Src's cells, row by row
	Cut   bool
	// Rows holds the sheet row of each of Cells' rows when a copy left
	// out rows the AutoFilter hides; nil when they are Src's rows.
	Rows []int
}

// srcRow is the sheet row clip row i was copied from.
func (c *Clip) srcRow(i int) int {
	if c.Rows != nil {
		return c.Rows[i]
	}
	return c.Src.R1 + i
}

// ErrPasteSize is returned when a paste would go past the sheet's edge.
var ErrPasteSize = errors.New("The paste area doesn't fit on the sheet")

// Copy takes a copy of the cells of r for pasting. Values spilled from a
// formula that is copied too are left out (the pasted formula spills
// again); others are copied as values. A copy (not a cut) leaves out the
// rows the AutoFilter hides, as Excel's does, so that they paste as
// adjacent rows.
func (b *Book) Copy(sheet int, r model.Range, cut bool) *Clip {
	sh := b.WB.Sheets[sheet]
	u, ok := usedPart(sh, r)
	if !ok {
		u = model.Range{R1: r.R1, C1: r.C1, R2: r.R1, C2: r.C1}
	}
	clip := &Clip{WB: b.WB, Sheet: sheet, Src: u, Cut: cut}
	if !cut {
		clip.Rows = shownRows(sh, u)
	}
	for row := u.R1; row <= u.R2; row++ {
		if clip.Rows != nil && sh.RowFiltered(row) {
			continue
		}
		line := make([]model.Cell, u.C2-u.C1+1)
		for col := u.C1; col <= u.C2; col++ {
			c := *sh.Get(row, col)
			if ar, ac, ok := sh.SpillAnchor(row, col); ok && c.Formula == "" && u.Contains(ar, ac) {
				c = model.Cell{Style: c.Style}
			}
			line[col-u.C1] = c
		}
		clip.Cells = append(clip.Cells, line)
	}
	return clip
}

// shownRows lists the rows of u the AutoFilter doesn't hide, or nil when
// it hides none of them (or all of them: the copy then takes them all).
func shownRows(sh *model.Sheet, u model.Range) []int {
	if len(sh.Filtered) == 0 {
		return nil
	}
	var rows []int
	for row := u.R1; row <= u.R2; row++ {
		if !sh.RowFiltered(row) {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 || len(rows) == u.R2-u.R1+1 {
		return nil
	}
	return rows
}

// pasteTarget is where a block of h×w cells goes when sel is selected:
// tiled over sel when its size is a multiple of the block's (a single
// cell fills the selection), otherwise once at its top-left cell.
func pasteTarget(sh *model.Sheet, sel model.Range, h, w int) (model.Range, error) {
	// A whole column or row selection only covers the used part.
	if sel.R2 == model.MaxRows-1 {
		sel.R2 = max(sel.R1+h-1, min(sel.R2, sh.UsedRows()-1))
	}
	if sel.C2 == model.MaxCols-1 {
		sel.C2 = max(sel.C1+w-1, min(sel.C2, sh.UsedCols()-1))
	}
	sh2, sw := sel.R2-sel.R1+1, sel.C2-sel.C1+1
	dest := sel
	if sh2%h != 0 || sw%w != 0 {
		dest.R2, dest.C2 = sel.R1+h-1, sel.C1+w-1
	}
	if dest.R2 >= model.MaxRows || dest.C2 >= model.MaxCols {
		return dest, ErrPasteSize
	}
	return dest, nil
}

// Paste pastes clip into sel on sheet: formulas with their relative
// references shifted, values and styles. A cut clip moves the cells
// instead, and formulas pointing at them follow. It returns the pasted
// range.
func (b *Book) Paste(clip *Clip, sheet int, sel model.Range) (model.Range, calc.Result, error) {
	defer b.Op("Paste")()
	h, w := len(clip.Cells), len(clip.Cells[0])
	if clip.Cut {
		sel = model.Range{R1: sel.R1, C1: sel.C1, R2: sel.R1, C2: sel.C1}
	}
	sh := b.WB.Sheets[sheet]
	dest, err := pasteTarget(sh, sel, h, w)
	if err != nil {
		return dest, calc.Result{}, err
	}
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
	if clip.Cut {
		// The source empties first; the destination may overlap it.
		for r := clip.Src.R1; r <= clip.Src.R2; r++ {
			for c := clip.Src.C1; c <= clip.Src.C2; c++ {
				set(clip.Sheet, r, c, model.Cell{})
			}
		}
	}
	dr, dc := dest.R1-clip.Src.R1, dest.C1-clip.Src.C1
	for r := dest.R1; r <= dest.R2; r++ {
		if h == 1 && !clip.Cut && sh.RowFiltered(r) {
			continue // a single row fills only the rows the AutoFilter shows
		}
		for c := dest.C1; c <= dest.C2; c++ {
			i, j := (r-dest.R1)%h, (c-dest.C1)%w
			v := clip.Cells[i][j]
			if v.Formula != "" && !clip.Cut {
				// Each tile shifts by its own offset.
				v.Formula = formula.Shift(v.Formula, r-clip.srcRow(i), c-(clip.Src.C1+j))
			}
			set(sheet, r, c, v)
		}
	}
	if clip.Cut {
		// Formulas anywhere that point into the moved block follow it.
		target := b.WB.Sheets[clip.Sheet].Name
		for si, s := range b.WB.Sheets {
			for r := 0; r < s.UsedRows(); r++ {
				for c := 0; c < s.RowLen(r); c++ {
					k := [3]int{si, r, c}
					var f string
					if ch, ok := cells[k]; ok {
						f = ch.after.Formula
					} else {
						f = s.Get(r, c).Formula
					}
					if f == "" {
						continue
					}
					if si == sheet && clip.Sheet != sheet && dest.Contains(r, c) {
						continue // moved to another sheet: refs keep pointing home
					}
					g := formula.MoveRefs(f, s.Name, target, clip.Src, dr, dc)
					if g == f {
						continue
					}
					if ch, ok := cells[k]; ok {
						ch.after.Formula = g
					} else {
						nv := *s.Get(r, c)
						nv.Formula = g
						set(si, r, c, nv)
					}
				}
			}
		}
	}
	list := make([]cellChange, 0, len(order))
	for _, k := range order {
		list = append(list, *cells[k])
	}
	if clip.Cut && clip.Sheet == sheet {
		if names := b.moveNames(b.WB.Sheets[clip.Sheet].Name, clip.Src, dr, dc); names != nil {
			// Names pointing into the moved block follow it too.
			names.set(b.WB, true)
			b.setCells(list, true)
			res := b.recalculateAll()
			b.record(&change{where: Where{sheet, dest}, cells: list, names: names})
			return dest, res, nil
		}
	}
	res := b.apply(Where{sheet, dest}, list)
	return dest, res, nil
}

// PasteText pastes text from another program: tab-separated fields,
// one row per line, each interpreted as typed input. It goes at sel's
// top-left cell (a single value fills the selection) and keeps the
// destination cells' styles.
func (b *Book) PasteText(text string, sheet int, sel model.Range) (model.Range, calc.Result, error) {
	defer b.Op("Paste")()
	rows := SplitText(text)
	if len(rows) == 0 {
		return sel, calc.Result{}, nil
	}
	w := 0
	for _, r := range rows {
		w = max(w, len(r))
	}
	sh := b.WB.Sheets[sheet]
	h := len(rows)
	dest := model.Range{R1: sel.R1, C1: sel.C1, R2: sel.R1 + h - 1, C2: sel.C1 + w - 1}
	if h == 1 && w == 1 {
		var err error
		if dest, err = pasteTarget(sh, sel, 1, 1); err != nil {
			return dest, calc.Result{}, err
		}
	}
	if dest.R2 >= model.MaxRows || dest.C2 >= model.MaxCols {
		return dest, calc.Result{}, ErrPasteSize
	}
	child := spillChildren(sh)
	var cells []cellChange
	for r := dest.R1; r <= dest.R2; r++ {
		if h == 1 && sh.RowFiltered(r) {
			continue // a single value fills only the rows the AutoFilter shows
		}
		line := rows[(r-dest.R1)%h]
		for c := dest.C1; c <= dest.C2; c++ {
			field := ""
			if j := c - dest.C1; j < len(line) {
				field = line[j]
			}
			before := snapshot(sh, child, r, c)
			v, err := b.inputCell(before.Style, field)
			if err != nil {
				// A formula that doesn't parse is pasted as text.
				v = model.Cell{Style: before.Style, Kind: model.KindText, Text: field}
			}
			cells = append(cells, cellChange{sheet, r, c, before, v})
		}
	}
	return dest, b.apply(Where{sheet, dest}, cells), nil
}

// SplitText splits clipboard text into rows of tab-separated fields.
// Fields in double quotes may hold tabs, line breaks and doubled quotes,
// as Excel writes cells with line breaks. A final line break is ignored.
func SplitText(s string) [][]string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var rows [][]string
	var row []string
	var field strings.Builder
	for i := 0; i < len(s); {
		if field.Len() == 0 && s[i] == '"' {
			// A quoted field runs to the closing quote before a tab or
			// line break; otherwise the quote is literal.
			j, ok := i+1, false
			var q strings.Builder
			for j < len(s) {
				if s[j] == '"' {
					if j+1 < len(s) && s[j+1] == '"' {
						q.WriteByte('"')
						j += 2
						continue
					}
					ok = j+1 == len(s) || s[j+1] == '\t' || s[j+1] == '\n'
					break
				}
				q.WriteByte(s[j])
				j++
			}
			if ok {
				field.WriteString(q.String())
				i = j + 1
				continue
			}
		}
		switch s[i] {
		case '\t':
			row = append(row, field.String())
			field.Reset()
		case '\n':
			rows = append(rows, append(row, field.String()))
			row = nil
			field.Reset()
		default:
			field.WriteByte(s[i])
		}
		i++
	}
	if field.Len() > 0 || row != nil {
		rows = append(rows, append(row, field.String()))
	}
	return rows
}

// CopyText renders cells as tab-separated text for other programs, with
// fields holding tabs, line breaks or quotes in double quotes, leaving
// out the rows the AutoFilter hides.
func CopyText(sh *model.Sheet, r model.Range) string {
	u, ok := usedPart(sh, r)
	if !ok {
		return "\r\n"
	}
	var b strings.Builder
	rows := shownRows(sh, u)
	for row := u.R1; row <= u.R2; row++ {
		if rows != nil && sh.RowFiltered(row) {
			continue // as Copy leaves them out
		}
		for col := u.C1; col <= u.C2; col++ {
			if col > u.C1 {
				b.WriteByte('\t')
			}
			t := sh.Get(row, col).Text
			if strings.ContainsAny(t, "\t\n\"") {
				t = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
			}
			b.WriteString(t)
		}
		b.WriteString("\r\n")
	}
	return b.String()
}
