// ColorPicker, Office's color menu (theme colors and shades, standard and
// recent colors, More Colors...), and the HSL / HSV conversions it uses.

package gui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
)

// The Office theme's colors (Office 2023 and later), as Excel and Word
// show them at the top of their color menus, with their names.
var themeColors = []struct {
	name string
	c    color.NRGBA
}{
	{"White, Background 1", colors.RGB(0xFFFFFF)},
	{"Black, Text 1", colors.RGB(0x000000)},
	{"Light Gray, Background 2", colors.RGB(0xE8E8E8)},
	{"Dark Blue, Text 2", colors.RGB(0x0E2841)},
	{"Blue, Accent 1", colors.RGB(0x156082)},
	{"Orange, Accent 2", colors.RGB(0xE97132)},
	{"Green, Accent 3", colors.RGB(0x196B24)},
	{"Blue, Accent 4", colors.RGB(0x0F9ED5)},
	{"Purple, Accent 5", colors.RGB(0xA02B93)},
	{"Green, Accent 6", colors.RGB(0x4EA72E)},
}

// StandardColors are Office's ten standard colors.
var StandardColors = []struct {
	Name string
	C    color.NRGBA
}{
	{"Dark Red", colors.RGB(0xC00000)}, {"Red", colors.RGB(0xFF0000)},
	{"Orange", colors.RGB(0xFFC000)}, {"Yellow", colors.RGB(0xFFFF00)},
	{"Light Green", colors.RGB(0x92D050)}, {"Green", colors.RGB(0x00B050)},
	{"Light Blue", colors.RGB(0x00B0F0)}, {"Blue", colors.RGB(0x0070C0)},
	{"Dark Blue", colors.RGB(0x002060)}, {"Purple", colors.RGB(0x7030A0)},
}

// themeShades returns the five lighter and darker variants Office lists
// under a theme color, with their names: which ones depends on how light
// the color is.
func themeShades(c color.NRGBA) ([5]color.NRGBA, [5]string) {
	h, s, l := toHSL(c)
	// Excel works in whole steps of 1/255.
	q := func(v float64) float64 { return math.Round(v*255) / 255 }
	h, s, l = q(h), q(s), q(l)
	var amounts [5]float64 // > 0 lighter, < 0 darker, in percent
	switch {
	case l == 0:
		amounts = [5]float64{50, 35, 25, 15, 5}
	case l == 1:
		amounts = [5]float64{-5, -15, -25, -35, -50}
	case l < 0.2:
		amounts = [5]float64{90, 75, 50, 25, 10}
	case l > 0.8:
		amounts = [5]float64{-10, -25, -50, -75, -90}
	default:
		amounts = [5]float64{80, 60, 40, -25, -50}
	}
	var out [5]color.NRGBA
	var names [5]string
	for i, a := range amounts {
		if a > 0 {
			out[i] = fromHSL(h, s, math.Floor((l+(1-l)*a/100)*255)/255)
			names[i] = fmt.Sprintf("Lighter %.0f%%", a)
		} else {
			out[i] = fromHSL(h, s, math.Floor(l*(1+a/100)*255)/255)
			names[i] = fmt.Sprintf("Darker %.0f%%", -a)
		}
	}
	return out, names
}

// toHSL converts c to hue, saturation and lightness, each in [0, 1].
func toHSL(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

// fromHSL returns the opaque color of hue h, saturation s and lightness l,
// each in [0, 1]; l is clamped to that range.
func fromHSL(h, s, l float64) color.NRGBA {
	l = min(max(l, 0), 1)
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return color.NRGBA{R: v, G: v, B: v, A: 0xFF}
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	ch := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(v * 255))
	}
	return color.NRGBA{R: ch(h + 1.0/3), G: ch(h), B: ch(h - 1.0/3), A: 0xFF}
}

