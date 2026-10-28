package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // DrawImageData decodes PNGs
	"slices"
	"strconv"
)

// imageObj is an image XObject: raw samples (Flate) or a JPEG (DCT).
type imageObj struct {
	name       string
	obj        int
	w, h       int
	colorSpace string
	bits       int
	filter     string // "" (Flate) or "/DCTDecode"
	decode     string
	data       []byte
	alpha      []byte // soft mask samples (Flate), nil when opaque
}

func (p *Page) useImage(im *imageObj, x, y, w, h float64) {
	if !slices.Contains(p.imgs, im) {
		p.imgs = append(p.imgs, im)
	}
	p.Save()
	p.op(num(w), "0 0", num(h), num(x), num(p.y(y+h)), "cm")
	p.op("/"+im.name, "Do")
	p.Restore()
}

func (d *Document) addImage(im *imageObj) *imageObj {
	im.name = "Im" + strconv.Itoa(len(d.images)+1)
	d.images = append(d.images, im)
	return im
}

// Image is an image added to the document, which pages can draw any
// number of times.
type Image struct{ obj *imageObj }

// Size is the image's size in pixels.
func (im *Image) Size() (int, int) { return im.obj.w, im.obj.h }

// AddImage adds a decoded image (RGB samples, with a soft mask when it has
// transparency).
func (d *Document) AddImage(img image.Image) *Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgb := make([]byte, 0, 3*w*h)
	var alpha []byte
	opaque := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a != 0xFFFF {
				opaque = false
			}
			// Un-premultiply.
			if a > 0 && a < 0xFFFF {
				r, g, bl = r*0xFFFF/a, g*0xFFFF/a, bl*0xFFFF/a
			}
			rgb = append(rgb, byte(r>>8), byte(g>>8), byte(bl>>8))
			alpha = append(alpha, byte(a>>8))
		}
	}
	if opaque {
		alpha = nil
	}
	return &Image{d.addImage(&imageObj{w: w, h: h, colorSpace: "/DeviceRGB", bits: 8, data: rgb, alpha: alpha})}
}

// AddJPEG adds a JPEG file as is (DCTDecode), without decoding it.
func (d *Document) AddJPEG(data []byte) (*Image, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	im := &imageObj{w: cfg.Width, h: cfg.Height, bits: 8, filter: "/DCTDecode", data: data}
	switch comps := jpegComponents(data); comps {
	case 1:
		im.colorSpace = "/DeviceGray"
	case 4:
		// Adobe CMYK JPEGs store inverted values.
		im.colorSpace, im.decode = "/DeviceCMYK", "[1 0 1 0 1 0 1 0]"
	default:
		im.colorSpace = "/DeviceRGB"
	}
	return &Image{d.addImage(im)}, nil
}

// AddImageData adds an encoded image: JPEGs are embedded as they are,
// other formats (PNG, GIF, BMP... whatever is registered) are decoded.
func (d *Document) AddImageData(data []byte) (*Image, error) {
	if len(data) > 2 && data[0] == 0xFF && data[1] == 0xD8 {
		if im, err := d.AddJPEG(data); err == nil {
			return im, nil
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return d.AddImage(img), nil
}

// DrawImage draws im stretched to the rectangle.
func (p *Page) DrawImage(im *Image, x, y, w, h float64) {
	if im == nil || w <= 0 || h <= 0 {
		return
	}
	p.useImage(im.obj, x, y, w, h)
}

// jpegComponents reads the component count from a JPEG's SOF marker.
func jpegComponents(b []byte) int {
	for i := 2; i+9 < len(b); {
		if b[i] != 0xFF {
			i++
			continue
		}
		m := b[i+1]
		if m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7) || m == 0xFF {
			i++
			continue
		}
		l := int(b[i+2])<<8 | int(b[i+3])
		if m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC {
			return int(b[i+9])
		}
		i += 2 + l
	}
	return 3
}

func (o *objWriter) image(im *imageObj) {
	mask := ""
	if im.alpha != nil {
		m := o.alloc()
		o.stream(m, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 ", im.w, im.h), im.alpha, true)
		mask = " /SMask " + ref(m)
	}
	dict := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent %d%s ",
		im.w, im.h, im.colorSpace, im.bits, mask)
	if im.decode != "" {
		dict += "/Decode " + im.decode + " "
	}
	if im.filter != "" {
		o.stream(im.obj, "/Filter "+im.filter+" "+dict, im.data, false)
		return
	}
	o.stream(im.obj, dict, im.data, true)
}
