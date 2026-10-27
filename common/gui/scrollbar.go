package gui

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

var (
	scrollTrack = color.NRGBA{R: 0xF1, G: 0xF1, B: 0xF1, A: 0xFF}
	scrollThumb = color.NRGBA{R: 0xC1, G: 0xC1, B: 0xC1, A: 0xFF}
	scrollHot   = color.NRGBA{R: 0xA0, G: 0xA0, B: 0xA0, A: 0xFF}
)

// Scrollbar is a draggable scrollbar. Positions are in caller units (rows,
// columns...): pos is the first visible unit, visible how many fit on
// screen and total the scrollable extent.
type Scrollbar struct {
	Vertical bool

	dragging  bool
	dragStart float32
	dragPos   float64
	hover     bool
}

// Layout fills the constraints' max size and returns the (possibly
// updated) position.
func (s *Scrollbar) Layout(gtx layout.Context, pos, visible, total float64) float64 {
	size := gtx.Constraints.Max
	length, along := float32(size.X), func(p pointer.Event) float32 { return p.Position.X }
	if s.Vertical {
		length, along = float32(size.Y), func(p pointer.Event) float32 { return p.Position.Y }
	}
	total = max(total, visible, 1)
	thumbLen := max(float32(visible/total)*length, float32(gtx.Dp(16)))
	travel := length - thumbLen
	thumbAt := func(pos float64) float32 {
		if total <= visible || travel <= 0 {
			return 0
		}
		return float32(pos/(total-visible)) * travel
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Enter | pointer.Leave | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			s.hover = true
		case pointer.Leave:
			s.hover = false
		case pointer.Press:
			at, t0 := along(e), thumbAt(pos)
			switch {
			case at < t0:
				pos -= visible
			case at > t0+thumbLen:
				pos += visible
			default:
				s.dragging, s.dragStart, s.dragPos = true, at, pos
			}
		case pointer.Drag:
			if s.dragging && travel > 0 {
				pos = s.dragPos + float64((along(e)-s.dragStart)/travel)*(total-visible)
			}
		case pointer.Release, pointer.Cancel:
			s.dragging = false
		}
	}
	pos = min(max(pos, 0), max(total-visible, 0))

	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	area.Pop()

	FillRect(gtx.Ops, image.Rectangle{Max: size}, scrollTrack)
	t0 := int(thumbAt(pos))
	pad := gtx.Dp(3)
	r := image.Rect(t0, pad, t0+int(thumbLen), size.Y-pad)
	if s.Vertical {
		r = image.Rect(pad, t0, size.X-pad, t0+int(thumbLen))
	}
	c := scrollThumb
	if s.hover || s.dragging {
		c = scrollHot
	}
	FillRect(gtx.Ops, r, c)
	return pos
}
