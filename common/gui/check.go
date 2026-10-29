// The Theme's labeled check box row.

package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/colors"
)

// Check is a check box row with a label, for page options (e.g. the PDF
// export settings); the caller toggles its value when btn is clicked.
func (t *Theme) Check(gtx layout.Context, btn *widget.Clickable, label string, on bool) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return Hoverable(gtx, btn, color.NRGBA{}, HoverBG, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 5, Bottom: 5, Left: 4, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					n := gtx.Dp(16)
					box := image.Rect(0, 0, n, n)
					line := MutedText
					if on {
						FillRect(gtx.Ops, box, t.Accent)
						gtx := gtx
						gtx.Constraints = layout.Exact(box.Size())
						IconCheck.Layout(gtx, colors.White)
					} else {
						FillRect(gtx.Ops, box, line)
						FillRect(gtx.Ops, box.Inset(max(1, gtx.Dp(1))), colors.White)
					}
					return layout.Dimensions{Size: box.Size()}
				}),
				layout.Rigid(layout.Spacer{Width: 10}.Layout),
				layout.Rigid(t.Label(13, label, ChromeText, font.Normal).Layout),
			)
		})
	})
}
