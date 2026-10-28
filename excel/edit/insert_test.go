package edit

import (
	"testing"

	"officelite/excel/model"
)

func TestInsertDeleteCellsShift(t *testing.T) {
	b := newBook()
	for ref, v := range map[string]string{"A1": "1", "A2": "2", "A3": "3", "B1": "10", "B2": "20", "B3": "30"} {
		b.enter(t, 0, ref, v)
	}
	b.enter(t, 0, "C1", "=SUM(A1:A3)")
	b.enter(t, 0, "C2", "=A3")
	b.enter(t, 0, "C3", "=SUM(A1:B3)")
	b.enter(t, 1, "A1", "=Sheet1!A2")
	r1, c1 := pos("A2")
	b.WB.Sheets[0].SetNote(r1, c1, &model.Note{Text: "two"})

	// Insert A2 shifting down: A2:A3 move to A3:A4.
	if _, err := b.InsertCells(0, rng("A2", "A2"), ShiftDown); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "insert down", map[string]string{"A1": "1", "A2": "", "A3": "2", "A4": "3", "B2": "20", "C1": "6", "C2": "3"})
	for ref, want := range map[string]string{"C1": "SUM(A1:A4)", "C2": "A4", "C3": "SUM(A1:B3)"} {
		if got := b.cell(0, ref).Formula; got != want {
			t.Errorf("insert down: %s = %q, want %q", ref, got, want)
		}
	}
	if got := b.cell(1, "A1").Formula; got != "Sheet1!A3" {
		t.Errorf("other sheet: %q", got)
	}
	if _, ok := b.WB.Sheets[0].NoteAt(2, 0); !ok {
		t.Error("note did not move with its cell")
	}
	b.Undo()
	b.expect(t, "undo", map[string]string{"A2": "2", "A3": "3", "A4": ""})
	if got := b.cell(0, "C2").Formula; got != "A3" {
		t.Errorf("undo: C2 = %q", got)
	}
	if _, ok := b.WB.Sheets[0].NoteAt(1, 0); !ok {
		t.Error("undo: note not back")
	}

	// Delete A1 shifting up: references to it become #REF!.
	b.enter(t, 0, "D1", "=A1")
	if _, err := b.DeleteCells(0, rng("A1", "A1"), ShiftUp); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "delete up", map[string]string{"A1": "2", "A2": "3", "D1": "#REF!", "C1": "5"})
	if got := b.cell(0, "C2").Formula; got != "A2" {
		t.Errorf("delete up: C2 = %q", got)
	}
	b.Undo()

	// Insert B1:B2 shifting right.
	if _, err := b.InsertCells(0, rng("B1", "B2"), ShiftRight); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "insert right", map[string]string{"B1": "", "C1": "10", "B3": "30", "C3": "36", "D1": "6"})
	b.Undo()
	b.expect(t, "undo right", map[string]string{"B1": "10", "C1": "6"})
}

func TestShiftCellsMerged(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	sh.Merges = []model.Range{rng("A3", "B3")}
	if _, err := b.InsertCells(0, rng("A1", "A1"), ShiftDown); err != ErrMergedShift {
		t.Errorf("err = %v", err)
	}
	if _, err := b.InsertCells(0, rng("A1", "B1"), ShiftDown); err != nil {
		t.Fatal(err)
	}
	if sh.Merges[0] != rng("A4", "B4") {
		t.Errorf("merge = %+v", sh.Merges[0])
	}
}

