// DropDown, the combo-box style button with a drop-down list or panel,
// and its DropItem entries.

package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"officelite/common/colors"
)

// IconDropArrow is the small arrow on drop-down buttons.
var IconDropArrow = mustIcon(icons.NavigationArrowDropDown)

// DropItem is an entry of a drop-down list.
type DropItem struct {
	Label     string
	Font      font.Font   // zero: the UI font
	Size      float32     // label size in sp; 0 = 13
	Color     color.NRGBA // label color; zero = ChromeText
	Swatch    color.NRGBA // when set, a color square is drawn before the label
	Separator bool        // a divider line; the other fields are ignored
	Header    bool        // a non-selectable section title
}

// DropDown is a button that opens a scrolling list of choices below it,
// like a combo box.
type DropDown struct {
	// ListWidth is the list's minimum width; Layout otherwise makes it as
	// wide as the box.
	ListWidth unit.Dp

	open    bool
	btn     widget.Clickable
	items   []widget.Clickable
	list    widget.List
	catcher int
	size    image.Point // the button's size
}

// IsOpen reports whether the list is showing.
func (d *DropDown) IsOpen() bool { return d.open }

// Button is the button that opens the list (for tests that click it).
func (d *DropDown) Button() *widget.Clickable { return &d.btn }

// Close hides the list.
func (d *DropDown) Close() { d.open = false }

// Layout draws a combo box width wide showing current, and returns the
// index of an item chosen since the last frame, or -1.
func (d *DropDown) Layout(gtx layout.Context, th *Theme, width unit.Dp, current string, items []DropItem) int {
	return d.LayoutButton(gtx, th, max(width, d.ListWidth), items, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(width)
		h := gtx.Dp(24)
		gtx.Constraints = layout.Exact(image.Pt(w, h))
		FillRect(gtx.Ops, image.Rect(0, 0, w, h), colors.White)
		border := ChromeLine
		if d.btn.Hovered() || d.open {
			border = th.Accent
		}
		strokeRect(gtx.Ops, image.Rect(0, 0, w, h), border)
		layout.Inset{Left: 6, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Middle(gtx, th.Label(12, current, ChromeText, font.Normal).Layout)
		})
		ic := gtx
		ic.Constraints = layout.Exact(image.Pt(gtx.Dp(18), gtx.Dp(18)))
		off := op.Offset(image.Pt(w-gtx.Dp(19), (h-gtx.Dp(18))/2)).Push(gtx.Ops)
		IconDropArrow.Layout(ic, MutedText)
		off.Pop()
		tipButton(gtx, &d.btn, d.btn.Hovered() && !d.open, image.Pt(w, h))
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// LayoutButton is Layout with a custom button; the list is listWidth wide.
func (d *DropDown) LayoutButton(gtx layout.Context, th *Theme, listWidth unit.Dp, items []DropItem, button layout.Widget) int {
	chosen := -1
	if len(d.items) != len(items) {
		d.items = make([]widget.Clickable, len(items))
	}
	for d.btn.Clicked(gtx) {
		d.open = !d.open
		d.list.Position = layout.Position{}
	}
	for i := range d.items {
		for d.items[i].Clicked(gtx) {
			if !items[i].Separator && !items[i].Header {
				chosen = i
				d.open = false
			}
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &d.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			d.open = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	dims := d.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := button(gtx)
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
		return dims
	})
	d.size = dims.Size
	if d.open {
		d.popup(gtx, th, listWidth, items)
	}
	return chosen
}

// popup draws the list below the button, above everything else.
func (d *DropDown) popup(gtx layout.Context, th *Theme, width unit.Dp, items []DropItem) {
	rec := op.Record(gtx.Ops)
	big := image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)
	area := clip.Rect(big).Push(gtx.Ops)
	event.Op(gtx.Ops, &d.catcher)
	area.Pop()

	off := op.Offset(image.Pt(0, d.size.Y)).Push(gtx.Ops)
	w := max(gtx.Dp(width), d.size.X)
	rowH := gtx.Dp(26)
	h := 0
	for _, it := range items {
		switch {
		case it.Separator:
			h += gtx.Dp(9)
		case it.Size > 16:
			h += gtx.Dp(unit.Dp(it.Size + 14))
		default:
			h += rowH
		}
	}
	h = min(h+gtx.Dp(8), gtx.Dp(420))
	sz := image.Pt(w, h)
	FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22))
	FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &d.items) // absorb clicks between items
	g := gtx
	g.Constraints = layout.Exact(sz)
	d.list.Axis = layout.Vertical
	layout.UniformInset(4).Layout(g, func(gtx layout.Context) layout.Dimensions {
		return material.List(th.Theme, &d.list).Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
			return d.item(gtx, th, i, items[i])
		})
	})
	stop.Pop()
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
}

