package calc

import (
	"slices"
	"strings"
	"time"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// Area is a rectangle of cells on one sheet.
type Area struct {
	Sheet int
	model.Range
}

// CellArea is the one-cell area at sheet, row, col.
func CellArea(sheet, row, col int) Area {
	return Area{sheet, model.Range{R1: row, C1: col, R2: row, C2: col}}
}

func (a Area) intersects(b Area) bool {
	return a.Sheet == b.Sheet && a.R1 <= b.R2 && b.R1 <= a.R2 && a.C1 <= b.C2 && b.C1 <= a.C2
}

func (a Area) size() int { return (a.R2 - a.R1 + 1) * (a.C2 - a.C1 + 1) }

// wideCols is the width above which a range is kept in a sheet's list of
// wide ranges instead of under each of its columns.
const wideCols = 16

// Calculator recalculates a workbook as it is edited. It keeps every
// formula's parsed form and the areas it reads, so Update recalculates
// only the formulas that depend on what changed instead of the whole
// workbook (a full recalculation of 100k formulas takes ~200ms).
type Calculator struct {
	wb    *model.Workbook
	funcs *formula.Registry
	built bool
	gen   int

	f      map[key]*fentry
	points map[key][]dref    // single cells read
	cols   map[[2]int][]dref // (sheet, col) -> narrow ranges read
	wide   map[int][]dref    // sheet -> wide ranges read
	always map[key]bool      // volatile, or reads cells it can't name in advance
	dyn    map[key]bool      // dynamic-array formulas
	stale  int               // index entries of removed or replaced formulas
}

type fentry struct {
	text    string
	node    formula.Node // nil when the formula doesn't parse
	areas   []Area
	gen     int
	dynamic bool
}

// dref is an index entry: formula f (as of generation gen) reads area.
type dref struct {
	area Area
	f    key
	gen  int
}

// NewCalculator returns a calculator for wb. Its dependency information is
// built on first use.
func NewCalculator(wb *model.Workbook, funcs *formula.Registry) *Calculator {
	return &Calculator{wb: wb, funcs: funcs}
}

// Invalidate drops the dependency information, after changes that move
// cells or rename sheets (inserting rows, deleting sheets ...).
func (c *Calculator) Invalidate() { c.built = false }

func (c *Calculator) build() {
	c.f = map[key]*fentry{}
	c.points = map[key][]dref{}
	c.cols = map[[2]int][]dref{}
	c.wide = map[int][]dref{}
	c.always = map[key]bool{}
	c.dyn = map[key]bool{}
	c.stale = 0
	for si, sh := range c.wb.Sheets {
		for r := 0; r < sh.UsedRows(); r++ {
			for col := 0; col < sh.RowLen(r); col++ {
				if cell := sh.Get(r, col); cell.Formula != "" {
					c.add(key{si, r, col}, cell)
				}
			}
		}
	}
	c.built = true
}

// add parses the formula at k and indexes what it reads.
func (c *Calculator) add(k key, cell *model.Cell) {
	c.gen++
	e := &fentry{text: cell.Formula, gen: c.gen, dynamic: cell.Dynamic}
	node, err := formula.Parse(cell.Formula)
	if err == nil {
		e.node = node
		always := false
		e.areas = c.resolve(k.sheet, formula.Dependencies(node), 0, &always)
		if always {
			c.always[k] = true
		}
	}
	c.f[k] = e
	if e.dynamic {
		c.dyn[k] = true
	}
	for _, a := range e.areas {
		d := dref{area: a, f: k, gen: e.gen}
		switch {
		case a.size() == 1:
			p := key{a.Sheet, a.R1, a.C1}
			c.points[p] = append(c.points[p], d)
		case a.C2-a.C1 < wideCols:
			for col := a.C1; col <= a.C2; col++ {
				ck := [2]int{a.Sheet, col}
				c.cols[ck] = append(c.cols[ck], d)
			}
		default:
			c.wide[a.Sheet] = append(c.wide[a.Sheet], d)
		}
	}
}

// volatileExtra are functions whose result depends on more than their
// arguments, besides those marked Volatile in the registry.
var volatileExtra = map[string]bool{"CELL": true, "INFO": true}

// resolve turns a formula's dependencies into areas: sheet names become
// indexes and defined names are followed (to a limited depth). always is
// set for volatile formulas and ones whose reads can't be listed.
func (c *Calculator) resolve(home int, d formula.Deps, depth int, always *bool) []Area {
	if d.Dynamic {
		*always = true
	}
	for _, fn := range d.Funcs {
		if s := c.funcs.Lookup(fn); (s != nil && s.Volatile) || volatileExtra[fn] {
			*always = true
		}
	}
	var out []Area
	for _, r := range d.Refs {
		si := home
		if r.Sheet != "" {
			if si = c.sheetIndex(r.Sheet); si < 0 {
				continue
			}
		}
		out = append(out, Area{si, model.Range{R1: r.R1, C1: r.C1, R2: r.R2, C2: r.C2}})
	}
	for _, n := range d.Names {
		scope := home
		if n.Sheet != "" {
			if scope = c.sheetIndex(n.Sheet); scope < 0 {
				continue
			}
		}
		src, ok := c.name(n.Name, scope)
		if !ok {
			continue // a LET or LAMBDA variable
		}
		if depth >= 8 {
			*always = true
			continue
		}
		node, err := formula.Parse(src)
		if err != nil {
			continue
		}
		out = append(out, c.resolve(home, formula.Dependencies(node), depth+1, always)...)
	}
	return out
}

func (c *Calculator) sheetIndex(name string) int {
	for i, sh := range c.wb.Sheets {
		if strings.EqualFold(sh.Name, name) {
			return i
		}
	}
	return -1
}

func (c *Calculator) name(name string, sheet int) (string, bool) {
	return (&engine{wb: c.wb}).Name(name, sheet)
}

func (c *Calculator) remove(k key) {
	if e, ok := c.f[k]; ok {
		c.stale += len(e.areas)
		delete(c.f, k)
		delete(c.always, k)
		delete(c.dyn, k)
	}
}

func (c *Calculator) valid(d dref) bool {
	e := c.f[d.f]
	return e != nil && e.gen == d.gen
}

// refresh re-reads the formulas in area a after an edit.
func (c *Calculator) refresh(a Area) {
	sh := c.wb.Sheets[a.Sheet]
	for r := a.R1; r <= min(a.R2, sh.UsedRows()-1); r++ {
		for col := a.C1; col <= min(a.C2, sh.RowLen(r)-1); col++ {
			k := key{a.Sheet, r, col}
			cell := sh.Get(r, col)
			e := c.f[k]
			switch {
			case cell.Formula == "":
				c.remove(k)
			case e == nil || e.text != cell.Formula || e.dynamic != cell.Dynamic:
				c.remove(k)
				c.add(k, cell)
			}
		}
	}
	if c.stale > 4096 && c.stale > 2*len(c.f) {
		c.build()
	}
}

// dependents calls fn for each formula that reads a cell of a.
func (c *Calculator) dependents(a Area, fn func(key)) {
	if a.size() <= 4096 {
		for r := a.R1; r <= a.R2; r++ {
			for col := a.C1; col <= a.C2; col++ {
				for _, d := range c.points[key{a.Sheet, r, col}] {
					if c.valid(d) {
						fn(d.f)
					}
				}
			}
		}
	} else {
		for p, ds := range c.points {
			if p.sheet == a.Sheet && a.Contains(p.row, p.col) {
				for _, d := range ds {
					if c.valid(d) {
						fn(d.f)
					}
				}
			}
		}
	}
	for col := a.C1; col <= min(a.C2, model.MaxCols-1); col++ {
		for _, d := range c.cols[[2]int{a.Sheet, col}] {
			if d.area.intersects(a) && c.valid(d) {
				fn(d.f)
			}
		}
	}
	for _, d := range c.wide[a.Sheet] {
		if d.area.intersects(a) && c.valid(d) {
			fn(d.f)
		}
	}
}

// formulasIn calls fn for each formula cell inside a.
func (c *Calculator) formulasIn(a Area, fn func(key)) {
	if a.size() <= 4096 {
		for r := a.R1; r <= a.R2; r++ {
			for col := a.C1; col <= a.C2; col++ {
				if k := (key{a.Sheet, r, col}); c.f[k] != nil {
					fn(k)
				}
			}
		}
		return
	}
	for k := range c.f {
		if k.sheet == a.Sheet && a.Contains(k.row, k.col) {
			fn(k)
		}
	}
}

// Full recalculates every formula (F9), reusing parsed formulas.
func (c *Calculator) Full(now time.Time) Result {
	if !c.built {
		c.build()
	}
	e := newEngine(c.wb, c.funcs, now)
	defer e.nf.Close()
	for k, f := range c.f {
		e.parsed[k] = f.node
	}
	return e.recalculate(Full)
}

// Update recalculates the formulas affected by edits to the cells in
// changed (new values, new or removed formulas): those that read the
// changed cells, directly or through other formulas, plus volatile
// formulas and dynamic arrays whose spill area was written into or that
// were blocked (#SPILL!) on an edited sheet.
func (c *Calculator) Update(changed []Area, now time.Time) Result {
	if !c.built {
		c.build()
	}
	queue := slices.Clone(changed)
	for _, a := range changed {
		queue = append(queue, c.dropSpills(a, changed)...)
	}
	for _, a := range changed {
		c.refresh(a)
	}
	edited := func(sheet, r, col int) bool {
		for _, a := range changed {
			if a.Sheet == sheet && a.Contains(r, col) {
				return true
			}
		}
		return false
	}
	var res Result
	var from []key // the anchor whose spill changed each queued area
	for round := 0; round < 8 && len(queue) > 0; round++ {
		dirty := c.propagate(queue, from)
		old := map[key]model.Range{}
		for k := range dirty {
			if rng, ok := c.wb.Sheets[k.sheet].Spills[[2]int{k.row, k.col}]; ok {
				old[k] = rng
			}
		}
		e := newEngine(c.wb, c.funcs, now)
		e.res = res
		todo := make([]key, 0, len(dirty))
		for k := range dirty {
			todo = append(todo, k)
		}
		slices.SortFunc(todo, func(a, b key) int {
			if a.sheet != b.sheet {
				return a.sheet - b.sheet
			}
			if a.row != b.row {
				return a.row - b.row
			}
			return a.col - b.col
		})
		for _, k := range todo {
			e.state[k] = pending
			e.parsed[k] = c.f[k].node
			if c.f[k].dynamic {
				e.dynamic[k.sheet] = append(e.dynamic[k.sheet], k)
			}
			// Spilled values are recomputed; values typed over them stay.
			if rng, ok := old[k]; ok {
				sh := c.wb.Sheets[k.sheet]
				anchor := [2]int{k.row, k.col}
				for r := rng.R1; r <= rng.R2; r++ {
					for col := rng.C1; col <= rng.C2; col++ {
						if [2]int{r, col} == anchor || (round == 0 && edited(k.sheet, r, col)) {
							continue
						}
						if cell := sh.Get(r, col); cell.Formula == "" && cell.Kind != model.KindEmpty {
							sh.Set(r, col, model.Cell{Style: cell.Style})
						}
					}
				}
				delete(sh.Spills, anchor)
			}
		}
		for _, k := range todo {
			if e.state[k] == pending {
				e.compute(k)
			}
		}
		e.nf.Close()
		res = e.res
		// A spill that grew changes cells nothing above depended on yet;
		// one that shrank may unblock another. Go round again for those.
		queue, from = nil, nil
		for _, k := range todo {
			if !c.f[k].dynamic {
				continue
			}
			was, had := old[k]
			now, has := c.wb.Sheets[k.sheet].Spills[[2]int{k.row, k.col}]
			if had == has && was == now {
				continue
			}
			if has && !(had && contains(was, now)) {
				queue, from = append(queue, Area{k.sheet, now}), append(from, k)
			}
			if had && !(has && contains(now, was)) {
				queue, from = append(queue, Area{k.sheet, was}), append(from, k)
			}
		}
	}
	return res
}

// dropSpills empties the spill areas of dynamic arrays in a that are no
// longer dynamic-array formulas (overwritten or cleared), except for cells
// in changed, which hold new content. It returns the emptied areas.
func (c *Calculator) dropSpills(a Area, changed []Area) []Area {
	sh := c.wb.Sheets[a.Sheet]
	var out []Area
	for anchor, rng := range sh.Spills {
		if !a.Contains(anchor[0], anchor[1]) {
			continue
		}
		if cell := sh.Get(anchor[0], anchor[1]); cell.Formula != "" && cell.Dynamic {
			continue
		}
		for r := rng.R1; r <= rng.R2; r++ {
			for col := rng.C1; col <= rng.C2; col++ {
				keep := false
				for _, ch := range changed {
					keep = keep || (ch.Sheet == a.Sheet && ch.Contains(r, col))
				}
				if cell := sh.Get(r, col); !keep && cell.Formula == "" && cell.Kind != model.KindEmpty {
					sh.Set(r, col, model.Cell{Style: cell.Style})
				}
			}
		}
		delete(sh.Spills, anchor)
		out = append(out, Area{a.Sheet, rng})
	}
	return out
}

func contains(outer, inner model.Range) bool {
	return inner.R1 >= outer.R1 && inner.R2 <= outer.R2 && inner.C1 >= outer.C1 && inner.C2 <= outer.C2
}

// propagate returns the formulas to recalculate after the cells in queue
// changed. On the first round (from == nil) the queue holds edited cells,
// whose formulas and the volatile ones are included; later rounds hold the
// cells whose spilled values changed, from[i] being the anchor that
// spilled queue[i].
func (c *Calculator) propagate(queue []Area, from []key) map[key]bool {
	dirty := map[key]bool{}
	var work []Area
	mark := func(k key) {
		if dirty[k] {
			return
		}
		dirty[k] = true
		work = append(work, CellArea(k.sheet, k.row, k.col))
		if rng, ok := c.wb.Sheets[k.sheet].Spills[[2]int{k.row, k.col}]; ok {
			work = append(work, Area{k.sheet, rng})
		}
	}
	first := from == nil
	if first {
		for k := range c.always {
			mark(k)
		}
	}
	// Dynamic arrays spilling over a changed cell, or blocked on a
	// changed sheet.
	sheets := map[int]bool{}
	for _, a := range queue {
		sheets[a.Sheet] = true
	}
	for k := range c.dyn {
		if !sheets[k.sheet] {
			continue
		}
		sh := c.wb.Sheets[k.sheet]
		if sh.Get(k.row, k.col).Text == string(formula.ErrSpill) {
			mark(k)
			continue
		}
		if rng, ok := sh.Spills[[2]int{k.row, k.col}]; ok {
			for i, a := range queue {
				if (first || from[i] != k) && a.intersects(Area{k.sheet, rng}) {
					mark(k)
					break
				}
			}
		}
	}
	for _, a := range queue {
		if first {
			c.formulasIn(a, mark)
		}
		c.dependents(a, mark)
	}
	for len(work) > 0 {
		a := work[len(work)-1]
		work = work[:len(work)-1]
		c.formulasIn(a, mark)
		c.dependents(a, mark)
	}
	return dirty
}
