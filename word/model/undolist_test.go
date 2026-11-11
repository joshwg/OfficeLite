// Tests for the Undo list: step labels, multi-step undo/redo and the
// history cap.

package model

import (
	"slices"
	"testing"

	"officelite/word"
)

// typeText types s one character at a time, as a user would.
func typeText(e *Editor, s string) {
	for _, c := range s {
		e.InsertText(string(c))
	}
}

// TestUndoLabels checks the names the Undo and Redo lists show: typing
// steps with their text (truncated with …), command names, and the
// outermost Op name winning.
func TestUndoLabels(t *testing.T) {
	_, e := newDoc("one two three", "four")
	e.SetCaret(Pos{1, 4}, false)
	typeText(e, " hello") // one typing step
	e.Select(Pos{0, 0}, Pos{0, 3})
	e.ToggleBold()
	e.SetSize(14)
	func() {
		defer e.Op("Center")() // the caller's name wins
		e.SetAlign(AlignCenter)
	}()
	e.ToggleList(true)
	e.DeleteSelection()
	e.SetCaret(Pos{0, 0}, false)
	typeText(e, "a very long typed text indeed")
	e.InsertTable(2, 2)

	want := []string{"Insert Table", "Typing 'a very long typed te…'", "Delete", "Bullets", "Center",
		"Font Size", "Bold", "Typing 'hello'"}
	if got := e.UndoLabels(); !slices.Equal(got, want) {
		t.Errorf("UndoLabels = %q\nwant %q", got, want)
	}
	e.UndoN(2)
	if got, want := e.RedoLabels(), []string{"Typing 'a very long typed te…'", "Insert Table"}; !slices.Equal(got, want) {
		t.Errorf("RedoLabels = %q, want %q", got, want)
	}
	if e.label != "" {
		t.Errorf("label left set: %q", e.label)
	}
}

// TestUndoN checks that UndoN and RedoN match n single steps, stop at the
// ends of the stacks and report whether anything changed.
func TestUndoN(t *testing.T) {
	build := func() (*Document, *Editor) {
		d, e := newDoc("alpha beta", "gamma")
		e.SetCaret(Pos{0, 5}, false)
		typeText(e, "XY")
		e.Select(Pos{1, 0}, Pos{1, 5})
		e.ToggleItalic()
		e.Enter()
		typeText(e, "new")
		e.SetCaret(Pos{0, 0}, false)
		e.Delete()
		return d, e
	}
	d1, one := build()
	d2, many := build()
	start := texts(d1)
	for range 3 {
		one.Undo()
	}
	if !many.UndoN(3) {
		t.Fatal("UndoN(3) did nothing")
	}
	if texts(d1) != texts(d2) || one.Sel != many.Sel {
		t.Fatalf("UndoN(3): %q %v, want %q %v", texts(d2), many.Sel, texts(d1), one.Sel)
	}
	if len(many.UndoLabels()) != 2 || len(many.RedoLabels()) != 3 {
		t.Errorf("stacks %q / %q", many.UndoLabels(), many.RedoLabels())
	}
	for range 2 {
		one.Redo()
	}
	many.RedoN(2)
	if texts(d1) != texts(d2) || one.Sel != many.Sel {
		t.Fatalf("RedoN(2): %q, want %q", texts(d2), texts(d1))
	}
	many.RedoN(10)
	if texts(d2) != start || many.CanRedo() {
		t.Errorf("RedoN(10): %q, want %q", texts(d2), start)
	}
	many.UndoN(100)
	if texts(d2) != "alpha beta|gamma" || many.CanUndo() {
		t.Errorf("UndoN(100): %q", texts(d2))
	}
	if many.UndoN(1) {
		t.Error("UndoN on an empty stack reported a change")
	}
}

// TestUndoHistoryCap checks that the undo list keeps only the newest
// word.UndoHistory steps, and that undoing all of them works and yields
// the document as it was after the dropped steps.
func TestUndoHistoryCap(t *testing.T) {
	d, e := newDoc("start")
	var after50 string
	for i := range 150 {
		e.SetCaret(Pos{len(d.Body) - 1, 0}, false)
		e.Enter()
		if i == 49 {
			after50 = texts(d)
		}
	}
	if n := len(e.UndoLabels()); n != word.UndoHistory {
		t.Fatalf("%d undo labels after 150 edits, want %d", n, word.UndoHistory)
	}
	for e.CanUndo() {
		if !e.Undo() {
			t.Fatal("Undo failed")
		}
	}
	if got := texts(d); got != after50 {
		t.Errorf("after undoing everything: %q, want %q (the state after the 50 dropped steps)", got, after50)
	}
	if n := len(e.RedoLabels()); n != word.UndoHistory {
		t.Errorf("%d redo labels, want %d", n, word.UndoHistory)
	}
}
