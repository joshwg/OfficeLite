package lookup

import (
	"slices"
	"strconv"

	f "officelite/excel/formula"
)

// GROUPBY and PIVOTBY: group the rows of a table by the values of one or
// more key columns and aggregate value columns per group with a LAMBDA or
// a function named as a value (SUM, AVERAGE, PERCENTOF ...).

// aggFn is one aggregation: the function, its column label, and whether
// it takes a second argument, the set to compare with (PERCENTOF).
type aggFn struct {
	fn    *f.Lambda
	label f.Value
	two   bool
}

// aggFns reads the function argument: one lambda, or a row (or column) of
// them, optionally with a second row (column) of labels.
func aggFns(v f.Value) ([]aggFn, f.Value) {
	arr, ok := v.(*f.Array)
	if !ok {
		arr = &f.Array{Rows: 1, Cols: 1, V: []f.Value{v}}
	}
	_, labelled := arr.At(arr.Rows-1, 0).(*f.Lambda)
	labelled = !labelled && arr.Rows == 2
	if arr.Cols == 1 && !labelled || arr.Cols == 2 && arr.Rows > 2 {
		arr = transposed(arr) // a column of functions, laid out as a row
	}
	if arr.Rows > 2 {
		return nil, f.ErrValue
	}
	out := make([]aggFn, arr.Cols)
	for i := range out {
		switch x := arr.At(0, i).(type) {
		case *f.Lambda:
			out[i] = aggFn{fn: x, label: f.String(x.Name())}
			if x.Func != nil {
				out[i].two = x.Func.Min == 2
			} else {
				out[i].two = len(x.Params) == 2
			}
		case f.Error:
			return nil, x
		default:
			return nil, f.ErrValue
		}
		if arr.Rows == 2 {
			out[i].label = arr.At(1, i)
		}
	}
	return out, nil
}

// pivot holds the inputs shared by GROUPBY and PIVOTBY.
type pivot struct {
	e      *f.Env
	rowF   *f.Array // row fields, data rows only
	colF   *f.Array // column fields (PIVOTBY)
	vals   *f.Array // values, data rows only
	fns    []aggFn
	all    []int // the data rows kept by filter_array
	show   bool  // field headers in the output
	rowHdr []f.Value
	colHdr []f.Value
	valHdr []f.Value
}

// headerMode is field_headers: 0 none, 1 present but hidden, 2 generated,
// 3 present and shown; when omitted, the values are taken to have a
// header if the first is text and the second a number.
func headerMode(a []f.Value, i int, vals *f.Array) (has, show bool, err f.Value) {
	mode := 0
	if omitted(a, i) {
		if vals.Rows > 1 {
			_, txt := vals.At(0, 0).(f.String)
			_, num := vals.At(1, 0).(f.Number)
			if txt && num {
				mode = 3
			}
		}
	} else if mode, err = f.Int(one(a[i])); err != nil {
		return
	}
	if mode < 0 || mode > 3 {
		return false, false, f.ErrValue
	}
	return mode == 1 || mode == 3, mode >= 2, nil
}