func TestSort(t *testing.T) {
	b := newBook()
	rows := [][2]string{{"Name", "Qty"}, {"pear", "3"}, {"Apple", "10"}, {"fig", ""}, {"banana", "1"}}
	for i, r := range rows {
		b.enter(t, 0, model.CellName(i, 0), r[0])
		b.enter(t, 0, model.CellName(i, 1), r[1])
		if i > 0 {
			b.enter(t, 0, model.CellName(i, 2), "=B"+itoa(i+1)+"*2")
		}
	}
	region := CurrentRegion(b.WB.Sheets[0], 2, 1)
	if region != rng("A1", "C5") {
		t.Fatalf("region = %+v", region)
	}
	if !b.GuessHeader(0, region) {
		t.Error("header not detected")
	}
	if _, err := b.Sort(0, SortSpec{Range: region, Header: true, Keys: []SortKey{{Col: 1}}}); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "by qty", map[string]string{"A1": "Name", "A2": "banana", "A3": "pear", "A4": "Apple", "A5": "fig", "C2": "2", "C4": "20"})
	if got := b.cell(0, "C2").Formula; got != "B2*2" {
		t.Errorf("C2 = %q", got)
	}
	b.Undo()
	b.expect(t, "undo", map[string]string{"A2": "pear", "A3": "Apple"})
	if _, err := b.Sort(0, SortSpec{Range: region, Header: true, Keys: []SortKey{{Col: 0, Descending: true}}}); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "by name desc", map[string]string{"A2": "pear", "A3": "fig", "A4": "banana", "A5": "Apple"})
}

func itoa(n int) string { return model.CellName(n-1, 0)[1:] }

func TestFill(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "5")
	b.enter(t, 0, "B1", "=A1*2")
	b.FillDown(0, rng("A1", "B3"))
	b.expect(t, "fill down", map[string]string{"A3": "5", "B3": "10"})
	if got := b.cell(0, "B3").Formula; got != "A3*2" {
		t.Errorf("B3 = %q", got)
	}
	b.Undo()
	b.expect(t, "undo", map[string]string{"A3": "", "B3": ""})
	b.FillRight(0, rng("C1", "C1"))
	if got := b.cell(0, "C1").Formula; got != "B1*2" {
		t.Errorf("fill right C1 = %q", got)
	}
}

func TestAutoFill(t *testing.T) {
	b := newBook()
	for ref, v := range map[string]string{
		"A1": "1", "A2": "3", // trend
		"B1": "7",            // copy
		"C1": "Item9",        // counter
		"D1": "Mon",          // weekday
		"E1": "1/30/2026",    // date
		"F1": "=A1+1",        // formula
		"G1": "x", "G2": "y", // pattern
		"H1": "January", // month
		"I1": "Q01", "I2": "Q03",
	} {
		b.enter(t, 0, ref, v)
	}
	fill := func(src, to model.Range) {
		t.Helper()
		if _, err := b.AutoFill(0, src, to); err != nil {
			t.Fatal(err)
		}
	}
	fill(rng("A1", "A2"), rng("A1", "A5"))
	fill(rng("G1", "G2"), rng("G1", "G5"))
	fill(rng("I1", "I2"), rng("I1", "I4"))
	fill(rng("B1", "F1"), rng("B1", "F4"))
	fill(rng("H1", "H1"), rng("H1", "H14"))
	b.expect(t, "autofill", map[string]string{
		"A3": "5", "A5": "9", "B4": "7", "C2": "Item10", "C4": "Item12",
		"D2": "Tue", "D4": "Thu", "E2": "1/31/2026", "E3": "2/1/2026",
		"G3": "x", "G4": "y", "H2": "February", "H13": "January", "I3": "Q05", "I4": "Q07",
	})
	if got := b.cell(0, "F3").Formula; got != "A3+1" {
		t.Errorf("F3 = %q", got)
	}
	// Upward: the trend continues backwards.
	b.enter(t, 0, "K5", "10")
	b.enter(t, 0, "K6", "20")
	fill(rng("K5", "K6"), rng("K3", "K6"))
	b.expect(t, "up", map[string]string{"K4": "0", "K3": "-10"})
	// Dragging back clears.
	fill(rng("K3", "K6"), rng("K3", "K4"))
	b.expect(t, "shrink", map[string]string{"K5": "", "K4": "0"})
	b.Undo()
	b.expect(t, "undo shrink", map[string]string{"K5": "10"})
}

