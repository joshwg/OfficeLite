package gui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"gioui.org/gpu/headless"

	"officelite/common/colors"
)

// TestColorPickerShot renders the open picker (and More Colors...) to the
// PNGs named by OFFICELITE_PICKERSHOT (skipped when unset), to eyeball it.
func TestColorPickerShot(t *testing.T) {
	out := os.Getenv("OFFICELITE_PICKERSHOT")
	if out == "" {
		t.Skip("OFFICELITE_PICKERSHOT not set")
	}
	g := &pickerRig{th: NewTheme(NewFontLibrary(DefaultFontDirs()...), ChromeText, ChromeText)}
	g.p.addRecent(colorsFromHex(t, "8B4513"))
	for i, name := range []string{out, out + ".more.png"} {
		g.p.dd.open = true
		if i == 1 {
			g.p.custom.start(themeColors[5].c)
		}
		g.frame()
		g.frame()
		size := image.Pt(360, 420)
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Fatal(err)
		}
		if err := win.Frame(&g.ops); err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rectangle{Max: size})
		if err := win.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		win.Release()
		f, _ := os.Create(name)
		png.Encode(f, img)
		f.Close()
	}
}

func colorsFromHex(t *testing.T, s string) color.NRGBA {
	v, ok := colors.ParseHex(s)
	if !ok {
		t.Fatal(s)
	}
	return v
}
