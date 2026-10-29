package formula

import (
	"math"
	"strings"
	"sync"
	"time"

	"officelite/excel/model"
)

// Context gives formulas access to the workbook. Implemented by the
// recalculation engine (excel/calc) and by TestContext.
type Context interface {
	// CellValue returns a cell's value, calculating it first if it holds a
	// formula that has not been calculated yet.
	CellValue(sheet, row, col int) Value
	// CellFormula returns a cell's formula text ("" for constants).
	CellFormula(sheet, row, col int) string
	SheetIndex(name string) int // -1 if there is no such sheet
	SheetName(sheet int) string
	SheetCount() int
	// UsedSize is the extent of a sheet's data; whole-column and whole-row
	// references are trimmed to it.
	UsedSize(sheet int) (rows, cols int)
	// Name returns the formula of a defined name visible from sheet.
	Name(name string, sheet int) (string, bool)
	// Spill returns the range a dynamic-array formula at row, col spilled
	// into, for A1# references.
	Spill(sheet, row, col int) (Ref, bool)
	Now() time.Time
	Date1904() bool
	Rand() float64 // uniform in [0, 1)
}

type scope struct {
	name string // upper-case
	val  Value
	next *scope
}

func (s *scope) lookup(name string) (Value, bool) {
	for ; s != nil; s = s.next {
		if s.name == name {
			return s.val, true
		}
	}
	return nil, false
}

// maxDepth bounds nested evaluation (deep LAMBDA recursion).
const maxDepth = 1000

// Env evaluates formulas for one cell.
type Env struct {
	Ctx             Context
	Funcs           *Registry
	Sheet, Row, Col int // the cell being calculated

	scope *scope
	depth int

	// Volatile is set when a volatile function was called.
	Volatile bool

	// Legacy marks an ordinary (not dynamic-array, not array) formula:
	// Excel 2019 and earlier semantics, where a range given in place of
	// one value is implicitly intersected with the formula's row or
	// column (=B1:B3*2 in row 2 is B2*2). Arguments of functions that
	// take arrays (SUMPRODUCT ...) are still evaluated as arrays.
	Legacy bool
	arrays int // > 0 while evaluating such an argument

	root   Node // the formula being evaluated
	extent *[2]int
}

// NewEnv returns an environment for calculating the cell at sheet, row, col.
func NewEnv(ctx Context, funcs *Registry, sheet, row, col int) *Env {
	return &Env{Ctx: ctx, Funcs: funcs, Sheet: sheet, Row: row, Col: col}
}

func (e *Env) Now() time.Time { return e.Ctx.Now() }
func (e *Env) Date1904() bool { return e.Ctx.Date1904() }
func (e *Env) Rand() float64  { return e.Ctx.Rand() }

// Num coerces v like the package-level Num, but parses date text in the
// workbook's date system (Num always reads date text as a 1900-system
// serial).
func (e *Env) Num(v Value) (float64, Value) {
	if s, ok := v.(String); ok && e.Date1904() {
		if d, ok := ParseDateTime(string(s), true); ok {
			return d, nil
		}
	}
	return Num(v)
}

// Result is a top-level formula result.
type Result struct {
	Value  Value  // scalar or *Array (never a Ref)
	Format string // number format suggested by the outermost function
}

// Evaluate calculates a parsed formula for the environment's cell. The
// result is dereferenced; a Lambda result is #CALC!.
func (e *Env) Evaluate(n Node) Result {
	var res Result
	if c, ok := n.(callNode); ok {
		if s := e.Funcs.Lookup(c.name); s != nil {
			res.Format = s.Format
		}
	}
	e.root, e.extent = n, nil
	var v Value
	if b, ok := n.(binaryNode); ok && (b.op == "+" || b.op == "-") {
		// Excel sets a final addition or subtraction whose result is only
		// rounding noise to 0: =1-0.9-0.1 is 0, not -2.8E-17.
		v = e.binary(b.op, e.operand(b.l), e.operand(b.r), true)
	} else {
		v = e.operand(n)
	}
	switch x := v.(type) {
	case *Lambda:
		v = ErrCalc
	case *Array:
		if x.Single() {
			v = x.V[0]
		}
		for i, el := range x.V {
			if _, ok := el.(*Lambda); ok {
				x.V[i] = ErrCalc
			}
		}
	case Missing:
		v = Number(0)
	}
	res.Value = v
	return res
}

