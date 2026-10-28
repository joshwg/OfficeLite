package ui

import (
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/word/model"
)

// click is a fresh single click (not part of a double-click).
func (h *harness) click(pt f32.Point, mods key.Modifiers) {
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, pt, mods)
	h.mouse(pointer.Release, pt, mods)
}

func (h *harness) rightClick(pt f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: pt})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pt})
	h.frame()
}

// at is the view point of the middle of the caret at p.
func (h *harness) at(p model.Pos) f32.Point {
	c, ok := h.d.eng.CaretAt(p)
	if !ok {
		h.t.Fatalf("no caret at %v", p)
	}
	return h.viewPoint(c.Page, c.X, c.Y+c.H/2)
}

func menuLabels(d *window) map[string]int {
	out := map[string]int{}
	for i, it := range d.v.m.items {
		out[it.Label] = i
	}
	return out
}

func TestContextMenu(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("alpha beta gamma")
	h.frame()
	// Right-click outside the selection moves the caret there.
	d.ed.Select(model.Pos{Off: 0}, model.Pos{Off: 5})
	h.rightClick(h.at(model.Pos{Off: 8}))
	if !d.v.m.menu.IsOpen() {
		t.Fatal("menu not open")
	}
	if !d.ed.Sel.Empty() || d.ed.Sel.Caret.Off != 8 {
		t.Errorf("caret %v", d.ed.Sel)
	}
	labels := menuLabels(d)
	for _, want := range []string{"Cut", "Copy", "Paste", "Paste Special…", "Link…"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("no %q in %v", want, labels)
		}
	}
	if _, ok := labels["Insert Row Above"]; ok {
		t.Error("table items outside a table")
	}
	d.v.m.menu.Close()
	// Inside the selection it is kept.
	d.ed.Select(model.Pos{Off: 6}, model.Pos{Off: 10})
	h.rightClick(h.at(model.Pos{Off: 8}))
	if a, b := d.ed.Sel.Ordered(); a.Off != 6 || b.Off != 10 {
		t.Errorf("selection lost: %v", d.ed.Sel)
	}
	d.v.m.menu.Close()
	// In a table: the table commands, run from the menu.
	d.ed.SetCaret(model.Pos{Off: 16}, false)
	d.ed.Enter()
	d.ed.InsertTable(2, 2)
	h.frame()
	h.rightClick(h.at(d.ed.Sel.Caret))
	labels = menuLabels(d)
	i, ok := labels["Insert Row Below"]
	if !ok {
		t.Fatalf("no table items: %v", labels)
	}
	d.v.m.acts[i]()
	if tb := d.doc.Body[1].(*model.Table); len(tb.Rows) != 3 {
		t.Errorf("rows %d", len(tb.Rows))
	}
	d.v.m.menu.Close()
	// On a link: Edit, Open and Remove Link.
	d.ed.SetCaret(model.Pos{}, false)
	d.ed.InsertLink("site", "example.com")
	h.frame()
	h.rightClick(h.at(model.Pos{Off: 2}))
	labels = menuLabels(d)
	for _, want := range []string{"Edit Link…", "Open Link", "Remove Link"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("no %q in %v", want, labels)
		}
	}
	d.v.m.acts[labels["Remove Link"]]()
	if _, ok := d.ed.LinkAt(model.Pos{Off: 2}); ok {
		t.Error("link not removed")
	}
}

func TestMarginSelect(t *testing.T) {
	h := newHarness(t)
	d := h.d
	long := "The quick brown fox jumps over the lazy dog. "
	h.typeText(long + long + long + long)
	h.press(key.NameReturn, 0)
	h.typeText("Second paragraph")
	h.frame()
	c, _ := d.eng.CaretAt(model.Pos{Off: 0})
	margin := func(p model.Pos) f32.Point {
		c, _ := d.eng.CaretAt(p)
		return h.viewPoint(c.Page, d.eng.TextLeft()/2, c.Y+c.H/2)
	}
	// The cursor is an arrow there.
	pt := margin(model.Pos{})
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pt})
	h.frame()
	if _, _, ok := d.marginAt(d.v.m.hover); !ok {
		t.Fatal("not in the margin")
	}
	// A click selects the first line.
	h.click(pt, 0)
	a, b := d.ed.Sel.Ordered()
	_, end, _ := d.eng.LineAtY(c.Page, c.Y+c.H/2)
	if a != (model.Pos{}) || b != end || b.Off == 0 || b.Off >= d.ed.Para(0).Len() {
		t.Fatalf("line: %v..%v (line end %v)", a, b, end)
	}
	// Dragging down selects line by line, to the second paragraph.
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, pt, 0)
	h.mouse(pointer.Drag, margin(model.Pos{Para: 1}), 0)
	h.mouse(pointer.Release, margin(model.Pos{Para: 1}), 0)
	if a, b := d.ed.Sel.Ordered(); a != (model.Pos{}) || b != (model.Pos{Para: 1, Off: 16}) {
		t.Errorf("drag: %v..%v", a, b)
	}
	// Double-click: the paragraph.
	h.d.v.lastClick = time.Time{}
	pt2 := margin(model.Pos{Para: 1})
	h.mouse(pointer.Press, pt2, 0)
	h.mouse(pointer.Release, pt2, 0)
	h.mouse(pointer.Press, pt2, 0)
	h.mouse(pointer.Release, pt2, 0)
	if a, b := d.ed.Sel.Ordered(); a != (model.Pos{Para: 1}) || b != (model.Pos{Para: 1, Off: 16}) {
		t.Errorf("paragraph: %v..%v", a, b)
	}
	// Ctrl+click: everything.
	h.click(pt, key.ModCtrl)
	if a, b := d.ed.Sel.Ordered(); a != (model.Pos{}) || b != (model.Pos{Para: 1, Off: 16}) {
		t.Errorf("all: %v..%v", a, b)
	}
	// Shift+click extends by lines from the caret.
	d.ed.SetCaret(model.Pos{Para: 1, Off: 3}, false)
	h.click(pt, key.ModShift)
	if a, b := d.ed.Sel.Ordered(); a != (model.Pos{}) || b != (model.Pos{Para: 1, Off: 3}) {
		t.Errorf("shift: %v..%v", a, b)
	}
}

