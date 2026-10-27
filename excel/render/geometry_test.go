package render

import (
	"testing"

	"officelite/common/gui"
	"officelite/excel/model"
)

// Excel's digit widths for common default fonts. Fonts that are not
// installed are skipped (the fallback face would give a different width).
func TestDigitWidth(t *testing.T) {
	lib := gui.NewFontLibrary(append([]string{"../../Fonts"}, gui.DefaultFontDirs()...)...)
	for _, c := range []struct {
		family string
		pt     float32
		want   float64
	}{
		{"Arial", 10, 7},
		{"Calibri", 11, 7},
		{"Aptos Narrow", 11, 7},
		{"Verdana", 12, 10},
	} {
		if len(lib.Faces(c.family)) == 0 {
			t.Logf("%s not installed; skipped", c.family)
			continue
		}
		got := DigitWidth(lib.Shaper(c.family), model.Font{Family: c.family, SizePt: c.pt})
		if got != c.want {
			t.Errorf("DigitWidth(%s %vpt) = %v, want %v", c.family, c.pt, got, c.want)
		}
	}
}