// ColorChoice is a color picked from a ColorPicker; None is the
// "No Fill" / "Automatic" entry.
type ColorChoice struct {
	Color color.NRGBA
	None  bool
}

// ColorPicker is Office's color menu: the theme colors with their lighter
// and darker shades, the standard colors, recently chosen custom colors,
// a "No Fill" or "Automatic" entry and More Colors..., which opens a
// custom color panel (hue, shade and hex value).
type ColorPicker struct {
	dd     DropDown
	cells  [70]widget.Clickable // theme row, 5 shade rows, standard row
	recent []color.NRGBA
	recBtn [10]widget.Clickable
	none   widget.Clickable
	more   widget.Clickable
	custom customColor
	tipped bool
	tips   tipper // the swatch under the pointer, for its tooltip
}

// IsOpen reports whether the menu is showing.
func (p *ColorPicker) IsOpen() bool { return p.dd.IsOpen() }

// Arrow is the button that opens the menu, Swatch the button of color i
// (the theme row, its five shade rows, then the standard colors), NoColor
// the "No Fill" / "Automatic" entry and SwatchColor the color of swatch i,
// for tests that click them.
func (p *ColorPicker) Arrow() *widget.Clickable       { return &p.dd.btn }
func (p *ColorPicker) Swatch(i int) *widget.Clickable { return &p.cells[i] }
func (p *ColorPicker) NoColor() *widget.Clickable     { return &p.none }
func (p *ColorPicker) SwatchColor(i int) color.NRGBA  { return p.cellColor(i) }

// SplitButton draws a ribbon split button (btn applies the last color;
// the arrow opens the picker) and returns a color picked from the menu.
// noneLabel is "No Fill" or "Automatic".
func (p *ColorPicker) SplitButton(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *Icon, bar color.NRGBA, noneLabel string) (ColorChoice, bool, layout.Dimensions) {
	var choice ColorChoice
	var ok bool
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ToolButton(gtx, th, btn, icon, "", false, bar)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			p.dd.LayoutPanel(gtx, func(gtx layout.Context) layout.Dimensions {
				return dropFace(gtx, th, &p.dd.btn, nil, "", color.NRGBA{}, p.dd.IsOpen(), true)
			}, func(gtx layout.Context) (layout.Dimensions, bool) {
				d, c, done := p.panel(gtx, th, noneLabel)
				if done && c != nil {
					choice, ok = *c, true
				}
				return d, done
			})
			p.syncCustom()
			return layout.Dimensions{Size: p.dd.size}
		}),
	)
	tipButton(gtx, p, p.dd.btn.Hovered() && !p.dd.open, p.dd.size)
	return choice, ok, dims
}

// Combo draws a combo box width wide showing current's swatch (or
// noneLabel when current is the none entry) and returns a color picked.
func (p *ColorPicker) Combo(gtx layout.Context, th *Theme, width unit.Dp, current ColorChoice, noneLabel string) (ColorChoice, bool, layout.Dimensions) {
	var choice ColorChoice
	var ok bool
	p.dd.LayoutPanel(gtx, func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(width), gtx.Dp(24)
		gtx.Constraints = layout.Exact(image.Pt(w, h))
		FillRect(gtx.Ops, image.Rect(0, 0, w, h), colors.White)
		border := ChromeLine
		if p.dd.btn.Hovered() || p.dd.open {
			border = th.Accent
		}
		strokeRect(gtx.Ops, image.Rect(0, 0, w, h), border)
		pad := gtx.Dp(4)
		if current.None {
			off := op.Offset(image.Pt(gtx.Dp(6), 0)).Push(gtx.Ops)
			g := gtx
			g.Constraints = layout.Exact(image.Pt(w-gtx.Dp(26), h))
			Middle(g, th.Label(12, noneLabel, ChromeText, font.Normal).Layout)
			off.Pop()
		} else {
			r := image.Rect(pad, pad, w-gtx.Dp(22), h-pad)
			FillRect(gtx.Ops, r, current.Color)
			strokeRect(gtx.Ops, r, ChromeLine)
		}
		ic := gtx
		ic.Constraints = layout.Exact(image.Pt(gtx.Dp(18), gtx.Dp(18)))
		off := op.Offset(image.Pt(w-gtx.Dp(19), (h-gtx.Dp(18))/2)).Push(gtx.Ops)
		IconDropArrow.Layout(ic, MutedText)
		off.Pop()
		return layout.Dimensions{Size: image.Pt(w, h)}
	}, func(gtx layout.Context) (layout.Dimensions, bool) {
		d, c, done := p.panel(gtx, th, noneLabel)
		if done && c != nil {
			choice, ok = *c, true
		}
		return d, done
	})
	p.syncCustom()
	return choice, ok, layout.Dimensions{Size: p.dd.size}
}