func TestDragMovesText(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("one two three")
	h.frame()
	d.ed.Select(model.Pos{Off: 4}, model.Pos{Off: 8}) // "two "
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, h.at(model.Pos{Off: 5}), 0)
	h.mouse(pointer.Drag, h.at(model.Pos{Off: 11}), 0)
	if !d.v.m.moving || !d.v.m.dropOK {
		t.Fatal("not moving")
	}
	h.mouse(pointer.Drag, h.at(model.Pos{Off: 13}), 0)
	h.mouse(pointer.Release, h.at(model.Pos{Off: 13}), 0)
	if got := h.para(0).Text(); got != "one threetwo " {
		t.Fatalf("moved: %q", got)
	}
	d.ed.Undo()
	if got := h.para(0).Text(); got != "one two three" {
		t.Fatalf("undo: %q", got)
	}
	// Ctrl at the release copies.
	d.ed.Select(model.Pos{Off: 0}, model.Pos{Off: 4})
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, h.at(model.Pos{Off: 1}), 0)
	h.mouse(pointer.Drag, h.at(model.Pos{Off: 13}), key.ModCtrl)
	h.mouse(pointer.Release, h.at(model.Pos{Off: 13}), key.ModCtrl)
	if got := h.para(0).Text(); got != "one two threeone " {
		t.Fatalf("copied: %q", got)
	}
	d.ed.Undo()
	// A click in the selection without dragging places the caret.
	d.ed.Select(model.Pos{Off: 4}, model.Pos{Off: 8})
	h.click(h.at(model.Pos{Off: 6}), 0)
	if !d.ed.Sel.Empty() || d.ed.Sel.Caret.Off != 6 {
		t.Errorf("click: %v", d.ed.Sel)
	}
	// Esc during the drag cancels.
	d.ed.Select(model.Pos{Off: 4}, model.Pos{Off: 8})
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, h.at(model.Pos{Off: 5}), 0)
	h.mouse(pointer.Drag, h.at(model.Pos{Off: 13}), 0)
	h.press(key.NameEscape, 0)
	h.mouse(pointer.Release, h.at(model.Pos{Off: 13}), 0)
	if got := h.para(0).Text(); got != "one two three" {
		t.Errorf("Esc didn't cancel: %q", got)
	}
}

func TestTableBorderDrag(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.ed.InsertTable(2, 3)
	h.typeText("cell")
	h.frame()
	tb := d.doc.Body[0].(*model.Table)
	w := tb.Columns[0]
	rb := d.eng.Pages[0].TableRows[0]
	edge := rb.Edges[1]
	from := h.viewPoint(0, edge.X, (rb.Y0+rb.Y1)/2)
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: from})
	h.frame()
	if b, ok := d.borderAt(d.v.m.hover); !ok || !b.Vertical || b.Grid != 1 {
		t.Fatalf("border hover: %+v %v", b, ok)
	}
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, from, 0)
	to := from.Add(f32.Pt(float32(40*d.v.scale), 0))
	h.mouse(pointer.Drag, to, 0)
	if !d.v.m.resizing || math.Abs(d.v.m.borderNow-(edge.X+40)) > 0.5 {
		t.Fatalf("guide at %v", d.v.m.borderNow)
	}
	h.mouse(pointer.Release, to, 0)
	tb = d.doc.Body[0].(*model.Table)
	if math.Abs(tb.Columns[0]-(w+40)) > 0.5 || math.Abs(tb.Columns[1]-(w-40)) > 0.5 || tb.Columns[2] != w {
		t.Fatalf("columns %v", tb.Columns)
	}
	if got := h.para(0).Text(); got != "cell" {
		t.Errorf("text changed: %q", got)
	}
	// The bottom border of the first row sets its height.
	h.frame()
	rb = d.eng.Pages[0].TableRows[0]
	from = h.viewPoint(0, (rb.Edges[0].X+rb.Edges[1].X)/2, rb.Y1)
	h.d.v.lastClick = time.Time{}
	h.mouse(pointer.Press, from, 0)
	to = from.Add(f32.Pt(0, float32(30*d.v.scale)))
	h.mouse(pointer.Drag, to, 0)
	h.mouse(pointer.Release, to, 0)
	tb = d.doc.Body[0].(*model.Table)
	if want := rb.Y1 - rb.Y0 + 30; math.Abs(tb.Rows[0].Height-want) > 0.5 {
		t.Errorf("row height %v, want %v", tb.Rows[0].Height, want)
	}
	d.ed.Undo()
	d.ed.Undo()
	if tb = d.doc.Body[0].(*model.Table); tb.Columns[0] != w || tb.Rows[0].Height != 0 {
		t.Errorf("undo: %v %v", tb.Columns, tb.Rows[0].Height)
	}
}
