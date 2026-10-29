// ContextMenu, the right-click menu.

package gui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"officelite/common/colors"
)

// ContextMenu is a right-click menu: Open it at the pointer with its
// items, then call Layout every frame from the same coordinates the
// position was given in. It draws above everything else, stays inside
// the area Layout is given, and closes on a choice, a click elsewhere or
// Esc.
type ContextMenu struct {
	open    bool
	pos     image.Point
	items   []MenuItem
	btns    []widget.Clickable
	catcher int // event tag covering the window while open
	body    int // event tag absorbing clicks between items
	tips    tipper
}

// Open shows items at pos (replacing any menu already open).
func (c *ContextMenu) Open(pos image.Point, items []MenuItem) {
	c.open, c.pos, c.items = true, pos, items
	c.btns = make([]widget.Clickable, len(items))
}

// IsOpen reports whether the menu is showing.
func (c *ContextMenu) IsOpen() bool { return c.open }

// Close hides the menu.
func (c *ContextMenu) Close() { c.open = false }

// Layout draws the open menu and returns the index of the item chosen
// since the last frame, or -1.
func (c *ContextMenu) Layout(gtx layout.Context, th *Theme) int {
	if !c.open {
		return -1
	}
	chosen := -1
	for i := range c.btns {
		if c.btns[i].Clicked(gtx) && !c.items[i].Disabled && !c.items[i].Separator {
			chosen = i
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &c.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			c.open = false
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			c.open = false
		}
	}
	if chosen >= 0 {
		c.open = false
	}
	if !c.open {
		gtx.Execute(op.InvalidateCmd{})
		return chosen
	}

	rec := op.Record(gtx.Ops)
	area := clip.Rect(image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)).Push(gtx.Ops)
	event.Op(gtx.Ops, &c.catcher)
	area.Pop()

	width := gtx.Dp(220)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, 1<<15)}
	body := op.Record(gtx.Ops)
	d := layout.Inset{Top: 4, Bottom: 4}.Layout(g, func(gtx layout.Context) layout.Dimensions {
		return menuItems(gtx, th, &c.tips, c.btns, c.items)
	})
	call := body.Stop()
	sz := d.Size
	// Keep it inside the area: flip left of / above the pointer when it
	// would run off the right or bottom edge.
	pos := c.pos
	if lim := gtx.Constraints.Max; lim.X > 0 && lim.Y > 0 {
		if pos.X+sz.X > lim.X {
			pos.X = max(pos.X-sz.X, 0)
		}
		if pos.Y+sz.Y > lim.Y {
			pos.Y = max(pos.Y-sz.Y, 0)
		}
	}
	off := op.Offset(pos).Push(gtx.Ops)
	FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22)) // shadow
	FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &c.body)
	stop.Pop()
	call.Add(gtx.Ops)
	bounds := image.Rectangle{Max: gtx.Constraints.Max}
	if bounds.Dy() == 0 {
		bounds.Max.Y = 1 << 15
	}
	c.tips.layout(gtx, th, bounds.Sub(pos), true)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
	return chosen
}