// EvalString parses and evaluates src (for tests and one-off use).
func EvalString(src string, ctx Context, funcs *Registry, sheet, row, col int) Value {
	n, err := Parse(src)
	if err != nil {
		return ErrName
	}
	return NewEnv(ctx, funcs, sheet, row, col).Evaluate(n).Value
}

// Eval evaluates a node. The result may be a Ref; use Deref, Scalar or
// ToArray to get at values.
func (e *Env) Eval(n Node) Value {
	switch n := n.(type) {
	case numNode:
		return Number(n.v)
	case strNode:
		return String(n.v)
	case boolNode:
		return Bool(n.v)
	case errNode:
		return n.v
	case missNode:
		return Missing{}
	case arrayNode:
		a := &Array{Rows: len(n.rows), Cols: len(n.rows[0])}
		for _, row := range n.rows {
			for _, el := range row {
				a.V = append(a.V, e.Eval(el))
			}
		}
		return a
	case refNode:
		return e.ref(n)
	case nameNode:
		return e.name(n)
	case callNode:
		return e.call(n)
	case invokeNode:
		return e.invoke(n)
	case unaryNode:
		return e.unary(n)
	case binaryNode:
		if n.op == ":" {
			return e.rangeOp(n)
		}
		return e.binary(n.op, e.operand(n.l), e.operand(n.r), false)
	case spillNode:
		v := e.Eval(n.x)
		r, ok := v.(Ref)
		if !ok {
			return ErrRef
		}
		if sr, ok := e.Ctx.Spill(r.Sheet, r.R1, r.C1); ok {
			return sr
		}
		return ErrRef
	}
	return ErrValue
}

func (e *Env) sheetIndex(name string) int {
	if name == "" {
		return e.Sheet
	}
	return e.Ctx.SheetIndex(name)
}

func (e *Env) ref(n refNode) Value {
	sh := e.sheetIndex(n.sheet)
	if sh < 0 {
		return ErrRef
	}
	r := Ref{Sheet: sh, R1: n.r1, C1: n.c1, R2: n.r2, C2: n.c2}
	if n.wholeCols {
		r.R1, r.R2 = 0, model.MaxRows-1
	}
	if n.wholeRows {
		r.C1, r.C2 = 0, model.MaxCols-1
	}
	return normRef(r)
}

func normRef(r Ref) Ref {
	if r.R1 > r.R2 {
		r.R1, r.R2 = r.R2, r.R1
	}
	if r.C1 > r.C2 {
		r.C1, r.C2 = r.C2, r.C1
	}
	return r
}

func (e *Env) rangeOp(n binaryNode) Value {
	l, r := e.Eval(n.l), e.Eval(n.r)
	a, ok1 := l.(Ref)
	b, ok2 := r.(Ref)
	if !ok1 || !ok2 {
		// #REF!:B1 (a reference shifted off the sheet) is #REF!.
		if err, ok := l.(Error); ok {
			return err
		}
		if err, ok := r.(Error); ok {
			return err
		}
		return ErrValue
	}
	if a.Sheet != b.Sheet {
		return ErrRef
	}
	return Ref{Sheet: a.Sheet,
		R1: min(a.R1, b.R1), C1: min(a.C1, b.C1),
		R2: max(a.R2, b.R2), C2: max(a.C2, b.C2)}
}

var parsedNames sync.Map // formula text -> Node

