package ui

import (
	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/widget"

	"officelite/common/gui"
)

// ribbonArea is the ribbon, in window coordinates.
var ribbonArea = image.Rect(0, 36+30, 700, 36+30+ribbonHeight-18)

// locate finds a point over btn by moving the pointer across area.
func (h *harness) locate(btn *widget.Clickable, area image.Rectangle, step int) f32.Point {
	h.t.Helper()
	for y := area.Min.Y + step/2; y < area.Max.Y; y += step {
		for x := area.Min.X + step/2; x < area.Max.X; x += step {
			p := f32.Pt(float32(x), float32(y))
			h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p})
			h.frame()
			if btn.Hovered() {
				return p
			}
		}
	}
	h.t.Fatalf("button not found in %v", area)
	return f32.Point{}
}

// pickHistory opens hl's list from the ribbon and clicks entry n (from 1).
func (h *harness) pickHistory(hl *gui.HistoryList, n int) {
	h.t.Helper()
	p := h.locate(hl.Arrow(), ribbonArea, 4)
	h.click(p, 0)
	if !hl.IsOpen() {
		h.t.Fatal("the arrow didn't open the list")
	}
	for y := p.Y + 8; y < p.Y+200; y += 2 {
		q := f32.Pt(p.X+40, y)
		h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: q})
		h.frame()
		h.frame()
		if hl.Hovered() == n {
			h.click(q, 0)
			return
		}
	}
	h.t.Fatalf("entry %d not found", n)
}

// text is the document's paragraphs' text, joined by "|".
func (h *harness) text() string {
	var out []string
	for i := range h.d.ed.NumParas() {
		out = append(out, h.para(i).Text())
	}
	return strings.Join(out, "|")
}

func TestUndoHistoryList(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("hello")
	h.press(key.NameLeftArrow, 0)
	h.press("B", key.ModCtrl) // the word around the caret
	h.press(key.NameEnd, 0)
	h.press(key.NameReturn, 0)
	for _, c := range "worlds" {
		h.typeText(string(c)) // as keys come
	}
	h.press(key.NameDeleteBackward, 0) // part of the typing
	h.press(key.NameHome, 0)
	h.press(key.NameDeleteForward, 0)
	want := []string{"Delete", "Typing 'world'", "Typing", "Bold", "Typing 'hello'"}
	if got := d.ed.UndoLabels(); !slices.Equal(got, want) {
		t.Fatalf("UndoLabels %q", got)
	}
	if got := d.menus()[menuHome].Items[homeUndo].Label; got != "Undo Delete" {
		t.Errorf("Home menu: %q", got)
	}
	// The 3rd entry takes back the Delete, "world" and the new
	// paragraph.
	h.pickHistory(&d.tb.undoHL, 3)
	if got := h.text(); got != "hello" || d.tb.undoHL.IsOpen() {
		t.Fatalf("undo 3: %q", got)
	}
	if got := d.ed.RedoLabels(); !slices.Equal(got, []string{"Typing", "Typing 'world'", "Delete"}) {
		t.Errorf("RedoLabels %q", got)
	}
	h.pickHistory(&d.tb.redoHL, 2)
	if got := h.text(); got != "hello|world" {
		t.Errorf("redo 2: %q", got)
	}
	// A ribbon command's step takes its button's name.
	d.ed.SelectAll()
	p := h.locate(&d.tb.italic, ribbonArea, 4)
	h.click(p, 0)
	if got := d.ed.UndoLabels(); got[0] != "Italic" {
		t.Errorf("after Italic: %q", got)
	}
}
