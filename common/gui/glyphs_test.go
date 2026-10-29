// Tests for TextLabel, the Label that paints through PaintGlyphs.

package gui

import (
	"image"
	"testing"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"
)

// TestTextLabelMaxLinesTrailingNewline checks that text ending in a
// newline on the last line MaxLines allows is no taller than the same text
// without it, that MaxLines still limits the lines, and that a trailing
// newline never adds a line's height (the empty line it starts has no
// width, so it adds nothing to the bounds).
func TestTextLabelMaxLinesTrailingNewline(t *testing.T) {
	sh := NewFontLibrary().Shaper()
	var ops op.Ops
	height := func(txt string, maxLines int) int {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(400, 400)}}
		p := text.Parameters{Font: font.Font{Typeface: "Go"}, PxPerEm: fixed.I(16), MaxLines: maxLines}
		return TextLabel{}.Layout(gtx, sh, p, txt, ChromeText).Size.Y
	}
	one := height("A", 1)
	if one == 0 {
		t.Fatal("no height for one line")
	}
	for _, tt := range []struct {
		txt      string
		maxLines int
		lines    int // the height wanted, in lines
	}{
		{"A\n", 1, 1},
		{"A\nB\n", 2, 2},
		{"A\nB\nC", 2, 2},
		{"A\n", 0, 1}, // no limit: the empty second line has no width, so no height
		{"A\nB", 0, 2},
	} {
		got := height(tt.txt, tt.maxLines)
		want := one * tt.lines
		if got < want-2*tt.lines || got > want+2*tt.lines { // a pixel or two of rounding per line
			t.Errorf("%q with MaxLines %d: height %d, want about %d (%d lines of %d)", tt.txt, tt.maxLines, got, want, tt.lines, one)
		}
	}
}
