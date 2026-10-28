package layout

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	xdraw "golang.org/x/image/draw"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// Pictures. An inline picture is an ItemObject whose Obj.Picture is set;
// it sits on the baseline and takes Obj.Width × Obj.Height points. Other
// renderers (PDF export) find them the same way: for each placed line pl
// of a page and each item it with it.Obj != nil && it.Obj.Picture != nil,
// the picture's box is PictureBox(pl, it), and it.Obj.Picture.Image()
// decodes it (it.Obj.Picture.Data holds the original file).

var (
	frameColor  = colors.RGB(0x7F7F7F)
	handleColor = colors.RGB(0xFFFFFF)
)

// picCache is a picture scaled to a size near the one it is drawn at.
type picCache struct {
	bucket int // width the image was scaled to (pixels)
	op     paint.ImageOp
}

// PictureBox returns the page rectangle (points) of a picture item on a
// placed line.
func PictureBox(pl *Placed, it *Item) Rect {
	base := pl.Y + pl.Line.Baseline
	x := pl.X + it.X
	return Rect{x, base - it.Obj.Height, x + it.Obj.Width, base}
}

// isPicture reports whether an item is a picture that can be drawn.
func isPicture(it *Item) bool {
	return it.Kind == ItemObject && it.Obj != nil && it.Obj.Picture != nil && it.Obj.Width > 0 && it.Obj.Height > 0
}

// pictureOp returns the picture as an image op for drawing w pixels wide:
// large images are scaled down first (to the next power of two above w),
// so they look smooth and don't fill the GPU's memory.
func (e *Engine) pictureOp(p *model.Picture, w int) (paint.ImageOp, bool) {
	img, err := p.Image()
	if err != nil || img.Bounds().Empty() {
		return paint.ImageOp{}, false
	}
	iw := img.Bounds().Dx()
	bucket := 16
	for bucket < w && bucket < 8192 {
		bucket *= 2
	}
	bucket = min(bucket, iw)
	if e.pics == nil {
		e.pics = map[*model.Picture]picCache{}
	}
	if c, ok := e.pics[p]; ok && c.bucket == bucket {
		return c.op, true
	}
	src := img
	if bucket < iw {
		ih := img.Bounds().Dy()
		h := max(1, int(math.Round(float64(ih)*float64(bucket)/float64(iw))))
		dst := image.NewRGBA(image.Rect(0, 0, bucket, h))
		xdraw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
		src = dst
	}
	io := paint.NewImageOp(src)
	io.Filter = paint.FilterLinear
	e.pics[p] = picCache{bucket: bucket, op: io}
	return io, true
}

// drawPicture draws a picture item with its bottom-left corner at (x,
// base) points; it reports false if the image can't be decoded (the
// placeholder is drawn instead).
func (e *Engine) drawPicture(ops *op.Ops, it *Item, x, base, s float64) bool {
	px := func(v float64) int { return int(math.Round(v * s)) }
	r := image.Rect(px(x), px(base-it.Obj.Height), px(x+it.Obj.Width), px(base))
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return true
	}
	io, ok := e.pictureOp(it.Obj.Picture, r.Dx())
	if !ok {
		return false
	}
	sz := io.Size()
	cl := clip.Rect(r).Push(ops)
	t := op.Affine(f32.AffineId().
		Scale(f32.Point{}, f32.Pt(float32(r.Dx())/float32(sz.X), float32(r.Dy())/float32(sz.Y))).
		Offset(f32.Pt(float32(r.Min.X), float32(r.Min.Y)))).Push(ops)
	io.Add(ops)
	paint.PaintOp{}.Add(ops)
	t.Pop()
	cl.Pop()
	return true
}

// drawPictureFrame outlines a selected picture with Word's sizing handles.
func (e *Engine) drawPictureFrame(ops *op.Ops, pg *Page, sel model.Selection, s float64) {
	a, b := sel.Ordered()
	if a.Para != b.Para || b.Off != a.Off+1 {
		return
	}
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		if pl.Para != a.Para || a.Off < pl.Line.Start || a.Off >= pl.Line.End {
			continue
		}
		for k := range pl.Line.Items {
			it := &pl.Line.Items[k]
			if it.Off != a.Off || !isPicture(it) {
				continue
			}
			box := PictureBox(pl, it)
			px := func(v float64) int { return int(math.Round(v * s)) }
			r := image.Rect(px(box.X0), px(box.Y0), px(box.X1), px(box.Y1))
			gui.HLine(ops, r.Min.X, r.Max.X, r.Min.Y, 1, frameColor)
			gui.HLine(ops, r.Min.X, r.Max.X, r.Max.Y-1, 1, frameColor)
			gui.VLine(ops, r.Min.X, r.Min.Y, r.Max.Y, 1, frameColor)
			gui.VLine(ops, r.Max.X-1, r.Min.Y, r.Max.Y, 1, frameColor)
			for _, h := range PictureHandles(box) {
				hr := image.Rect(px(h.X0), px(h.Y0), px(h.X1), px(h.Y1))
				gui.FillRect(ops, hr, frameColor)
				gui.FillRect(ops, hr.Inset(1), handleColor)
			}
			return
		}
	}
}

// PictureHandles returns the eight sizing handles of a picture's box,
// clockwise from the top-left corner (page points).
func PictureHandles(box Rect) [8]Rect {
	const h = 3.5
	mx, my := (box.X0+box.X1)/2, (box.Y0+box.Y1)/2
	pts := [8][2]float64{
		{box.X0, box.Y0}, {mx, box.Y0}, {box.X1, box.Y0}, {box.X1, my},
		{box.X1, box.Y1}, {mx, box.Y1}, {box.X0, box.Y1}, {box.X0, my},
	}
	var out [8]Rect
	for i, p := range pts {
		out[i] = Rect{p[0] - h, p[1] - h, p[0] + h, p[1] + h}
	}
	return out
}

// PictureAt returns the picture under page point (x, y) on page pg: its
// caret position (the picture is the character after it) and its box.
func (e *Engine) PictureAt(page int, x, y float64) (model.Pos, Rect, bool) {
	if page < 0 || page >= len(e.Pages) {
		return model.Pos{}, Rect{}, false
	}
	pg := e.Pages[page]
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		for k := range pl.Line.Items {
			it := &pl.Line.Items[k]
			if !isPicture(it) || it.St == nil || it.St.Hidden {
				continue
			}
			if r := PictureBox(pl, it); x >= r.X0 && x < r.X1 && y >= r.Y0 && y < r.Y1 {
				return model.Pos{Para: pl.Para, Off: it.Off}, r, true
			}
		}
	}
	return model.Pos{}, Rect{}, false
}

// PictureRect returns the page and box of the picture just after p.
func (e *Engine) PictureRect(p model.Pos) (int, Rect, bool) {
	pl, ok := e.lineFor(p)
	if !ok {
		return 0, Rect{}, false
	}
	for k := range pl.Line.Items {
		if it := &pl.Line.Items[k]; it.Off == p.Off && isPicture(it) {
			return e.pageOf(pl), PictureBox(pl, it), true
		}
	}
	return 0, Rect{}, false
}
