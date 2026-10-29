package edit

import (
	"slices"
	"testing"

	"officelite/excel/model"
)

// dump is sheet 0's cells A1:D6 as display text, to compare states.
func (b *Book) dump() []string {
	var out []string
	for r := range 6 {
		for c := range 4 {
			cell := b.WB.Sheets[0].Get(r, c)
			out = append(out, cell.Text+"|"+cell.Formula)
		}
	}
	return out
}

func TestUndoLabels(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "B5", "Total")
	b.enter(t, 0, "A1", "a very long piece of text indeed")
	b.Format(0, rng("A1", "B2"), func(st *model.Style, _, _ int) { st.Font.Bold = true })
	func() {
		defer b.Op("Italic")()
		b.Format(0, rng("A1", "A1"), func(st *model.Style, _, _ int) { st.Font.Italic = true })
	}()
	b.InsertRows(0, 2, 1)
	b.FillDown(0, rng("A1", "A3"))
	// A group takes its first change's name, or the caller's.
	b.BeginGroup()
	b.Clear(0, rng("A1", "A1"))
	b.Clear(0, rng("B1", "B1"))
	b.EndGroup()
	b.ResizeCols(0, []int{1}, 20)

	want := []string{"Column Width", "Clear", "Fill Down", "Insert Rows", "Italic", "Format Cells",
		"Typing 'a very long piece of…' in A1", "Typing 'Total' in B5"}
	if got := b.UndoLabels(); !slices.Equal(got, want) {
		t.Errorf("UndoLabels = %q\nwant %q", got, want)
	}
	b.UndoN(3)
	if got, want := b.RedoLabels(), []string{"Fill Down", "Clear", "Column Width"}; !slices.Equal(got, want) {
		t.Errorf("RedoLabels = %q, want %q", got, want)
	}
	if b.label != "" {
		t.Errorf("label left set: %q", b.label)
	}
}

func TestUndoN(t *testing.T) {
	build := func() *Book {
		b := newBook()
		b.enter(t, 0, "A1", "1")
		b.enter(t, 0, "A2", "=A1*2")
		b.enter(t, 0, "B1", "x")
		b.BeginGroup() // a group is one step
		b.enter(t, 0, "C1", "c")
		b.enter(t, 0, "C2", "d")
		b.EndGroup()
		b.InsertRows(0, 0, 1)
		b.enter(t, 0, "A1", "top")
		return b
	}
	one, many := build(), build()
	before := one.dump()
	for range 3 {
		one.Undo()
	}
	where, _, ok := many.UndoN(3)
	if !ok || !slices.Equal(one.dump(), many.dump()) {
		t.Fatalf("UndoN(3) ok=%v\n got %q\nwant %q", ok, many.dump(), one.dump())
	}
	if where != (Where{0, rng("C2", "C2")}) { // the group's last change
		t.Errorf("UndoN where = %+v", where)
	}
	if len(many.UndoLabels()) != 3 || len(many.RedoLabels()) != 3 {
		t.Errorf("stacks %d/%d after UndoN(3)", len(many.UndoLabels()), len(many.RedoLabels()))
	}
	for range 2 {
		one.Redo()
	}
	many.RedoN(2)
	if !slices.Equal(one.dump(), many.dump()) {
		t.Fatalf("RedoN(2)\n got %q\nwant %q", many.dump(), one.dump())
	}
	many.RedoN(10) // past the end: what there is
	if !slices.Equal(before, many.dump()) || many.CanRedo() {
		t.Errorf("RedoN(10): %q", many.dump())
	}
	many.UndoN(100)
	if many.CanUndo() || many.cell(0, "A1").Text != "" {
		t.Errorf("UndoN(100) left %q", many.UndoLabels())
	}
	if _, _, ok := many.UndoN(1); ok {
		t.Error("UndoN on an empty stack reported ok")
	}
}
