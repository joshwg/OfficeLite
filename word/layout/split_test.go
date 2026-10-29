// split_test.go tests splitting a word item across lines.

package layout

import "testing"

// TestSplitItemInTrailingSpaces checks that the tail of an item split
// inside its trailing spaces keeps no more spaces than it has runes.
func TestSplitItemInTrailingSpaces(t *testing.T) {
	a := Item{Kind: ItemText, Text: "ab   ", N: 5, Adv: []float32{1, 1, 1, 1, 1}, W: 5, trail: 3, spaces: 3}
	for k := 1; k < 5; k++ {
		h, tl := splitItem(a, k)
		if h.spaces != 0 || h.trail != 0 {
			t.Errorf("k=%d head keeps spaces %d trail %v", k, h.spaces, h.trail)
		}
		if tl.spaces > tl.N || tl.trail > tl.W {
			t.Errorf("k=%d tail spaces %d of %d runes, trail %v of width %v", k, tl.spaces, tl.N, tl.trail, tl.W)
		}
	}
}

// TestClipY checks which rectangles a band keeps.
func TestClipY(t *testing.T) {
	for _, c := range []struct {
		r    Rect
		want bool
	}{
		{Rect{0, 10, 5, 20}, true},  // inside
		{Rect{0, 0, 5, 20}, true},   // ends inside: clipped
		{Rect{0, 0, 5, 10}, false},  // ends where the band starts
		{Rect{0, 20, 5, 30}, false}, // starts where it ends
		{Rect{0, 10, 5, 10}, true},  // a line on the band's first edge
		{Rect{0, 20, 5, 20}, false}, // a line on the next band's first edge
	} {
		if _, ok := clipY(c.r, 10, 20); ok != c.want {
			t.Errorf("clipY(%+v, 10, 20) = %v, want %v", c.r, ok, c.want)
		}
	}
	if c := splitCase("ab cd"); len(c) != 1 || !c[0].lower {
		t.Errorf("splitCase(\"ab cd\") = %+v, want one lowercase stretch", c)
	}
}