func TestFindReplace(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "apple pie")
	b.enter(t, 0, "B2", "Apple")
	b.enter(t, 0, "C3", "=LEN(\"apple\")")
	b.enter(t, 1, "A1", "apple")
	o := FindOptions{What: "apple"}
	all, _ := b.FindAll(0, o)
	if len(all) != 2 {
		t.Errorf("values on sheet: %v", all)
	}
	o.Formulas = true
	all, _ = b.FindAll(0, o)
	if len(all) != 3 {
		t.Errorf("formulas on sheet: %v", all)
	}
	o.Workbook = true
	all, _ = b.FindAll(0, o)
	if len(all) != 4 {
		t.Errorf("workbook: %v", all)
	}
	o.MatchCase, o.WholeCell = true, true
	all, _ = b.FindAll(0, o)
	if len(all) != 1 || all[0] != (Found{1, 0, 0}) {
		t.Errorf("case+whole: %v", all)
	}
	f, err := b.FindNext(Found{0, 0, 0}, FindOptions{What: "a*e"}, false)
	if err != nil || f != (Found{0, 1, 1}) {
		t.Errorf("next = %v %v", f, err)
	}
	n, _, err := b.ReplaceAll(0, FindOptions{What: "apple", Replacement: "pear"})
	if err != nil || n != 3 {
		t.Errorf("replace all = %d %v", n, err)
	}
	b.expect(t, "replaced", map[string]string{"A1": "pear pie", "B2": "pear", "C3": "4"})
	b.Undo()
	b.expect(t, "undo", map[string]string{"A1": "apple pie", "B2": "Apple", "C3": "5"})
	if _, err := b.FindNext(Found{}, FindOptions{What: "zzz"}, false); err != ErrNotFound {
		t.Errorf("not found: %v", err)
	}
}

func TestResolveRef(t *testing.T) {
	b := newBook()
	b.WB.Names = []model.DefinedName{{Name: "Tax", Sheet: -1, Formula: "Data!$B$2:$B$4"}}
	for _, tc := range []struct {
		in    string
		sheet int
		r     model.Range
	}{
		{"B12", 0, rng("B12", "B12")},
		{"data!a1:c5", 1, rng("A1", "C5")},
		{"'Data'!$C$3", 1, rng("C3", "C3")},
		{"C:D", 0, model.Range{C1: 2, C2: 3, R2: model.MaxRows - 1}},
		{"3:4", 0, model.Range{R1: 2, R2: 3, C2: model.MaxCols - 1}},
		{"tax", 1, rng("B2", "B4")},
	} {
		s, r, err := b.ResolveRef(tc.in, 0)
		if err != nil || s != tc.sheet || r != tc.r {
			t.Errorf("%q = %d %+v %v", tc.in, s, r, err)
		}
	}
	for _, bad := range []string{"", "Nope!A1", "A", "foo"} {
		if _, _, err := b.ResolveRef(bad, 0); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestNotesAndLinksUndo(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	b.SetNote(0, 0, 1, &model.Note{Author: "Josh", Text: "hi"})
	if n, ok := sh.NoteAt(0, 1); !ok || n.Text != "hi" {
		t.Fatal("note not set")
	}
	b.Undo()
	if _, ok := sh.NoteAt(0, 1); ok {
		t.Error("undo kept note")
	}
	b.Redo()
	if _, ok := sh.NoteAt(0, 1); !ok {
		t.Error("redo lost note")
	}
	if _, err := b.SetHyperlink(0, 2, 2, "", model.Hyperlink{Target: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	c := sh.Get(2, 2)
	if c.Text != "https://example.com" || !b.WB.Styles[c.Style].Font.Underline {
		t.Errorf("link cell = %+v", c)
	}
	b.Undo()
	if _, ok := sh.LinkAt(2, 2); ok || sh.Get(2, 2).Text != "" {
		t.Error("undo kept link")
	}
	b.Redo()
	// Rows inserted above move notes and links.
	if _, err := b.InsertRows(0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := sh.LinkAt(4, 2); !ok {
		t.Error("link did not move")
	}
	if _, ok := sh.NoteAt(2, 1); !ok {
		t.Error("note did not move")
	}
	if !b.ClearNotes(0, model.Range{R2: 10, C2: 10}) || len(sh.Notes) != 0 {
		t.Error("clear notes")
	}
	b.Undo()
	if len(sh.Notes) != 1 {
		t.Error("undo clear notes")
	}
}
