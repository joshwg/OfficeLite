package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
)

// Chrome colors shared by the apps.
var (
	ChromeBG   = colors.RGB(0xF3F3F3)
	ChromeLine = colors.RGB(0xD4D4D4)
	ChromeText = colors.RGB(0x262626)
	MutedText  = colors.RGB(0x6E6E6E)
	HoverBG    = colors.RGB(0xE6E6E6)
	ErrorText  = colors.RGB(0xC42B1C)
)

// Theme is a material theme plus the app's accent colors (green for
// ExcelLite, blue for WordLite).
type Theme struct {
	*material.Theme
	Accent     color.NRGBA // title bar, Home tab, backstage nav
	AccentDark color.NRGBA // selected backstage page, buttons on Accent
	AccentTint color.NRGBA // selection on light backgrounds
}

// NewTheme returns a theme using the UI font.
func NewTheme(fonts *FontLibrary, accent, accentDark color.NRGBA) *Theme {
	th := material.NewTheme()
	th.Shaper = fonts.Shaper("Segoe UI", "Tahoma", "Calibri")
	th.Face = UIFont.Typeface
	th.Palette.ContrastBg = accent
	return &Theme{Theme: th, Accent: accent, AccentDark: accentDark, AccentTint: colors.Mix(accent, colors.White, 0.85)}
}

// Label is a single-line UI label.
func (t *Theme) Label(size float32, s string, c color.NRGBA, weight font.Weight) material.LabelStyle {
	l := material.Label(t.Theme, unit.Sp(size), s)
	l.Color = c
	l.Font = UIFont
	l.Font.Weight = weight
	l.MaxLines = 1
	return l
}

// Paragraph is wrapped body text.
func (t *Theme) Paragraph(s string, c color.NRGBA) layout.Widget {
	l := t.Label(13, s, c, font.Normal)
	l.MaxLines = 0
	return l.Layout
}

// Title is a page title as in Excel's File pages.
func (t *Theme) Title(s string) layout.Widget {
	l := t.Label(28, s, ChromeText, font.Light)
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 20}.Layout(gtx, l.Layout)
	}
}

// Heading is a section heading.
func (t *Theme) Heading(s string) layout.Widget {
	l := t.Label(15, s, ChromeText, font.SemiBold)
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 12, Bottom: 10}.Layout(gtx, l.Layout)
	}
}

// Hoverable lays out w inside btn, painting bg behind it (hover when the
// pointer is over it) and showing a hand cursor.
func Hoverable(gtx layout.Context, btn *widget.Clickable, bg, hover color.NRGBA, w layout.Widget) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		m := op.Record(gtx.Ops)
		d := w(gtx)
		call := m.Stop()
		c := bg
		if btn.Hovered() && gtx.Enabled() {
			c = hover
		}
		if c.A > 0 {
			FillRect(gtx.Ops, image.Rectangle{Max: d.Size}, c)
		}
		call.Add(gtx.Ops)
		if gtx.Enabled() {
			area := clip.Rect{Max: d.Size}.Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			area.Pop()
		}
		return d
	})
}

// Button is a filled accent button.
func (t *Theme) Button(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return Hoverable(gtx, btn, t.Accent, t.AccentDark, func(gtx layout.Context) layout.Dimensions {
		c := colors.White
		if !gtx.Enabled() {
			c = colors.Mix(c, t.Accent, 0.4)
		}
		l := t.Label(14, label, c, font.SemiBold)
		return layout.Inset{Top: 8, Bottom: 8, Left: 22, Right: 22}.Layout(gtx, l.Layout)
	})
}

// Option is a large selectable row with an icon, a title and a
// description, used for Open's Browse and the Save As / Export choices.
func (t *Theme) Option(gtx layout.Context, btn *widget.Clickable, icon *widget.Icon, title, desc string, selected bool) layout.Dimensions {
	bg := color.NRGBA{}
	if selected {
		bg = t.AccentTint
	}
	hover := HoverBG
	if selected {
		hover = bg
	}
	return Hoverable(gtx, btn, bg, hover, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = min(gtx.Dp(460), gtx.Constraints.Max.X)
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		textColor, iconColor := ChromeText, t.Accent
		if !gtx.Enabled() {
			textColor, iconColor = MutedText, MutedText
		}
		return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(32)
					return icon.Layout(gtx, iconColor)
				}),
				layout.Rigid(layout.Spacer{Width: 14}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(t.Label(14, title, textColor, font.Normal).Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if desc == "" {
								return layout.Dimensions{}
							}
							l := t.Label(12, desc, MutedText, font.Normal)
							l.MaxLines = 2
							return l.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
}

// Property is a "label   value" row as on the Info page.
func (t *Theme) Property(label, value string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(140)
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					return t.Label(13, label, MutedText, font.Normal).Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := t.Label(13, value, ChromeText, font.Normal)
					l.MaxLines = 3
					return l.Layout(gtx)
				}),
			)
		})
	}
}

// Middle lays out w centered vertically in the minimum height, with no
// minimum height of its own (layout.W keeps Min.Y, which top-aligns
// labels in fixed-height bars).
func Middle(gtx layout.Context, w layout.Widget) layout.Dimensions {
	h := gtx.Constraints.Min.Y
	gtx.Constraints.Min.Y = 0
	m := op.Record(gtx.Ops)
	d := w(gtx)
	call := m.Stop()
	dy := max(h-d.Size.Y, 0) / 2
	off := op.Offset(image.Pt(0, dy)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: image.Pt(d.Size.X, max(h, d.Size.Y)), Baseline: d.Baseline + max(h-d.Size.Y, 0) - dy}
}