// syncCustom ends More Colors... when the menu has closed any way but its
// own OK and Cancel (a click outside, the arrow, Close), so that the menu
// opens on the palette next time.
func (p *ColorPicker) syncCustom() {
	if !p.dd.open {
		p.custom.open = false
	}
}

// panel draws the open menu. It returns the color chosen, if any, and
// whether the menu should close.
func (p *ColorPicker) panel(gtx layout.Context, th *Theme, noneLabel string) (layout.Dimensions, *ColorChoice, bool) {
	if !p.tipped {
		p.tipped = true
		for col, tc := range themeColors {
			SetTip(&p.cells[col], tc.name, "", "")
			shades, names := themeShades(tc.c)
			for i := range shades {
				SetTip(&p.cells[10*(i+1)+col], tc.name+", "+names[i], "", "")
			}
		}
		for i, sc := range StandardColors {
			SetTip(&p.cells[60+i], sc.Name, "", "")
		}
	}
	if p.custom.open {
		return p.custom.layout(gtx, th, p)
	}
	var picked *ColorChoice
	pick := func(c ColorChoice) { picked = &c }
	for i := range p.cells {
		if p.cells[i].Clicked(gtx) {
			pick(ColorChoice{Color: p.cellColor(i)})
		}
	}
	for i := range p.recent {
		if p.recBtn[i].Clicked(gtx) {
			pick(ColorChoice{Color: p.recent[i]})
		}
	}
	if p.none.Clicked(gtx) {
		pick(ColorChoice{None: true})
	}
	if p.more.Clicked(gtx) {
		p.custom.start(p.lastCustom())
	}
	if picked != nil {
		return layout.Dimensions{}, picked, true
	}

	sw, gap := gtx.Dp(16), gtx.Dp(5)
	pad := gtx.Dp(10)
	w := 2*pad + 10*sw + 9*gap
	y := gtx.Dp(6)
	heading := func(s string) {
		off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		d := th.Label(12, s, ChromeText, font.SemiBold).Layout(gtx)
		off.Pop()
		y += d.Size.Y + gtx.Dp(6)
	}
	swatch := func(btn *widget.Clickable, x, y int, c color.NRGBA) {
		off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(sw, sw))
		btn.Layout(g, func(gtx layout.Context) layout.Dimensions {
			r := image.Rect(0, 0, sw, sw)
			FillRect(gtx.Ops, r, c)
			if btn.Hovered() {
				strokeRect(gtx.Ops, r.Inset(-1), colors.RGB(0xE3A21A))
				strokeRect(gtx.Ops, r, colors.White)
			} else if lum := 0.3*float64(c.R) + 0.59*float64(c.G) + 0.11*float64(c.B); lum > 225 {
				strokeRect(gtx.Ops, r, ChromeLine) // light swatches need an edge
			}
			pointer.CursorPointer.Add(gtx.Ops)
			// The swatch reports its own place to the picker's tipper, as
			// menu items do: the popup is outside the ribbon's area, which
			// is where a tipper would learn the pointer's position.
			if btn.Hovered() {
				if tip, ok := tipFor(btn); ok {
					p.tips.hover(gtx, btn, tip, image.Rect(x, y, x+sw, y+sw))
				}
			}
			return layout.Dimensions{Size: r.Size()}
		})
		off.Pop()
	}
	heading("Theme Colors")
	for col := range themeColors {
		swatch(&p.cells[col], pad+col*(sw+gap), y, themeColors[col].c)
	}
	y += sw + gtx.Dp(6)
	for row := 1; row <= 5; row++ {
		for col, tc := range themeColors {
			shades, _ := themeShades(tc.c)
			swatch(&p.cells[10*row+col], pad+col*(sw+gap), y, shades[row-1])
		}
		y += sw + gtx.Dp(1)
	}
	y += gtx.Dp(8)
	heading("Standard Colors")
	for i, sc := range StandardColors {
		swatch(&p.cells[60+i], pad+i*(sw+gap), y, sc.C)
	}
	y += sw + gtx.Dp(8)
	if len(p.recent) > 0 {
		heading("Recent Colors")
		for i, c := range p.recent {
			swatch(&p.recBtn[i], pad+i*(sw+gap), y, c)
		}
		y += sw + gtx.Dp(8)
	}
	HLine(gtx.Ops, 0, w, y, 1, ChromeLine)
	y += gtx.Dp(2)
	entry := func(btn *widget.Clickable, label string) {
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		g := gtx
		h := gtx.Dp(28)
		g.Constraints = layout.Exact(image.Pt(w, h))
		Hoverable(g, btn, colors.Transparent, HoverBG, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return Middle(gtx, th.Label(13, label, ChromeText, font.Normal).Layout)
			})
		})
		off.Pop()
		y += h
	}
	entry(&p.none, noneLabel)
	entry(&p.more, "More Colors...")
	p.tips.layout(gtx, th, image.Rect(0, 0, w, 1<<15), false)
	return layout.Dimensions{Size: image.Pt(w, y+gtx.Dp(4))}, nil, false
}

