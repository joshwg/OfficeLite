// Tests for GlyphButton inside a ribbon.

package gui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// TestGlyphButtonClicks checks that a glyph button with a tip, in a ribbon
// (which hands out tips), still takes clicks: the tip's handler must not
// cover the button.
func TestGlyphButtonClicks(t *testing.T) {
	th := NewTheme(NewFontLibrary(), ChromeText, ChromeText)
	var (
		r      input.Router
		ops    op.Ops
		ribbon Ribbon
		btn    widget.Clickable
	)
	SetTip(&btn, "Show/Hide ¶", "Ctrl+Shift+8", "")
	clicks := 0
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(400, 200)), Source: r.Source()}
		for btn.Clicked(gtx) {
			clicks++
		}
		ribbon.Layout(gtx, th, RibbonGroup{Caption: "Paragraph", Content: RibbonRow(
			func(gtx layout.Context) layout.Dimensions { return GlyphButton(gtx, th, &btn, "¶", false) },
		)})
		r.Frame(&ops)
	}
	frame()
	// Find the button by hovering across the ribbon.
	var at f32.Point
	for y := float32(2); y < 120 && at == (f32.Point{}); y += 2 {
		for x := float32(2); x < 120; x += 2 {
			r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
			frame()
			if btn.Hovered() {
				at = f32.Pt(x+4, y+4)
				break
			}
		}
	}
	if at == (f32.Point{}) {
		t.Fatal("button not found")
	}
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
	frame()
	frame()
	if clicks != 1 {
		t.Fatalf("clicks = %d, want 1", clicks)
	}
}
