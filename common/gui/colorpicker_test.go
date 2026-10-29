// Tests for the ColorPicker: theme shades and picking a swatch.

package gui

import (
	"image"
	"image/color"
	"math"
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

// TestColorPickerCustomClosedOutside checks that dismissing the menu from
// More Colors... (a click outside, the arrow, Close) opens it on the
// palette next time, not on the custom color panel.
func TestColorPickerCustomClosedOutside(t *testing.T) {
	g := &pickerRig{th: NewTheme(NewFontLibrary(), ChromeText, ChromeText)}
	g.frame()
	arrow := g.p.dd.size
	arrowAt := func() (float32, float32) { return float32(RibbonRowHeight) + float32(arrow.X)/2, float32(arrow.Y) / 2 }
	for name, dismiss := range map[string]func(){
		"click outside": func() { g.click(500, 500) },
		"the arrow":     func() { g.click(arrowAt()) },
		"Close":         func() { g.p.dd.Close(); g.frame() },
	} {
		g.p.dd.open = true
		g.frame()
		g.p.custom.start(themeColors[5].c)
		g.frame()
		if !g.p.custom.open || !g.p.dd.IsOpen() {
			t.Fatalf("%s: More Colors... didn't open", name)
		}
		dismiss()
		if g.p.dd.IsOpen() {
			t.Fatalf("%s: the menu didn't close", name)
		}
		if g.p.custom.open {
			t.Errorf("%s: More Colors... still open after the menu closed", name)
		}
		g.p.dd.open = true // opened again
		g.frame()
		if g.p.custom.open {
			t.Errorf("%s: the menu reopened on More Colors...", name)
		}
		g.p.dd.Close()
		g.frame()
	}
}

// TestColorPickerSwatchTip checks that hovering a swatch in the open menu
// shows its tooltip at the swatch's own place, though nothing here tracks
// the pointer outside the menu (as in a ribbon, where the menu is drawn
// beyond the ribbon's area, and in a Combo, which has no ribbon).
func TestColorPickerSwatchTip(t *testing.T) {
	g := &pickerRig{th: NewTheme(NewFontLibrary(), ChromeText, ChromeText)}
	g.p.tips.force = true
	g.frame()
	arrow := g.p.dd.size
	btnW := float32(RibbonRowHeight)
	g.click(btnW+float32(arrow.X)/2, float32(arrow.Y)/2)
	if !g.p.dd.IsOpen() {
		t.Fatal("the arrow didn't open the picker")
	}
	x := btnW + 10 + 4*21 + 8 // the fifth theme color, Blue, Accent 1
	for y := float32(arrow.Y) + 4; y < float32(arrow.Y)+60; y += 2 {
		g.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		g.frame()
		g.frame()
		if g.p.tips.key != &g.p.cells[4] {
			continue
		}
		if r := g.p.tips.rect; r.Min.X != 10+4*21 || r.Dx() != 16 || r.Dy() != 16 {
			t.Errorf("tip anchored at %v, want the swatch at x %d, 16 wide", r, 10+4*21)
		}
		if g.p.tips.tip.Name != themeColors[4].name {
			t.Errorf("tip %q, want %q", g.p.tips.tip.Name, themeColors[4].name)
		}
		if g.p.tips.lastShow.IsZero() {
			t.Error("the tip was never drawn")
		}
		return
	}
	t.Fatal("hovering the swatch never reported it to the tipper")
}

// TestFromHSVWraps checks that hues outside [0, 1), negative ones
// included, give the color of the same hue within it.
func TestFromHSVWraps(t *testing.T) {
	for _, h := range []float64{0, 0.1, 0.5, 0.9, 0.999} {
		want := fromHSV(h, 0.8, 0.9)
		for _, shift := range []float64{-3, -1, 1, 2} {
			if got := fromHSV(h+shift, 0.8, 0.9); got != want {
				t.Errorf("fromHSV(%v) = %v, want %v (hue %v)", h+shift, got, want, h)
			}
		}
	}
	red := color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	for _, h := range []float64{0, 1, -1e-20, math.NaN()} {
		if got := fromHSV(h, 1, 1); got != red {
			t.Errorf("fromHSV(%v, 1, 1) = %v, want red", h, got)
		}
	}
	// A quarter turn back from red is violet-pink, not the default case's.
	if got := fromHSV(-0.25, 1, 1); got != (color.NRGBA{R: 128, G: 0, B: 255, A: 255}) {
		t.Errorf("fromHSV(-0.25, 1, 1) = %v", got)
	}
}
