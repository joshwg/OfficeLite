package edit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"officelite/common/colors"
	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
	"officelite/excel/numfmt"
	"officelite/excel/pivot"
)

// PivotTables: creating one, changing its fields and options, refreshing
// it from its source and moving or deleting it. Every change lays the
// report out again (excel/pivot) and writes its cells, as one undo step
// that also restores the definition (kept with the sheet's properties).
// A report's cells can't be edited by hand.

var (
	// ErrPivotChange is Excel's refusal to edit a report's cells.
	ErrPivotChange = errors.New("You can't change this part of a PivotTable report.")
	// ErrPivotOverlap is Excel's refusal to let two reports overlap.
	ErrPivotOverlap = errors.New("A PivotTable report cannot overlap another PivotTable report.")
	// ErrPivotOverwrite asks before a report covers other cells with
	// data; the caller asks "There's already data in ..." and tries
	// again with overwrite set.
	ErrPivotOverwrite = errors.New("There's already data in the cells the PivotTable report needs.")
	// ErrPivotSource is a source that isn't a reference.
	ErrPivotSource = errors.New("Reference isn't valid.")
)

// The PivotStyleLight16 colors: the header and total bands and their
// lines (Office theme accent 1, 80% and 40% lighter).
var (
	pivotBand = colors.RGB(0xDDEBF7)
	pivotLine = colors.RGB(0x9BC2E6)
)

// PivotSourceRange resolves a source's formula text (a reference or a
// defined name) on sheet to a sheet and range.
func (b *Book) PivotSourceRange(sheet int, src string) (int, model.Range, error) {
	src = strings.TrimPrefix(strings.TrimSpace(src), "=")
	if src == "" {
		return 0, model.Range{}, ErrPivotSource
	}
	_, ref := calc.Evaluate(b.WB, functions.Default(), src, sheet, 0, 0, b.now(), nil)
	if ref == nil || ref.Sheet < 0 || ref.Sheet >= len(b.WB.Sheets) {
		return 0, model.Range{}, ErrPivotSource
	}
	return ref.Sheet, model.Range{R1: ref.R1, C1: ref.C1, R2: ref.R2, C2: ref.C2}, nil
}

// PivotSource reads a report's source data.
func (b *Book) PivotSource(sheet int, pt *model.PivotTable) (*pivot.Source, error) {
	si, r, err := b.PivotSourceRange(sheet, pt.Source)
	if err != nil {
		return nil, err
	}
	return pivot.ReadSource(b.WB.Sheets[si], si, r, b.isDateStyle)
}

func (b *Book) isDateStyle(i uint32) bool {
	st := b.style(i)
	return numfmt.IsDate(st.NumFmtID, st.NumFmtCode)
}

// SourceText is the formula text naming range r of sheet si as a
// report's source: 'Sales Data'!$A$1:$D$9.
func (b *Book) SourceText(si int, r model.Range) string {
	ref := "$" + model.ColName(r.C1) + "$" + fmt.Sprint(r.R1+1) + ":$" + model.ColName(r.C2) + "$" + fmt.Sprint(r.R2+1)
	return formula.QuoteSheet(b.WB.Sheets[si].Name) + "!" + ref
}

// NewPivot starts a report on source (formula text) with its body's
// top-left at row, col of sheet: the source's columns as fields, none in
// the areas yet. It doesn't change the workbook; SetPivot puts it in.
func (b *Book) NewPivot(sheet int, source string, row, col int) (*model.PivotTable, error) {
	pt := &model.PivotTable{Name: b.nextPivotName(), Source: source, Row: row, Col: col}
	src, err := b.PivotSource(sheet, pt)
	if err != nil {
		return nil, err
	}
	if src.Range.R2 <= src.Range.R1 && len(src.Fields) == 1 && src.Fields[0].Name == "" {
		return nil, pivot.ErrFieldName
	}
	for _, f := range src.Fields {
		pt.Fields = append(pt.Fields, model.PivotField{Name: f.Name})
	}
	return pt, nil
}