func (e *Env) name(n nameNode) Value {
	if n.sheet == "" {
		if v, ok := e.scope.lookup(n.name); ok {
			return v
		}
	}
	sh := e.sheetIndex(n.sheet)
	if sh < 0 {
		return ErrRef
	}
	src, ok := e.Ctx.Name(n.name, sh)
	if !ok {
		// A worksheet function named without its parentheses is a function
		// value (an eta-reduced lambda), as in BYROW(A1:B3,SUM).
		if s := e.Funcs.Lookup(n.name); s != nil && s.Lazy == nil && n.sheet == "" {
			return &Lambda{Func: s}
		}
		return ErrName
	}
	node, ok := parsedNames.Load(src)
	if !ok {
		p, err := Parse(src)
		if err != nil {
			return ErrName
		}
		node, _ = parsedNames.LoadOrStore(src, p)
	}
	return e.nested(func() Value { return e.Eval(node.(Node)) })
}

func (e *Env) nested(f func() Value) Value {
	if e.depth >= maxDepth {
		return ErrCalc
	}
	e.depth++
	defer func() { e.depth-- }()
	return f()
}

// --- references and arrays ---------------------------------------------

// Deref replaces a reference by its contents: the cell value for a single
// cell, otherwise an *Array. Other values are returned unchanged.
func (e *Env) Deref(v Value) Value {
	r, ok := v.(Ref)
	if !ok {
		return v
	}
	if r.Single() {
		return e.Ctx.CellValue(r.Sheet, r.R1, r.C1)
	}
	r = e.trim(r)
	a := &Array{Rows: r.Rows(), Cols: r.Cols(), V: make([]Value, 0, r.Rows()*r.Cols())}
	for row := r.R1; row <= r.R2; row++ {
		for col := r.C1; col <= r.C2; col++ {
			a.V = append(a.V, e.Ctx.CellValue(r.Sheet, row, col))
		}
	}
	return a
}

// trim shrinks whole-column/whole-row references to the sheet's used area
// so they can be materialized. When a formula has whole columns (rows) on
// more than one sheet they are all trimmed to the tallest (widest) of
// those sheets, so they still combine element by element:
// =SUMPRODUCT((Sheet2!A:A>1)*B:B).
func (e *Env) trim(r Ref) Ref {
	wholeCols, wholeRows := r.R2 >= model.MaxRows-1, r.C2 >= model.MaxCols-1
	if !wholeCols && !wholeRows {
		return r
	}
	rows, cols := e.usedSize(r)
	ext := e.commonExtent()
	if wholeCols {
		r.R2 = max(r.R1, min(r.R2, max(rows, ext[0])-1))
	}
	if wholeRows {
		r.C2 = max(r.C1, min(r.C2, max(cols, ext[1])-1))
	}
	return r
}

// RangeSizer is implemented by contexts in which formulas not calculated
// yet can change a sheet's used area (dynamic arrays spilling into a
// whole column read before them). UsedSizeFor is UsedSize after
// calculating whatever could spill into r.
type RangeSizer interface {
	UsedSizeFor(r Ref) (rows, cols int)
}

func (e *Env) usedSize(r Ref) (rows, cols int) {
	if rs, ok := e.Ctx.(RangeSizer); ok {
		return rs.UsedSizeFor(r)
	}
	return e.Ctx.UsedSize(r.Sheet)
}