// newPivot reads the field, value, function, header and filter
// arguments; cols is nil for GROUPBY.
func newPivot(e *f.Env, rowsV, colsV, valsV, fnV f.Value, a []f.Value, hdrArg, filterArg int) (*pivot, f.Value) {
	p := &pivot{e: e}
	var err f.Value
	if p.rowF, err = array(e, rowsV); err != nil {
		return nil, err
	}
	if p.vals, err = array(e, valsV); err != nil {
		return nil, err
	}
	if colsV != nil {
		if p.colF, err = array(e, colsV); err != nil {
			return nil, err
		}
		if p.colF.Rows != p.rowF.Rows {
			return nil, f.ErrValue
		}
	}
	if p.vals.Rows != p.rowF.Rows {
		return nil, f.ErrValue
	}
	if p.fns, err = aggFns(fnV); err != nil {
		return nil, err
	}
	has, show, err := headerMode(a, hdrArg, p.vals)
	if err != nil {
		return nil, err
	}
	p.show = show
	n := p.rowF.Rows
	start := 0
	if has {
		start = 1
		p.rowHdr, p.valHdr = slices.Clone(p.rowF.Row(0)), slices.Clone(p.vals.Row(0))
		if p.colF != nil {
			p.colHdr = slices.Clone(p.colF.Row(0))
		}
	} else if show {
		p.rowHdr = generated("Row Field", p.rowF.Cols)
		p.valHdr = generated("Value", p.vals.Cols)
		if p.colF != nil {
			p.colHdr = generated("Col Field", p.colF.Cols)
		}
	}
	dataRows := func(a *f.Array) *f.Array {
		if a == nil || start == 0 {
			return a
		}
		return &f.Array{Rows: a.Rows - 1, Cols: a.Cols, V: a.V[a.Cols:]}
	}
	p.rowF, p.colF, p.vals = dataRows(p.rowF), dataRows(p.colF), dataRows(p.vals)
	var keep []f.Value
	if !omitted(a, filterArg) {
		fa, err := array(e, a[filterArg])
		if err != nil {
			return nil, err
		}
		if fa.Cols != 1 {
			return nil, f.ErrValue
		}
		switch fa.Rows {
		case n:
			keep = fa.V[start:]
		case n - start:
			keep = fa.V
		default:
			return nil, f.ErrValue
		}
	}
	for r := 0; r < n-start; r++ {
		if keep != nil {
			b, err := f.Truth(keep[r])
			if err != nil {
				return nil, err
			}
			if !b {
				continue
			}
		}
		p.all = append(p.all, r)
	}
	if len(p.all) == 0 {
		return nil, f.ErrCalc
	}
	return p, nil
}

func generated(prefix string, n int) []f.Value {
	out := make([]f.Value, n)
	for i := range out {
		out[i] = f.String(prefix + " " + strconv.Itoa(i+1))
	}
	return out
}

// pairs is the number of aggregated columns per group: every function
// applied to every value column, function by function.
func (p *pivot) pairs() int { return len(p.fns) * p.vals.Cols }

// agg aggregates value column/function pair j over rows (ascending data
// row numbers); total is the comparison set for two-argument functions.
// No rows give an empty cell.
func (p *pivot) agg(rows, total []int, j int) f.Value {
	if len(rows) == 0 {
		return empty
	}
	fn, col := p.fns[j/p.vals.Cols], j%p.vals.Cols
	column := func(rows []int) *f.Array {
		out := &f.Array{Rows: len(rows), Cols: 1, V: make([]f.Value, len(rows))}
		for i, r := range rows {
			out.V[i] = p.vals.At(r, col)
		}
		return out
	}
	args := []f.Value{column(rows)}
	if fn.two {
		args = append(args, column(total))
	}
	v := p.e.Deref(p.e.CallLambda(fn.fn, args...))
	switch x := v.(type) {
	case *f.Array:
		if x.Single() {
			return x.V[0]
		}
		return f.ErrCalc
	case *f.Lambda:
		return f.ErrCalc
	case f.Missing:
		return f.Number(0)
	}
	return v
}

// label is pair j's function label.
func (p *pivot) label(j int) f.Value { return p.fns[j/p.vals.Cols].label }

// valHeader is pair j's value column header.
func (p *pivot) valHeader(j int) f.Value { return p.valHdr[j%p.vals.Cols] }

// group is a node of the grouping tree: the rows sharing a key path.
type group struct {
	key  f.Value
	rows []int
	kids []*group
	at   map[string]*group
	sort map[int]f.Value // aggregates used for sorting, by pair
}

