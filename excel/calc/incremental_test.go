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
