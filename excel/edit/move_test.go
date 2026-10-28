package edit

import (
	"testing"

	"officelite/excel/model"
)

func TestMoveCells(t *testing.T) {
	b := newBook()
	for ref, v := range map[string]string{"A1": "1", "A2": "2", "B1": "=A1*10", "C1": "=SUM(A1:A2)", "D5": "old"} {
		b.enter(t, 0, ref, v)
	}
	b.enter(t, 1, "A1", "=Sheet1!A2")
	sh := b.WB.Sheets[0]
	r, c := pos("A2")
	sh.SetNote(r, c, &model.Note{Text: "two"})
	sh.Merges = []model.Range{rng("A1", "A1")} // trivial merge inside src

	src := rng("A1", "B2")
	// Move A1:B2 to D4:E5 (3 down, 3 right): D5 is overwritten.
	if !b.MoveOverwrites(0, src, 3, 3, false) {
		t.Error("MoveOverwrites missed D5")
	}
	if b.MoveOverwrites(0, src, 1, 0, false) {
		t.Error("moving onto itself counts as overwriting")
	}
	dest, _, err := b.MoveCells(0, src, 3, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if dest != rng("D4", "E5") {
		t.Errorf("dest = %+v", dest)
	}
	b.expect(t, "move", map[string]string{"A1": "", "A2": "", "B1": "", "D4": "1", "D5": "2", "E4": "10", "C1": "3"})
	for ref, want := range map[string]string{"E4": "D4*10", "C1": "SUM(D4:D5)"} {
		if got := b.cell(0, ref).Formula; got != want {
			t.Errorf("move: %s = %q, want %q", ref, got, want)
		}
	}
	if got := b.cell(1, "A1").Formula; got != "Sheet1!D5" {
		t.Errorf("other sheet: %q", got)
	}
	if _, ok := sh.NoteAt(4, 3); !ok {
		t.Error("note did not move")
	}
	if _, ok := sh.NoteAt(1, 0); ok {
		t.Error("note stayed behind")
	}
	if len(sh.Merges) != 1 || sh.Merges[0] != rng("D4", "D4") {
		t.Errorf("merges = %+v", sh.Merges)
	}

	b.Undo()
	b.expect(t, "undo", map[string]string{"A1": "1", "A2": "2", "B1": "10", "D5": "old", "D4": "", "C1": "3"})
	if got := b.cell(0, "C1").Formula; got != "SUM(A1:A2)" {
		t.Errorf("undo: C1 = %q", got)
	}
	if _, ok := sh.NoteAt(1, 0); !ok {
		t.Error("undo: note not back")
	}
	if len(sh.Merges) != 1 || sh.Merges[0] != rng("A1", "A1") {
		t.Errorf("undo: merges = %+v", sh.Merges)
	}
	b.Redo()
	b.expect(t, "redo", map[string]string{"A1": "", "D4": "1", "E4": "10"})
	b.Undo()

	// Copy: the source stays, relative references shift.
	if _, _, err := b.MoveCells(0, rng("B1", "B1"), 1, 1, true); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "copy", map[string]string{"B1": "10", "C2": "0"})
	if got := b.cell(0, "C2").Formula; got != "B2*10" {
		t.Errorf("copy: C2 = %q", got)
	}

	// Overlapping move one row down.
	b.Undo()
	if _, _, err := b.MoveCells(0, rng("A1", "A2"), 1, 0, false); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "overlap", map[string]string{"A1": "", "A2": "1", "A3": "2", "B1": "10", "C1": "3"})

	if _, _, err := b.MoveCells(0, rng("A1", "A2"), -1, 0, false); err == nil {
		t.Error("moving above row 1 succeeded")
	}
}

func TestGroupUndo(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "1")
	b.enter(t, 0, "C3", "3")
	b.BeginGroup()
	b.Clear(0, rng("A1", "A1"))
	b.Clear(0, rng("C3", "C3"))
	b.EndGroup()
	b.expect(t, "cleared", map[string]string{"A1": "", "C3": ""})
	where, _, _ := b.Undo()
	b.expect(t, "undo", map[string]string{"A1": "1", "C3": "3"})
	if where.Range != rng("C3", "C3") {
		t.Errorf("undo selects %+v", where.Range)
	}
	b.Redo()
	b.expect(t, "redo", map[string]string{"A1": "", "C3": ""})
	b.Undo()
	b.Undo() // C3's entry
	b.expect(t, "undo entry", map[string]string{"A1": "1", "C3": ""})
}
