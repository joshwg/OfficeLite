// Computing a report: filtering, aggregating, axis trees and the cell layout.

package pivot

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Look is how a report cell is formatted, after PivotStyleLight16.
type Look uint8

const (
	LookFill   Look = 1 << iota // the light blue band of header and total rows
	LookBold                    // headers, outer items, subtotals and totals
	LookTop                     // a line above (the Grand Total row)
	LookBottom                  // a line below (the last header row)
	LookSrcFmt                  // a number label keeps its source cell's number format
)

// Cell is one cell of a computed report.
type Cell struct {
	Row, Col int
	Kind     model.Kind
	Num      float64
	Text     string
	Look     Look
	Indent   int
	// NumFmtID and NumFmtCode format a value; SrcStyle is the source
	// cell whose number format a number label keeps (LookSrcFmt).
	NumFmtID   int
	NumFmtCode string
	SrcStyle   uint32
}

// Result is a computed report.
type Result struct {
	Cells []Cell
	Out   model.PivotOut
	// Row, Col is the body's top-left, moved down when the report
	// filters need the rows above it.
	Row, Col int
}

// Order returns the indexes of field f's items in the report's order
// (pf's sort).
func Order(pf *model.PivotField, f *Field) []int {
	idx := make([]int, len(f.Items))
	for i := range idx {
		idx[i] = i
	}
	switch pf.Sort {
	case model.SortAscending:
		slices.SortStableFunc(idx, func(a, b int) int { return compareItems(&f.Items[a], &f.Items[b]) })
	case model.SortDescending:
		slices.SortStableFunc(idx, func(a, b int) int {
			ia, ib := &f.Items[a], &f.Items[b]
			if (ia.Key == "") != (ib.Key == "") { // blanks stay last
				return compareItems(ia, ib)
			}
			return -compareItems(ia, ib)
		})
	case model.SortManual:
		var out []int
		seen := make([]bool, len(f.Items))
		for _, k := range pf.Order {
			if i := f.Item(k); i >= 0 && !seen[i] {
				out = append(out, i)
				seen[i] = true
			}
		}
		for i := range f.Items {
			if !seen[i] {
				out = append(out, i)
			}
		}
		idx = out
	}
	return idx
}

// DefaultFunc is the function a new value field gets: Sum for a field
// of numbers, Count otherwise (text or blanks in it), as in Excel.
func DefaultFunc(f *Field) model.PivotFunc {
	if f.Numeric {
		return model.FuncSum
	}
	return model.FuncCount
}

// ValueName is a value field's default name, "Sum of Revenue".
func ValueName(fn model.PivotFunc, field string) string {
	return model.FuncNames[fn] + " of " + field
}

// acc accumulates the values of one value field in one cell.
type acc struct {
	n, nums          int // non-blank values, numbers
	sum, sumSq, prod float64
	min, max         float64
	errs             string // the first error value
	has              bool
}

// add counts item it into the accumulator; a blank marks the cell as
// having records but adds no value.
func (a *acc) add(it *Item) {
	a.has = true
	if it.Key == "" {
		return
	}
	a.n++
	switch it.Kind {
	case model.KindNumber:
		v := it.Num
		if a.nums == 0 {
			a.min, a.max, a.prod = v, v, 1
		}
		a.nums++
		a.sum += v
		a.sumSq += v * v
		a.prod *= v
		a.min, a.max = min(a.min, v), max(a.max, v)
	case model.KindError:
		if a.errs == "" {
			a.errs = it.Label
		}
	}
}

