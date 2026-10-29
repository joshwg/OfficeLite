// Tests for the ColorPicker: theme shades and picking a swatch.

package gui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
)

// TestThemeShades checks themeShades against Excel's lighter/darker
// variants of a theme color, within one step of rounding.
func TestThemeShades(t *testing.T) {
	for _, tc := range []struct {
		base uint32
		want [5]uint32
	}{
		{0x4472C4, [5]uint32{0xD9E1F2, 0xB4C6E7, 0x8EA9DB, 0x305496, 0x203764}}, // Office 2013 Blue, Accent 1
		{0xFFFFFF, [5]uint32{0xF2F2F2, 0xD9D9D9, 0xBFBFBF, 0xA6A6A6, 0x808080}},
		{0x000000, [5]uint32{0x808080, 0x595959, 0x404040, 0x262626, 0x0D0D0D}},
	} {
		got, _ := themeShades(colors.RGB(tc.base))
		for i, w := range tc.want {
			wc := colors.RGB(w)
			d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
			if d(got[i].R, wc.R) > 1 || d(got[i].G, wc.G) > 1 || d(got[i].B, wc.B) > 1 {
				t.Errorf("%06X shade %d = %s, want #%06X", tc.base, i, hexOf(got[i]), w)
			}
		}
	}
}

type pickerRig struct {
	r      input.Router
	ops    op.Ops
	th     *Theme
	p      ColorPicker
	btn    widget.Clickable
	choice ColorChoice
	ok     bool
}

// frame lays out the picker's split button and records any color picked.
func (g *pickerRig) frame() {
	g.ops.Reset()
	gtx := layout.Context{Ops: &g.ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(600, 600)}, Source: g.r.Source(), Now: time.Unix(1e9, 0)}
	if c, ok, _ := g.p.SplitButton(gtx, g.th, &g.btn, IconFill, color.NRGBA{}, "No Fill"); ok {
		g.choice, g.ok = c, true
	}
	g.r.Frame(&g.ops)
}

// click presses and releases the primary button at (x, y), then runs two
// frames.
func (g *pickerRig) click(x, y float32) {
	for _, k := range []pointer.Kind{pointer.Press, pointer.Release} {
		e := pointer.Event{Kind: k, Source: pointer.Mouse, Position: f32.Pt(x, y)}
		if k == pointer.Press {
			e.Buttons = pointer.ButtonPrimary
		}
		g.r.Queue(e)
	}
	g.frame()
	g.frame()
}

// TestColorPickerPicks opens the picker with its arrow, clicks a theme
// swatch and checks that its color is returned and the menu closes.
func TestColorPickerPicks(t *testing.T) {
	g := &pickerRig{th: NewTheme(NewFontLibrary(), ChromeText, ChromeText)}
	g.frame()
	arrow := g.p.dd.size
	btnW := float32(RibbonRowHeight) // the color button before the arrow
	g.click(btnW+float32(arrow.X)/2, float32(arrow.Y)/2)
	if !g.p.dd.IsOpen() {
		t.Fatal("the arrow didn't open the picker")
	}
	// The panel starts below the arrow, at the arrow's left edge: padding
	// 10, swatches 16 wide 5 apart. Find the first theme swatch's row by
	// scanning down for a hit.
	x0 := btnW + 10 + 4*21 + 8 // 5th column: Blue, Accent 1
	for y := float32(arrow.Y) + 4; y < float32(arrow.Y)+60 && !g.ok; y += 2 {
		g.click(x0, y)
		if !g.p.dd.IsOpen() && !g.ok {
			t.Fatal("picker closed without a choice")
		}
	}
	if !g.ok || g.choice.None || g.choice.Color != themeColors[4].c {
		t.Fatalf("picked %+v (ok %v), want Blue, Accent 1", g.choice, g.ok)
	}
	if g.p.dd.IsOpen() {
		t.Error("picker still open after a pick")
	}
}