// nextPivotName is PivotTable<n> with the first n no report has.
func (b *Book) nextPivotName() string {
	used := map[string]bool{}
	for _, sh := range b.WB.Sheets {
		for _, p := range sh.Pivots {
			used[strings.ToLower(p.Name)] = true
		}
	}
	for n := 1; ; n++ {
		name := fmt.Sprintf("PivotTable%d", n)
		if !used[strings.ToLower(name)] {
			return name
		}
	}
}

// pivotChange is the cells of a report laid out on sheet with the sheet's
// reports after the change; overwrite allows covering other data.
func (b *Book) pivotCells(sheet int, pts []*model.PivotTable, i int, overwrite bool) ([]cellChange, error) {
	sh := b.WB.Sheets[sheet]
	pt := pts[i]
	src, err := b.PivotSource(sheet, pt)
	if err != nil {
		return nil, err
	}
	pivot.Sync(pt, src)
	res := pivot.Compute(pt, src, b.nf)
	if res.Out.Area.R2 >= model.MaxRows || res.Out.Area.C2 >= model.MaxCols {
		return nil, errors.New("The PivotTable report doesn't fit on the sheet.")
	}
	var old model.Range
	hadOld := i < len(sh.Pivots)
	if hadOld {
		old = sh.Pivots[i].Out.Area
	}
	area := res.Out.Area
	for j, p := range pts {
		if j != i && overlaps(p.Out.Area, area) {
			return nil, ErrPivotOverlap
		}
	}
	if !overwrite {
		for r := area.R1; r <= min(area.R2, sh.UsedRows()-1); r++ {
			for c := area.C1; c <= min(area.C2, sh.RowLen(r)-1); c++ {
				if (!hadOld || !old.Contains(r, c)) && !sh.Get(r, c).IsBlank() {
					return nil, ErrPivotOverwrite
				}
			}
		}
	}
	pt.Row, pt.Col, pt.Out = res.Row, res.Col, res.Out

	after := map[[2]int]model.Cell{}
	for r := area.R1; r <= area.R2; r++ {
		for c := area.C1; c <= area.C2; c++ {
			after[[2]int{r, c}] = model.Cell{}
		}
	}
	styles := map[pivotLook]uint32{}
	for _, pc := range res.Cells {
		after[[2]int{pc.Row, pc.Col}] = b.pivotCell(pc, styles)
	}
	child := spillChildren(sh)
	var cells []cellChange
	add := func(r, c int, v model.Cell) {
		if before := snapshot(sh, child, r, c); before != v {
			cells = append(cells, cellChange{sheet, r, c, before, v})
		}
	}
	if hadOld {
		for r := old.R1; r <= old.R2; r++ {
			for c := old.C1; c <= old.C2; c++ {
				if !area.Contains(r, c) {
					add(r, c, model.Cell{})
				}
			}
		}
	}
	for r := area.R1; r <= area.R2; r++ {
		for c := area.C1; c <= area.C2; c++ {
			add(r, c, after[[2]int{r, c}])
		}
	}
	return cells, nil
}

// pivotLook is a report cell's style, before it is added to the
// workbook's styles.
type pivotLook struct {
	look     pivot.Look
	indent   int
	fmtID    int
	fmtCode  string
	srcStyle uint32
}