// value is the summary: a number, or an error text (#DIV/0!).
func (a *acc) value(fn model.PivotFunc) (float64, string) {
	if a.errs != "" && fn != model.FuncCount && fn != model.FuncCountNums {
		return 0, a.errs
	}
	n := float64(a.nums)
	switch fn {
	case model.FuncSum:
		return a.sum, ""
	case model.FuncCount:
		return float64(a.n), ""
	case model.FuncCountNums:
		return n, ""
	case model.FuncAverage:
		if a.nums == 0 {
			return 0, "#DIV/0!"
		}
		return a.sum / n, ""
	case model.FuncMax:
		return a.max, ""
	case model.FuncMin:
		return a.min, ""
	case model.FuncProduct:
		if a.nums == 0 {
			return 0, ""
		}
		return a.prod, ""
	case model.FuncStdDev, model.FuncVar, model.FuncStdDevp, model.FuncVarp:
		sample := fn == model.FuncStdDev || fn == model.FuncVar
		d := n
		if sample {
			d--
		}
		if d <= 0 {
			return 0, "#DIV/0!"
		}
		v := max((a.sumSq-a.sum*a.sum/n)/d, 0)
		if fn == model.FuncStdDev || fn == model.FuncStdDevp {
			v = math.Sqrt(v)
		}
		return v, ""
	}
	return 0, ""
}

// node is an item of an axis tree.
type node struct {
	field int // source field, or model.DataField
	item  int // the field's item, or the value index under Σ Values
	path  []int
	data  int // the value field set by Σ Values above or at it, else -1
	recs  []int32
	kids  []*node
}

// line is a row or column of the report with what it shows.
type line struct {
	kind   model.PivotLineKind
	path   []int // items of the axis's source fields, outer first
	data   int
	level  int // the depth of its item (compact indent, header row)
	labels []label
	bold   bool
}

// label is an axis label cell: at level (the field's position on the
// axis), its text, and the item it names (field -1 for none).
type label struct {
	level       int
	text        string
	field, item int
}

// computer holds a report being computed.
type computer struct {
	pt      *model.PivotTable
	src     *Source
	nf      *numfmt.Formatter
	order   map[int][]int // each axis field's items in report order
	hidden  [][]bool      // by field and item: filtered out
	keep    []bool        // records passing the filters
	accs    map[string][]acc
	rowAxis []int
	colAxis []int
}

// Compute lays out report pt over source src. Number cells get their
// text from nf.
func Compute(pt *model.PivotTable, src *Source, nf *numfmt.Formatter) *Result {
	if len(pt.Fields) < len(src.Fields) {
		// The source gained columns since the report was synced: compute on
		// a copy with a blank field for each new column.
		cp := *pt
		cp.Fields = slices.Clone(pt.Fields)
		for _, f := range src.Fields[len(pt.Fields):] {
			cp.Fields = append(cp.Fields, model.PivotField{Name: f.Name})
		}
		pt = &cp
	}
	c := &computer{pt: pt, src: src, nf: nf, order: map[int][]int{}}
	c.rowAxis, c.colAxis = pt.Rows, pt.Cols
	c.filter()
	c.aggregate()
	return c.layout()
}

// sourceFields are the axis's fields other than Σ Values.
func sourceFields(axis []int) []int {
	var out []int
	for _, f := range axis {
		if f != model.DataField {
			out = append(out, f)
		}
	}
	return out
}

// filter works out which records the hidden items of the fields in the
// areas leave out.
func (c *computer) filter() {
	c.hidden = make([][]bool, len(c.src.Fields))
	active := slices.Concat(sourceFields(c.pt.Rows), sourceFields(c.pt.Cols), c.pt.Pages)
	for _, f := range active {
		if f < 0 || f >= len(c.src.Fields) || f >= len(c.pt.Fields) || c.hidden[f] != nil {
			continue
		}
		sf := c.src.Fields[f]
		h := make([]bool, len(sf.Items))
		for i, it := range sf.Items {
			h[i] = c.pt.Fields[f].IsHidden(it.Key)
		}
		c.hidden[f] = h
	}
	c.keep = make([]bool, c.src.N)
	for i := range c.keep {
		c.keep[i] = true
		for f, h := range c.hidden {
			if h != nil && h[c.src.Fields[f].Rec[i]] {
				c.keep[i] = false
				break
			}
		}
	}
}

// pathKey encodes the items of a path.
func pathKey(p []int) string {
	var b strings.Builder
	for _, i := range p {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('|')
	}
	return b.String()
}

