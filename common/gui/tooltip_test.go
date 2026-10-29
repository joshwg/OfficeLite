// Tests for tooltips: show delay, switching and timeout on a ribbon,
// menu item tips, and the weak tip registry.

package gui

import (
	"image"
	"image/color"
	"runtime"
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

// tipRig is a ribbon of two tool buttons driven through a router.
type tipRig struct {
	th     *Theme
	r      input.Router
	ops    op.Ops
	ribbon Ribbon
	a, b   widget.Clickable
	now    time.Time
}

// newTipRig returns a rig whose buttons carry Bold and Italic tips.
func newTipRig() *tipRig {
	g := &tipRig{th: NewTheme(NewFontLibrary(), ChromeText, ChromeText), now: time.Unix(1e9, 0)}
	SetTip(&g.a, "Bold", "Ctrl+B", "Make your text bold.")
	SetTip(&g.b, "Italic", "Ctrl+I", "")
	return g
}

// frame lays the ribbon out at size and g.now.
func (g *tipRig) frame(size image.Point) {
	g.ops.Reset()
	gtx := layout.Context{Ops: &g.ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size), Source: g.r.Source(), Now: g.now}
	g.ribbon.Layout(gtx, g.th, RibbonGroup{Caption: "Font", Content: RibbonRow(
		func(gtx layout.Context) layout.Dimensions {
			return ToolButton(gtx, g.th, &g.a, IconBold, "", false, color.NRGBA{})
		},
		func(gtx layout.Context) layout.Dimensions {
			return ToolButton(gtx, g.th, &g.b, IconItalic, "", false, color.NRGBA{})
		},
	)})
	g.r.Frame(&g.ops)
}

// move moves the mouse to (x, y) and runs a frame.
func (g *tipRig) move(x, y float32) {
	g.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
	g.frame(image.Pt(800, 300))
}

// after advances the clock by d and runs a frame.
func (g *tipRig) after(d time.Duration) {
	g.now = g.now.Add(d)
	g.frame(image.Pt(800, 300))
}

// shown reports whether the last frame drew a tip.
func (g *tipRig) shown() bool { return g.ribbon.tips.lastShow.Equal(g.now) }

// TestTooltipTiming checks that a tip shows after the delay, switches at
// once to a neighbor, hides after the timeout, and is forgotten on leaving
// and suppressed by a press.
func TestTooltipTiming(t *testing.T) {
	g := newTipRig()
	g.frame(image.Pt(800, 300))
	// Buttons sit at x 6..34 and 34..62, y 4..32.
	g.move(20, 15)
	if g.ribbon.tips.key != &g.a || g.shown() {
		t.Fatalf("hover: key %v shown %v", g.ribbon.tips.key, g.shown())
	}
	if r := g.ribbon.tips.rect; r.Min != image.Pt(6, 4) {
		t.Errorf("button rect %v, want at (6,4)", r)
	}
	g.after(400 * time.Millisecond)
	if g.shown() {
		t.Fatal("shown before the delay")
	}
	g.after(200 * time.Millisecond)
	if !g.shown() {
		t.Fatal("not shown after the delay")
	}
	g.move(45, 15) // the neighbor switches at once
	if g.ribbon.tips.key != &g.b || !g.shown() {
		t.Fatalf("switch: key %v shown %v", g.ribbon.tips.key, g.shown())
	}
	g.after(11 * time.Second)
	if g.shown() {
		t.Fatal("still shown after the timeout")
	}
	g.move(400, 200) // off the ribbon
	if g.ribbon.tips.key != nil {
		t.Fatal("key kept after leaving")
	}
	g.after(time.Second)
	g.move(20, 15)
	g.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 15)})
	g.after(time.Second)
	if g.shown() {
		t.Fatal("shown after a press")
	}
}

// TestMenuTip checks that a hovered context-menu item shows its Tip.
func TestMenuTip(t *testing.T) {
	th := NewTheme(NewFontLibrary(), ChromeText, ChromeText)
	var r input.Router
	var ops op.Ops
	var c ContextMenu
	c.Open(image.Pt(700, 10), []MenuItem{{Label: "Cut", Tip: "Move the selection to the Clipboard."}})
	now := time.Unix(1e9, 0)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(800, 300)), Source: r.Source(), Now: now}
		c.Layout(gtx, th)
		r.Frame(&ops)
	}
	frame()
	// The menu flips left of x 700: it spans 580..800.
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(600, 25)})
	frame()
	if c.tips.key != &c.btns[0] {
		t.Fatal("item not hovered")
	}
	now = now.Add(time.Second)
	frame()
	if !c.tips.lastShow.Equal(now) {
		t.Fatal("tip not shown")
	}
}

// TestTipsDontPinWindows checks that tips keyed by fields of a window's
// state let the window be collected and drop out of the registry.
func TestTipsDontPinWindows(t *testing.T) {
	type window struct {
		_     [64]byte
		bold  widget.Clickable
		cells [4]widget.Clickable
		dd    DropDown
	}
	w := &window{}
	SetTip(&w.bold, "Bold", "Ctrl+B", "")
	SetTip(&w.cells[2], "Red", "", "")
	SetTip(&w.dd, "Font", "", "")
	SetTip(&w.bold, "Bold", "Ctrl+B", "Make your text bold.") // replaces
	if tp, ok := tipFor(&w.bold); !ok || tp.Desc != "Make your text bold." {
		t.Fatalf("tipFor(bold) = %+v, %v", tp, ok)
	}
	if _, ok := tipFor(&w.dd.btn); !ok {
		t.Fatal("a DropDown's tip isn't on its button")
	}
	if _, ok := tipFor(&w.dd); ok {
		t.Fatal("a DropDown and its button share a key")
	}
	if _, ok := tipFor(&w.cells[1]); ok {
		t.Fatal("tip on an untipped cell")
	}
	// The keys (weak, so they don't hold w) of the three entries.
	var keys []tipKey
	for _, b := range []any{&w.bold, &w.cells[2], &w.dd.btn} {
		k, _ := tipKeyOf(b)
		keys = append(keys, k)
	}
	left := func() int {
		tipsMu.RLock()
		defer tipsMu.RUnlock()
		n := 0
		for _, k := range keys {
			if _, ok := tips[k]; ok {
				n++
			}
		}
		return n
	}
	if n := left(); n != 3 {
		t.Fatalf("%d of the 3 tips registered", n)
	}
	gone := make(chan struct{})
	runtime.AddCleanup(w, func(c chan struct{}) { close(c) }, gone)
	w = nil
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		select {
		case <-gone:
		default:
			if time.Now().After(deadline) {
				t.Fatal("window still reachable after GC")
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if left() == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d tips kept after the window was collected", left())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
