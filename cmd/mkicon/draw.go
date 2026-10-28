package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

var (
	// Each letter is drawn in its ink on a page of pale paper of the same
	// hue, edged in the ink: a violet W, an amber X.
	violet     = color.NRGBA{0x5E, 0x3F, 0xBF, 0xFF}
	paleViolet = color.NRGBA{0xEE, 0xE9, 0xFB, 0xFF}
	amber      = color.NRGBA{0xD9, 0x80, 0x0B, 0xFF}
	paleAmber  = color.NRGBA{0xFF, 0xF2, 0xDC, 0xFF}
	gridLine   = color.NRGBA{0xD9, 0x80, 0x0B, 0x50}
	white      = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	shadow     = color.NRGBA{0x10, 0x20, 0x30, 0x40}
	featherInk = color.NRGBA{0x8A, 0x9B, 0xAE, 0xFF}
)

type pt struct{ x, y float64 }

// canvas draws shapes given in a 256×256 design space onto an image of
// any size.
type canvas struct {
	img   *image.RGBA
	scale float64
}

func (c *canvas) fill(col color.NRGBA, polys ...[]pt) {
	c.draw(draw.Over, col, polys, c.img)
}

// erase clears the polygons to transparent.
func (c *canvas) erase(polys ...[]pt) {
	b := c.img.Bounds()
	mask := image.NewAlpha(b)
	c.draw(draw.Src, color.NRGBA{A: 0xFF}, polys, mask)
	for i, m := range mask.Pix {
		keep := 255 - uint32(m)
		for j := 4 * i; j < 4*i+4; j++ {
			c.img.Pix[j] = uint8((uint32(c.img.Pix[j])*keep + 127) / 255)
		}
	}
}

func (c *canvas) draw(op draw.Op, col color.NRGBA, polys [][]pt, dst draw.Image) {
	b := c.img.Bounds()
	r := vector.NewRasterizer(b.Dx(), b.Dy())
	for _, p := range polys {
		// Every polygon wound the same way, so overlaps add up instead of
		// cancelling.
		if area(p) < 0 {
			p = reversed(p)
		}
		r.MoveTo(float32(p[0].x*c.scale), float32(p[0].y*c.scale))
		for _, q := range p[1:] {
			r.LineTo(float32(q.x*c.scale), float32(q.y*c.scale))
		}
		r.ClosePath()
	}
	r.DrawOp = op
	r.Draw(dst, b, image.NewUniform(col), image.Point{})
}

func area(p []pt) float64 {
	var a float64
	for i := range p {
		q := p[(i+1)%len(p)]
		a += p[i].x*q.y - q.x*p[i].y
	}
	return a / 2
}

func reversed(p []pt) []pt {
	r := make([]pt, len(p))
	for i, q := range p {
		r[len(p)-1-i] = q
	}
	return r
}