// aggregate summarizes the records for every pair of row and column
// item paths (prefixes included: subtotals and grand totals).
func (c *computer) aggregate() {
	c.accs = map[string][]acc{}
	nv := len(c.pt.Values)
	if nv == 0 {
		return
	}
	rf, cf := c.valid(sourceFields(c.rowAxis)), c.valid(sourceFields(c.colAxis))
	rp, cp := make([]int, len(rf)), make([]int, len(cf))
	for i := range c.src.N {
		if !c.keep[i] {
			continue
		}
		for k, f := range rf {
			rp[k] = int(c.src.Fields[f].Rec[i])
		}
		for k, f := range cf {
			cp[k] = int(c.src.Fields[f].Rec[i])
		}
		for a := 0; a <= len(rp); a++ {
			rk := pathKey(rp[:a]) + "/"
			for b := 0; b <= len(cp); b++ {
				k := rk + pathKey(cp[:b])
				s := c.accs[k]
				if s == nil {
					s = make([]acc, nv)
					c.accs[k] = s
				}
				for v, pv := range c.pt.Values {
					if pv.Field >= 0 && pv.Field < len(c.src.Fields) {
						sf := c.src.Fields[pv.Field]
						s[v].add(&sf.Items[sf.Rec[i]])
					}
				}
			}
		}
	}
}

// valid drops fields the source doesn't have.
func (c *computer) valid(fs []int) []int {
	return slices.DeleteFunc(slices.Clone(fs), func(f int) bool { return f < 0 || f >= len(c.src.Fields) })
}

// itemOrder is field f's items in report order.
func (c *computer) itemOrder(f int) []int {
	if o, ok := c.order[f]; ok {
		return o
	}
	o := Order(&c.pt.Fields[f], c.src.Fields[f])
	c.order[f] = o
	return o
}

// tree groups the records along an axis.
func (c *computer) tree(axis []int) *node {
	var recs []int32
	for i := range c.src.N {
		if c.keep[i] {
			recs = append(recs, int32(i))
		}
	}
	root := &node{field: -1, item: -1, data: -1, recs: recs}
	axis = slices.DeleteFunc(slices.Clone(axis), func(f int) bool {
		return f != model.DataField && (f < 0 || f >= len(c.src.Fields))
	})
	c.grow(root, axis)
	return root
}

// grow adds n's children for the first field of axis, one per item with
// records (one per value field under Σ Values), and recurses on the rest.
func (c *computer) grow(n *node, axis []int) {
	if len(axis) == 0 {
		return
	}
	f := axis[0]
	if f == model.DataField {
		for v := range c.pt.Values {
			k := &node{field: f, item: v, path: n.path, data: v, recs: n.recs}
			c.grow(k, axis[1:])
			n.kids = append(n.kids, k)
		}
		return
	}
	sf := c.src.Fields[f]
	groups := make([][]int32, len(sf.Items))
	for _, r := range n.recs {
		it := sf.Rec[r]
		groups[it] = append(groups[it], r)
	}
	for _, it := range c.itemOrder(f) {
		if len(groups[it]) == 0 {
			continue
		}
		k := &node{field: f, item: it, path: append(slices.Clone(n.path), it), data: n.data, recs: groups[it]}
		c.grow(k, axis[1:])
		n.kids = append(n.kids, k)
	}
}

// itemLabel is the label of node n's item.
func (c *computer) itemLabel(n *node) string {
	if n.field == model.DataField {
		return c.pt.Values[n.item].Name
	}
	return c.src.Fields[n.field].Items[n.item].Label
}

// dataBelow reports whether Σ Values comes after level on the axis.
func dataBelow(axis []int, level int) bool {
	return slices.Contains(axis[level+1:], model.DataField)
}

