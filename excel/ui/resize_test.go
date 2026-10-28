package ui

import (
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
)

// gridTop is the grid's offset in the window: title and menu bars, the
// ribbon and the formula bar.
const gridTop = 36 + 30 + ribbonHeight + 30

func (h *harness) drag(from, to f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: from})
	h.frame()
	mid := from.Add(to).Mul(0.5)
	for _, p := range []f32.Point{mid, to} {
		h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
		h.frame()
	}
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: to})
	h.frame()
}

func TestResizeByDragging(t *testing.T) {
	h := newHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	_, hh := v.HeaderSize()

	// The resize cursor shows over a divider, the crosshair elsewhere.
	_, bEnd, _ := v.ColSpan(1)
	hover := func(x, y int) pointer.Cursor {
		h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(x), float32(gridTop+y))})
		h.frame()
		return h.r.Cursor()
	}
	if c := hover(bEnd, hh/2); c != pointer.CursorColResize {
		t.Errorf("cursor on B|C divider = %v", c)
	}
	if c := hover(bEnd-20, hh/2); c == pointer.CursorColResize {
		t.Errorf("cursor inside B's header = %v", c)
	}
	hw0, _ := v.HeaderSize()
	if _, r1End, _ := v.RowSpan(0); hover(hw0/2, r1End) != pointer.CursorRowResize {
		t.Errorf("cursor on row 1|2 divider = %v", h.r.Cursor())
	}

	// Drag the divider between B and C 40px to the right.
	h.drag(f32.Pt(float32(bEnd), float32(gridTop+hh/2)), f32.Pt(float32(bEnd+40), float32(gridTop+hh/2)))
	w, ok := sh.ColWidth[1]
	if !ok {
		t.Fatalf("column B not resized: %v", sh.ColWidth)
	}
	if _, x1, _ := v.ColSpan(1); x1 != bEnd+40 {
		t.Errorf("B ends at %d, want %d (width %v)", x1, bEnd+40, w)
	}
	if !a.modified {
		t.Error("resize didn't mark the workbook modified")
	}
	if v.Sel.Range.C1 != 0 || v.Sel.Range.C2 != 0 {
		t.Errorf("divider drag changed the selection: %+v", v.Sel.Range)
	}

	// Drag row 3's bottom divider up to hide it.
	hw, _ := v.HeaderSize()
	_, r3End, _ := v.RowSpan(2)
	y0, _, _ := v.RowSpan(2)
	h.drag(f32.Pt(float32(hw/2), float32(gridTop+r3End)), f32.Pt(float32(hw/2), float32(gridTop+y0-5)))
	if hgt, ok := sh.RowHeight[2]; !ok || hgt != 0 {
		t.Errorf("row 3 height = %v, %v; want hidden", hgt, ok)
	}

	h.press("Z", key.ModShortcut)
	if _, ok := sh.RowHeight[2]; ok {
		t.Error("undo didn't restore row 3")
	}
	h.press("Z", key.ModShortcut)
	if _, ok := sh.ColWidth[1]; ok {
		t.Error("undo didn't restore column B")
	}
	if _, x1, _ := v.ColSpan(1); x1 != bEnd {
		t.Errorf("after undo B ends at %d, want %d", x1, bEnd)
	}
}

func TestAutoFitByDoubleClick(t *testing.T) {
	h := newHarness(t)
	a := h.a
	v := &a.state().view
	sh := v.Sheet
	h.typeText("A much longer entry than fits")
	h.press(key.NameReturn, 0)
	h.typeText("short")
	h.press(key.NameReturn, 0)

	_, hh := v.HeaderSize()
	now := time.Second
	doubleClickAt := func(x, y int) {
		for range 2 {
			now += 100 * time.Millisecond
			p := f32.Pt(float32(x), float32(gridTop+y))
			h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p, Time: now})
			h.frame()
			h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p, Time: now})
			h.frame()
		}
	}

	// Column A widens to its longest entry.
	_, aEnd, _ := v.ColSpan(0)
	doubleClickAt(aEnd, hh/2)
	need, _ := v.FitColWidth(a.gtx, 0)
	got := float64(sh.ColWidthPx(0, v.DigitWidth))
	if _, ok := sh.ColWidth[0]; !ok || got < need || got > need+2 {
		t.Fatalf("column A is %vpx after AutoFit, want %vpx", got, need)
	}
	if v.Sel.Range.C1 != 0 || v.Sel.Range.R1 != 2 {
		t.Errorf("AutoFit changed the selection: %+v", v.Sel.Range)
	}
	// An empty column keeps its width.
	_, bEnd, _ := v.ColSpan(1)
	doubleClickAt(bEnd, hh/2)
	if _, ok := sh.ColWidth[1]; ok {
		t.Errorf("empty column B was resized to %v", sh.ColWidth[1])
	}
	// Undo restores the default width.
	h.press("Z", key.ModShortcut)
	if _, ok := sh.ColWidth[0]; ok {
		t.Error("undo didn't restore column A")
	}
}
