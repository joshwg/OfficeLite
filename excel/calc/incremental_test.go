package calc

import (
	"fmt"
	"math/rand"
	"testing"

	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

// edit sets cells on sheet 0 and updates c, as the editor does.
func edit(c *Calculator, wb *model.Workbook, cells map[string]model.Cell) Result {
	var areas []Area
	for ref, cell := range cells {
		row, col := cellPos(ref)
		wb.Sheets[0].Set(row, col, cell)
		areas = append(areas, CellArea(0, row, col))
	}
	return c.Update(areas, now)
}

func TestUpdateChain(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(1),
		"B1": fx("A1*10"),
		"C1": fx("B1+SUM(A1:A3)"),
		"D1": fx("Data!A1+1"),
		"E1": fx("Total*2"),
		"F1": fx("A2"), // not affected by A1
	})
	wb.Names = []model.DefinedName{{Name: "Total", Sheet: -1, Formula: "Sheet1!$C$1"}}
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	if got := get(wb, "E1").Text; got != "22" {
		t.Fatalf("E1 = %q, want 22", got)
	}
	res := edit(c, wb, map[string]model.Cell{"A1": num(2)})
	for ref, want := range map[string]string{"B1": "20", "C1": "22", "E1": "44", "F1": "0"} {
		if got := get(wb, ref).Text; got != want {
			t.Errorf("%s = %q, want %q", ref, got, want)
		}
	}
	if res.Calculated != 3 {
		t.Errorf("calculated %d formulas, want 3 (B1, C1, E1)", res.Calculated)
	}
	// A range read: A3 is inside SUM(A1:A3).
	edit(c, wb, map[string]model.Cell{"A3": num(5)})
	if got := get(wb, "C1").Text; got != "27" {
		t.Errorf("C1 = %q, want 27", got)
	}
	// Another sheet.
	wb.Sheets[1].Set(0, 0, num(41))
	c.Update([]Area{CellArea(1, 0, 0)}, now)
	if got := get(wb, "D1").Text; got != "42" {
		t.Errorf("D1 = %q, want 42", got)
	}
	// A new formula, then one replaced by a constant.
	edit(c, wb, map[string]model.Cell{"G1": fx("B1+1")})
	edit(c, wb, map[string]model.Cell{"A1": num(3)})
	if got := get(wb, "G1").Text; got != "31" {
		t.Errorf("G1 = %q, want 31", got)
	}
	edit(c, wb, map[string]model.Cell{"B1": num(0)})
	if got := get(wb, "G1").Text; got != "1" {
		t.Errorf("G1 = %q, want 1 after B1 became a constant", got)
	}
}

func TestUpdateCircular(t *testing.T) {
	wb := book(map[string]model.Cell{"A1": num(1), "B1": fx("A1+1")})
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	res := edit(c, wb, map[string]model.Cell{"A1": fx("B1+1")})
	if len(res.Circular) == 0 {
		t.Error("circular reference not reported")
	}
}

func dyn(f string) model.Cell { return model.Cell{Formula: f, Dynamic: true} }

func TestUpdateSpill(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(3),
		"B1": dyn("SEQUENCE(A1)"),
		"C1": fx("B3*10"),    // reads a spilled cell
		"D1": fx("SUM(B1#)"), // reads the spill
	})
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	if got := get(wb, "C1").Text + "," + get(wb, "D1").Text; got != "30,6" {
		t.Fatalf("C1,D1 = %s, want 30,6", got)
	}
	// The spill grows: B4 and B5 appear, C1 and D1 follow.
	edit(c, wb, map[string]model.Cell{"A1": num(5)})
	if got := get(wb, "B5").Text + "," + get(wb, "D1").Text; got != "5,15" {
		t.Errorf("B5,D1 = %s, want 5,15", got)
	}
	// It shrinks: B3 empties.
	edit(c, wb, map[string]model.Cell{"A1": num(2)})
	if got := get(wb, "B3").Kind; got != model.KindEmpty {
		t.Errorf("B3 kind %d after the spill shrank", got)
	}
	if got := get(wb, "C1").Text; got != "0" {
		t.Errorf("C1 = %q, want 0", got)
	}
	// Typing inside the spill blocks it; the typed value stays.
	edit(c, wb, map[string]model.Cell{"B2": num(7)})
	if got := get(wb, "B1").Text; got != "#SPILL!" {
		t.Errorf("B1 = %q, want #SPILL!", got)
	}
	if got := get(wb, "B2").Text; got != "x" {
		t.Errorf("B2 = %q, want the typed value", got)
	}
	// Clearing it unblocks the spill.
	edit(c, wb, map[string]model.Cell{"B2": {}})
	if got := get(wb, "B1").Text + "," + get(wb, "B2").Text; got != "1,2" {
		t.Errorf("B1,B2 = %s, want 1,2", got)
	}
}

