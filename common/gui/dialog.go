package gui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
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

// BlockFilter is the filter of a modal layer's event tag: it takes the
// presses and the mouse wheel's scrolls (any amount), so neither reaches
// what is behind the layer.
func BlockFilter(tag event.Tag) pointer.Filter {
	all := pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30}
	return pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Scroll, ScrollX: all, ScrollY: all}
}

// Layout draws the dialog centered in the window, dimming what is behind
// it, when it is visible.
func (d *Dialog) Layout(gtx layout.Context, th *Theme) {
	if !d.Visible {
		return
	}
	btns := make([]layout.Widget, len(d.Buttons))
	for i, label := range d.Buttons {
		btns[i] = func(gtx layout.Context) layout.Dimensions {
			if i == 0 {
				return th.Button(gtx, &d.btns[i], label)
			}
			return th.SecondaryButton(gtx, &d.btns[i], label)
		}
	}
	th.ModalDialog(gtx, &d.block, d.Title, 440, th.Paragraph(d.Message, ChromeText), btns...)
}

// Modal dims the window, swallows the presses and scrolls outside the
// panel (on tag) and draws body in a white panel width wide (narrower on
// a narrow window), centered across and a quarter of the way down.
func Modal(gtx layout.Context, tag event.Tag, width unit.Dp, body layout.Widget) {
	size := gtx.Constraints.Max
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	for {
		if _, ok := gtx.Event(BlockFilter(tag)); !ok {
			break
		}
	}
	FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.RGBA(0, 0, 0, 0x30))
	w := max(min(gtx.Dp(width), size.X-gtx.Dp(16)), 0)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, size.Y)}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(18).Layout(g, body)
	call := m.Stop()
	pos := image.Pt((size.X-dims.Size.X)/2, max((size.Y-dims.Size.Y)/4, 0))
	off := op.Offset(pos).Push(gtx.Ops)
	PanelBox(gtx.Ops, dims.Size)
	call.Add(gtx.Ops)
	off.Pop()
}

// PanelBox paints a dialog panel's white box of the given size, with its
// border and drop shadow.
func PanelBox(ops *op.Ops, size image.Point) {
	box := image.Rectangle{Max: size}
	FillRect(ops, box.Add(image.Pt(3, 3)), colors.RGBA(0, 0, 0, 0x28)) // shadow
	FillRect(ops, box.Inset(-1), ChromeLine)
	FillRect(ops, box, colors.White)
}

// ModalDialog is a Modal with a title over body and a row of buttons at
// the bottom right.
func (th *Theme) ModalDialog(gtx layout.Context, tag event.Tag, title string, width unit.Dp, body layout.Widget, buttons ...layout.Widget) {
	Modal(gtx, tag, width, func(gtx layout.Context) layout.Dimensions {
		btns := []layout.FlexChild{layout.Flexed(1, layout.Spacer{}.Layout)}
		for _, b := range buttons {
			btns = append(btns, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 8}.Layout(gtx, b)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(16, title, ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(body),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, btns...)
			}),
		)
	})
}

// SecondaryButton is a dialog's secondary button: dark text on a light
// background (Button is the accent-colored default one).
func (th *Theme) SecondaryButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return Hoverable(gtx, btn, ChromeBG, HoverBG, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Bottom: 8, Left: 18, Right: 18}.Layout(gtx, th.Label(14, label, ChromeText, font.Normal).Layout)
	})
}
