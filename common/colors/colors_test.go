// Tests for the hex color parsing and formatting helpers.

package colors

import (
	"image/color"
	"testing"
)

// TestHex checks Hex formatting and that ParseHex accepts the Office hex
// forms and rejects malformed ones.
func TestHex(t *testing.T) {
	c := color.NRGBA{R: 0x0A, G: 0xBC, B: 0xF0, A: 0x80}
	if got := Hex(c); got != "0ABCF0" {
		t.Errorf("Hex = %q, want 0ABCF0", got)
	}
	want := RGB(0x0ABCF0)
	for _, s := range []string{"0ABCF0", "#0abcf0", "  #0ABCF0\n", "FF0ABCF0", Hex(c)} {
		if got, ok := ParseHex(s); !ok || got != want {
			t.Errorf("ParseHex(%q) = %v, %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "#", "0ABCF", "+ABCF0", "GGGGGG"} {
		if _, ok := ParseHex(s); ok {
			t.Errorf("ParseHex(%q) ok", s)
		}
	}
}

// TestMixClamps checks that a fraction outside [0, 1] gives the nearer
// end instead of overflowing a channel.
func TestMixClamps(t *testing.T) {
	a, b := color.NRGBA{R: 10, G: 200, B: 0, A: 7}, color.NRGBA{R: 250, G: 20, B: 255, A: 9}
	for frac, want := range map[float64]color.NRGBA{
		-0.5: {R: 10, G: 200, B: 0, A: 255}, 0: {R: 10, G: 200, B: 0, A: 255},
		1: {R: 250, G: 20, B: 255, A: 255}, 3: {R: 250, G: 20, B: 255, A: 255},
		0.5: {R: 130, G: 110, B: 128, A: 255},
	} {
		if got := Mix(a, b, frac); got != want {
			t.Errorf("Mix(frac %v) = %v, want %v", frac, got, want)
		}
	}
}
