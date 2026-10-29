package edit

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// copiedCell is the cell at from as copied to to: formulas with their
// relative references shifted, values spilled from elsewhere as values.
func copiedCell(sh *model.Sheet, child func(r, c int) bool, fr, fc, tr, tc int) model.Cell {
	cell := *sh.Get(fr, fc)
	if cell.Formula != "" {
		cell.Formula = formula.Shift(cell.Formula, tr-fr, tc-fc)
	}
	return cell
}

// FillDown copies the top row of sel into the rest of it (Ctrl+D); a
// single-row selection copies the row above it.
func (b *Book) FillDown(sheet int, sel model.Range) calc.Result {
	defer b.Op("Fill Down")()
	return b.fill(sheet, sel, true)
}

// FillRight copies the left column of sel into the rest of it (Ctrl+R);
// a single-column selection copies the column to its left.
func (b *Book) FillRight(sheet int, sel model.Range) calc.Result {
	defer b.Op("Fill Right")()
	return b.fill(sheet, sel, false)
}

func (b *Book) fill(sheet int, sel model.Range, down bool) calc.Result {
	sh := b.WB.Sheets[sheet]
	// Whole columns or rows only fill the used area.
	if sel.R2 == model.MaxRows-1 {
		sel.R2 = max(sh.UsedRows()-1, sel.R1)
	}
	if sel.C2 == model.MaxCols-1 {
		sel.C2 = max(sh.UsedCols()-1, sel.C1)
	}
	src, dest := sel, sel
	if down {
		if sel.R1 == sel.R2 {
			if sel.R1 == 0 {
				return calc.Result{}
			}
			src.R1, src.R2 = sel.R1-1, sel.R1-1
		} else {
			// Under an AutoFilter the first row shown is copied.
			top := sel.R1
			for top <= sel.R2 && sh.RowFiltered(top) {
				top++
			}
			if top >= sel.R2 {
				return calc.Result{}
			}
			src.R1, src.R2 = top, top
			dest.R1 = top + 1
		}
	} else {
		if sel.C1 == sel.C2 {
			if sel.C1 == 0 {
				return calc.Result{}
			}
			src.C1, src.C2 = sel.C1-1, sel.C1-1
		} else {
			src.C2 = sel.C1
			dest.C1 = sel.C1 + 1
		}
	}
	child := spillChildren(sh)
	var cells []cellChange
	for r := dest.R1; r <= dest.R2; r++ {
		if sh.RowFiltered(r) {
			continue // as in Excel, rows the AutoFilter hides are left alone
		}
		for c := dest.C1; c <= dest.C2; c++ {
			fr, fc := src.R1, c
			if !down {
				fr, fc = r, src.C1
			}
			before := snapshot(sh, child, r, c)
			after := copiedCell(sh, child, fr, fc, r, c)
			if before != after {
				cells = append(cells, cellChange{sheet, r, c, before, after})
			}
		}
	}
	return b.apply(Where{sheet, sel}, cells)
}

// ErrFillShape is returned for an AutoFill target that doesn't extend
// the source along one side.
var ErrFillShape = errors.New("The fill area must extend the selection in one direction")

