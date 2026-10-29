// Toolbar buttons: formatting icons, the square ToolButton and
// GlyphButton, and the separator between groups.

package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"officelite/common/colors"
)

// Formatting icons for toolbars (Material Design, Apache 2.0).
var (
	IconBold         = mustIcon(icons.EditorFormatBold)
	IconItalic       = mustIcon(icons.EditorFormatItalic)
	IconUnderline    = mustIcon(icons.EditorFormatUnderlined)
	IconStrike       = mustIcon(icons.EditorFormatStrikethrough)
	IconAlignLeft    = mustIcon(icons.EditorFormatAlignLeft)
	IconAlignCenter  = mustIcon(icons.EditorFormatAlignCenter)
	IconAlignRight   = mustIcon(icons.EditorFormatAlignRight)
	IconAlignJustify = mustIcon(icons.EditorFormatAlignJustify)
	IconBullets      = mustIcon(icons.EditorFormatListBulleted)
	IconNumbering    = mustIcon(icons.EditorFormatListNumbered)
	IconIndentMore   = mustIcon(icons.EditorFormatIndentIncrease)
	IconIndentLess   = mustIcon(icons.EditorFormatIndentDecrease)
	IconLineSpacing  = mustIcon(icons.EditorFormatLineSpacing)
	IconClearFormat  = mustIcon(icons.EditorFormatClear)
	IconTextColor    = mustIcon(icons.EditorFormatColorText)
	IconUndo         = mustIcon(icons.ContentUndo)
	IconRedo         = mustIcon(icons.ContentRedo)
	IconContentCopy  = mustIcon(icons.ContentContentCopy)
	IconContentCut   = mustIcon(icons.ContentContentCut)
	IconContentPaste = mustIcon(icons.ContentContentPaste)
)

// ToolButton is a small square toolbar button showing an icon (or a short
// label when icon is nil), drawn pressed when checked. bar, when its alpha
// is set, is a color strip under the icon (font color).
func ToolButton(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *Icon, label string, checked bool, bar color.NRGBA) layout.Dimensions {
	bg := colors.Transparent
	if checked {
		bg = th.AccentTint
	}
	hover := HoverBG
	if checked {
		hover = colors.Mix(th.AccentTint, th.Accent, 0.15)
	}
	face := func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(28)
		w := size
		if icon == nil {
			w = 0
		}
		gtx.Constraints = layout.Constraints{Min: image.Pt(w, size), Max: image.Pt(gtx.Constraints.Max.X, size)}
		fg := ChromeText
		if !gtx.Enabled() {
			fg = colors.Mix(ChromeText, ChromeBG, 0.6)
		}
		if icon == nil {
			return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return Middle(gtx, th.Label(12, label, fg, font.Normal).Layout)
			})
		}
		is := gtx.Dp(18)
		ic := gtx
		ic.Constraints = layout.Exact(image.Pt(is, is))
		dy := (size - is) / 2
		if bar.A > 0 {
			dy -= gtx.Dp(2)
		}
		off := op.Offset(image.Pt((size-is)/2, dy)).Push(gtx.Ops)
		icon.Layout(ic, fg)
		off.Pop()
		if bar.A > 0 {
			y := dy + is
			FillRect(gtx.Ops, image.Rect((size-is)/2, y, (size+is)/2, y+gtx.Dp(4)), bar)
		}
		if checked {
			strokeRect(gtx.Ops, image.Rect(0, 0, size, size), colors.Mix(th.Accent, colors.White, 0.5))
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	}
	return Hoverable(gtx, btn, bg, hover, func(gtx layout.Context) layout.Dimensions {
		d := face(gtx)
		tipButton(gtx, btn, btn.Hovered(), d.Size)
		return d
	})
}

// ToolSeparator is a thin vertical divider between toolbar groups.
func ToolSeparator(gtx layout.Context) layout.Dimensions {
	w, h := gtx.Dp(13), gtx.Dp(28)
	VLine(gtx.Ops, w/2, gtx.Dp(4), h-gtx.Dp(4), 1, ChromeLine)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// GlyphButton is ToolButton showing one character drawn at icon size, for
// commands whose symbol is a character (Word's ¶).
func GlyphButton(gtx layout.Context, th *Theme, btn *widget.Clickable, glyph string, checked bool) layout.Dimensions {
	bg, hover := colors.Transparent, HoverBG
	if checked {
		bg, hover = th.AccentTint, colors.Mix(th.AccentTint, th.Accent, 0.15)
	}
	return Hoverable(gtx, btn, bg, hover, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(28)
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		fg := ChromeText
		if !gtx.Enabled() {
			fg = colors.Mix(ChromeText, ChromeBG, 0.6)
		}
		layout.Center.Layout(gtx, th.Label(17, glyph, fg, font.Normal).Layout)
		if checked {
			strokeRect(gtx.Ops, image.Rect(0, 0, size, size), colors.Mix(th.Accent, colors.White, 0.5))
		}
		// The tip's handler goes inside the button's area, as in
		// ToolButton: on top of it, it would take the button's clicks.
		tipButton(gtx, btn, btn.Hovered(), image.Pt(size, size))
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}
