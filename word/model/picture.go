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

// Decodable reports whether the picture is in a format that can be drawn.
func (p *Picture) Decodable() bool {
	_, _, err := image.DecodeConfig(bytes.NewReader(p.Data))
	return err == nil
}
