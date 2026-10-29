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

	"officelite/common/colors"
)

// TestAutoSaveSwitch: a click toggles an available switch; an unavailable
// one ignores it and its tooltip says why.
func TestAutoSaveSwitch(t *testing.T) {
	th := NewTheme(NewFontLibrary(), ChromeText, ChromeText)
	var s AutoSaveSwitch
	var r input.Router
	var ops op.Ops
	frame := func(why string) bool {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(300, 36)), Source: r.Source()}
		clicked, d := s.Layout(gtx, th, false, why, colors.RGB(0x107C41))
		if d.Size.X < 80 || d.Size.Y < 20 {
			t.Errorf("size %v", d.Size)
		}
		r.Frame(&ops)
		return clicked
	}
	click := func(why string) bool {
		frame(why)
		r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(40, 14)})
		frame(why)
		r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(40, 14)})
		return frame(why)
	}
	if !click("") {
		t.Error("click not reported")
	}
	if tip, _ := tipFor(&s.btn); tip.Name != "AutoSave" || tip.Desc == "" {
		t.Errorf("tip %+v", tip)
	}
	why := AutoSaveWorksWith(".docx")
	if click(why) {
		t.Error("an unavailable switch reported a click")
	}
	if tip, _ := tipFor(&s.btn); tip.Desc != why {
		t.Errorf("tip %+v", tip)
	}
}
