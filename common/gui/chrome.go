// Shared window chrome widgets: the accent title bar and the status bar's
// zoom control.

package gui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
)

// TitleBar draws the 36dp accent bar shared by the apps: app name, AutoSave
// switch, and the document name with its modified dot and save status.
func TitleBar(gtx layout.Context, th *Theme, accent color.NRGBA, appName, docName string, modified bool, status string, autoSave layout.Widget) layout.Dimensions {
	h := gtx.Dp(36)
	FillRect(gtx.Ops, image.Rect(0, 0, gtx.Constraints.Max.X, h), accent)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	name := docName
	if modified {
		name += " •"
	}
	if status != "" {
		name += " – " + status
	}
	return layout.Inset{Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(th.Label(15, appName, colors.White, font.Bold).Layout),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(autoSave),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Flexed(1, th.Label(13, name, colors.White, font.Normal).Layout),
			)
		})
	})
}

// ZoomControl is the status bar's "−  100%  +" zoom control.
type ZoomControl struct {
	outBtn, resetBtn, inBtn widget.Clickable
}

// Layout draws the zoom control and reports any new zoom factor requested by
// clicking −, the percentage (reset to 1.0), or +.
func (z *ZoomControl) Layout(gtx layout.Context, th *Theme, zoom float64) (float64, bool, layout.Dimensions) {
	next, changed := zoom, false
	for z.inBtn.Clicked(gtx) {
		next, changed = next+0.1, true
	}
	for z.outBtn.Clicked(gtx) {
		next, changed = next-0.1, true
	}
	for z.resetBtn.Clicked(gtx) {
		next, changed = 1, true
	}
	btn := func(c *widget.Clickable, s string, w int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Hoverable(gtx, c, colors.Transparent, HoverBG, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(w)), gtx.Constraints.Max.Y))
				l := th.Label(12, s, ChromeText, font.Normal)
				l.Alignment = text.Middle
				return layout.Center.Layout(gtx, l.Layout)
			})
		})
	}
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		btn(&z.outBtn, "−", 22),
		btn(&z.resetBtn, fmt.Sprintf("%d%%", int(zoom*100+0.5)), 44),
		btn(&z.inBtn, "+", 22),
	)
	return next, changed, dims
}