// lines lays out an axis: rows (with the report layout) or columns
// (always like the tabular form, subtotals after their group).
func (c *computer) lines(axis []int, rows bool) []line {
	axis = slices.DeleteFunc(slices.Clone(axis), func(f int) bool {
		return f != model.DataField && (f < 0 || f >= len(c.src.Fields))
	})
	hasValues := len(c.pt.Values) > 0
	if len(axis) == 0 {
		if !rows && !hasValues {
			return nil // no data column
		}
		return []line{{kind: model.LineItem, data: -1}}
	}
	root := c.tree(axis)
	lay := model.LayoutTabular
	if rows {
		lay = c.pt.Layout
	}
	var out []line
	var walk func(n *node, level int, pending []label, outer []label)
	walk = func(n *node, level int, pending []label, outer []label) {
		last := level == len(axis)-1
		for i, k := range n.kids {
			lb := label{level: level, text: c.itemLabel(k), field: k.field, item: k.item}
			if k.field == model.DataField {
				lb.field = model.DataField
			}
			var pend []label
			if i == 0 {
				pend = pending
			}
			if rows && c.pt.RepeatLabels && lay != model.LayoutCompact {
				pend = outer
			}
			if last {
				out = append(out, line{kind: model.LineItem, path: k.path, data: k.data, level: level, labels: append(slices.Clone(pend), lb)})
				continue
			}
			// A field has subtotals when a source field is nested in it
			// (with only Σ Values inside, its group shows each total).
			sub := hasValues && k.field != model.DataField && !c.pt.Fields[k.field].NoSubtotal &&
				len(sourceFields(axis[level+1:])) > 0
			below := dataBelow(axis, level)
			top := rows && sub && !c.pt.SubtotalsBottom && lay != model.LayoutTabular && !below
			if lay == model.LayoutTabular {
				walk(k, level+1, append(slices.Clone(pend), lb), append(slices.Clone(outer), lb))
			} else {
				kind := model.LineLabel
				if top {
					kind = model.LineItem
				}
				out = append(out, line{kind: kind, path: k.path, data: k.data, level: level, labels: append(slices.Clone(pend), lb), bold: true})
				walk(k, level+1, nil, append(slices.Clone(outer), lb))
			}
			if sub && !top {
				if below {
					for v, pv := range c.pt.Values {
						out = append(out, line{kind: model.LineSubtotal, path: k.path, data: v, level: level,
							labels: []label{{level: level, text: lb.text + " " + pv.Name, field: -1}}, bold: true})
					}
				} else {
					out = append(out, line{kind: model.LineSubtotal, path: k.path, data: k.data, level: level,
						labels: []label{{level: level, text: lb.text + " Total", field: -1}}, bold: true})
				}
			}
			if rows && c.pt.BlankRows && level == 0 {
				out = append(out, line{kind: model.LineBlank, data: -1})
			}
		}
	}
	walk(root, 0, nil, nil)
	if rows && c.pt.BlankRows && len(out) > 0 && out[len(out)-1].kind == model.LineBlank {
		out = out[:len(out)-1]
	}
	grand := !c.pt.NoColGrand
	if !rows {
		grand = !c.pt.NoRowGrand
	}
	if grand && hasValues && len(sourceFields(axis)) > 0 {
		if slices.Contains(axis, model.DataField) {
			for v, pv := range c.pt.Values {
				out = append(out, line{kind: model.LineGrand, data: v, labels: []label{{text: "Total " + pv.Name, field: -1}}, bold: true})
			}
		} else {
			out = append(out, line{kind: model.LineGrand, data: -1, labels: []label{{text: "Grand Total", field: -1}}, bold: true})
		}
	}
	return out
}