// commonExtent is the height (and width) that whole columns (rows) of the
// formula being evaluated are trimmed to when they span several sheets;
// zero otherwise.
func (e *Env) commonExtent() [2]int {
	if e.extent != nil {
		return *e.extent
	}
	e.extent = new([2]int)
	if e.root == nil {
		return *e.extent
	}
	var colRefs, rowRefs []Ref
	colSheets, rowSheets := map[int]bool{}, map[int]bool{}
	for _, d := range Dependencies(e.root).Refs {
		si := e.sheetIndex(d.Sheet)
		if si < 0 {
			continue
		}
		r := Ref{Sheet: si, R1: d.R1, C1: d.C1, R2: d.R2, C2: d.C2}
		if r.R2 >= model.MaxRows-1 {
			colRefs, colSheets[si] = append(colRefs, r), true
		}
		if r.C2 >= model.MaxCols-1 {
			rowRefs, rowSheets[si] = append(rowRefs, r), true
		}
	}
	var ext [2]int
	if len(colSheets) > 1 {
		for _, r := range colRefs {
			rows, _ := e.usedSize(r)
			ext[0] = max(ext[0], rows)
		}
	}
	if len(rowSheets) > 1 {
		for _, r := range rowRefs {
			_, cols := e.usedSize(r)
			ext[1] = max(ext[1], cols)
		}
	}
	*e.extent = ext
	return ext
}

// Trim is the reference r limited to the used area when it spans whole
// columns or rows (for iterating without visiting a million blanks).
func (e *Env) Trim(r Ref) Ref { return e.trim(r) }

// Scalar dereferences v and reduces a multi-cell value to its first
// element (callers that want lifting should declare scalar arguments in
// their Spec instead).
func (e *Env) Scalar(v Value) Value {
	v = e.Deref(v)
	if a, ok := v.(*Array); ok {
		if a.Len() == 0 {
			return Blank{}
		}
		return a.V[0]
	}
	return v
}

// implicit reports whether a range in place of a single value is
// implicitly intersected (see Legacy).
func (e *Env) implicit() bool { return e.Legacy && e.arrays == 0 }

// operand evaluates an operator's operand (or a whole formula): its value,
// with a range implicitly intersected in a legacy formula.
func (e *Env) operand(n Node) Value {
	v := e.Eval(n)
	if r, ok := v.(Ref); ok && !r.Single() && e.implicit() {
		return e.intersect(r)
	}
	return e.Deref(v)
}

// EvalArg evaluates a function argument that expects a single value (IF's
// condition ...): a range is implicitly intersected in a legacy formula,
// otherwise left as is for the caller to lift over.
func (e *Env) EvalArg(n Node) Value {
	v := e.Eval(n)
	if r, ok := v.(Ref); ok && !r.Single() && e.implicit() {
		return e.intersect(r)
	}
	return v
}

// legacyArrays lists the arguments that legacy Excel evaluates as arrays
// without Ctrl+Shift+Enter: the array-class parameters of the BIFF
// function table ([MS-XLS] 2.5.198.17), their Excel 2010 renamings, and
// the arrays of INDEX, LOOKUP, AGGREGATE and the regression functions
// (the usual ways around array formulas: LOOKUP(2,1/(A1:A9="x"),B1:B9)).
var legacyArrays = map[string]func(int) bool{
	"SUMPRODUCT": All, "MMULT": All, "MDETERM": All, "MINVERSE": All, "TRANSPOSE": All,
	"SUMXMY2": All, "SUMX2MY2": All, "SUMX2PY2": All,
	"CHITEST": All, "CHISQ.TEST": All, "CORREL": All, "COVAR": All, "COVARIANCE.P": All,
	"COVARIANCE.S": All, "FTEST": All, "F.TEST": All, "INTERCEPT": All, "PEARSON": All,
	"RSQ": All, "STEYX": All, "SLOPE": All, "MODE": All, "MODE.SNGL": All, "MODE.MULT": All,
	"FORECAST": From(1), "FORECAST.LINEAR": From(1),
	"TTEST": Only(0, 1), "T.TEST": Only(0, 1), "PROB": Only(0, 1),
	"LINEST": Only(0, 1), "LOGEST": Only(0, 1), "TREND": Only(0, 1, 2), "GROWTH": Only(0, 1, 2),
	"INDEX": Only(0), "LOOKUP": Only(1, 2), "AGGREGATE": From(2),
}

