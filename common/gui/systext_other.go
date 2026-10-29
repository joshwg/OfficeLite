// System text rendering stub for Linux and macOS: Gio draws all text.

//go:build !windows

package gui

import (
	"image/color"

	"gioui.org/op"
	"gioui.org/text"
)

// paintSystemGlyphs always reports false here, leaving the glyphs to Gio.
func paintSystemGlyphs(*op.Ops, *text.Shaper, []text.Glyph, float32, float32, color.NRGBA) bool {
	return false
}
