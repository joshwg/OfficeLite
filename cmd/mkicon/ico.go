// Icon images and the .ico file and RT_GROUP_ICON directory that list
// them.

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"log"
)

// iconImage is one image of an icon as stored in a .ico file or an RT_ICON
// resource: a PNG at 256 pixels, a 32-bit DIB below (which every Windows
// version reads).
type iconImage struct {
	size int
	data []byte
}

// encodeIconImage encodes img for an icon: as a PNG at 256 pixels and up,
// otherwise as a 32-bit DIB with its AND mask.
func encodeIconImage(img *image.RGBA) iconImage {
	n := img.Bounds().Dx()
	var b bytes.Buffer
	if n >= 256 {
		if err := png.Encode(&b, img); err != nil {
			log.Fatal(err)
		}
		return iconImage{n, b.Bytes()}
	}
	le := binary.LittleEndian
	// BITMAPINFOHEADER; the height counts the color and mask rows.
	hdr := make([]byte, 40)
	le.PutUint32(hdr[0:], 40)
	le.PutUint32(hdr[4:], uint32(n))
	le.PutUint32(hdr[8:], uint32(2*n))
	le.PutUint16(hdr[12:], 1)
	le.PutUint16(hdr[14:], 32)
	b.Write(hdr)
	// BGRA rows bottom-up, not premultiplied.
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			i := img.PixOffset(x, y)
			r, g, bl, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
			if a != 0 && a != 255 {
				r, g, bl = unpremul(r, a), unpremul(g, a), unpremul(bl, a)
			}
			b.Write([]byte{bl, g, r, a})
		}
	}
	// The AND mask: 1 bit per pixel, rows padded to 32 bits, set where
	// the pixel is fully transparent.
	stride := (n + 31) / 32 * 4
	for y := n - 1; y >= 0; y-- {
		row := make([]byte, stride)
		for x := 0; x < n; x++ {
			if img.Pix[img.PixOffset(x, y)+3] == 0 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		b.Write(row)
	}
	return iconImage{n, b.Bytes()}
}

// unpremul undoes alpha premultiplication of channel c, rounding.
func unpremul(c, a uint8) uint8 {
	return uint8(min(255, (int(c)*255+int(a)/2)/int(a)))
}

// dirEntry is the part of an ICONDIRENTRY / GRPICONDIRENTRY both share.
func dirEntry(img iconImage) []byte {
	e := make([]byte, 12)
	if img.size < 256 {
		e[0], e[1] = byte(img.size), byte(img.size) // 0 means 256
	}
	binary.LittleEndian.PutUint16(e[4:], 1)  // planes
	binary.LittleEndian.PutUint16(e[6:], 32) // bits per pixel
	binary.LittleEndian.PutUint32(e[8:], uint32(len(img.data)))
	return e
}

// iconDir is the ICONDIR header for count images.
func iconDir(count int) []byte {
	h := make([]byte, 6)
	binary.LittleEndian.PutUint16(h[2:], 1) // icon
	binary.LittleEndian.PutUint16(h[4:], uint16(count))
	return h
}

// icoFile returns a .ico file holding the images.
func icoFile(images []iconImage) []byte {
	var b bytes.Buffer
	b.Write(iconDir(len(images)))
	off := 6 + 16*len(images)
	for _, img := range images {
		b.Write(dirEntry(img))
		b.Write(binary.LittleEndian.AppendUint32(nil, uint32(off)))
		off += len(img.data)
	}
	for _, img := range images {
		b.Write(img.data)
	}
	return b.Bytes()
}

// groupIcon is the RT_GROUP_ICON resource for images stored as RT_ICON
// resources 1..n.
func groupIcon(images []iconImage) []byte {
	var b bytes.Buffer
	b.Write(iconDir(len(images)))
	for i, img := range images {
		b.Write(dirEntry(img))
		b.Write(binary.LittleEndian.AppendUint16(nil, uint16(i+1)))
	}
	return b.Bytes()
}