// ToArray dereferences v and wraps a scalar in a 1×1 array.
func (e *Env) ToArray(v Value) *Array {
	v = e.Deref(v)
	if a, ok := v.(*Array); ok {
		return a
	}
	return &Array{Rows: 1, Cols: 1, V: []Value{v}}
}

// Each calls fn for every scalar in v: each cell of a reference (trimmed to
// the used area), each element of an array, or v itself. direct is true
// only for a scalar passed directly (Excel aggregates treat those
// differently, e.g. SUM counts the text "3" typed as an argument but not
// in a cell). Iteration stops when fn returns false.
func (e *Env) Each(v Value, fn func(v Value, direct bool) bool) {
	switch x := v.(type) {
	case Ref:
		r := e.trim(x)
		for row := r.R1; row <= r.R2; row++ {
			for col := r.C1; col <= r.C2; col++ {
				if !fn(e.Ctx.CellValue(r.Sheet, row, col), false) {
					return
				}
			}
		}
	case *Array:
		for _, el := range x.V {
			if !fn(el, false) {
				return
			}
		}
	default:
		fn(v, true)
	}
}

// --- functions ------------------------------------------------------------

func (e *Env) call(n callNode) Value {
	// A LET/LAMBDA variable holding a lambda: f(1, 2).
	if v, ok := e.scope.lookup(n.name); ok {
		return e.invokeValue(v, n.args)
	}
	switch n.name {
	case "LET":
		return e.let(n.args)
	case "LAMBDA":
		return e.lambda(n.args)
	}
	s := e.Funcs.Lookup(n.name)
	if s == nil {
		return ErrName
	}
	if len(n.args) < s.Min || (s.Max != Variadic && len(n.args) > s.Max) {
		return ErrValue
	}
	if s.Volatile {
		e.Volatile = true
	}
	if s.Lazy != nil {
		return e.nested(func() Value { return s.Lazy(e, n.args) })
	}
	args := make([]Value, len(n.args))
	arrays := legacyArrays[s.Name]
	for i, a := range n.args {
		if arrays != nil && arrays(i) && e.Legacy {
			e.arrays++
			args[i] = e.Eval(a)
			e.arrays--
			continue
		}
		args[i] = e.Eval(a)
	}
	return e.nested(func() Value { return e.Apply(s, args) })
}

// Apply calls a function with already-evaluated arguments, dereferencing
// and lifting them according to its Spec.
func (e *Env) Apply(s *Spec, args []Value) Value {
	lift := false
	rows, cols := 1, 1
	for i, v := range args {
		if r, ok := v.(Ref); ok && s.Refs != nil && s.Refs(i) {
			args[i] = r
			continue
		}
		if r, ok := v.(Ref); ok && !r.Single() && e.implicit() && !(s.Arrays != nil && s.Arrays(i)) {
			v = e.intersect(r)
		}
		v = e.Deref(v)
		if a, ok := v.(*Array); ok {
			switch {
			case s.Arrays != nil && s.Arrays(i):
			case a.Single():
				v = a.V[0]
			default:
				lift = true
				rows, cols = max(rows, a.Rows), max(cols, a.Cols)
			}
		}
		args[i] = v
	}
	if !lift {
		return s.Fn(e, args)
	}
	out := &Array{Rows: rows, Cols: cols, V: make([]Value, 0, rows*cols)}
	el := make([]Value, len(args))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			for i, v := range args {
				if a, ok := v.(*Array); ok && !(s.Arrays != nil && s.Arrays(i)) {
					v = a.Broadcast(r, c)
				}
				el[i] = v
			}
			// Each element must be a scalar: references are dereferenced and
			// a nested array contributes its top-left value, as in Excel
			// (=SEQUENCE({2,3}) gives {1,1}).
			res := e.Scalar(s.Fn(e, append([]Value(nil), el...)))
			out.V = append(out.V, res)
		}
	}
	return out
}

// --- LET / LAMBDA -----------------------------------------------------------