// pivotCell turns a computed report cell into a sheet cell styled after
// PivotStyleLight16.
func (b *Book) pivotCell(pc pivot.Cell, styles map[pivotLook]uint32) model.Cell {
	k := pivotLook{look: pc.Look, indent: pc.Indent, fmtID: pc.NumFmtID, fmtCode: pc.NumFmtCode}
	if pc.Look&pivot.LookSrcFmt != 0 {
		k.srcStyle = pc.SrcStyle
	}
	idx, ok := styles[k]
	if !ok {
		st := b.style(0)
		if pc.Look&pivot.LookFill != 0 {
			st.Fill = pivotBand
		}
		if pc.Look&pivot.LookBold != 0 {
			st.Font.Bold = true
		}
		if pc.Look&pivot.LookTop != 0 {
			st.Border[model.Top] = model.Edge{Style: model.BorderThin, Color: pivotLine}
		}
		if pc.Look&pivot.LookBottom != 0 {
			st.Border[model.Bottom] = model.Edge{Style: model.BorderThin, Color: pivotLine}
		}
		st.Indent = pc.Indent
		if pc.Indent > 0 {
			st.HAlign = model.HAlignLeft
		}
		st.NumFmtID, st.NumFmtCode = pc.NumFmtID, pc.NumFmtCode
		if pc.Look&pivot.LookSrcFmt != 0 {
			s := b.style(pc.SrcStyle)
			st.NumFmtID, st.NumFmtCode = s.NumFmtID, s.NumFmtCode
			st.HAlign = model.HAlignLeft // labels, even numbers and dates
		}
		idx = b.addStyle(st)
		styles[k] = idx
	}
	c := model.Cell{Kind: pc.Kind, Num: pc.Num, Text: pc.Text, Style: idx}
	if pc.Kind == model.KindEmpty {
		c.Text = ""
	}
	return c
}

// SetPivot lays out pt on sheet, replacing report idx (or adding pt
// when idx < 0), as one undo step. Without overwrite, covering cells
// with data returns ErrPivotOverwrite and changes nothing.
func (b *Book) SetPivot(sheet, idx int, pt *model.PivotTable, overwrite bool) (calc.Result, error) {
	defer b.Op("PivotTable")()
	sh := b.WB.Sheets[sheet]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	pts := model.ClonePivots(sh.Pivots)
	pt = pt.Clone()
	if idx < 0 || idx >= len(pts) {
		idx = len(pts)
		pts = append(pts, pt)
	} else {
		pts[idx] = pt
	}
	cells, err := b.pivotCells(sheet, pts, idx, overwrite)
	if err != nil {
		return calc.Result{}, err
	}
	ch.after = propsOf(sh)
	ch.after.pivots = pts
	return b.applySheet(Where{sheet, pt.Out.Area}, ch, cells), nil
}

// RefreshPivot updates report idx from the current source data (Refresh).
func (b *Book) RefreshPivot(sheet, idx int, overwrite bool) (calc.Result, error) {
	defer b.Op("Refresh")()
	return b.SetPivot(sheet, idx, b.WB.Sheets[sheet].Pivots[idx], overwrite)
}

// RefreshAll refreshes every report of the workbook as one undo step.
// It stops at the first report that can't be refreshed.
func (b *Book) RefreshAll(overwrite bool) (calc.Result, error) {
	defer b.Op("Refresh All")()
	b.BeginGroup()
	defer b.EndGroup()
	var res calc.Result
	for si, sh := range b.WB.Sheets {
		for i := range sh.Pivots {
			r, err := b.RefreshPivot(si, i, overwrite)
			if err != nil {
				return res, fmt.Errorf("%s: %w", sh.Pivots[i].Name, err)
			}
			res = joinResults(res, r)
		}
	}
	return res, nil
}

// DeletePivot removes report idx and clears its cells.
func (b *Book) DeletePivot(sheet, idx int) calc.Result {
	defer b.Op("Delete PivotTable")()
	sh := b.WB.Sheets[sheet]
	pt := sh.Pivots[idx]
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.pivots = slices.Delete(model.ClonePivots(sh.Pivots), idx, idx+1)
	child := spillChildren(sh)
	var cells []cellChange
	a := pt.Out.Area
	for r := a.R1; r <= a.R2; r++ {
		for c := a.C1; c <= a.C2; c++ {
			if before := snapshot(sh, child, r, c); before != (model.Cell{}) {
				cells = append(cells, cellChange{sheet, r, c, before, model.Cell{}})
			}
		}
	}
	return b.applySheet(Where{sheet, model.Range{R1: a.R1, C1: a.C1, R2: a.R1, C2: a.C1}}, ch, cells)
}

