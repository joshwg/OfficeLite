// Package colors has the color parsing and mixing helpers shared by the
// Office formats.
package colors

import (
	"image/color"
	"strconv"
	"strings"
)

var (
	Black = color.NRGBA{A: 255}
	White = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	Transparent = color.NRGBA{}
)

// RGBA is a color with alpha.
func RGBA(r, g, b, a uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: a} }

// ParseHex parses "RRGGBB", "#RRGGBB" or "AARRGGBB" (alpha ignored, as
// Office does for cell and text colors), around any spaces.
func ParseHex(s string) (color.NRGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 8 {
		s = s[2:]
	}
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return RGB(uint32(v)), true
}

// Hex is c as "RRGGBB" (upper case, alpha dropped), as Office writes
// colors.
func Hex(c color.NRGBA) string {
	const digits = "0123456789ABCDEF"
	b := [6]byte{
		digits[c.R>>4], digits[c.R&15],
		digits[c.G>>4], digits[c.G&15],
		digits[c.B>>4], digits[c.B&15],
	}
	return string(b[:])
}

// HexOr is ParseHex with a fallback for unparseable input.
func HexOr(s string, fallback color.NRGBA) color.NRGBA {
	if c, ok := ParseHex(s); ok {
		return c
	}
	return fallback
}

// RGB builds an opaque color from 0xRRGGBB.
func RGB(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// Mix blends a toward b by frac (0 = a, 1 = b); the result is opaque.
func Mix(a, b color.NRGBA, frac float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac + 0.5) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 255}
}