// Draw renders the icon at size×size pixels.
func Draw(size int) *image.RGBA {
	c := &canvas{img: image.NewRGBA(image.Rect(0, 0, size, size)), scale: float64(size) / 256}
	draw.Draw(c.img, c.img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	wPage := page{10, 8, 150, 168, 34}
	xPage := page{106, 88, 246, 248, 34}
	edge, weight := 8.0, 15.0
	if size < 32 {
		// Bigger pages, heavier lines and no feather or grid: at this size
		// every pixel goes to the letters.
		wPage, xPage = page{0, 0, 150, 164, 40}, page{106, 92, 256, 256, 40}
		edge, weight = 1.2/c.scale, 1.7/c.scale
	} else {
		// Never thinner than about 1.3 pixels.
		edge, weight = max(edge, 1.2/c.scale), max(weight, 1.6/c.scale)
	}

	// Each letter sits off-center, away from where the pages overlap and
	// the feather crosses.
	wPage.draw(c, violet, paleViolet, edge)
	c.fill(violet, letterW(pt{70, 74}, weight)...)
	// The X page sits on top, a transparent gap separating it from the W.
	c.erase(xPage.inset(-max(5, 1/c.scale)))
	xPage.draw(c, amber, paleAmber, edge)
	if size >= 32 {
		// Faint cell lines make it a spreadsheet.
		var grid [][]pt
		for x := xPage.x0 + 46; x < xPage.x1-10; x += 46 {
			grid = append(grid, []pt{{x - 1, xPage.y0 + edge}, {x + 1, xPage.y0 + edge}, {x + 1, xPage.y1 - edge}, {x - 1, xPage.y1 - edge}})
		}
		for y := xPage.y0 + 40; y < xPage.y1-10; y += 40 {
			grid = append(grid, []pt{{xPage.x0 + edge, y - 1}, {xPage.x1 - edge, y - 1}, {xPage.x1 - edge, y + 1}, {xPage.x0 + edge, y + 1}})
		}
		c.fill(gridLine, grid...)
	}
	c.fill(amber, letterX(pt{xPage.x0 + 76, xPage.y0 + 92}, weight)...)

	if size >= 32 {
		// The details go as the icon shrinks: shadow, then shaft and
		// splits, which at these sizes only smudge.
		detail := size >= 48
		if size >= 64 {
			c.fill(shadow, offset(feather(0, true), 3, 5))
		}
		line := max(featherLine/c.scale, 1.5)
		c.fill(featherInk, feather(0, detail))
		c.fill(white, feather(line, detail))
		if detail {
			c.fill(featherInk, rachis())
		}
	}
	return c.img
}

// page is a sheet of paper with its top right corner folded over by fold.
type page struct{ x0, y0, x1, y1, fold float64 }

// inset returns the page's outline moved in by d (out, if negative).
func (p page) inset(d float64) []pt {
	k := d * (math.Sqrt2 - 1) // along the edges, for the 45° fold line
	return []pt{
		{p.x0 + d, p.y0 + d}, {p.x1 - p.fold - k, p.y0 + d},
		{p.x1 - d, p.y0 + p.fold + k}, {p.x1 - d, p.y1 - d}, {p.x0 + d, p.y1 - d},
	}
}

// draw paints the page in paper with an edge of ink, and the folded
// corner.
func (p page) draw(c *canvas, ink, paper color.NRGBA, edge float64) {
	c.fill(ink, p.inset(0))
	c.fill(paper, p.inset(edge))
	c.fill(ink, []pt{{p.x1 - p.fold, p.y0}, {p.x1 - p.fold, p.y0 + p.fold}, {p.x1, p.y0 + p.fold}})
}

// line is a stroke of weight w from a to b with round ends, so strokes
// joined end to end make the rounded joints of a single-line letter.
func line(a, b pt, w float64) []pt {
	ang := math.Atan2(b.y-a.y, b.x-a.x)
	var p []pt
	const n = 12
	for i := 0; i <= n; i++ { // around b, then around a
		t := ang - math.Pi/2 + math.Pi*float64(i)/n
		p = append(p, pt{b.x + w/2*math.Cos(t), b.y + w/2*math.Sin(t)})
	}
	for i := 0; i <= n; i++ {
		t := ang + math.Pi/2 + math.Pi*float64(i)/n
		p = append(p, pt{a.x + w/2*math.Cos(t), a.y + w/2*math.Sin(t)})
	}
	return p
}

// polyline joins the points with round-ended lines.
func polyline(w float64, ps ...pt) [][]pt {
	var out [][]pt
	for i := 1; i < len(ps); i++ {
		out = append(out, line(ps[i-1], ps[i], w))
	}
	return out
}

// letterW is a single-line W centered on c, in a light, round hand: wide,
// with the middle stroke stopping short of the top.
func letterW(c pt, w float64) [][]pt {
	return polyline(w,
		pt{c.x - 40, c.y - 26}, pt{c.x - 22, c.y + 28}, pt{c.x, c.y - 10},
		pt{c.x + 22, c.y + 28}, pt{c.x + 40, c.y - 26})
}

// letterX is a single-line X centered on c.
func letterX(c pt, w float64) [][]pt {
	return append(polyline(w, pt{c.x - 26, c.y - 27}, pt{c.x + 26, c.y + 27}),
		polyline(w, pt{c.x + 26, c.y - 27}, pt{c.x - 26, c.y + 27})...)
}

func offset1(p pt, d float64) pt { return pt{p.x + d, p.y + d} }

func offset(p []pt, dx, dy float64) []pt {
	q := make([]pt, len(p))
	for i, v := range p {
		q[i] = pt{v.x + dx, v.y + dy}
	}
	return q
}

// The feather lies across the other diagonal, over the pages, from the
// quill bottom left to the tip top right.
var (
	quillEnd = pt{30, 228}
	bend     = pt{116, 152}
	tip      = pt{234, 26}
)

// spine returns the point and unit normal at t along the feather's shaft.
func spine(t float64) (pt, pt) {
	u := 1 - t
	p := pt{u*u*quillEnd.x + 2*u*t*bend.x + t*t*tip.x, u*u*quillEnd.y + 2*u*t*bend.y + t*t*tip.y}
	d := pt{2*u*(bend.x-quillEnd.x) + 2*t*(tip.x-bend.x), 2*u*(bend.y-quillEnd.y) + 2*t*(tip.y-bend.y)}
	l := math.Hypot(d.x, d.y)
	return p, pt{-d.y / l, d.x / l}
}

// vaneStart is where the barbs begin; below it is bare quill.
const vaneStart = 0.16

// featherLine is the width in pixels of the feather's outline.
const featherLine = 1.2

// feather returns the vane's outline, inset by in: two sides that swell
// and taper to the tip, each with a split where the barbs have parted.
func feather(in float64, splits bool) []pt {
	type split struct{ at, depth float64 }
	side := func(sign, width float64, cuts []split) []pt {
		if !splits {
			cuts = nil
		}
		var p []pt
		const n = 120
		for i := 0; i <= n; i++ {
			t := vaneStart + (1-vaneStart)*float64(i)/n
			u := (t - vaneStart) / (1 - vaneStart)
			w := width * math.Pow(math.Sin(math.Pi*math.Pow(u, 0.75)), 0.8)
			for _, s := range cuts {
				// The cut runs back toward the quill, then the next barbs
				// start at full width.
				if d := s.at - u; d >= 0 && d < 0.1 {
					w *= 1 - s.depth*(1-d/0.1)
				}
			}
			s, n := spine(t)
			w = max(0, w-in)
			p = append(p, pt{s.x + sign*w*n.x, s.y + sign*w*n.y})
		}
		return p
	}
	left := side(1, 26, []split{{0.42, 0.4}, {0.7, 0.3}})
	right := side(-1, 21, []split{{0.3, 0.35}, {0.58, 0.35}})
	return append(left, reversed(right)...)
}

// rachis is the shaft: the bare quill and the line up the middle of the
// vane, thinning toward the tip.
func rachis() []pt {
	var l, r []pt
	const n = 80
	for i := 0; i <= n; i++ {
		t := 0.97 * float64(i) / n
		w := 2.6 - 1.9*t
		s, nv := spine(t)
		l = append(l, pt{s.x + w*nv.x, s.y + w*nv.y})
		r = append(r, pt{s.x - w*nv.x, s.y - w*nv.y})
	}
	return append(l, reversed(r)...)
}