// grouping builds the tree of rows by the key columns of fields.
func grouping(fields *f.Array, rows []int) *group {
	root := &group{rows: rows}
	for _, r := range rows {
		g := root
		for c := 0; c < fields.Cols; c++ {
			v := fields.At(r, c)
			id := identity(v)
			kid := g.at[id]
			if kid == nil {
				if g.at == nil {
					g.at = map[string]*group{}
				}
				kid = &group{key: v}
				g.at[id] = kid
				g.kids = append(g.kids, kid)
			}
			kid.rows = append(kid.rows, r)
			g = kid
		}
	}
	return root
}

// pivotLine is one output row (column) of a pivot: a leaf group, a
// subtotal or the grand total.
type pivotLine struct {
	keys   []f.Value // one per key column, empty below a subtotal's level
	rows   []int
	parent []int // the rows of the enclosing group
}

// sortSpec is one sort_order entry: a zero-based output column (key
// columns first, then aggregated pairs) and 1 or -1.
type sortSpec struct{ col, order int }

// sortSpecs reads a sort_order argument against k key columns and n
// aggregated pairs. Several numbers may only name key columns.
func sortSpecs(a []f.Value, i, k, n int) ([]sortSpec, f.Value) {
	if omitted(a, i) {
		return nil, nil
	}
	if err, ok := a[i].(f.Error); ok {
		return nil, err
	}
	idx, err := numbers(a[i])
	if err != nil {
		return nil, err
	}
	var out []sortSpec
	for _, x := range idx {
		c, o := x, 1
		if x < 0 {
			c, o = -x, -1
		}
		if c < 1 || c > k+n || c > k && len(idx) > 1 {
			return nil, f.ErrValue
		}
		out = append(out, sortSpec{c - 1, o})
	}
	return out, nil
}

// totalsArg reads a total_depth argument for k key columns: the number of
// levels with subtotals, whether totals go first, and whether there is a
// grand total. Omitted, it is every level that can have subtotals.
func totalsArg(a []f.Value, i, k int, table bool) (sub int, top, grand bool, err f.Value) {
	t := k
	if table {
		t = 1
	}
	if !omitted(a, i) {
		if t, err = f.Int(one(a[i])); err != nil {
			return
		}
	}
	if t < 0 {
		t, top = -t, true
	}
	sub = max(t-1, 0)
	if sub > k-1 || table && sub > 0 {
		return 0, false, false, f.ErrValue
	}
	return sub, top, t > 0, nil
}