// cellColor is the color of swatch i: the theme row, its shades, then
// the standard colors.
func (p *ColorPicker) cellColor(i int) color.NRGBA {
	switch {
	case i < 10:
		return themeColors[i].c
	case i < 60:
		shades, _ := themeShades(themeColors[i%10].c)
		return shades[i/10-1]
	}
	return StandardColors[i-60].C
}

// lastCustom is the color More Colors... starts from: the most recent
// custom color, else the first accent.
func (p *ColorPicker) lastCustom() color.NRGBA {
	if len(p.recent) > 0 {
		return p.recent[0]
	}
	return themeColors[4].c
}

// addRecent puts a custom color first in the Recent Colors row.
func (p *ColorPicker) addRecent(c color.NRGBA) {
	out := []color.NRGBA{c}
	for _, r := range p.recent {
		if r != c && len(out) < len(p.recBtn) {
			out = append(out, r)
		}
	}
	p.recent = out
}

// customColor is More Colors...: a saturation/value square for the hue
// picked on a strip beside it, a hex box, and the new color beside the
// one it started from.
type customColor struct {
	open          bool
	h, s, v       float64
	start0        color.NRGBA
	square, strip int // event tags
	hex           widget.Editor
	ok, cancel    widget.Clickable
	sqImg         paint.ImageOp
	sqHue         float64
	sqValid       bool
	stripImg      paint.ImageOp
	stripValid    bool
}

// start opens the panel at color c, which it also shows as Current.
func (cc *customColor) start(c color.NRGBA) {
	cc.open, cc.start0 = true, c
	cc.h, cc.s, cc.v = toHSV(c)
	cc.hex.SingleLine = true
	cc.hex.SetText(hexOf(c))
}

// color returns the color being picked.
func (cc *customColor) color() color.NRGBA { return fromHSV(cc.h, cc.s, cc.v) }

