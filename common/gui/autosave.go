// AutoSave controls: the on/off Switch, the title bar's AutoSave switch and
// the backstage Info page's AutoSave default setting.

package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"officelite/common/autosave"
)

// Switch draws an on/off toggle, as Office's: a pill with a round knob
// and "On" / "Off" in it. Off is an outline in fg with the knob at the
// left; on is filled with fg, the knob (in bg, the color behind it) at
// the right.
func Switch(gtx layout.Context, th *Theme, on bool, fg, bg color.NRGBA) layout.Dimensions {
	w, h := gtx.Dp(50), gtx.Dp(20)
	r := h / 2
	pill := clip.UniformRRect(image.Rect(0, 0, w, h), r)
	knob := gtx.Dp(12)
	pad := (h - knob) / 2
	txt, txtColor := "Off", fg
	knobAt := image.Pt(pad, pad)
	if on {
		paint.FillShape(gtx.Ops, fg, pill.Op(gtx.Ops))
		txt, txtColor = "On", bg
		knobAt.X = w - pad - knob
	} else {
		paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: pill.Path(gtx.Ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	}
	dot := clip.Ellipse{Min: knobAt, Max: knobAt.Add(image.Pt(knob, knob))}
	paint.FillShape(gtx.Ops, txtColor, dot.Op(gtx.Ops))
	// The text in the half the knob isn't in.
	l := th.Label(11, txt, txtColor, font.Normal)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(w, h)}
	m := op.Record(gtx.Ops)
	d := l.Layout(g)
	call := m.Stop()
	// The text is centered in the room beside the knob.
	room := image.Pt(pad+knob+gtx.Dp(2), w-pad)
	if on {
		room = image.Pt(pad, knobAt.X-gtx.Dp(2))
	}
	x := room.X + (room.Y-room.X-d.Size.X)/2
	o := op.Offset(image.Pt(x, (h-d.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// AutoSaveSwitch is the title bar's "AutoSave (On)" control, left of the
// document's name, as in Office: the label and a Switch, with a tooltip
// that says what it does or why it can't be turned on.
type AutoSaveSwitch struct {
	btn  widget.Clickable
	tips tipper
	why  string // the tip's reason, once set
	set  bool
}

// AutoSaveWorksWith is the tooltip of a switch that can't be turned on
// because the document isn't a saved file of the kind AutoSave writes
// (ext: ".xlsx" or ".docx").
func AutoSaveWorksWith(ext string) string {
	return "AutoSave works with " + ext + " files you've saved. Save a copy as " + ext + " to turn it on."
}

// Layout draws the control in white on a title bar of color bg. why, when
// not "", is why AutoSave isn't available: the switch is grayed, ignores
// clicks and its tooltip says why. It reports whether it was clicked.
func (s *AutoSaveSwitch) Layout(gtx layout.Context, th *Theme, on bool, why string, bg color.NRGBA) (bool, layout.Dimensions) {
	clicked := false
	for s.btn.Clicked(gtx) {
		clicked = why == ""
	}
	if !s.set || why != s.why {
		s.why, s.set = why, true
		desc := why
		if desc == "" {
			desc = "Save your changes to the file automatically, a few seconds after each edit. There's no version history: Undo is the way back."
		}
		SetTip(&s.btn, "AutoSave", "", desc)
	}
	fg := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	if why != "" {
		fg.A = 0x80
	}
	// The tipper follows the pointer over the area the switch is in.
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	gtx = withTipper(gtx, &s.tips)
	s.tips.track(gtx)
	d := s.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		d := layout.Inset{Left: 4, Right: 4, Top: 4, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(th.Label(12, "AutoSave", fg, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Width: 6}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Switch(gtx, th, on, fg, bg)
				}),
			)
		})
		if why == "" {
			area := clip.Rect{Max: d.Size}.Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			area.Pop()
		}
		tipButton(gtx, &s.btn, s.btn.Hovered(), d.Size)
		return d
	})
	area.Pop()
	// The tooltip goes under the switch, over the menu bar and ribbon.
	tm := op.Record(gtx.Ops)
	s.tips.layout(gtx, th, image.Rect(-gtx.Dp(400), 0, gtx.Dp(800), 1<<15), false)
	op.Defer(gtx.Ops, tm.Stop())
	return clicked, d
}

// AutoSaveRows are the Info page's AutoSave setting: whether AutoSave
// starts on for the files it works with.
func (b *Backstage) AutoSaveRows(th *Theme, ext string) []layout.Widget {
	items := []DropItem{{Label: "Off"}, {Label: "On"}}
	cur := "Off"
	if autosave.Default() {
		cur = "On"
	}
	return []layout.Widget{
		th.Heading("AutoSave"),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(th.Label(13, "AutoSave "+ext+" files when they're opened or saved", ChromeText, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Width: 10}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if i := b.autosave.Layout(gtx, th, 80, cur, items); i >= 0 {
						go autosave.SetDefault(i == 1) // a file write
					}
					return layout.Dimensions{Size: image.Pt(gtx.Dp(80), gtx.Dp(24))}
				}),
			)
		},
		th.Paragraph("The AutoSave switch in the title bar saves into the file itself a few seconds after each edit. "+
			"Unlike OneDrive there's no version history, so it's off unless you turn it on.", MutedText),
	}
}
