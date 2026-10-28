package gui

import (
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"os"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// DirectWrite text: Gio fills glyph outlines with plain grayscale
// antialiasing, no hinting and blending in linear light, which makes
// small text soft and light next to Word's. Here the glyphs Gio shaped
// (same faces, glyph IDs and positions) are rasterized by DirectWrite
// instead: hinted, at subpixel positions, with Windows' contrast
// enhancement, and the coverage corrected so that blending in linear
// light comes close to the weight of Windows' own blending. Text is
// grayscale antialiased, as Word draws it. Each run's image is cached.
//
// Word also snaps stems to whole pixels, which needs glyphs placed at
// whole-pixel (hinted) advances; with Gio's natural advances that leaves
// uneven gaps, so the default is DirectWrite's natural symmetric mode:
// hinted vertically, positioned to a quarter pixel horizontally.
//
// For comparing, OFFICELITE_TEXT selects the mode: "symmetric" (default),
// "natural", "gdi" (GDI classic), "gdinatural" or "gio" (Gio's own
// outlines). OFFICELITE_TEXT_CONTRAST (default 0.5) sets the contrast
// enhancement, OFFICELITE_TEXT_LINEAR (0 to 1, default 0.5) how far the
// coverage is corrected for blending in linear light.

var dw struct {
	once     sync.Once
	mu       sync.Mutex
	factory  unsafe.Pointer // IDWriteFactory
	mode     uintptr        // DWRITE_RENDERING_MODE
	contrast float64
	linear   float64                     // how far to correct for Gio's linear-light blending
	faces    map[FaceFile]unsafe.Pointer // IDWriteFontFace; nil if it failed
	cur, old map[string]*runImage
}

type runImage struct {
	img  paint.ImageOp
	off  image.Point // of the image's top left from the pen's pixel
	size image.Point // zero when the run has no ink
}

const (
	dwriteRenderingModeGDIClassic       = 2
	dwriteRenderingModeGDINatural       = 3
	dwriteRenderingModeNatural          = 4
	dwriteRenderingModeNaturalSymmetric = 5
	dwriteTextureAliased1x1             = 0
	dwriteMeasuringModeNatural          = 0
	dwriteGridFitModeEnabled            = 2
	dwriteTextAntialiasModeGrayscale    = 1

	// cacheGen is how many run images a cache generation holds; images
	// unused for a whole generation are dropped.
	cacheGen = 4000
)

// IDWriteFactory2 {0439fc60-ca44-4994-8dee-3a9af7b732ec} (Windows 8.1+),
// for grayscale glyph run analysis.
var iidDWriteFactory2 = syscall.GUID{Data1: 0x0439fc60, Data2: 0xca44, Data3: 0x4994,
	Data4: [8]byte{0x8d, 0xee, 0x3a, 0x9a, 0xf7, 0xb7, 0x32, 0xec}}

// Vtable slots (IUnknown's three come first).
const (
	mRelease                  = 2
	mCreateFontFileReference  = 7  // IDWriteFactory
	mCreateFontFace           = 9  // IDWriteFactory
	mCreateGlyphRunAnalysis2  = 30 // IDWriteFactory2
	mAnalyze                  = 5  // IDWriteFontFile
	mGetAlphaTextureBounds    = 3  // IDWriteGlyphRunAnalysis
	mCreateAlphaTexture       = 4  // IDWriteGlyphRunAnalysis
	dwriteFactoryTypeShared   = 0
	dwriteFontSimulationsNone = 0
)

// comCall calls method m of the COM object obj.
func comCall(obj unsafe.Pointer, m int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, m*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func f32arg(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func dwInit() {
	dw.mode = dwriteRenderingModeNaturalSymmetric
	switch os.Getenv("OFFICELITE_TEXT") {
	case "gio":
		return
	case "natural":
		dw.mode = dwriteRenderingModeNatural
	case "gdi":
		dw.mode = dwriteRenderingModeGDIClassic
	case "gdinatural":
		dw.mode = dwriteRenderingModeGDINatural
	}
	dw.contrast = 0.5
	if v, err := strconv.ParseFloat(os.Getenv("OFFICELITE_TEXT_CONTRAST"), 64); err == nil {
		dw.contrast = v
	}
	dw.linear = 0.5
	if v, err := strconv.ParseFloat(os.Getenv("OFFICELITE_TEXT_LINEAR"), 64); err == nil {
		dw.linear = v
	}
	proc := syscall.NewLazyDLL("dwrite.dll").NewProc("DWriteCreateFactory")
	if proc.Find() != nil {
		return
	}
	var f unsafe.Pointer
	if hr, _, _ := proc.Call(dwriteFactoryTypeShared, uintptr(unsafe.Pointer(&iidDWriteFactory2)), uintptr(unsafe.Pointer(&f))); hr != 0 || f == nil {
		return
	}
	dw.factory = f
	dw.faces = map[FaceFile]unsafe.Pointer{}
	dw.cur, dw.old = map[string]*runImage{}, map[string]*runImage{}
}

// fontFace returns the DirectWrite face for ff, loading it on first use.
func fontFace(ff FaceFile) unsafe.Pointer {
	if face, ok := dw.faces[ff]; ok {
		return face
	}
	dw.faces[ff] = nil
	path, err := syscall.UTF16PtrFromString(ff.Path)
	if err != nil {
		return nil
	}
	var file unsafe.Pointer
	if comCall(dw.factory, mCreateFontFileReference, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&file))) != 0 || file == nil {
		return nil
	}
	defer comCall(file, mRelease)
	var supported int32
	var fileType, faceType, n uint32
	if comCall(file, mAnalyze, uintptr(unsafe.Pointer(&supported)), uintptr(unsafe.Pointer(&fileType)),
		uintptr(unsafe.Pointer(&faceType)), uintptr(unsafe.Pointer(&n))) != 0 || supported == 0 || uint32(ff.Index) >= n {
		return nil
	}
	files := [1]unsafe.Pointer{file}
	var face unsafe.Pointer
	if comCall(dw.factory, mCreateFontFace, uintptr(faceType), 1, uintptr(unsafe.Pointer(&files[0])),
		uintptr(ff.Index), dwriteFontSimulationsNone, uintptr(unsafe.Pointer(&face))) != 0 {
		return nil
	}
	dw.faces[ff] = face
	return face
}

func paintSystemGlyphs(ops *op.Ops, sh *text.Shaper, gs []text.Glyph, x, y float32, c color.NRGBA) bool {
	dw.once.Do(dwInit)
	if dw.factory == nil {
		return false
	}
	files := faceFiles(sh)
	if files == nil {
		return false
	}
	ink := make([]text.Glyph, 0, len(gs))
	for _, g := range gs {
		if g.Flags&text.FlagTowardOrigin != 0 {
			return false // right-to-left: leave it to Gio
		}
		if _, _, gid := splitGlyphID(g.ID); gid > math.MaxUint16 {
			return false
		}
		// Skip the shaper's stand-in for a line break; it has no ink.
		if g.Flags&text.FlagParagraphBreak == 0 {
			ink = append(ink, g)
		}
	}
	gs = ink
	dw.mu.Lock()
	defer dw.mu.Unlock()
	// Glyphs from one face at one size make a DirectWrite glyph run.
	for len(gs) > 0 {
		ppem, faceIdx, _ := splitGlyphID(gs[0].ID)
		n := 1
		for n < len(gs) {
			p, f, _ := splitGlyphID(gs[n].ID)
			if p != ppem || f != faceIdx {
				break
			}
			n++
		}
		run := gs[:n]
		gs = gs[n:]
		var face unsafe.Pointer
		if faceIdx < len(files) && files[faceIdx].Path != "" {
			face = fontFace(files[faceIdx])
		}
		if face == nil || !paintRun(ops, face, ppem, run, x, y, c) {
			paintOutlines(ops, sh, run, x, y, c)
		}
	}
	return true
}

// splitGlyphID unpacks Gio's text.GlyphID (gioui.org/text/shaper.go):
// glyph index in the low 32 bits, then 16 bits of size (26.6 pixels per
// em) and 16 of face index.
func splitGlyphID(id text.GlyphID) (ppem fixed.Int26_6, face int, gid uint32) {
	return fixed.Int26_6((id >> 32) & 0xFFFF), int(uint64(id) >> 48), uint32(id)
}

// dwriteGlyphRun is DWRITE_GLYPH_RUN.
type dwriteGlyphRun struct {
	fontFace      unsafe.Pointer
	fontEmSize    float32
	glyphCount    uint32
	glyphIndices  *uint16
	glyphAdvances *float32
	glyphOffsets  *[2]float32 // DWRITE_GLYPH_OFFSET: advance, ascender
	isSideways    int32
	bidiLevel     uint32
}

func paintRun(ops *op.Ops, face unsafe.Pointer, ppem fixed.Int26_6, gs []text.Glyph, x, y float32, c color.NRGBA) bool {
	// The pen starts on a whole pixel; the run keeps its fraction, in
	// quarter pixels, as DirectWrite positions glyphs.
	pen := float64(x) + float64(gs[0].X)/64
	px := math.Floor(pen)
	frac := math.Round((pen-px)*4) / 4
	py := math.Round(float64(y))

	var key []byte
	key = binary.LittleEndian.AppendUint64(key, uint64(uintptr(face)))
	key = binary.LittleEndian.AppendUint32(key, uint32(ppem))
	key = append(key, c.R, c.G, c.B, c.A, byte(frac*4))
	idx := make([]uint16, 0, len(gs))
	adv := make([]float32, 0, len(gs))
	offs := make([][2]float32, 0, len(gs))
	for i, g := range gs {
		gid := uint16(g.ID)
		next := g.X + g.Advance
		if i+1 < len(gs) {
			next = gs[i+1].X
		}
		idx = append(idx, gid)
		adv = append(adv, float32(next-g.X)/64)
		offs = append(offs, [2]float32{-float32(g.Offset.X) / 64, float32(g.Offset.Y) / 64})
		key = binary.LittleEndian.AppendUint16(key, gid)
		key = binary.LittleEndian.AppendUint32(key, uint32(g.X-gs[0].X))
		key = binary.LittleEndian.AppendUint32(key, uint32(g.Offset.X))
		key = binary.LittleEndian.AppendUint32(key, uint32(g.Offset.Y))
	}
	k := string(key)
	ri := dw.cur[k]
	if ri == nil {
		if ri = dw.old[k]; ri == nil {
			ri = rasterize(face, float32(ppem)/64, idx, adv, offs, float32(frac), c)
			if ri == nil {
				return false
			}
		}
		if len(dw.cur) >= cacheGen {
			dw.old, dw.cur = dw.cur, map[string]*runImage{}
		}
		dw.cur[k] = ri
	}
	if ri.size == (image.Point{}) {
		return true
	}
	at := image.Pt(int(px), int(py)).Add(ri.off)
	t := op.Offset(at).Push(ops)
	cl := clip.Rect{Max: ri.size}.Push(ops)
	ri.img.Add(ops)
	paint.PaintOp{}.Add(ops)
	cl.Pop()
	t.Pop()
	return true
}

// rasterize draws a glyph run with its origin at (frac, 0) and returns it
// as an image in color c.
func rasterize(face unsafe.Pointer, size float32, idx []uint16, adv []float32, offs [][2]float32, frac float32, c color.NRGBA) *runImage {
	run := dwriteGlyphRun{
		fontFace:      face,
		fontEmSize:    size,
		glyphCount:    uint32(len(idx)),
		glyphIndices:  &idx[0],
		glyphAdvances: &adv[0],
		glyphOffsets:  &offs[0],
	}
	var an unsafe.Pointer
	if comCall(dw.factory, mCreateGlyphRunAnalysis2, uintptr(unsafe.Pointer(&run)), 0, dw.mode,
		dwriteMeasuringModeNatural, dwriteGridFitModeEnabled, dwriteTextAntialiasModeGrayscale,
		f32arg(frac), f32arg(0), uintptr(unsafe.Pointer(&an))) != 0 || an == nil {
		return nil
	}
	defer comCall(an, mRelease)
	var r struct{ left, top, right, bottom int32 }
	if comCall(an, mGetAlphaTextureBounds, dwriteTextureAliased1x1, uintptr(unsafe.Pointer(&r))) != 0 {
		return nil
	}
	w, h := int(r.right-r.left), int(r.bottom-r.top)
	if w <= 0 || h <= 0 {
		return &runImage{}
	}
	buf := make([]byte, w*h)
	if comCall(an, mCreateAlphaTexture, dwriteTextureAliased1x1, uintptr(unsafe.Pointer(&r)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))) != 0 {
		return nil
	}
	lut := coverageLUT(c)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		a := uint32(lut[buf[i]]) * uint32(c.A) / 255
		p := img.Pix[4*i : 4*i+4 : 4*i+4]
		p[0] = uint8((uint32(c.R)*a + 127) / 255)
		p[1] = uint8((uint32(c.G)*a + 127) / 255)
		p[2] = uint8((uint32(c.B)*a + 127) / 255)
		p[3] = uint8(a)
	}
	op := paint.NewImageOp(img)
	op.Filter = paint.FilterNearest
	return &runImage{img: op, off: image.Pt(int(r.left), int(r.top)), size: image.Pt(w, h)}
}

// coverageLUT maps glyph coverage to the alpha to paint text of color c
// with. Windows thickens dark text by enhancing contrast (less for light
// text). Gio blends in linear light, which makes the edges of dark text
// on a light page lighter than blending in sRGB would, so the alpha moves
// (by dw.linear) toward the one that lands on the sRGB blend, taking the
// page to be white for dark text and black for light text.
func coverageLUT(c color.NRGBA) *[256]uint8 {
	lum := (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
	k := dw.contrast * min(max(4*(0.75-lum), 0), 1)
	var lut [256]uint8
	for i := range lut {
		a := float64(i) / 255
		a = a * (k + 1) / (a*k + 1)
		var b float64
		if lum < 0.5 {
			b = 1 - srgbToLinear(1-a)
		} else {
			b = srgbToLinear(a)
		}
		a += (b - a) * dw.linear
		lut[i] = uint8(math.Round(a * 255))
	}
	return &lut
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
