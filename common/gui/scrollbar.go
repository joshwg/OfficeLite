package gui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"officelite/common/colors"
)

var (
	scrollTrack   = color.NRGBA{R: 0xF1, G: 0xF1, B: 0xF1, A: 0xFF}
	scrollThumb   = color.NRGBA{R: 0xC1, G: 0xC1, B: 0xC1, A: 0xFF}
	scrollHot     = color.NRGBA{R: 0xA8, G: 0xA8, B: 0xA8, A: 0xFF}
	scrollPressed = color.NRGBA{R: 0x78, G: 0x78, B: 0x78, A: 0xFF}
	scrollArrow   = color.NRGBA{R: 0x60, G: 0x60, B: 0x60, A: 0xFF}
)

// Auto-repeat while an arrow or the track is held, as Windows does.
const (
	scrollRepeatDelay = 400 * time.Millisecond
	scrollRepeatEvery = 50 * time.Millisecond
)

// scrollPart is a part of a scrollbar.
type scrollPart int

const (
	partNone     scrollPart = iota
	partBack                // arrow up / left
	partFwd                 // arrow down / right
	partPageBack            // track before the thumb
	partPageFwd             // track after the thumb
	partThumb
)

// Scrollbar is a Windows-style scrollbar: arrow buttons at both ends
// that step (and repeat while held), a track that pages toward the
// pointer while held (Shift+click jumps there instead), and a thumb to
// drag, which snaps back if the pointer strays far from the bar. Positions
// are in caller units (rows, points...): pos is the first visible unit,
// visible how many fit on screen and total the scrollable extent.
type Scrollbar struct {
	Vertical bool
	// Step is how far an arrow click scrolls; zero means a twentieth of
	// the visible amount.
	Step float64
	// Label, with Theme, shows a hint beside the thumb while it is
	// dragged, as Excel shows "Row: 120" and Word the page.
	Label func(pos float64) string
	Theme *Theme
	// OpenEnded lets the forward arrow scroll past the end, as Excel's
	// arrows go beyond the data; the caller then grows total to match.
	OpenEnded bool

	hover     scrollPart
	held      scrollPart // arrow or track part being held
	heldAt    float32    // pointer position along the bar while held
	next      time.Time  // next auto-repeat
	dragging  bool
	dragStart float32
	dragPos   float64
}

