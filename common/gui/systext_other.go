//go:build !windows

package gui

import (
	"image/color"

	"gioui.org/op"
	"gioui.org/text"
)

func paintSystemGlyphs(*op.Ops, *text.Shaper, []text.Glyph, float32, float32, color.NRGBA) bool {
	return false
}
