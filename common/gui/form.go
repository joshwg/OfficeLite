// Dialog form helpers: label/control rows, combos, check boxes, radio
// buttons and text fields.

package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
)

// Form lays out the "label  control" rows of a dialog.
type Form struct {
	Th         *Theme
	LabelW     unit.Dp     // width of the label column
	LabelColor color.NRGBA // zero: ChromeText
	RowH       unit.Dp     // minimum row height; 0 for none
}

// Label is a label in the form's label column.
func (f Form) Label(s string) layout.FlexChild {
	c := f.LabelColor
	if c == (color.NRGBA{}) {
		c = ChromeText
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(f.LabelW)
		return Middle(gtx, f.Th.Label(13, s, c, font.Normal).Layout)
	})
}

// Row lays children out left to right, centered vertically, with space
// below.
func (f Form) Row(children ...layout.FlexChild) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(f.RowH)
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

// Field is a row of a label and a control that takes the rest of the
// width.
func (f Form) Field(label string, w layout.Widget) layout.FlexChild {
	return f.Row(f.Label(label), layout.Flexed(1, w))
}

// Combo is a drop-down list w wide showing cur; apply gets the picked
// item's index.
func (f Form) Combo(dd *DropDown, w unit.Dp, cur string, items []DropItem, apply func(int)) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			if i := dd.Layout(gtx, f.Th, w, cur, items); i >= 0 {
				apply(i)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(w), gtx.Dp(24))}
		})
	})
}

// CheckBox is a check box with a label.
func (th *Theme) CheckBox(b *widget.Bool, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		cb := material.CheckBox(th.Theme, b, label)
		cb.Font = UIFont
		cb.TextSize = 13
		cb.Color = ChromeText
		cb.IconColor = th.Accent
		return cb.Layout(gtx)
	}
}

// Radio is a radio button of group e.
func (th *Theme) Radio(e *widget.Enum, key, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		rb := material.RadioButton(th.Theme, e, key, label)
		rb.Font = UIFont
		rb.TextSize = 13
		rb.Color = ChromeText
		rb.IconColor = th.Accent
		return rb.Layout(gtx)
	}
}

// TextField draws an editor in a bordered box w wide (at most the
// available width) and lines tall; hint shows while it is empty.
func TextField(gtx layout.Context, th *Theme, ed *widget.Editor, w unit.Dp, lines int, hint string) layout.Dimensions {
	pw := min(gtx.Dp(w), gtx.Constraints.Max.X)
	h := gtx.Dp(unit.Dp(8 + 18*lines))
	border := ChromeLine
	if gtx.Focused(ed) {
		border = th.Accent
	}
	FillRect(gtx.Ops, image.Rect(0, 0, pw, h), border)
	FillRect(gtx.Ops, image.Rect(1, 1, pw-1, h-1), colors.White)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(pw-gtx.Dp(12), 0), Max: image.Pt(pw-gtx.Dp(12), h-gtx.Dp(8))}
	cs := clip.Rect{Max: image.Pt(pw, h)}.Push(gtx.Ops)
	off := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(4))).Push(gtx.Ops)
	e := material.Editor(th.Theme, ed, hint)
	e.Font = UIFont
	e.TextSize = 13
	e.HintColor = MutedText
	e.Layout(g)
	off.Pop()
	cs.Pop()
	return layout.Dimensions{Size: image.Pt(pw, h)}
}
