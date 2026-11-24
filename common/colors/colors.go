// Package colors has the color parsing and mixing helpers shared by the
// Office formats.
package colors

import (
	"image/color"
	"math"
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

// Mix blends a toward b by frac (0 = a, 1 = b, limited to that range); the
// result is opaque.
func Mix(a, b color.NRGBA, frac float64) color.NRGBA {
	frac = max(0, min(frac, 1))
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac + 0.5) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 255}
}

// ToHSL converts c's RGB (ignoring alpha) to hue, saturation and lightness,
// each in [0, 1].
func ToHSL(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

// FromHSL returns the opaque color of hue h, saturation s and lightness l,
// each in [0, 1]; l is clamped to that range.
func FromHSL(h, s, l float64) color.NRGBA {
	l = min(max(l, 0), 1)
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	ch := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(min(max(v, 0), 1) * 255))
	}
	return color.NRGBA{R: ch(h + 1.0/3), G: ch(h), B: ch(h - 1.0/3), A: 255}
}