// MovePivot moves report idx of sheet so its body starts at row, col of
// sheet to (Move PivotTable), as one undo step.
func (b *Book) MovePivot(sheet, idx, to, row, col int, overwrite bool) (calc.Result, error) {
	defer b.Op("Move PivotTable")()
	pt := b.WB.Sheets[sheet].Pivots[idx].Clone()
	pt.Row, pt.Col = row, col
	if to == sheet {
		return b.SetPivot(sheet, idx, pt, overwrite)
	}
	// Lay it out on the other sheet first: that is what can fail.
	b.BeginGroup()
	defer b.EndGroup()
	pt.Out = model.PivotOut{}
	res, err := b.SetPivot(to, -1, pt, overwrite)
	if err != nil {
		return res, err
	}
	return joinResults(res, b.DeletePivot(sheet, idx)), nil
}

// PivotOverlap returns the report r overlaps on sheet, if any.
func PivotOverlap(sh *model.Sheet, r model.Range) *model.PivotTable {
	for _, p := range sh.Pivots {
		if overlaps(p.Out.Area, r) {
			return p
		}
	}
	return nil
}

// pivotCut reports whether inserting (n > 0) or deleting rows (columns)
// at would cut through a report, which Excel refuses; deleting rows
// that hold a whole report deletes it.
func pivotCut(sh *model.Sheet, rows bool, at, n int) bool {
	for _, p := range sh.Pivots {
		lo, hi := p.Out.Area.C1, p.Out.Area.C2
		if rows {
			lo, hi = p.Out.Area.R1, p.Out.Area.R2
		}
		if n > 0 {
			if at > lo && at <= hi {
				return true
			}
			continue
		}
		end := at - n - 1
		if at <= hi && end >= lo && !(at <= lo && end >= hi) {
			return true
		}
	}
	return false
}

// StylePivots formats the reports of a workbook just read as Excel draws
// them (a file keeps the cells' values but the look comes from the
// PivotTable style) and works out where their buttons are. It changes
// no values and isn't an undo step. Reports whose source can't be read
// keep their cells as they are.
func (b *Book) StylePivots() {
	for si, sh := range b.WB.Sheets {
		for _, pt := range sh.Pivots {
			src, err := b.PivotSource(si, pt)
			if err != nil {
				continue
			}
			pivot.Sync(pt, src)
			res := pivot.Compute(pt, src, b.nf)
			if res.Out.Body.R1 != pt.Out.Body.R1 && pt.Out.Body != (model.Range{}) {
				continue // the file put it elsewhere: leave it
			}
			pt.Row, pt.Col = res.Row, res.Col
			pt.Out = res.Out
			styles := map[pivotLook]uint32{}
			empty := pivot.Empty(pt)
			for _, pc := range res.Cells {
				nc := b.pivotCell(pc, styles)
				if empty {
					sh.Set(pc.Row, pc.Col, nc) // the placeholder isn't saved
					continue
				}
				v := *sh.Get(pc.Row, pc.Col)
				v.Style = nc.Style
				if v.Kind == model.KindNumber && v.Formula == "" {
					st := b.style(v.Style)
					v.Text = b.nf.Format(v.Num, st.NumFmtID, st.NumFmtCode)
				}
				sh.Set(pc.Row, pc.Col, v)
			}
		}
	}
}

// PivotRecords returns the source rows (sheet rows) behind the value at
// row, col of a report: the records of its row and column items that
// the filters let through (Show Details).
func (b *Book) PivotRecords(sheet int, pt *model.PivotTable, row, col int) (*pivot.Source, []int, error) {
	src, err := b.PivotSource(sheet, pt)
	if err != nil {
		return nil, nil, err
	}
	o := &pt.Out
	ri, ci := row-o.Body.R1-o.FirstDataRow, col-o.Body.C1-o.FirstDataCol
	if ri < 0 || ri >= len(o.RowLines) || ci < 0 || (len(o.ColLines) > 0 && ci >= len(o.ColLines)) {
		return src, nil, ErrPivotChange
	}
	var cl model.PivotLine
	if len(o.ColLines) > 0 {
		cl = o.ColLines[ci]
	}
	return src, pivot.Records(pt, src, o.RowLines[ri], cl), nil
}