// let implements LET(name1, value1, [name2, value2, ...], calculation).
func (e *Env) let(args []Node) Value {
	if len(args) < 3 || len(args)%2 == 0 {
		return ErrValue
	}
	saved := e.scope
	defer func() { e.scope = saved }()
	for i := 0; i+1 < len(args); i += 2 {
		nm, ok := args[i].(nameNode)
		if !ok || nm.sheet != "" {
			return ErrValue
		}
		v := e.Eval(args[i+1])
		e.scope = &scope{name: nm.name, val: v, next: e.scope}
	}
	return e.Eval(args[len(args)-1])
}

// lambda implements LAMBDA([param1, ...], calculation).
func (e *Env) lambda(args []Node) Value {
	if len(args) == 0 {
		return ErrValue
	}
	l := &Lambda{Body: args[len(args)-1], scope: e.scope}
	for _, a := range args[:len(args)-1] {
		nm, ok := a.(nameNode)
		if !ok || nm.sheet != "" {
			return ErrValue
		}
		l.Params = append(l.Params, nm.name)
	}
	return l
}

func (e *Env) invoke(n invokeNode) Value {
	return e.invokeValue(e.Eval(n.fn), n.args)
}

func (e *Env) invokeValue(fn Value, argNodes []Node) Value {
	args := make([]Value, len(argNodes))
	for i, a := range argNodes {
		args[i] = e.Eval(a)
	}
	return e.CallLambda(fn, args...)
}

// CallLambda calls a LAMBDA value with the given arguments (fewer
// arguments than parameters leaves the rest Missing; see ISOMITTED), or
// the worksheet function of an eta-reduced lambda.
func (e *Env) CallLambda(fn Value, args ...Value) Value {
	l, ok := fn.(*Lambda)
	if !ok {
		if err, isErr := fn.(Error); isErr {
			return err
		}
		return ErrValue
	}
	if !l.Accepts(len(args)) {
		return ErrValue
	}
	if l.Func != nil {
		if l.Func.Volatile {
			e.Volatile = true
		}
		return e.nested(func() Value { return e.Apply(l.Func, append([]Value(nil), args...)) })
	}
	saved := e.scope
	defer func() { e.scope = saved }()
	e.scope = l.scope
	for i, p := range l.Params {
		var v Value = Missing{}
		if i < len(args) {
			v = args[i]
		}
		e.scope = &scope{name: p, val: v, next: e.scope}
	}
	return e.nested(func() Value { return e.Eval(l.Body) })
}

// --- operators ------------------------------------------------------------

func (e *Env) unary(n unaryNode) Value {
	if n.op == "@" {
		return e.intersect(e.Eval(n.x))
	}
	v := e.operand(n.x)
	f := func(x Value) Value {
		switch n.op {
		case "+":
			return x
		case "-":
			f, err := Num(x)
			if err != nil {
				return err
			}
			return Number(-f)
		case "%":
			f, err := Num(x)
			if err != nil {
				return err
			}
			return Number(f / 100)
		}
		return ErrValue
	}
	if a, ok := v.(*Array); ok {
		out := &Array{Rows: a.Rows, Cols: a.Cols, V: make([]Value, len(a.V))}
		for i, x := range a.V {
			out.V[i] = f(x)
		}
		return out
	}
	return f(v)
}

// intersect implements '@' (implicit intersection): the cell of a range in
// the formula's row or column (on whatever sheet the range is), or the
// first element of an array.
func (e *Env) intersect(v Value) Value {
	switch x := v.(type) {
	case Ref:
		inRows, inCols := e.Row >= x.R1 && e.Row <= x.R2, e.Col >= x.C1 && e.Col <= x.C2
		switch {
		case x.Single():
			return e.Deref(x)
		case x.Cols() == 1 && inRows:
			return e.Ctx.CellValue(x.Sheet, e.Row, x.C1)
		case x.Rows() == 1 && inCols:
			return e.Ctx.CellValue(x.Sheet, x.R1, e.Col)
		case inRows && inCols:
			return e.Ctx.CellValue(x.Sheet, e.Row, e.Col)
		}
		return ErrValue
	case *Array:
		if x.Len() == 0 {
			return ErrValue
		}
		return x.V[0]
	}
	return v
}