// AutoFill extends the cells of src over to (which contains src and
// extends it down, up, right or left), as dragging the fill handle does:
// numbers continue their trend (a single number is copied), dates step
// by a day, day and month names and text ending in a number count on,
// and anything else, formulas included, is copied with references
// shifted. A to inside src clears the cells of src outside it. Undoable
// as one step.
func (b *Book) AutoFill(sheet int, src, to model.Range) (calc.Result, error) {
	defer b.Op("AutoFill")()
	sh := b.WB.Sheets[sheet]
	child := spillChildren(sh)
	var cells []cellChange
	set := func(r, c int, v model.Cell) {
		before := snapshot(sh, child, r, c)
		if before != v {
			cells = append(cells, cellChange{sheet, r, c, before, v})
		}
	}
	switch {
	case to == src:
		return calc.Result{}, nil
	case to.R1 == src.R1 && to.C1 == src.C1 && to.R2 <= src.R2 && to.C2 <= src.C2 && (to.R2 == src.R2 || to.C2 == src.C2):
		// Dragged back into the selection: clear what was left out.
		for r := src.R1; r <= src.R2; r++ {
			for c := src.C1; c <= src.C2; c++ {
				if !to.Contains(r, c) {
					set(r, c, model.Cell{Style: sh.Get(r, c).Style})
				}
			}
		}
		return b.apply(Where{sheet, to}, cells), nil
	}
	if to.R2 >= model.MaxRows || to.C2 >= model.MaxCols || to.R1 < 0 || to.C1 < 0 {
		return calc.Result{}, ErrPasteSize
	}
	vertical := to.C1 == src.C1 && to.C2 == src.C2
	horizontal := to.R1 == src.R1 && to.R2 == src.R2
	if vertical == horizontal {
		return calc.Result{}, ErrFillShape
	}
	if vertical {
		backward := to.R1 < src.R1
		for c := src.C1; c <= src.C2; c++ {
			var line []fpos
			for r := src.R1; r <= src.R2; r++ {
				line = append(line, fpos{r, c})
			}
			var targets []fpos
			if backward {
				reverse(line)
				for r := src.R1 - 1; r >= to.R1; r-- {
					targets = append(targets, fpos{r, c})
				}
			} else {
				for r := src.R2 + 1; r <= to.R2; r++ {
					targets = append(targets, fpos{r, c})
				}
			}
			b.series(sh, child, line, targets, set)
		}
	} else {
		backward := to.C1 < src.C1
		for r := src.R1; r <= src.R2; r++ {
			var line []fpos
			for c := src.C1; c <= src.C2; c++ {
				line = append(line, fpos{r, c})
			}
			var targets []fpos
			if backward {
				reverse(line)
				for c := src.C1 - 1; c >= to.C1; c-- {
					targets = append(targets, fpos{r, c})
				}
			} else {
				for c := src.C2 + 1; c <= to.C2; c++ {
					targets = append(targets, fpos{r, c})
				}
			}
			b.series(sh, child, line, targets, set)
		}
	}
	return b.apply(Where{sheet, to}, cells), nil
}

type fpos struct{ r, c int }

func reverse(p []fpos) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

// series fills targets (in fill order) from the source cells line (in
// fill order: nearest the targets last).
func (b *Book) series(sh *model.Sheet, child func(r, c int) bool, line, targets []fpos, set func(r, c int, v model.Cell)) {
	k := len(line)
	src := make([]model.Cell, k)
	for i, p := range line {
		src[i] = snapshot(sh, child, p.r, p.c)
	}
	// Numbers: a trend; a single date steps by a day.
	if allCells(src, func(c *model.Cell) bool { return c.Formula == "" && c.Kind == model.KindNumber }) {
		st := b.style(src[0].Style)
		if k > 1 || numfmt.IsDate(st.NumFmtID, st.NumFmtCode) {
			slope, icpt := 1.0, src[0].Num
			if k > 1 {
				slope, icpt = linearFit(src)
			}
			for t, p := range targets {
				v := src[(k+t)%k]
				v.Num = icpt + slope*float64(k+t)
				if k > 1 {
					v.Num = formula.Round15(v.Num)
				}
				st := b.style(v.Style)
				v.Text = b.nf.Format(v.Num, st.NumFmtID, st.NumFmtCode)
				set(p.r, p.c, v)
			}
			return
		}
	}
	// Text ending in a number: Item1, Item2, ...
	if prefix, suffixes, ok := numberedTexts(src); ok {
		nums := make([]model.Cell, k)
		for i, s := range suffixes {
			nums[i].Num = float64(s.n)
		}
		slope, icpt := 1.0, nums[0].Num
		if k > 1 {
			slope, icpt = linearFit(nums)
		}
		width := suffixes[k-1].width
		for t, p := range targets {
			v := src[(k+t)%k]
			n := int(math.Round(icpt + slope*float64(k+t)))
			v.Text = prefix + padNumber(n, width)
			set(p.r, p.c, v)
		}
		return
	}
	// Day and month names.
	if list, idx, form, ok := namedSeries(src); ok {
		step := 1
		if k > 1 {
			step = idx[1] - idx[0]
		}
		n := len(list.long)
		for t, p := range targets {
			v := src[(k+t)%k]
			i := ((idx[k-1]+step*(t+1))%n + n) % n
			v.Text = form(list, i)
			set(p.r, p.c, v)
		}
		return
	}
	// Otherwise copy the pattern, shifting formulas.
	for t, p := range targets {
		from := line[(k+t)%k]
		set(p.r, p.c, copiedCell(sh, child, from.r, from.c, p.r, p.c))
	}
}

