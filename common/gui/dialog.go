package gui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"officelite/common/colors"
)

// Dialog is a modal message box drawn over the window, such as "Want to
// save your changes?". Buttons[0] is the default, drawn in the accent
// color.
type Dialog struct {
	Visible bool
	Title   string
	Message string
	Buttons []string

	btns  []widget.Clickable
	block int // event tag that absorbs clicks outside the box
}

// Show opens the dialog.
func (d *Dialog) Show(title, message string, buttons ...string) {
	d.Visible, d.Title, d.Message, d.Buttons = true, title, message, buttons
	d.btns = make([]widget.Clickable, len(buttons))
}

// Hide closes the dialog.
func (d *Dialog) Hide() { d.Visible = false }

// Clicked returns the index of a button clicked since the last frame and
// hides the dialog, or -1.
func (d *Dialog) Clicked(gtx layout.Context) int {
	for i := range d.btns {
		if d.btns[i].Clicked(gtx) {
			d.Visible = false
			return i
		}
	}
	return -1
}

// Layout draws the dialog centered in the window, dimming what is behind
// it, when it is visible.
func (d *Dialog) Layout(gtx layout.Context, th *Theme) {
	if !d.Visible {
		return
	}
	size := gtx.Constraints.Max
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &d.block)
	area.Pop()
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &d.block, Kinds: pointer.Press}); !ok {
			break
		}
	}
	FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.RGBA(0, 0, 0, 0x30))

	w := min(gtx.Dp(440), size.X-gtx.Dp(32))
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, size.Y)}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(20).Layout(g, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(th.Label(16, d.Title, ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 10}.Layout),
			layout.Rigid(th.Paragraph(d.Message, ChromeText)),
			layout.Rigid(layout.Spacer{Height: 20}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var btns []layout.FlexChild
				btns = append(btns, layout.Flexed(1, layout.Spacer{}.Layout))
				for i, label := range d.Buttons {
					btns = append(btns, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if i == 0 {
								return th.Button(gtx, &d.btns[i], label)
							}
							return d.plainButton(gtx, th, &d.btns[i], label)
						})
					}))
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, btns...)
			}),
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := m.Stop()
	pos := image.Pt((size.X-dims.Size.X)/2, max((size.Y-dims.Size.Y)/3, 0))
	off := op.Offset(pos).Push(gtx.Ops)
	box := image.Rectangle{Max: dims.Size}
	FillRect(gtx.Ops, box.Add(image.Pt(3, 3)), colors.RGBA(0, 0, 0, 0x28)) // shadow
	FillRect(gtx.Ops, box.Inset(-1), ChromeLine)
	FillRect(gtx.Ops, box, colors.White)
	call.Add(gtx.Ops)
	off.Pop()
}

// plainButton is a secondary button: dark text on a light background.
func (d *Dialog) plainButton(gtx layout.Context, th *Theme, btn *widget.Clickable, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return Hoverable(gtx, btn, ChromeBG, HoverBG, func(gtx layout.Context) layout.Dimensions {
		l := th.Label(14, label, ChromeText, font.Normal)
		return layout.Inset{Top: 8, Bottom: 8, Left: 18, Right: 18}.Layout(gtx, l.Layout)
	})
}