// binary applies an operator, element by element to arrays. snap applies
// Excel's rounding of a near-zero sum or difference (see snapZero).
func (e *Env) binary(op string, l, r Value, snap bool) Value {
	la, lok := l.(*Array)
	ra, rok := r.(*Array)
	if lok && la.Single() {
		l, lok = la.V[0], false
	}
	if rok && ra.Single() {
		r, rok = ra.V[0], false
	}
	if !lok && !rok {
		if snap {
			return snapZero(l, r, scalarOp(op, l, r))
		}
		return scalarOp(op, l, r)
	}
	rows, cols := 1, 1
	if lok {
		rows, cols = max(rows, la.Rows), max(cols, la.Cols)
	}
	if rok {
		rows, cols = max(rows, ra.Rows), max(cols, ra.Cols)
	}
	out := &Array{Rows: rows, Cols: cols, V: make([]Value, 0, rows*cols)}
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			x, y := l, r
			if lok {
				x = la.Broadcast(i, j)
			}
			if rok {
				y = ra.Broadcast(i, j)
			}
			v := scalarOp(op, x, y)
			if snap {
				v = snapZero(x, y, v)
			}
			out.V = append(out.V, v)
		}
	}
	return out
}

// snapZero returns 0 instead of the sum or difference v of l and r when v
// is only floating-point noise: under 2^-48 of the operands' magnitude,
// which Excel (and LibreOffice, rtl::math::approxSub) does for a formula's
// final addition or subtraction (Microsoft KB 78113).
func snapZero(l, r, v Value) Value {
	n, ok := v.(Number)
	if !ok || n == 0 {
		return v
	}
	a, err1 := Num(l)
	b, err2 := Num(r)
	if err1 != nil || err2 != nil {
		return v
	}
	if math.Abs(float64(n)) < max(math.Abs(a), math.Abs(b))*0x1p-48 {
		return Number(0)
	}
	return v
}

func scalarOp(op string, l, r Value) Value {
	switch op {
	case "&":
		a, err := Str(l)
		if err != nil {
			return err
		}
		b, err := Str(r)
		if err != nil {
			return err
		}
		return String(a + b)
	case "=", "<>", "<", "<=", ">", ">=":
		c, err := Compare(l, r)
		if err != nil {
			return err
		}
		switch op {
		case "=":
			return Bool(c == 0)
		case "<>":
			return Bool(c != 0)
		case "<":
			return Bool(c < 0)
		case "<=":
			return Bool(c <= 0)
		case ">":
			return Bool(c > 0)
		}
		return Bool(c >= 0)
	}
	if e, ok := l.(Error); ok {
		return e
	}
	if e, ok := r.(Error); ok {
		return e
	}
	a, err := Num(l)
	if err != nil {
		return err
	}
	b, err := Num(r)
	if err != nil {
		return err
	}
	var v float64
	switch op {
	case "+":
		v = a + b
	case "-":
		v = a - b
	case "*":
		v = a * b
	case "/":
		if b == 0 {
			return ErrDiv0
		}
		v = a / b
	case "^":
		if a == 0 && b == 0 {
			return ErrNum
		}
		if a == 0 && b < 0 {
			return ErrDiv0
		}
		v = math.Pow(a, b)
	default:
		return ErrValue
	}
	return NumberOrErr(v)
}

// NumberOrErr returns v as a Number, or #NUM! for NaN and infinities.
func NumberOrErr(v float64) Value {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ErrNum
	}
	return Number(v)
}

// FormulaName returns the canonical upper-case function name used in a
// formula (prefixes such as _xlfn. removed).
func FormulaName(s string) string { return strings.ToUpper(stripPrefixes(s)) }
