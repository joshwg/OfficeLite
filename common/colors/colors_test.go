package colors

import (
	"image/color"
	"testing"
)

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
