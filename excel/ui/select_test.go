package ui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/fileio"
	"officelite/excel/model"
)

// TestDragSelect drags from E6 to H10 in Book1.xlsx and checks the selection survives the
// mouse-up.
func TestDragSelect(t *testing.T) {
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	a := &App{ui: gui.NewTheme(fonts, accent, accentDark), fonts: fonts, recent: recent.Open(""), zoom: 1, states: map[int]*sheetState{}}
	a.th = a.ui.Theme
	a.shaper = a.th.Shaper
	wb, err := fileio.Open("../../Samples/Book1.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	a.setWorkbook(wb)

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1000, 700)), Source: r.Source()}
		a.layout(gtx)
		r.Frame(gtx.Ops)
	}
	frame()
	frame()
	v := &a.state().view
	pt := func(row, col int) f32.Point {
		for y := float32(0); y < 700; y++ {
			for x := float32(0); x < 1000; x += 4 {
				h := v.HitTest(x, y)
				if h.Row == row && h.Col == col {
					// HitTest is relative to the grid; find the grid's origin.
					return f32.Pt(x+2, y+2)
				}
			}
		}
		t.Fatalf("cell %d,%d not found", row, col)
		return f32.Point{}
	}
	// Grid origin in window coordinates: title + menu + formula bar.
	off := f32.Pt(0, float32(36+30+30))
	b2, d5 := pt(5, 4).Add(off), pt(9, 7).Add(off) // E6, H10
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: b2})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: d5})
	frame()
	t.Logf("E6 at %v, H10 at %v; during drag: %+v", b2, d5, v.Sel)
	// Windows: ReleaseCapture in WM_POINTERUP sends a capture-changed
	// Cancel, then the Release follows.
	r.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, Position: d5})
	frame()
	t.Logf("after cancel: %+v", v.Sel)
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: d5})
	frame()
	t.Logf("after release: %+v", v.Sel)
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: d5.Add(f32.Pt(3, 1))})
	frame()
	frame()
	want := model.Range{R1: 5, C1: 4, R2: 9, C2: 7}
	if got := a.state().view.Sel.Range; got != want {
		t.Errorf("after release: %+v, want %+v", got, want)
	}
}
