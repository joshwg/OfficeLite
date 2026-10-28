package gui

import (
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
)

// barHarness lays out a 14×300 vertical bar: arrows 0-14 and 286-300,
// 100 units visible of 1000, so the thumb is 27 px over a 272 px track.
type barHarness struct {
	r   input.Router
	s   Scrollbar
	pos float64
	now time.Time
}

func (h *barHarness) frame() {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(14, 300)),
		Source:      h.r.Source(),
		Now:         h.now,
	}
	h.pos = h.s.Layout(gtx, h.pos, 100, 1000)
	h.r.Frame(gtx.Ops)
}

func (h *barHarness) send(k pointer.Kind, x, y float32, mods key.Modifiers) {
	e := pointer.Event{Kind: k, Source: pointer.Mouse, Position: f32.Pt(x, y), Modifiers: mods, Time: time.Duration(h.now.UnixNano())}
	if k == pointer.Press || k == pointer.Drag {
		e.Buttons = pointer.ButtonPrimary
	}
	if k == pointer.Drag {
		e.Kind = pointer.Move // the router turns moves with a button down into drags
	}
	h.r.Queue(e)
	h.frame()
}

func newBar() *barHarness {
	h := &barHarness{now: time.Unix(1000, 0)}
	h.s.Vertical = true
	h.s.Step = 1
	h.frame()
	return h
}

func TestScrollbarArrowsStepAndRepeat(t *testing.T) {
	h := newBar()
	h.send(pointer.Press, 7, 293, 0)
	if h.pos != 1 {
		t.Fatalf("down arrow: pos %v, want 1", h.pos)
	}
	// Held: nothing until the delay, then a step every repeat interval.
	h.now = h.now.Add(scrollRepeatDelay - time.Millisecond)
	h.frame()
	if h.pos != 1 {
		t.Fatalf("before the repeat delay: pos %v", h.pos)
	}
	h.now = h.now.Add(time.Millisecond)
	h.frame()
	h.now = h.now.Add(scrollRepeatEvery)
	h.frame()
	if h.pos != 3 {
		t.Fatalf("after repeating: pos %v, want 3", h.pos)
	}
	h.send(pointer.Release, 7, 293, 0)
	h.now = h.now.Add(time.Second)
	h.frame()
	if h.pos != 3 {
		t.Fatalf("after release: pos %v, want 3", h.pos)
	}
	h.send(pointer.Press, 7, 5, 0)
	h.send(pointer.Release, 7, 5, 0)
	if h.pos != 2 {
		t.Fatalf("up arrow: pos %v, want 2", h.pos)
	}
}

func TestScrollbarTrackPagesTowardPointer(t *testing.T) {
	h := newBar()
	// Hold the track at y=100: pages until the thumb reaches the pointer.
	h.send(pointer.Press, 7, 100, 0)
	if h.pos != 100 {
		t.Fatalf("first page: pos %v, want 100", h.pos)
	}
	for range 20 {
		h.now = h.now.Add(scrollRepeatDelay)
		h.frame()
	}
	// Thumb top at 14+pos/900*245 must now be past y=100-27 and no further
	// than one page beyond.
	top := 14 + h.pos/900*245
	if top+27 <= 100 || h.pos > 400 {
		t.Fatalf("paging stopped at pos %v (thumb %v..%v), pointer at 100", h.pos, top, top+27)
	}
}

func TestScrollbarShiftClickJumpsAndDrags(t *testing.T) {
	h := newBar()
	h.send(pointer.Press, 7, 150, key.ModShift)
	want := float64((150-14-27.0/2)/245) * 900
	if math.Abs(h.pos-want) > 5 {
		t.Fatalf("shift+click: pos %v, want about %v", h.pos, want)
	}
	h.send(pointer.Drag, 7, 200, 0)
	if h.pos <= want {
		t.Fatalf("dragging after shift+click: pos %v", h.pos)
	}
	// Far off the bar the thumb snaps back to where the drag began.
	h.send(pointer.Drag, 400, 200, 0)
	if math.Abs(h.pos-want) > 1 {
		t.Fatalf("snap back: pos %v, want %v", h.pos, want)
	}
	h.send(pointer.Release, 400, 200, 0)
}
