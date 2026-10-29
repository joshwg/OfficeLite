// Tests for HistoryList: the split button, hover highlighting, the footer
// and the disabled empty state.

package gui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// historyRig drives a HistoryList's split button through an input router;
// n is the last nonzero step count it returned.
type historyRig struct {
	r      input.Router
	ops    op.Ops
	th     *Theme
	h      HistoryList
	btn    widget.Clickable
	labels []string
	n      int
}

// frame lays out one frame of the split button and records any steps it
// chose in g.n.
func (g *historyRig) frame() {
	g.ops.Reset()
	gtx := layout.Context{Ops: &g.ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(600, 600)}, Source: g.r.Source(), Now: time.Unix(1e9, 0)}
	if n, _ := g.h.SplitButton(gtx, g.th, &g.btn, IconUndo, "Undo", "Ctrl+Z", g.labels); n > 0 {
		g.n = n
	}
	g.r.Frame(&g.ops)
}

// move moves the mouse to x, y and runs two frames.
func (g *historyRig) move(x, y float32) {
	g.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
	g.frame()
	g.frame()
}

// click presses and releases the primary button at x, y and runs two
// frames.
func (g *historyRig) click(x, y float32) {
	for _, k := range []pointer.Kind{pointer.Press, pointer.Release} {
		e := pointer.Event{Kind: k, Source: pointer.Mouse, Position: f32.Pt(x, y)}
		if k == pointer.Press {
			e.Buttons = pointer.ButtonPrimary
		}
		g.r.Queue(e)
	}
	g.frame()
	g.frame()
}

// TestHistoryList checks the tooltip, a one-step button click, choosing
// several steps from the list, and that an empty history stays disabled.
func TestHistoryList(t *testing.T) {
	g := &historyRig{th: NewTheme(NewFontLibrary(), ChromeText, ChromeText),
		labels: []string{"Typing 'c'", "Bold", "Paste", "Typing 'a'"}}
	g.frame()
	if tip, _ := tipFor(&g.btn); tip.Name != "Undo Typing 'c'" || tip.Keys != "Ctrl+Z" {
		t.Errorf("tooltip %+v", tip)
	}
	// The button undoes one step.
	g.click(10, 10)
	if g.n != 1 {
		t.Fatalf("button chose %d", g.n)
	}
	g.n = 0
	btnW := float32(28) // the Undo button before the arrow
	arrow := g.h.dd.size
	g.click(btnW+float32(arrow.X)/2, float32(arrow.Y)/2)
	if !g.h.IsOpen() {
		t.Fatal("the arrow didn't open the list")
	}
	if got := g.h.Footer("Undo"); got != "Cancel" {
		t.Errorf("footer before pointing = %q", got)
	}
	// Rows are 22 high below a 2 pixel pad, under the arrow.
	y3 := float32(arrow.Y) + 2 + 2*22 + 11
	g.move(btnW+60, y3)
	if g.h.Hovered() != 3 || g.h.Footer("Undo") != "Undo 3 Actions" {
		t.Errorf("pointing at the 3rd entry: hovered %d, footer %q", g.h.Hovered(), g.h.Footer("Undo"))
	}
	g.move(btnW+60, float32(arrow.Y)+2+11)
	if g.h.Footer("Undo") != "Undo 1 Action" {
		t.Errorf("footer at the 1st entry = %q", g.h.Footer("Undo"))
	}
	g.click(btnW+60, y3)
	if g.n != 3 || g.h.IsOpen() {
		t.Errorf("clicking the 3rd entry chose %d (open %v), want 3", g.n, g.h.IsOpen())
	}

	// Empty: disabled, and the list doesn't open.
	g.labels, g.n = nil, 0
	g.frame()
	g.click(btnW+float32(arrow.X)/2, float32(arrow.Y)/2)
	g.click(10, 10)
	if g.h.IsOpen() || g.n != 0 {
		t.Errorf("empty history: open %v, chose %d", g.h.IsOpen(), g.n)
	}
}