// value computes the cell where row line r and column line cl cross.
func (c *computer) value(r, cl *line) (cell Cell, ok bool) {
	if r.kind == model.LineLabel || r.kind == model.LineBlank || len(c.pt.Values) == 0 {
		return Cell{}, false
	}
	d := cl.data
	if d < 0 {
		d = r.data
	}
	if d < 0 {
		if len(c.pt.Values) != 1 {
			return Cell{}, false
		}
		d = 0
	}
	pv := c.pt.Values[d]
	a := c.accs[pathKey(r.path)+"/"+pathKey(cl.path)]
	if a == nil || !a[d].has {
		return Cell{}, false
	}
	v, errText := a[d].value(pv.Func)
	if errText == "" && pv.ShowAs != model.ShowNormal {
		var tk string
		switch pv.ShowAs {
		case model.ShowPercentOfTotal:
			tk = "/"
		case model.ShowPercentOfCol:
			tk = "/" + pathKey(cl.path)
		case model.ShowPercentOfRow:
			tk = pathKey(r.path) + "/"
		}
		total := 0.0
		if t := c.accs[tk]; t != nil {
			total, errText = t[d].value(pv.Func)
		}
		if errText == "" {
			if total == 0 {
				errText = "#DIV/0!"
			} else {
				v /= total
			}
		}
	}
	if errText != "" {
		return Cell{Kind: model.KindError, Text: errText}, true
	}
	id, code := pv.NumFmtID, pv.NumFmtCode
	if pv.ShowAs != model.ShowNormal && numfmt.IsGeneral(id, code) {
		id, code = 10, "" // 0.00%
	}
	cell = Cell{Kind: model.KindNumber, Num: v, NumFmtID: id, NumFmtCode: code}
	cell.Text = c.format(v, id, code)
	return cell, true
}

// format renders v with the report's formatter, or as General when it has none.
func (c *computer) format(v float64, id int, code string) string {
	if c.nf == nil {
		return numfmt.General(v)
	}
	return c.nf.Format(v, id, code)
}

// keys turns a line's item path into item keys.
func (c *computer) keys(axis []int, l *line) model.PivotLine {
	fs := c.valid(sourceFields(axis))
	pl := model.PivotLine{Kind: l.kind, Data: l.data}
	for i, it := range l.path {
		pl.Items = append(pl.Items, c.src.Fields[fs[i]].Items[it].Key)
	}
	return pl
}

