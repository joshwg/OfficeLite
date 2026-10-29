package ui

import (
	"slices"
	"strconv"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"

	"officelite/common/gui"
)

// pickHistory opens hl's list from the ribbon and clicks entry n (from 1).
func (h *harness) pickHistory(hl *gui.HistoryList, n int) {
	h.t.Helper()
	p := h.locate(hl.Arrow(), ribbonArea, 4)
	h.click(p.X, p.Y)
	if !hl.IsOpen() {
		h.t.Fatal("the arrow didn't open the list")
	}
	for y := p.Y + 8; y < p.Y+200; y += 2 {
		q := f32.Pt(p.X+40, y)
		h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: q})
		h.frame()
		h.frame()
		if hl.Hovered() == n {
			if got, want := hl.Footer("Undo"), "Undo "+strconv.Itoa(n)+" Actions"; n > 1 && got != want {
				h.t.Errorf("footer %q, want %q", got, want)
			}
			h.click(q.X, q.Y)
			return
		}
	}
	h.t.Fatalf("entry %d not found", n)
}

func TestUndoHistoryList(t *testing.T) {
	h := newHarness(t)
	a := h.a
	for i, v := range []string{"one", "two", "three", "four"} {
		h.enter("A"+string(rune('1'+i)), v)
	}
	want := []string{"Typing 'four' in A4", "Typing 'three' in A3", "Typing 'two' in A2", "Typing 'one' in A1"}
	if got := a.book.UndoLabels(); !slices.Equal(got, want) {
		t.Fatalf("UndoLabels %q", got)
	}
	if got := a.menus()[menuHome].Items[homeUndo].Label; got != "Undo Typing 'four' in A4" {
		t.Errorf("Home menu: %q", got)
	}
	// The 3rd entry takes back three steps.
	h.pickHistory(&a.home.undoHL, 3)
	h.expect("undo 3", map[string]string{"A1": "one", "A2": "", "A3": "", "A4": ""})
	if a.home.undoHL.IsOpen() || len(a.book.RedoLabels()) != 3 || !a.modified {
		t.Errorf("after undo 3: open %v, redo %q", a.home.undoHL.IsOpen(), a.book.RedoLabels())
	}
	// Redo two of them.
	h.pickHistory(&a.home.redoHL, 2)
	h.expect("redo 2", map[string]string{"A1": "one", "A2": "two", "A3": "three", "A4": ""})
	if got := a.menus()[menuHome].Items[homeRedo].Label; got != "Redo Typing 'four' in A4" {
		t.Errorf("Home menu: %q", got)
	}
	// A ribbon command's step takes its button's name.
	p := h.locate(&a.home.bold, ribbonArea, 4)
	h.click(p.X, p.Y)
	if got := a.book.UndoLabels(); len(got) == 0 || got[0] != "Bold" {
		t.Errorf("after Bold: %q", got)
	}
}
