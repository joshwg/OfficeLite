// Command mkicon draws the OfficeLite icon and writes it out as a PNG, a
// Windows .ico and the .syso resource object that go build links into
// officelite.exe (Gio loads icon resource 1 for the window and taskbar;
// Explorer shows the same icon for the file).
//
//	go generate ./cmd/officelite    (runs: go run ../mkicon)
//
// The icon: two sheets of paper overlapping on the diagonal, a violet one
// with a single-line W and an amber one ruled in cells with an X, and a
// white feather across them for "lite". Below 32 pixels the feather and
// the cell lines are left out; they would only blur the letters.
package main

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// sizes are the images in the .ico and the resource, smallest first.
var sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

func main() {
	dir := flag.String("o", ".", "output directory")
	preview := flag.String("preview", "", "also write a PNG of every size on light and dark backgrounds")
	flag.Parse()

	var images []*image.RGBA
	for _, s := range sizes {
		images = append(images, Draw(s))
	}
	if *preview != "" {
		writePreview(*preview, images)
	}
	var big bytes.Buffer
	if err := png.Encode(&big, images[len(images)-1]); err != nil {
		log.Fatal(err)
	}
	entries := make([]iconImage, len(images))
	for i, img := range images {
		entries[i] = encodeIconImage(img)
	}
	syso, err := resourceObject(entries)
	if err != nil {
		log.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"officelite.png":          big.Bytes(),
		"officelite.ico":          icoFile(entries),
		"rsrc_windows_amd64.syso": syso,
	} {
		if err := os.WriteFile(filepath.Join(*dir, name), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// writePreview lays the images out side by side, each at its own size and
// magnified 4×, on white and on a dark gray, as Explorer and the taskbar
// show them.
func writePreview(path string, images []*image.RGBA) {
	const gap = 16
	w, h := gap, 0
	for _, img := range images {
		n := img.Bounds().Dx()
		zoom := 4
		if n >= 64 {
			zoom = 1
		}
		w += n*zoom + gap
		h = max(h, n*zoom)
	}
	h = 2*h + 3*gap
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, image.Rect(0, 0, w, h/2), image.White, image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(0, h/2, w, h), image.NewUniform(color.Gray{0x20}), image.Point{}, draw.Src)
	x := gap
	for _, img := range images {
		n := img.Bounds().Dx()
		zoom := 4
		if n >= 64 {
			zoom = 1
		}
		for _, y0 := range []int{gap, h/2 + gap/2} {
			for y := 0; y < n*zoom; y++ {
				for xx := 0; xx < n*zoom; xx++ {
					c := img.RGBAAt(xx/zoom, y/zoom)
					draw.Draw(out, image.Rect(x+xx, y0+y, x+xx+1, y0+y+1), image.NewUniform(c), image.Point{}, draw.Over)
				}
			}
		}
		x += n*zoom + gap
	}
	var b bytes.Buffer
	if err := png.Encode(&b, out); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