// lines lays out the groups of fields (one key column per level) over
// rows: sorted by specs, with subtotals for the first sub levels and a
// grand total ("Total"), first when top. table sorts and lists the key
// combinations as a flat table rather than a hierarchy. sortVal is pair
// j's aggregate over a group.
func lines(fields *f.Array, rows []int, specs []sortSpec, sub int, top, grand, table bool,
	sortVal func(rows []int, j int) f.Value) []pivotLine {
	k := fields.Cols
	root := grouping(fields, rows)
	value := func(g *group, j int) f.Value {
		if v, ok := g.sort[j]; ok {
			return v
		}
		if g.sort == nil {
			g.sort = map[int]f.Value{}
		}
		g.sort[j] = sortVal(g.rows, j)
		return g.sort[j]
	}
	var out []pivotLine
	if table {
		// Every key combination, sorted column by column.
		type leaf struct {
			keys []f.Value
			g    *group
		}
		var leaves []leaf
		var walk func(g *group, keys []f.Value)
		walk = func(g *group, keys []f.Value) {
			if len(keys) == k {
				leaves = append(leaves, leaf{slices.Clone(keys), g})
				return
			}
			for _, kid := range g.kids {
				walk(kid, append(keys, kid.key))
			}
		}
		walk(root, nil)
		slices.SortStableFunc(leaves, func(x, y leaf) int {
			for _, s := range specs {
				var c int
				if s.col < k {
					c = sortCmp(x.keys[s.col], y.keys[s.col], s.order)
				} else {
					c = sortCmp(value(x.g, s.col-k), value(y.g, s.col-k), s.order)
				}
				if c != 0 {
					return c
				}
			}
			for c := range k {
				if d := sortCmp(x.keys[c], y.keys[c], 1); d != 0 {
					return d
				}
			}
			return 0
		})
		for _, l := range leaves {
			out = append(out, pivotLine{keys: l.keys, rows: l.g.rows, parent: rows})
		}
	} else {
		var walk func(g *group, depth int, keys []f.Value)
		walk = func(g *group, depth int, keys []f.Value) {
			kids := slices.Clone(g.kids)
			slices.SortStableFunc(kids, func(x, y *group) int {
				for _, s := range specs {
					var c int
					switch {
					case s.col == depth:
						c = sortCmp(x.key, y.key, s.order)
					case s.col >= k:
						c = sortCmp(value(x, s.col-k), value(y, s.col-k), s.order)
					}
					if c != 0 {
						return c
					}
				}
				return sortCmp(x.key, y.key, 1)
			})
			for _, kid := range kids {
				path := append(slices.Clone(keys), kid.key)
				if depth+1 == k {
					out = append(out, pivotLine{keys: path, rows: kid.rows, parent: g.rows})
					continue
				}
				subtotal := depth+1 <= sub
				st := pivotLine{keys: pad(path, k), rows: kid.rows, parent: g.rows}
				if subtotal && top {
					out = append(out, st)
				}
				walk(kid, depth+1, path)
				if subtotal && !top {
					out = append(out, st)
				}
			}
		}
		walk(root, 0, nil)
	}
	if grand {
		keys := pad([]f.Value{f.String("Total")}, k)
		total := pivotLine{keys: keys, rows: rows, parent: rows}
		if top {
			out = append([]pivotLine{total}, out...)
		} else {
			out = append(out, total)
		}
	}
	return out
}

// empty fills the cells of a pivot that hold nothing (the keys beside a
// total, missing combinations): empty text, so that they show empty when
// spilled rather than as 0.
var empty f.Value = f.String("")

// pad extends keys with empty cells to k values.
func pad(keys []f.Value, k int) []f.Value {
	out := slices.Clone(keys)
	for len(out) < k {
		out = append(out, empty)
	}
	return out
}

// intersect is the rows in both ascending lists.
func intersect(a, b []int) []int {
	var out []int
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			out = append(out, a[i])
			i, j = i+1, j+1
		}
	}
	return out
}

// relationship reads field_relationship: 0 hierarchy, 1 table.
func relationship(a []f.Value, i int) (bool, f.Value) {
	r, err := intArg(a, i, 0)
	if err != nil {
		return false, err
	}
	if r != 0 && r != 1 {
		return false, f.ErrValue
	}
	return r == 1, nil
}

// GROUPBY(row_fields, values, function, [field_headers], [total_depth],
// [sort_order], [filter_array], [field_relationship]). The result has the
// key columns and one column per function and value column; a header row
// names the functions when there are several, another the fields when
// field_headers shows them. total_depth is 0 (none), 1 (grand total), 2
// (grand total and subtotals of the first level) ..., negative for totals
// above their groups; sort_order numbers output columns, negative for
// descending.
func groupBy(e *f.Env, a []f.Value) f.Value {
	p, err := newPivot(e, a[0], nil, a[1], a[2], a, 3, 6)
	if err != nil {
		return err
	}
	table, err := relationship(a, 7)
	if err != nil {
		return err
	}
	k, n := p.rowF.Cols, p.pairs()
	sub, top, grand, err := totalsArg(a, 4, k, table)
	if err != nil {
		return err
	}
	specs, err := sortSpecs(a, 5, k, n)
	if err != nil {
		return err
	}
	ls := lines(p.rowF, p.all, specs, sub, top, grand, table, func(rows []int, j int) f.Value {
		return p.agg(rows, p.all, j)
	})
	var out [][]f.Value
	if len(p.fns) > 1 {
		row := pad(nil, k)
		for j := range n {
			row = append(row, p.label(j))
		}
		out = append(out, row)
	}
	if p.show {
		row := slices.Clone(p.rowHdr)
		for j := range n {
			row = append(row, p.valHeader(j))
		}
		out = append(out, row)
	}
	for _, l := range ls {
		row := slices.Clone(l.keys)
		for j := range n {
			row = append(row, p.agg(l.rows, p.all, j))
		}
		out = append(out, row)
	}
	return grid(out)
}