// TestUpdateMatchesFull makes random edits and checks every cell against a
// full recalculation of the same workbook.
func TestUpdateMatchesFull(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	const rows, cols = 12, 6
	cellAt := func(r, c int) string { return model.CellName(r, c) }
	// Formulas only read rows above their own, so there are no cycles,
	// whose values depend on the calculation order.
	randCell := func(r int) model.Cell {
		k := rnd.Intn(6)
		if r < 3 {
			k = rnd.Intn(3)
		}
		above := func() string { return cellAt(rnd.Intn(r), rnd.Intn(cols)) }
		switch k {
		case 0:
			return model.Cell{}
		case 1, 2:
			v := float64(rnd.Intn(20))
			return model.Cell{Kind: model.KindNumber, Num: v, Text: fmt.Sprint(v)}
		case 3:
			return fx(above() + "+" + above())
		case 4:
			r0, c := rnd.Intn(r-2), rnd.Intn(cols-1)
			return fx(fmt.Sprintf("SUM(%s:%s)", cellAt(r0, c), cellAt(r0+2, c+1)))
		default:
			return dyn(fmt.Sprintf("SEQUENCE(%d,1,%s)", 1+rnd.Intn(3), above()))
		}
	}
	wb := book(nil)
	ref := book(nil)
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	for step := 0; step < 400; step++ {
		r, col := rnd.Intn(rows), rnd.Intn(cols)
		cell := randCell(r)
		// Writing over a spilled value replaces it, as the editor does.
		wb.Sheets[0].Set(r, col, cell)
		c.Update([]Area{CellArea(0, r, col)}, now)
		// The reference workbook holds the same inputs, fully recalculated.
		for rr := 0; rr < rows+3; rr++ {
			for cc := 0; cc < cols; cc++ {
				src := *wb.Sheets[0].Get(rr, cc)
				if src.Formula == "" && spilled(wb.Sheets[0], rr, cc) {
					src = model.Cell{}
				}
				ref.Sheets[0].Set(rr, cc, src)
			}
		}
		Recalculate(ref, functions.Default(), Full, now)
		// Which of two overlapping spills wins depends on the order they
		// were entered in; compare only states without a conflict.
		if hasSpillError(wb) || hasSpillError(ref) {
			continue
		}
		for rr := 0; rr < rows+3; rr++ {
			for cc := 0; cc < cols; cc++ {
				got, want := wb.Sheets[0].Get(rr, cc), ref.Sheets[0].Get(rr, cc)
				if got.Text != want.Text {
					t.Fatalf("step %d (set %s to %+v): %s = %q, full recalculation gives %q",
						step, cellAt(r, col), cell, cellAt(rr, cc), got.Text, want.Text)
				}
			}
		}
	}
}

// spilled reports whether a cell holds a value spilled from another cell.
func spilled(sh *model.Sheet, r, c int) bool {
	for anchor, rng := range sh.Spills {
		if anchor != [2]int{r, c} && rng.Contains(r, c) {
			return true
		}
	}
	return false
}

func hasSpillError(wb *model.Workbook) bool {
	sh := wb.Sheets[0]
	for r := 0; r < sh.UsedRows(); r++ {
		for c := 0; c < sh.RowLen(r); c++ {
			if sh.Get(r, c).Text == "#SPILL!" {
				return true
			}
		}
	}
	return false
}

