// Icon: IconVG icons with per-size, per-color cached renderings, and the
// Material Design icons used by the chrome.

package gui

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"golang.org/x/exp/shiny/iconvg"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Material Design icons (Apache 2.0) used by the chrome.
var (
	IconBack   = mustIcon(icons.NavigationArrowBack)
	IconHome   = mustIcon(icons.ActionHome)
	IconNew    = mustIcon(icons.ContentAdd)
	IconOpen   = mustIcon(icons.FileFolderOpen)
	IconFolder = mustIcon(icons.FileFolder)
	IconInfo   = mustIcon(icons.ActionInfo)
	IconSave   = mustIcon(icons.ContentSave)
	IconPrint  = mustIcon(icons.ActionPrint)
	IconExport = mustIcon(icons.FileFileUpload)
	IconClose  = mustIcon(icons.NavigationClose)
	IconCheck  = mustIcon(icons.NavigationCheck)
	IconFile   = mustIcon(icons.EditorInsertDriveFile)
	IconText   = mustIcon(icons.ActionDescription)
	IconGrid   = mustIcon(icons.ImageGridOn)
)

// Icon is an IconVG icon. widget.Icon keeps one rendering and redraws
// it whenever the size or color changes, and is not safe for concurrent
// use; the icons here are shared by every window (each drawn on its own
// goroutine) and drawn at several sizes and colors a frame, so Icon keeps
// a rendering per size and color behind a lock.
type Icon struct {
	src []byte

	mu     sync.Mutex
	images map[iconKey]paint.ImageOp
}

type iconKey struct {
	size  int
	color color.NRGBA
}

// maxIconImages bounds an icon's renderings; past it they start over.
const maxIconImages = 32

// NewIcon returns an icon drawn from IconVG data.
func NewIcon(data []byte) (*Icon, error) {
	if _, err := iconvg.DecodeMetadata(data); err != nil {
		return nil, err
	}
	return &Icon{src: data}, nil
}

// mustIcon is NewIcon for built-in data; it panics on an error.
func mustIcon(data []byte) *Icon {
	ic, err := NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

// Layout draws the icon in color, as widget.Icon does: a square as wide
// as the minimum constraint (24dp when zero).
func (ic *Icon) Layout(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	sz := gtx.Constraints.Min.X
	if sz == 0 {
		sz = gtx.Dp(24)
	}
	size := gtx.Constraints.Constrain(image.Pt(sz, sz))
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	img := ic.image(size.X, c)
	img.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: img.Size()}
}

// image returns the icon rasterized sz pixels wide in color c, rendering
// and caching it on first use.
func (ic *Icon) image(sz int, c color.NRGBA) paint.ImageOp {
	k := iconKey{sz, c}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if img, ok := ic.images[k]; ok {
		return img
	}
	m, _ := iconvg.DecodeMetadata(ic.src)
	dx, dy := m.ViewBox.AspectRatio()
	rgba := image.NewRGBA(image.Rectangle{Max: image.Pt(sz, int(float32(sz)*dy/dx))})
	var r iconvg.Rasterizer
	r.SetDstImage(rgba, rgba.Bounds(), draw.Src)
	m.Palette[0] = linearRGBA(c)
	iconvg.Decode(&r, ic.src, &iconvg.DecodeOptions{Palette: &m.Palette})
	img := paint.NewImageOp(rgba)
	if ic.images == nil || len(ic.images) >= maxIconImages {
		ic.images = map[iconKey]paint.ImageOp{}
	}
	ic.images[k] = img
	return img
}

// linearRGBA is Gio's f32color.NRGBAToLinearRGBA, the palette color
// widget.Icon draws with: translucent colors go to linear light.
func linearRGBA(c color.NRGBA) color.RGBA {
	if c.A == 0xff {
		return color.RGBA(c)
	}
	lin := func(v uint8) uint8 {
		f := float64(v) / 0xff
		if f <= 0.04045 {
			f /= 12.92
		} else {
			f = math.Pow((f+0.055)/1.055, 2.4)
		}
		a := float64(c.A) / 0xff
		return uint8(f*a*0xff + .5)
	}
	return color.RGBA{R: lin(c.R), G: lin(c.G), B: lin(c.B), A: c.A}
}
