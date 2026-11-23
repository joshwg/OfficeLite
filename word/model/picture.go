// Pictures: the encoded image an opaque run shows, decoded once on demand,
// and the run that inserts one.

package model

import (
	"bytes"
	"image"
	"path/filepath"
	"strings"
	"sync"

	// Decoders for the picture formats Word documents hold.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Picture is the image shown by an opaque run (Opaque.Picture): the
// encoded image file as stored in the document. Pictures are immutable
// and shared between copies of a run.
type Picture struct {
	Data []byte // the image file's bytes (PNG, JPEG, GIF, BMP...)
	Ext  string // file extension without the dot, lower case: "png", "jpeg", "gif", "bmp"...
	Name string // original file name or description, "" if unknown

	once sync.Once
	img  image.Image
	err  error
}

// Image decodes the picture once and returns it; formats Go can't decode
// (EMF, WMF...) return an error and are shown as placeholders.
func (p *Picture) Image() (image.Image, error) {
	p.once.Do(func() {
		p.img, _, p.err = image.Decode(bytes.NewReader(p.Data))
	})
	return p.img, p.err
}

// ContentType is the picture's MIME type.
func (p *Picture) ContentType() string {
	switch p.Ext {
	case "jpg", "jpeg", "jpe":
		return "image/jpeg"
	case "tif", "tiff":
		return "image/tiff"
	case "emf":
		return "image/x-emf"
	case "wmf":
		return "image/x-wmf"
	case "svg":
		return "image/svg+xml"
	}
	return "image/" + p.Ext
}

// PictureExt normalizes a file name's extension for Picture.Ext.
func PictureExt(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "jpg" || ext == "jpe" {
		ext = "jpeg"
	}
	if ext == "tif" {
		ext = "tiff"
	}
	return ext
}

// NewPicture checks that data is an image Go can decode and returns it
// with its natural size in points (at 96 dpi, as Word inserts pictures
// without resolution information).
func NewPicture(data []byte, name string) (*Picture, float64, float64, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	ext := format
	if ext == "" {
		ext = PictureExt(name)
	}
	p := &Picture{Data: data, Ext: ext, Name: filepath.Base(name)}
	return p, float64(cfg.Width) * 0.75, float64(cfg.Height) * 0.75, nil
}

// PictureRun returns an opaque run showing pic at w × h points.
func PictureRun(pic *Picture, w, h float64) Run {
	return Run{Kind: RunOpaque, Text: "[picture]", Opaque: &Opaque{Width: w, Height: h, Picture: pic}}
}

// PictureLook is how a picture is shown besides its size and border: the
// parts cut off its edges and the text that describes it.
type PictureLook struct {
	// Crop is the fraction of the image cut off at the left, top, right
	// and bottom (each 0 to below 1), as Word's a:srcRect; the rest fills
	// the picture's box.
	Crop [4]float64
	// Alt is the alternative text read out in place of the picture;
	// Decorative marks a picture that needs none.
	Alt        string
	Decorative bool
}

// Cropped reports whether any edge is cut off.
func (l PictureLook) Cropped() bool { return l.Crop != [4]float64{} }

// Uncropped returns where the whole image goes (x, y, w, h) when the
// picture's box is x, y, w × h: the cropped part fills the box and the
// rest lies outside it, to be clipped.
func (l PictureLook) Uncropped(x, y, w, h float64) (float64, float64, float64, float64) {
	c := l.Crop
	fx, fy := 1-c[0]-c[2], 1-c[1]-c[3]
	if fx <= 0 || fy <= 0 {
		return x, y, w, h
	}
	fw, fh := w/fx, h/fy
	return x - c[0]*fw, y - c[1]*fh, fw, fh
}

// NaturalSize is the picture's size in points at 96 dpi (as Word inserts
// pictures without resolution information), or false when its data
// isn't an image Go can read.
func (p *Picture) NaturalSize() (float64, float64, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(p.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return float64(cfg.Width) * 0.75, float64(cfg.Height) * 0.75, true
}

// Decodable reports whether the picture is in a format that can be drawn.
func (p *Picture) Decodable() bool {
	_, _, err := image.DecodeConfig(bytes.NewReader(p.Data))
	return err == nil
}