// Layout fills the constraints' max size and returns the (possibly
// updated) position.
func (s *Scrollbar) Layout(gtx layout.Context, pos, visible, total float64) float64 {
	size := gtx.Constraints.Max
	length, thick := size.X, size.Y
	along := func(p f32.Point) float32 { return p.X }
	across := func(p f32.Point) float32 { return p.Y }
	if s.Vertical {
		length, thick = size.Y, size.X
		along, across = across, along
	}
	// Arrow buttons are square; they go when the bar is too short.
	btn := thick
	if length < 3*btn {
		btn = 0
	}
	track := float32(length - 2*btn)
	total = max(total, visible, 1)
	thumbLen := min(max(float32(visible/total)*track, float32(gtx.Dp(16))), track)
	travel := track - thumbLen
	maxPos := max(total-visible, 0)
	thumbAt := func(pos float64) float32 { // from the track's start
		if maxPos <= 0 || travel <= 0 {
			return 0
		}
		return float32(pos/maxPos) * travel
	}
	step := s.Step
	if step <= 0 {
		step = visible / 20
	}
	partAt := func(at float32) scrollPart {
		switch t0 := float32(btn) + thumbAt(pos); {
		case at < float32(btn):
			return partBack
		case at >= float32(length-btn):
			return partFwd
		case at < t0:
			return partPageBack
		case at >= t0+thumbLen:
			return partPageFwd
		}
		return partThumb
	}
	// act applies one step of the held part; paging stops once the thumb
	// reaches the pointer. past records an open-ended step beyond the end.
	past := false
	act := func(p scrollPart) {
		t0 := float32(btn) + thumbAt(pos)
		switch p {
		case partBack:
			pos -= step
		case partFwd:
			pos += step
			if s.OpenEnded && pos > maxPos {
				past = true
				return
			}
		case partPageBack:
			if s.heldAt < t0 {
				pos -= visible
			}
		case partPageFwd:
			if s.heldAt >= t0+thumbLen {
				pos += visible
			}
		}
		pos = min(max(pos, 0), maxPos)
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press | pointer.Drag | pointer.Release |
			pointer.Move | pointer.Enter | pointer.Leave | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		at := along(e.Position)
		switch e.Kind {
		case pointer.Enter, pointer.Move:
			s.hover = partAt(at)
		case pointer.Leave:
			s.hover = partNone
		case pointer.Press:
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			p := partAt(at)
			if (p == partPageBack || p == partPageFwd) && e.Modifiers.Contain(key.ModShift) && travel > 0 {
				// Shift+click: jump the thumb's middle to the pointer
				// and carry on dragging it.
				pos = min(max(float64((at-float32(btn)-thumbLen/2)/travel)*maxPos, 0), maxPos)
				p = partThumb
			}
			if p == partThumb {
				s.dragging, s.dragStart, s.dragPos = true, at, pos
				continue
			}
			s.held, s.heldAt = p, at
			act(p)
			s.next = gtx.Now.Add(scrollRepeatDelay)
		case pointer.Drag:
			switch {
			case s.dragging && travel > 0:
				// Far off the bar the thumb goes back where it started,
				// as in Windows.
				if off := across(e.Position); off < -float32(8*thick) || off > float32(9*thick) {
					pos = s.dragPos
				} else {
					pos = s.dragPos + float64((at-s.dragStart)/travel)*maxPos
				}
			case s.held != partNone:
				s.heldAt = at
			}
		case pointer.Release, pointer.Cancel:
			s.dragging, s.held = false, partNone
			if e.Kind == pointer.Release {
				s.hover = partAt(at)
			}
		}
	}
	if s.held != partNone {
		// Repeat only while the pointer is still on the part held, as
		// Windows does (paging toward it stops at the thumb).
		on := partAt(s.heldAt)
		still := on == s.held || (s.held == partPageBack || s.held == partPageFwd) && on != partBack && on != partFwd
		if !gtx.Now.Before(s.next) {
			if still {
				act(s.held)
			}
			s.next = gtx.Now.Add(scrollRepeatEvery)
		}
		gtx.Execute(op.InvalidateCmd{At: s.next})
	}
	if !past {
		pos = min(max(pos, 0), maxPos)
	}

	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	area.Pop()

	FillRect(gtx.Ops, image.Rectangle{Max: size}, scrollTrack)
	// rect maps a span along the bar and an inset across it to the bar.
	rect := func(a0, a1, inset int) image.Rectangle {
		if s.Vertical {
			return image.Rect(inset, a0, thick-inset, a1)
		}
		return image.Rect(a0, inset, a1, thick-inset)
	}
	if held := s.held; held == partPageBack || held == partPageFwd {
		t0 := btn + int(thumbAt(pos))
		r := rect(btn, t0, 0)
		if held == partPageFwd {
			r = rect(t0+int(thumbLen), length-btn, 0)
		}
		FillRect(gtx.Ops, r, HoverBG)
	}
	if btn > 0 {
		for _, p := range []scrollPart{partBack, partFwd} {
			a0 := 0
			if p == partFwd {
				a0 = length - btn
			}
			r := rect(a0, a0+btn, 0)
			c := scrollArrow
			switch {
			case s.held == p:
				FillRect(gtx.Ops, r, scrollPressed)
				c = colors.White
			case s.hover == p:
				FillRect(gtx.Ops, r, HoverBG)
			}
			if maxPos <= 0 {
				c = scrollThumb
			}
			s.arrow(gtx, r, p == partFwd, c)
		}
	}
	if maxPos > 0 {
		t0 := btn + int(thumbAt(pos))
		pad := max(thick/5, 2)
		c := scrollThumb
		switch {
		case s.dragging:
			c = scrollPressed
		case s.hover == partThumb:
			c = scrollHot
		}
		FillRect(gtx.Ops, rect(t0, t0+int(thumbLen), pad), c)
		if s.dragging && s.Label != nil && s.Theme != nil {
			s.hint(gtx, s.Label(pos), t0, int(thumbLen))
		}
	}
	return pos
}

// arrow draws a small solid triangle centered in r, pointing back (up or
// left) or forward (down or right).
func (s *Scrollbar) arrow(gtx layout.Context, r image.Rectangle, fwd bool, c color.NRGBA) {
	cx, cy := float32(r.Min.X+r.Max.X)/2, float32(r.Min.Y+r.Max.Y)/2
	h := float32(r.Dx()) * 0.18                                                                   // half the base
	d := h * 0.9                                                                                  // tip to base
	pts := [3]f32.Point{{X: cx - h, Y: cy + d/2}, {X: cx + h, Y: cy + d/2}, {X: cx, Y: cy - d/2}} // up
	for i, p := range pts {
		if fwd {
			p.Y = 2*cy - p.Y
		}
		if !s.Vertical {
			p = f32.Pt(cx+(p.Y-cy), cy+(p.X-cx))
		}
		pts[i] = p
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(pts[0])
	path.LineTo(pts[1])
	path.LineTo(pts[2])
	path.Close()
	paint.FillShape(gtx.Ops, c, clip.Outline{Path: path.End()}.Op())
}

// hint draws text in a box beside the thumb (left of a vertical bar,
// above a horizontal one), on top of everything else.
func (s *Scrollbar) hint(gtx layout.Context, txt string, t0, tl int) {
	rec := op.Record(gtx.Ops)
	m := op.Record(gtx.Ops)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(400), gtx.Dp(40))}
	d := layout.UniformInset(5).Layout(g, s.Theme.Label(12, txt, ChromeText, font.Normal).Layout)
	call := m.Stop()
	var at image.Point
	if s.Vertical {
		at = image.Pt(-d.Size.X-gtx.Dp(6), t0+tl/2-d.Size.Y/2)
	} else {
		at = image.Pt(t0+tl/2-d.Size.X/2, -d.Size.Y-gtx.Dp(6))
	}
	off := op.Offset(at).Push(gtx.Ops)
	FillRect(gtx.Ops, image.Rect(-1, -1, d.Size.X+1, d.Size.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: d.Size}, colors.White)
	call.Add(gtx.Ops)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
}