// OFFSET and INDIRECT read cells that don't appear in their formulas;
// being volatile, they and their dependents are recalculated on every
// update.
func TestUpdateOffsetIndirect(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(1), "A2": num(2), "A3": num(3),
		"B1": fx("OFFSET(A1,1,0)"),
		"C1": fx(`INDIRECT("A"&3)`),
		"D1": fx("B1+C1"),
		"E1": fx("SUM(OFFSET(A1,0,0,3,1))"),
		"F1": fx(`INDIRECT("Data!A1")*2`),
		"G1": fx(`CELL("address",INDIRECT("Data!B2"))`),
		"H1": fx(`INDIRECT("Pick")`),
		"I1": fx("SUM(A1:OFFSET(A1,2,0))"),
		"J5": fx(`INDIRECT("R[-3]C[-9]",FALSE)`),
		"K1": dyn("OFFSET(A1,0,0,3,1)"),
		"L1": fx("SUM(K1#)"),
	})
	data := wb.Sheets[1]
	data.Set(0, 0, num(10))
	data.Set(0, 1, fx(`INDIRECT("Pick")`))
	data.Set(0, 2, fx(`INDIRECT("Sheet1!Pick")`))
	wb.Names = []model.DefinedName{
		{Name: "Pick", Sheet: -1, Formula: "Sheet1!$A$1"},
		{Name: "Pick", Sheet: 0, Formula: "Sheet1!$A$3"}, // Sheet1's own
	}
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	check := func(when string, want map[string]string) {
		t.Helper()
		for ref, w := range want {
			if got := get(wb, ref).Text; got != w {
				t.Errorf("%s: %s = %q, want %q", when, ref, got, w)
			}
		}
	}
	check("full", map[string]string{"B1": "2", "C1": "3", "D1": "5", "E1": "6", "F1": "20",
		"G1": "Data!$B$2", "H1": "3", "I1": "6", "J5": "2", "K1": "1", "K3": "3", "L1": "6"})
	if b, c := data.Get(0, 1).Text, data.Get(0, 2).Text; b != "1" || c != "3" {
		t.Errorf("Data!B1, C1 = %q, %q; want the global and Sheet1's Pick (1, 3)", b, c)
	}

	edit(c, wb, map[string]model.Cell{"A2": num(20)})
	check("A2 edited", map[string]string{"B1": "20", "D1": "23", "E1": "24", "I1": "24", "J5": "20", "K2": "20", "L1": "24"})
	edit(c, wb, map[string]model.Cell{"A3": num(30)})
	check("A3 edited", map[string]string{"C1": "30", "D1": "50", "E1": "51", "H1": "30", "K3": "30", "L1": "51"})
	data.Set(0, 0, num(7))
	c.Update([]Area{CellArea(1, 0, 0)}, now)
	check("Data!A1 edited", map[string]string{"F1": "14"})

	// The text INDIRECT reads changes: it now points elsewhere.
	edit(c, wb, map[string]model.Cell{"C1": fx(`INDIRECT("A"&M1)`), "M1": num(1)})
	check("C1 replaced", map[string]string{"C1": "1", "D1": "21"})
	edit(c, wb, map[string]model.Cell{"M1": num(2)})
	check("M1 edited", map[string]string{"C1": "20", "D1": "40"})

	// Full recalculation of a fresh workbook agrees.
	Recalculate(wb, functions.Default(), Full, now)
	check("recalculated", map[string]string{"D1": "40", "E1": "51", "L1": "51"})
}

// Array-shaping results spill and follow their inputs; TRIMRANGE(A:A)
// grows with the column, and GROUPBY regroups.
func TestUpdateShaping(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(1), "A2": num(2), "A3": num(3),
		"B1": dyn("VSTACK(A1:A2,10)"),
		"C1": dyn("TRIMRANGE(A:A)"),
		"D1": fx("ROWS(TRIMRANGE(A:A))"),
		"E1": dyn("TAKE(A1:A3,-2)"),
		"F1": dyn("GROUPBY(A1:A3>1,A1:A3,SUM,0,0)"),
		"H1": fx("SUM(E1#)+SUM(DROP(B1#,1))"),
	})
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	check := func(when string, want map[string]string) {
		t.Helper()
		for ref, w := range want {
			if got := get(wb, ref).Text; got != w {
				t.Errorf("%s: %s = %q, want %q", when, ref, got, w)
			}
		}
	}
	check("full", map[string]string{"B1": "1", "B2": "2", "B3": "10", "C3": "3", "D1": "3", "E1": "2", "E2": "3",
		"F1": "FALSE", "G1": "1", "F2": "TRUE", "G2": "5", "H1": "17"})
	edit(c, wb, map[string]model.Cell{"A4": num(4)})
	check("A4 added", map[string]string{"C4": "4", "D1": "4"})
	edit(c, wb, map[string]model.Cell{"A2": num(20)})
	check("A2 edited", map[string]string{"B2": "20", "C2": "20", "E1": "20", "G2": "23", "H1": "53"})
}

// A database function depends on its criteria range, including computed
// criteria that read the database through relative references.
func TestUpdateDatabase(t *testing.T) {
	text := func(s string) model.Cell { return model.Cell{Kind: model.KindText, Text: s} }
	wb := book(map[string]model.Cell{
		"A1": text("Tree"), "B1": text("Profit"),
		"A2": text("Apple"), "B2": num(105),
		"A3": text("Pear"), "B3": num(96),
		"A4": text("Apple"), "B4": num(75),
		"D1": text("Tree"), "D2": text("Apple"),
		"E1": text("Big"), "E2": fx("B2>80"),
		"G1": fx(`DSUM(A1:B4,"Profit",D1:D2)`),
		"G2": fx(`DCOUNT(A1:B4,"Profit",E1:E2)`),
	})
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	check := func(when string, want map[string]string) {
		t.Helper()
		for ref, w := range want {
			if got := get(wb, ref).Text; got != w {
				t.Errorf("%s: %s = %q, want %q", when, ref, got, w)
			}
		}
	}
	check("full", map[string]string{"G1": "180", "G2": "2"})
	edit(c, wb, map[string]model.Cell{"D2": text("Pear")})
	check("criteria edited", map[string]string{"G1": "96"})
	edit(c, wb, map[string]model.Cell{"B3": num(50)})
	check("database edited", map[string]string{"G1": "50", "G2": "1"})
	edit(c, wb, map[string]model.Cell{"E2": fx("B2>60")})
	check("computed criteria edited", map[string]string{"G2": "2"})
}