// PIVOTBY(row_fields, col_fields, values, function, [field_headers],
// [row_total_depth], [row_sort_order], [col_total_depth],
// [col_sort_order], [filter_array], [relative_to]). Rows are grouped as
// in GROUPBY and columns likewise by col_fields: a header row per column
// field level holds the column keys ("Total" over the grand total
// column), followed by a row of function labels when there are several
// and a row of field headers when they are shown. relative_to picks the
// set a two-argument function (PERCENTOF) compares with: 0 the column
// total, 1 the row total, 2 the grand total, 3 the parent row group's
// total in the column, 4 the parent column group's total in the row.
func pivotBy(e *f.Env, a []f.Value) f.Value {
	p, err := newPivot(e, a[0], a[1], a[2], a[3], a, 4, 9)
	if err != nil {
		return err
	}
	k, c, n := p.rowF.Cols, p.colF.Cols, p.pairs()
	rsub, rtop, rgrand, err := totalsArg(a, 5, k, false)
	if err != nil {
		return err
	}
	rspecs, err := sortSpecs(a, 6, k, n)
	if err != nil {
		return err
	}
	csub, ctop, cgrand, err := totalsArg(a, 7, c, false)
	if err != nil {
		return err
	}
	cspecs, err := sortSpecs(a, 8, c, n)
	if err != nil {
		return err
	}
	rel, err := intArg(a, 10, 0)
	if err != nil {
		return err
	}
	if rel < 0 || rel > 4 {
		return f.ErrValue
	}
	sortVal := func(rows []int, j int) f.Value { return p.agg(rows, p.all, j) }
	rls := lines(p.rowF, p.all, rspecs, rsub, rtop, rgrand, false, sortVal)
	cls := lines(p.colF, p.all, cspecs, csub, ctop, cgrand, false, sortVal)

	var out [][]f.Value
	for lv := range c {
		row := pad(nil, k)
		if p.show {
			row[k-1] = p.colHdr[lv]
		}
		for _, cl := range cls {
			for range n {
				row = append(row, cl.keys[lv])
			}
		}
		out = append(out, row)
	}
	if len(p.fns) > 1 {
		row := pad(nil, k)
		for range cls {
			for j := range n {
				row = append(row, p.label(j))
			}
		}
		out = append(out, row)
	}
	if p.show {
		row := slices.Clone(p.rowHdr)
		for range cls {
			for j := range n {
				row = append(row, p.valHeader(j))
			}
		}
		out = append(out, row)
	}
	for _, rl := range rls {
		row := slices.Clone(rl.keys)
		for _, cl := range cls {
			rows := intersect(rl.rows, cl.rows)
			var total []int
			switch rel {
			case 0:
				total = cl.rows
			case 1:
				total = rl.rows
			case 2:
				total = p.all
			case 3:
				total = intersect(rl.parent, cl.rows)
			case 4:
				total = intersect(rl.rows, cl.parent)
			}
			for j := range n {
				row = append(row, p.agg(rows, total, j))
			}
		}
		out = append(out, row)
	}
	return grid(out)
}

// grid turns equal-length rows into an array.
func grid(rows [][]f.Value) f.Value {
	out := &f.Array{Rows: len(rows), Cols: len(rows[0])}
	for _, r := range rows {
		out.V = append(out.V, r...)
	}
	return single(out)
}