// layout places the report's cells.
func (c *computer) layout() *Result {
	pt := c.pt
	res := &Result{Row: pt.Row, Col: pt.Col}
	nPages := len(pt.Pages)
	if nPages > 0 && res.Row < nPages+1 {
		res.Row = nPages + 1
	}
	if len(pt.Rows) == 0 && len(pt.Cols) == 0 && len(pt.Values) == 0 {
		c.placeholder(res)
		c.pages(res)
		return res
	}
	rowAxis := slices.DeleteFunc(slices.Clone(pt.Rows), func(f int) bool { return f != model.DataField && (f < 0 || f >= len(c.src.Fields)) })
	colAxis := slices.DeleteFunc(slices.Clone(pt.Cols), func(f int) bool { return f != model.DataField && (f < 0 || f >= len(c.src.Fields)) })
	rls, cls := c.lines(rowAxis, true), c.lines(colAxis, false)
	if len(rowAxis) == 0 && len(rls) == 1 && len(c.pt.Values) == 0 {
		rls = nil
	}

	labelCols := 0
	if len(rowAxis) > 0 || len(sourceFields(colAxis)) > 0 {
		labelCols = 1
		if pt.Layout != model.LayoutCompact {
			labelCols = max(1, len(rowAxis))
		}
	}
	onlyData := len(colAxis) == 1 && colAxis[0] == model.DataField
	caption := len(colAxis) > 0 && !onlyData
	headerRows := 1
	if caption {
		headerRows = 1 + len(colAxis)
	}
	o := &res.Out
	o.FirstDataRow, o.FirstDataCol = headerRows, labelCols
	switch {
	case len(colAxis) == 0:
		o.FirstHeaderRow = 1
	case onlyData:
		o.FirstHeaderRow = 0
	default:
		o.FirstHeaderRow = 1
	}
	width := max(labelCols+len(cls), 1)
	height := headerRows + len(rls)
	o.Body = model.Range{R1: res.Row, C1: res.Col, R2: res.Row + height - 1, C2: res.Col + width - 1}

	grid := map[[2]int]*Cell{}
	put := func(r, col int, cell Cell) *Cell {
		cell.Row, cell.Col = res.Row+r, res.Col+col
		p := &cell
		grid[[2]int{r, col}] = p
		return p
	}
	text := func(r, col int, s string) *Cell {
		return put(r, col, Cell{Kind: model.KindText, Text: s})
	}
	button := func(r, col int, axis model.PivotAxis, f int) {
		b := model.PivotButton{Row: res.Row + r, Col: res.Col + col, Axis: axis, Field: f}
		fs := []int{f}
		if f < 0 {
			fs = sourceFields(rowAxis)
			if axis == model.AxisCols {
				fs = sourceFields(colAxis)
			}
		}
		for _, x := range fs {
			if x >= 0 && x < len(pt.Fields) && len(pt.Fields[x].Hidden) > 0 {
				b.Filtered = true
			}
		}
		o.Buttons = append(o.Buttons, b)
	}
	header := func(r, col, v int) {
		o.Headers = append(o.Headers, model.PivotHeader{Row: res.Row + r, Col: res.Col + col, Value: v})
	}
	fieldLabel := func(f int) string {
		if f == model.DataField {
			return "Values"
		}
		return pt.Fields[f].Label()
	}

	// The header rows.
	last := headerRows - 1
	if caption {
		if len(pt.Values) == 1 {
			text(0, 0, pt.Values[0].Name)
			header(0, 0, 0)
		}
		if pt.Layout == model.LayoutCompact {
			text(0, labelCols, "Column Labels")
			if len(sourceFields(colAxis)) > 0 {
				button(0, labelCols, model.AxisCols, -1)
			}
		} else {
			for k, f := range colAxis {
				text(0, labelCols+k, fieldLabel(f))
				if f != model.DataField {
					button(0, labelCols+k, model.AxisCols, f)
				}
			}
		}
	}
	if len(colAxis) == 0 && len(cls) == 1 && len(pt.Values) == 1 {
		text(0, labelCols, pt.Values[0].Name)
		header(0, labelCols, 0)
	}
	first := 0
	if caption {
		first = 1
	}
	for j := range cls {
		for _, lb := range cls[j].labels {
			cell := text(first+lb.level, labelCols+j, lb.text)
			if lb.field == model.DataField {
				header(first+lb.level, labelCols+j, lb.item)
			} else if lb.field >= 0 {
				c.itemCell(cell, lb)
			}
		}
	}
	if len(rowAxis) > 0 {
		if pt.Layout == model.LayoutCompact {
			text(last, 0, "Row Labels")
			if len(sourceFields(rowAxis)) > 0 {
				button(last, 0, model.AxisRows, -1)
			}
		} else {
			for k, f := range rowAxis {
				text(last, k, fieldLabel(f))
				if f != model.DataField {
					button(last, k, model.AxisRows, f)
				}
			}
		}
	}
	for r := 0; r < headerRows; r++ {
		for col := 0; col < width; col++ {
			cell := grid[[2]int{r, col}]
			if cell == nil {
				cell = put(r, col, Cell{})
			}
			cell.Look |= LookFill | LookBold
			if r == last {
				cell.Look |= LookBottom
			}
		}
	}

	// The data rows.
	for i := range rls {
		rl := &rls[i]
		r := headerRows + i
		for _, lb := range rl.labels {
			col := lb.level
			indent := 0
			if pt.Layout == model.LayoutCompact {
				col, indent = 0, lb.level
			}
			if col >= labelCols {
				continue
			}
			cell := text(r, col, lb.text)
			cell.Indent = indent
			if lb.field == model.DataField {
				header(r, col, lb.item)
			} else if lb.field >= 0 {
				c.itemCell(cell, lb)
			}
		}
		for j := range cls {
			if cell, ok := c.value(rl, &cls[j]); ok {
				put(r, labelCols+j, cell)
			}
		}
		for col := 0; col < width; col++ {
			cell := grid[[2]int{r, col}]
			if cell == nil {
				cell = put(r, col, Cell{})
			}
			cl := (*line)(nil)
			if col >= labelCols {
				cl = &cls[col-labelCols]
			}
			switch {
			case rl.kind == model.LineGrand:
				cell.Look |= LookFill | LookBold | LookTop
			case rl.bold:
				cell.Look |= LookBold
			case cl != nil && (cl.kind == model.LineGrand || cl.kind == model.LineSubtotal):
				cell.Look |= LookBold
			}
		}
	}

	for i := range rls {
		o.RowLines = append(o.RowLines, c.keys(rowAxis, &rls[i]))
	}
	for j := range cls {
		o.ColLines = append(o.ColLines, c.keys(colAxis, &cls[j]))
	}
	c.pages(res)
	keys := make([][2]int, 0, len(grid))
	for k := range grid {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	for _, k := range keys {
		res.Cells = append(res.Cells, *grid[k])
	}
	return res
}

// itemCell makes an item label cell a number when the item is one,
// formatted as its source cell.
func (c *computer) itemCell(cell *Cell, lb label) {
	it := &c.src.Fields[lb.field].Items[lb.item]
	if it.Kind == model.KindNumber {
		cell.Kind, cell.Num, cell.SrcStyle = model.KindNumber, it.Num, it.Style
		cell.Look |= LookSrcFmt
	}
}

// pages places the report filters above the body and sets the report's
// area.
func (c *computer) pages(res *Result) {
	pt, o := c.pt, &res.Out
	o.Area = o.Body
	n := len(pt.Pages)
	if n == 0 {
		return
	}
	top := res.Row - n - 1
	for i, f := range pt.Pages {
		if f < 0 || f >= len(c.src.Fields) {
			continue
		}
		r := top + i
		res.Cells = append(res.Cells, Cell{Row: r, Col: res.Col, Kind: model.KindText, Text: pt.Fields[f].Label()})
		val := Cell{Row: r, Col: res.Col + 1, Kind: model.KindText, Text: "(All)"}
		pf := &pt.Fields[f]
		shown := -1
		count := 0
		for i, it := range c.src.Fields[f].Items {
			if !pf.IsHidden(it.Key) {
				shown = i
				count++
			}
		}
		switch {
		case count == 1:
			it := &c.src.Fields[f].Items[shown]
			val.Text = it.Label
			if it.Kind == model.KindNumber {
				val.Kind, val.Num, val.SrcStyle = model.KindNumber, it.Num, it.Style
				val.Look |= LookSrcFmt
			}
		case count < len(c.src.Fields[f].Items):
			val.Text = "(Multiple Items)"
		}
		res.Cells = append(res.Cells, val)
		o.Buttons = append(o.Buttons, model.PivotButton{Row: r, Col: res.Col + 1, Axis: model.AxisPages, Field: f, Filtered: len(pf.Hidden) > 0})
	}
	o.Area = model.Range{R1: top, C1: o.Body.C1, R2: o.Body.R2, C2: max(o.Body.C2, o.Body.C1+1)}
}

// placeholder is an empty report: Excel's box asking for fields, three
// columns by eighteen rows.
func (c *computer) placeholder(res *Result) {
	o := &res.Out
	o.Body = model.Range{R1: res.Row, C1: res.Col, R2: res.Row + 17, C2: res.Col + 2}
	o.FirstHeaderRow, o.FirstDataRow = 1, 1
	name := c.pt.Name
	if name == "" {
		name = "PivotTable"
	}
	lines := []string{name, "", "To build a report, choose", "fields from the PivotTable", "Field List"}
	for i, s := range lines {
		if s != "" {
			cell := Cell{Row: res.Row + 1 + i, Col: res.Col, Kind: model.KindText, Text: s}
			if i == 0 {
				cell.Look = LookBold
			}
			res.Cells = append(res.Cells, cell)
		}
	}
}

// Empty reports whether the report has no fields: Excel shows its
// placeholder box.
func Empty(pt *model.PivotTable) bool {
	return len(pt.Rows) == 0 && len(pt.Cols) == 0 && len(pt.Values) == 0
}
