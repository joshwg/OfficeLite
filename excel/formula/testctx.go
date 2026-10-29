package formula

import (
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// TestContext is an in-memory workbook for tests: a few sheets of values
// and formulas, evaluated on demand.
type TestContext struct {
	Funcs   *Registry
	Sheets  []string
	NowTime time.Time
	Use1904 bool
	Names   map[string]string // defined names (upper-case) -> formula

	cells    map[[3]int]Value
	formulas map[[3]int]string
	cache    map[[3]int]Value
	busy     map[[3]int]bool
	rnd      *rand.Rand
}

// NewTestContext returns a context with sheets "Sheet1" and "Sheet2" and a
// fixed clock (2026-10-15 13:30).
func NewTestContext(funcs *Registry) *TestContext {
	return &TestContext{
		Funcs:    funcs,
		Sheets:   []string{"Sheet1", "Sheet2"},
		NowTime:  time.Date(2026, 10, 15, 13, 30, 0, 0, time.UTC),
		Names:    map[string]string{},
		cells:    map[[3]int]Value{},
		formulas: map[[3]int]string{},
		cache:    map[[3]int]Value{},
		busy:     map[[3]int]bool{},
		rnd:      rand.New(rand.NewSource(1)),
	}
}

func (t *TestContext) key(ref string) [3]int {
	sheet := 0
	if s, cell, ok := strings.Cut(ref, "!"); ok {
		sheet, ref = t.SheetIndex(strings.Trim(s, "'")), cell
	}
	n, err := Parse(ref)
	r, ok := n.(refNode)
	if err != nil || !ok {
		panic("TestContext: bad cell " + ref)
	}
	return [3]int{sheet, r.r1, r.c1}
}

// Set stores a value in a cell given as "A1" or "Sheet2!B3". v may be a
// Value, float64, int, string, bool or nil; a string starting with '=' is a
// formula.
func (t *TestContext) Set(ref string, v any) {
	k := t.key(ref)
	delete(t.formulas, k)
	t.cache = map[[3]int]Value{}
	switch x := v.(type) {
	case nil:
		delete(t.cells, k)
	case Value:
		t.cells[k] = x
	case float64:
		t.cells[k] = Number(x)
	case int:
		t.cells[k] = Number(x)
	case bool:
		t.cells[k] = Bool(x)
	case string:
		if strings.HasPrefix(x, "=") {
			t.formulas[k] = x
		} else {
			t.cells[k] = String(x)
		}
	default:
		panic("TestContext.Set: unsupported value")
	}
}

// SetRange fills cells starting at ref row by row: SetRange("A1", [][]any{{1, 2}, {3, 4}}).
func (t *TestContext) SetRange(ref string, rows [][]any) {
	k := t.key(ref)
	for i, row := range rows {
		for j, v := range row {
			t.Set(t.Sheets[k[0]]+"!"+cellName(k[1]+i, k[2]+j), v)
		}
	}
}

func cellName(row, col int) string {
	var b []byte
	for c := col + 1; c > 0; c = (c - 1) / 26 {
		b = append([]byte{byte('A' + (c-1)%26)}, b...)
	}
	return string(b) + strconv.Itoa(row+1)
}

// Eval evaluates src as if it were entered in Sheet1!A1000.
func (t *TestContext) Eval(src string) Value { return t.EvalAt("A1000", src) }

// EvalAt evaluates src as if entered in the given cell.
func (t *TestContext) EvalAt(ref, src string) Value {
	k := t.key(ref)
	return EvalString(src, t, t.Funcs, k[0], k[1], k[2])
}

func (t *TestContext) CellValue(sheet, row, col int) Value {
	k := [3]int{sheet, row, col}
	if f, ok := t.formulas[k]; ok {
		if v, ok := t.cache[k]; ok {
			return v
		}
		if t.busy[k] {
			return Number(0) // circular reference
		}
		t.busy[k] = true
		v := EvalString(f, t, t.Funcs, sheet, row, col)
		delete(t.busy, k)
		if a, ok := v.(*Array); ok {
			v = a.V[0]
		}
		t.cache[k] = v
		return v
	}
	if v, ok := t.cells[k]; ok {
		return v
	}
	return Blank{}
}

func (t *TestContext) CellFormula(sheet, row, col int) string {
	return strings.TrimPrefix(t.formulas[[3]int{sheet, row, col}], "=")
}

func (t *TestContext) SheetIndex(name string) int {
	for i, s := range t.Sheets {
		if strings.EqualFold(s, name) {
			return i
		}
	}
	return -1
}

func (t *TestContext) SheetName(i int) string { return t.Sheets[i] }
func (t *TestContext) SheetCount() int        { return len(t.Sheets) }

func (t *TestContext) UsedSize(sheet int) (rows, cols int) {
	for k := range t.cells {
		if k[0] == sheet {
			rows, cols = max(rows, k[1]+1), max(cols, k[2]+1)
		}
	}
	for k := range t.formulas {
		if k[0] == sheet {
			rows, cols = max(rows, k[1]+1), max(cols, k[2]+1)
		}
	}
	return max(rows, 1), max(cols, 1)
}

func (t *TestContext) Name(name string, sheet int) (string, bool) {
	f, ok := t.Names[strings.ToUpper(name)]
	return f, ok
}

func (t *TestContext) Spill(sheet, row, col int) (Ref, bool) { return Ref{}, false }
func (t *TestContext) Now() time.Time                        { return t.NowTime }
func (t *TestContext) Date1904() bool                        { return t.Use1904 }
func (t *TestContext) Rand() float64                         { return t.rnd.Float64() }