// item draws list entry i: a divider for a separator, an unclickable
// title for a header, else a hoverable row with its swatch and label.
func (d *DropDown) item(gtx layout.Context, th *Theme, i int, it DropItem) layout.Dimensions {
	w := gtx.Constraints.Max.X
	if it.Separator {
		h := gtx.Dp(9)
		HLine(gtx.Ops, gtx.Dp(4), w-gtx.Dp(4), h/2, 1, ChromeLine)
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
	size := it.Size
	if size == 0 {
		size = 13
	}
	h := gtx.Dp(26)
	if size > 16 {
		h = gtx.Dp(unit.Dp(size + 14))
	}
	label := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(w, h))
		return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if it.Swatch == (color.NRGBA{}) {
							return layout.Dimensions{}
						}
						s := gtx.Dp(16)
						FillRect(gtx.Ops, image.Rect(0, 0, s, s), it.Swatch)
						strokeRect(gtx.Ops, image.Rect(0, 0, s, s), ChromeLine)
						return layout.Dimensions{Size: image.Pt(s+gtx.Dp(8), s)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						c := it.Color
						if c == (color.NRGBA{}) {
							c = ChromeText
						}
						weight := font.Normal
						if it.Header {
							c, weight = MutedText, font.SemiBold
						}
						l := th.Label(size, it.Label, c, weight)
						if it.Font.Typeface != "" {
							l.Font = it.Font
						}
						return l.Layout(gtx)
					}),
				)
			})
		})
	}
	if it.Header {
		return label(gtx)
	}
	return Hoverable(gtx, &d.items[i], colors.Transparent, HoverBG, label)
}

// strokeRect draws a one-pixel outline inside r.
func strokeRect(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	HLine(ops, r.Min.X, r.Max.X, r.Min.Y, 1, c)
	HLine(ops, r.Min.X, r.Max.X, r.Max.Y-1, 1, c)
	VLine(ops, r.Min.X, r.Min.Y, r.Max.Y, 1, c)
	VLine(ops, r.Max.X-1, r.Min.Y, r.Max.Y, 1, c)
}

// LayoutPanel is LayoutButton with a panel of its own in place of the
// list: panel draws the open drop-down's content (its size is the
// panel's) and returns true to close it.
func (d *DropDown) LayoutPanel(gtx layout.Context, button layout.Widget, panel func(gtx layout.Context) (layout.Dimensions, bool)) {
	for d.btn.Clicked(gtx) {
		d.open = !d.open
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &d.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			d.open = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	dims := d.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := button(gtx)
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
		return dims
	})
	d.size = dims.Size
	if !d.open {
		return
	}
	rec := op.Record(gtx.Ops)
	area := clip.Rect(image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)).Push(gtx.Ops)
	event.Op(gtx.Ops, &d.catcher)
	area.Pop()
	off := op.Offset(image.Pt(0, d.size.Y)).Push(gtx.Ops)
	body := op.Record(gtx.Ops)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(1<<15, 1<<15)}
	pd, done := panel(g)
	call := body.Stop()
	sz := pd.Size
	FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22))
	FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &d.items) // absorb clicks between the panel's controls
	stop.Pop()
	call.Add(gtx.Ops)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
	if done {
		d.open = false
		gtx.Execute(op.InvalidateCmd{})
	}
}