func allCells(cs []model.Cell, f func(*model.Cell) bool) bool {
	for i := range cs {
		if !f(&cs[i]) {
			return false
		}
	}
	return len(cs) > 0
}

// linearFit is the least-squares line through (i, cs[i].Num), as Excel's
// AutoFill trend uses.
func linearFit(cs []model.Cell) (slope, icpt float64) {
	n := float64(len(cs))
	var sx, sy, sxx, sxy float64
	for i, c := range cs {
		x := float64(i)
		sx += x
		sy += c.Num
		sxx += x * x
		sxy += x * c.Num
	}
	slope = (n*sxy - sx*sy) / (n*sxx - sx*sx)
	icpt = (sy - slope*sx) / n
	return slope, icpt
}

type numSuffix struct{ n, width int }

// numberedTexts reports whether every cell is text with the same prefix
// followed by a number (Item1, Q3, Week 10).
func numberedTexts(cs []model.Cell) (string, []numSuffix, bool) {
	var prefix string
	var out []numSuffix
	for i, c := range cs {
		if c.Formula != "" || c.Kind != model.KindText {
			return "", nil, false
		}
		j := len(c.Text)
		for j > 0 && c.Text[j-1] >= '0' && c.Text[j-1] <= '9' {
			j--
		}
		digits := c.Text[j:]
		if digits == "" || len(digits) > 9 {
			return "", nil, false
		}
		if i == 0 {
			prefix = c.Text[:j]
		} else if c.Text[:j] != prefix {
			return "", nil, false
		}
		n, _ := strconv.Atoi(digits)
		w := 0
		if digits[0] == '0' && len(digits) > 1 {
			w = len(digits)
		}
		out = append(out, numSuffix{n, w})
	}
	return prefix, out, len(out) > 0
}

func padNumber(n, width int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	if neg {
		return "-" + s
	}
	return s
}

// nameList is a custom list AutoFill counts through.
type nameList struct{ long, short []string }

var fillLists = []nameList{
	{
		long:  []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		short: []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	},
	{
		long:  []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		short: []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	},
}

// namedSeries reports whether every cell is a day or month name of one
// list, with their indexes and a function writing an entry in the first
// cell's form (short or long, upper, lower or capitalized).
func namedSeries(cs []model.Cell) (nameList, []int, func(nameList, int) string, bool) {
	for _, list := range fillLists {
		var idx []int
		short := false
		ok := true
		for i, c := range cs {
			if c.Formula != "" || c.Kind != model.KindText {
				return nameList{}, nil, nil, false
			}
			j, s := findName(list, c.Text)
			if j < 0 {
				ok = false
				break
			}
			if i == 0 {
				short = s
			}
			idx = append(idx, j)
		}
		if !ok {
			continue
		}
		first := cs[0].Text
		caseOf := func(s string) string {
			switch {
			case first == strings.ToUpper(first):
				return strings.ToUpper(s)
			case first == strings.ToLower(first):
				return strings.ToLower(s)
			}
			return s
		}
		form := func(l nameList, i int) string {
			if short {
				return caseOf(l.short[i])
			}
			return caseOf(l.long[i])
		}
		return list, idx, form, true
	}
	return nameList{}, nil, nil, false
}

func findName(l nameList, s string) (int, bool) {
	for i := range l.long {
		if strings.EqualFold(s, l.long[i]) {
			return i, false
		}
	}
	for i := range l.short {
		if strings.EqualFold(s, l.short[i]) {
			return i, true
		}
	}
	return -1, false
}
