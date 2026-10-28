package gui

import (
	"image"
	"image/png"
	"os"
	"testing"

	"gioui.org/gpu/headless"
)

// TestTooltipShot renders a ribbon with a tip showing to the PNG named by
// OFFICELITE_TIPSHOT (skipped when unset), to eyeball the look.
func TestTooltipShot(t *testing.T) {
	out := os.Getenv("OFFICELITE_TIPSHOT")
	if out == "" {
		t.Skip("OFFICELITE_TIPSHOT not set")
	}
	g := newTipRig()
	size := image.Pt(500, 160)
	g.frame(size)
	g.move(20, 15)
	g.ribbon.tips.force = true
	g.frame(size)
	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer win.Release()
	if err := win.Frame(&g.ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