// ShowDetails puts the source rows behind the value at row, col of a
// report on a new sheet before the report's sheet, as Excel does for a
// double-click on a value, and returns the new sheet's index.
func (b *Book) ShowDetails(sheet, row, col int) (int, error) {
	defer b.Op("Show Details")()
	pt, _ := b.WB.Sheets[sheet].PivotAt(row, col)
	if pt == nil {
		return 0, ErrPivotChange
	}
	src, recs, err := b.PivotRecords(sheet, pt, row, col)
	if err != nil {
		return 0, err
	}
	b.BeginGroup()
	defer b.EndGroup()
	if _, err := b.InsertSheet(sheet); err != nil {
		return 0, err
	}
	srcSheet := src.Sheet
	if srcSheet >= sheet {
		srcSheet++ // moved by the new sheet
	}
	from := b.WB.Sheets[srcSheet]
	var cells []cellChange
	put := func(r, c int, v model.Cell) {
		if v.Formula != "" {
			v.Formula, v.Dynamic = "", false // values, as Excel copies them
		}
		if v != (model.Cell{}) {
			cells = append(cells, cellChange{sheet, r, c, model.Cell{}, v})
		}
	}
	bold := b.style(0)
	bold.Font.Bold = true
	head := b.addStyle(bold)
	for i, f := range src.Fields {
		put(0, i, model.Cell{Kind: model.KindText, Text: f.Name, Style: head})
	}
	for k, rec := range recs {
		for i := range src.Fields {
			put(k+1, i, *from.Get(src.Range.R1+1+rec, src.Range.C1+i))
		}
	}
	b.apply(Where{sheet, model.Range{R2: len(recs), C2: len(src.Fields) - 1}}, cells)
	return sheet, nil
}

// pivotEnter handles typing into a report cell: over a value field's
// name it renames the field, as in Excel; anything else is refused.
func (b *Book) pivotEnter(sheet, idx, row, col int, text string) (calc.Result, error) {
	pt := b.WB.Sheets[sheet].Pivots[idx]
	for _, h := range pt.Out.Headers {
		if h.Row != row || h.Col != col {
			continue
		}
		name := strings.TrimSpace(text)
		if name == "" || name == pt.Values[h.Value].Name {
			return calc.Result{}, nil
		}
		for i, v := range pt.Values {
			if i != h.Value && strings.EqualFold(v.Name, name) {
				return calc.Result{}, errors.New("PivotTable field name already exists.")
			}
		}
		p := pt.Clone()
		p.Values[h.Value].Name = name
		return b.SetPivot(sheet, idx, p, true)
	}
	return calc.Result{}, ErrPivotChange
}

// WidenPivotCols widens columns idx to widths (characters) as part of
// the last change, a PivotTable update: Excel fits a report's columns
// when it updates. Columns already wide enough keep their width.
func (b *Book) WidenPivotCols(sheet int, idx []int, widths []float32) {
	defer b.Op("Column Width")()
	n := len(b.undo)
	if n == 0 || b.undo[n-1].sheet == nil || b.undo[n-1].sheet.sheet != sheet || b.undo[n-1].sheet.cols != nil {
		return
	}
	sh := b.WB.Sheets[sheet]
	d := &dimChange{sheet: sheet, cols: true}
	for i, c := range idx {
		cur, ok := sh.ColWidth[c]
		if ok && cur == 0 {
			continue // hidden
		}
		if !ok {
			cur = sh.DefaultColWidth
			if cur == 0 {
				cur = sh.BaseColWidth + 0.71 // about the default, padding included
			}
		}
		if widths[i] <= cur {
			continue
		}
		d.idx = append(d.idx, c)
		d.after = append(d.after, widths[i])
		d.before = append(d.before, dimValue{sh.ColWidth[c], ok})
	}
	if len(d.idx) == 0 {
		return
	}
	d.set(b.WB, true)
	b.undo[n-1].sheet.cols = d
}