// layout handles the panel's input and draws it. Like ColorPicker.panel,
// it returns the color chosen, if any (OK also adds it to p's recent
// colors), and whether the menu should close.
func (cc *customColor) layout(gtx layout.Context, th *Theme, p *ColorPicker) (layout.Dimensions, *ColorChoice, bool) {
	sq, stripW := gtx.Dp(160), gtx.Dp(18)
	pad, gap := gtx.Dp(10), gtx.Dp(10)
	// Pointer on the square and the strip.
	for _, tag := range []*int{&cc.square, &cc.strip} {
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Drag})
			if !ok {
				break
			}
			e, ok := ev.(pointer.Event)
			if !ok {
				continue
			}
			x := min(max(float64(e.Position.X)/float64(sq), 0), 1)
			y := min(max(float64(e.Position.Y)/float64(sq), 0), 1)
			if tag == &cc.square {
				cc.s, cc.v = x, 1-y
			} else {
				cc.h = min(y, 0.9999)
			}
			cc.hex.SetText(hexOf(cc.color()))
		}
	}
	for {
		ev, ok := cc.hex.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			if c, ok := colors.ParseHex(cc.hex.Text()); ok {
				cc.h, cc.s, cc.v = toHSV(c)
			}
		}
	}
	if cc.cancel.Clicked(gtx) {
		cc.open = false
		return layout.Dimensions{}, nil, true
	}
	if cc.ok.Clicked(gtx) {
		cc.open = false
		c := cc.color()
		p.addRecent(c)
		return layout.Dimensions{}, &ColorChoice{Color: c}, true
	}

	y := gtx.Dp(6)
	off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
	d := th.Label(12, "Custom Color", ChromeText, font.SemiBold).Layout(gtx)
	off.Pop()
	y += d.Size.Y + gtx.Dp(6)

	// The square: saturation across, value down, for the hue.
	if !cc.sqValid || cc.sqHue != cc.h || cc.sqImg.Size() != image.Pt(sq, sq) {
		img := image.NewNRGBA(image.Rect(0, 0, sq, sq))
		for py := range sq {
			for px := range sq {
				img.SetNRGBA(px, py, fromHSV(cc.h, float64(px)/float64(sq-1), 1-float64(py)/float64(sq-1)))
			}
		}
		cc.sqImg, cc.sqHue, cc.sqValid = paint.NewImageOp(img), cc.h, true
	}
	if !cc.stripValid || cc.stripImg.Size() != image.Pt(stripW, sq) {
		img := image.NewNRGBA(image.Rect(0, 0, stripW, sq))
		for py := range sq {
			c := fromHSV(float64(py)/float64(sq), 1, 1)
			for px := range stripW {
				img.SetNRGBA(px, py, c)
			}
		}
		cc.stripImg, cc.stripValid = paint.NewImageOp(img), true
	}
	place := func(img paint.ImageOp, tag *int, x int, w int) {
		off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		cl := clip.Rect{Max: image.Pt(w, sq)}.Push(gtx.Ops)
		img.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		event.Op(gtx.Ops, tag)
		pointer.CursorCrosshair.Add(gtx.Ops)
		cl.Pop()
		strokeRect(gtx.Ops, image.Rect(0, 0, w, sq), ChromeLine)
		off.Pop()
	}
	place(cc.sqImg, &cc.square, pad, sq)
	place(cc.stripImg, &cc.strip, pad+sq+gap, stripW)
	// Markers: a ring on the square, a bar on the strip.
	mx, my := pad+int(cc.s*float64(sq-1)), y+int((1-cc.v)*float64(sq-1))
	ring := gtx.Dp(4)
	strokeRect(gtx.Ops, image.Rect(mx-ring, my-ring, mx+ring+1, my+ring+1), colors.White)
	strokeRect(gtx.Ops, image.Rect(mx-ring-1, my-ring-1, mx+ring+2, my+ring+2), colors.RGB(0x202020))
	hy := y + int(cc.h*float64(sq))
	strokeRect(gtx.Ops, image.Rect(pad+sq+gap-2, hy-2, pad+sq+gap+stripW+2, hy+3), colors.RGB(0x202020))

	// New / Current preview and the hex box beside the strip.
	x := pad + sq + gap + stripW + gap
	pw := gtx.Dp(56)
	labelAt := func(s string, at int) {
		off := op.Offset(image.Pt(x, at)).Push(gtx.Ops)
		th.Label(11, s, MutedText, font.Normal).Layout(gtx)
		off.Pop()
	}
	labelAt("New", y)
	FillRect(gtx.Ops, image.Rect(x, y+gtx.Dp(16), x+pw, y+gtx.Dp(44)), cc.color())
	FillRect(gtx.Ops, image.Rect(x, y+gtx.Dp(44), x+pw, y+gtx.Dp(72)), cc.start0)
	strokeRect(gtx.Ops, image.Rect(x, y+gtx.Dp(16), x+pw, y+gtx.Dp(72)), ChromeLine)
	labelAt("Current", y+gtx.Dp(74))
	labelAt("Hex", y+gtx.Dp(100))
	box := image.Rect(x, y+gtx.Dp(116), x+pw+gtx.Dp(10), y+gtx.Dp(140))
	FillRect(gtx.Ops, box, colors.White)
	strokeRect(gtx.Ops, box, ChromeLine)
	off = op.Offset(box.Min.Add(image.Pt(gtx.Dp(4), gtx.Dp(3)))).Push(gtx.Ops)
	g := gtx
	g.Constraints = layout.Exact(box.Size().Sub(image.Pt(gtx.Dp(8), gtx.Dp(6))))
	ed := material.Editor(th.Theme, &cc.hex, "")
	ed.TextSize = unit.Sp(12)
	ed.Layout(g)
	off.Pop()
	w := x + pw + gtx.Dp(10) + pad
	y += sq + gtx.Dp(10)

	// OK / Cancel.
	bw, bh := gtx.Dp(64), gtx.Dp(26)
	button := func(btn *widget.Clickable, label string, at int, primary bool) {
		off := op.Offset(image.Pt(at, y)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(bw, bh))
		bg, fg := ChromeBG, ChromeText
		if primary {
			bg, fg = th.Accent, colors.White
		}
		Hoverable(g, btn, bg, colors.Mix(bg, colors.RGB(0), 0.1), func(gtx layout.Context) layout.Dimensions {
			strokeRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, ChromeLine)
			return layout.Center.Layout(gtx, th.Label(12, label, fg, font.Normal).Layout)
		})
		off.Pop()
	}
	button(&cc.ok, "OK", w-pad-2*bw-gtx.Dp(6), true)
	button(&cc.cancel, "Cancel", w-pad-bw, false)
	y += bh + pad
	return layout.Dimensions{Size: image.Pt(w, y)}, nil, false
}

// toHSV converts c to hue, saturation and value, each in [0, 1].
func toHSV(c color.NRGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	if mx == mn {
		return 0, s, v
	}
	d := mx - mn
	switch mx {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, v
}

// fromHSV returns the opaque color of hue h (wrapped into [0, 1)),
// saturation s and value v.
func fromHSV(h, s, v float64) color.NRGBA {
	h -= math.Floor(h) // negative hues wrap too
	if !(h < 1) {      // rounding up to 1 for a hue just below 0, or NaN
		h = 0
	}
	h *= 6
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch int(i) {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	u := func(x float64) uint8 { return uint8(math.Round(x * 255)) }
	return color.NRGBA{R: u(r), G: u(g), B: u(b), A: 0xFF}
}

// hexOf formats c as "#RRGGBB" for the hex box.
func hexOf(c color.NRGBA) string { return "#" + colors.Hex(c) }
