package gui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/widget"

	"officelite/common/colors"
)

// MenuItem is one entry of a drop-down menu.
type MenuItem struct {
	Label     string
	Shortcut  string // shown right-aligned, e.g. "Ctrl+S"
	Checked   bool
	Disabled  bool
	Separator bool   // a divider line; the other fields are ignored
	Tip       string // one-line description shown when the pointer rests on it
}

// Menu is a menu-bar entry. A menu without items acts as a button; Primary
// draws it in the accent color, like Excel's File tab.
type Menu struct {
	Label   string
	Items   []MenuItem
	Primary bool
}

// MenuEvent reports a chosen item, or Item -1 for a click on a menu
// without items.
type MenuEvent struct{ Menu, Item int }

// MenuBar is a row of menus with at most one drop-down open.
type MenuBar struct {
	open    int // open drop-down + 1; 0 when none
	heads   []widget.Clickable
	items   [][]widget.Clickable
	rects   []image.Rectangle // header bounds, in bar coordinates
	catcher int               // event tag for clicks outside the drop-down
	tips    tipper
}

// IsOpen reports whether a drop-down is showing.
func (m *MenuBar) IsOpen() bool { return m.open > 0 }

// OpenMenu opens menu i's drop-down (n is the number of menus), as if its
// header had been clicked.
func (m *MenuBar) OpenMenu(i, n int) {
	if len(m.heads) != n {
		m.heads = make([]widget.Clickable, n)
		m.items = make([][]widget.Clickable, n)
		m.rects = make([]image.Rectangle, n)
	}
	m.open = i + 1
}

// Close closes the open drop-down.
func (m *MenuBar) Close() { m.open = 0 }

// Layout draws the bar across the full width and calls choose for each
// selection made since the last frame.
func (m *MenuBar) Layout(gtx layout.Context, th *Theme, menus []Menu, choose func(MenuEvent)) layout.Dimensions {
	if len(m.heads) != len(menus) {
		m.OpenMenu(-1, len(menus))
	}
	m.update(gtx, menus, choose)

	h := gtx.Dp(30)
	w := gtx.Constraints.Max.X
	FillRect(gtx.Ops, image.Rect(0, 0, w, h), ChromeBG)
	HLine(gtx.Ops, 0, w, h-1, 1, ChromeLine)
	x := gtx.Dp(4)
	for i, mn := range menus {
		g := gtx
		g.Constraints = layout.Constraints{Min: image.Pt(0, h), Max: image.Pt(w-x, h)}
		off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		d := m.header(g, th, i, mn)
		off.Pop()
		m.rects[i] = image.Rectangle{Min: image.Pt(x, 0), Max: image.Pt(x+d.Size.X, h)}
		x += d.Size.X + gtx.Dp(2)
	}
	if m.open > 0 {
		m.dropDown(gtx, th, m.open-1, menus[m.open-1].Items)
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (m *MenuBar) update(gtx layout.Context, menus []Menu, choose func(MenuEvent)) {
	for i := range menus {
		for m.heads[i].Clicked(gtx) {
			switch {
			case len(menus[i].Items) == 0:
				m.open = 0
				choose(MenuEvent{i, -1})
			case m.open == i+1:
				m.open = 0
			default:
				m.open = i + 1
			}
		}
		if len(m.items[i]) != len(menus[i].Items) {
			m.items[i] = make([]widget.Clickable, len(menus[i].Items))
		}
		for j := range m.items[i] {
			for m.items[i][j].Clicked(gtx) {
				m.open = 0
				choose(MenuEvent{i, j})
			}
		}
	}
	// While a drop-down is open the catcher covers the window: a click
	// outside closes it (or switches menus), hovering a header switches.
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &m.catcher, Kinds: pointer.Press | pointer.Move})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || m.open == 0 {
			continue
		}
		hit := -1
		for i, r := range m.rects {
			if e.Position.Round().In(r) {
				hit = i
			}
		}
		switch {
		case e.Kind == pointer.Move:
			if hit >= 0 && len(menus[hit].Items) > 0 {
				m.open = hit + 1
			}
		case hit < 0 || hit == m.open-1:
			m.open = 0
		case len(menus[hit].Items) == 0:
			m.open = 0
			choose(MenuEvent{hit, -1})
		default:
			m.open = hit + 1
		}
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (m *MenuBar) header(gtx layout.Context, th *Theme, i int, mn Menu) layout.Dimensions {
	bg, hover, fg := colors.Transparent, HoverBG, ChromeText
	if mn.Primary {
		bg, hover, fg = th.Accent, th.AccentDark, colors.White
	}
	if m.open == i+1 {
		bg = hover
	}
	return Hoverable(gtx, &m.heads[i], bg, hover, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Middle(gtx, th.Label(13, mn.Label, fg, font.Normal).Layout)
		})
	})
}

// dropDown draws the open menu below its header, above everything else.
func (m *MenuBar) dropDown(gtx layout.Context, th *Theme, i int, items []MenuItem) {
	rec := op.Record(gtx.Ops)
	big := image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)
	area := clip.Rect(big).Push(gtx.Ops)
	event.Op(gtx.Ops, &m.catcher)
	area.Pop()

	pos := image.Pt(m.rects[i].Min.X, m.rects[i].Max.Y)
	off := op.Offset(pos).Push(gtx.Ops)
	width := gtx.Dp(240)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
	body := op.Record(gtx.Ops)
	d := layout.Inset{Top: 4, Bottom: 4}.Layout(g, func(gtx layout.Context) layout.Dimensions {
		return menuItems(gtx, th, &m.tips, m.items[i], items)
	})
	call := body.Stop()
	sz := d.Size
	FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22)) // shadow
	FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	// The body absorbs clicks between items so they don't reach the catcher.
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &m.items)
	stop.Pop()
	call.Add(gtx.Ops)
	m.tips.layout(gtx, th, image.Rect(-pos.X, -pos.Y, gtx.Constraints.Max.X-pos.X, 1<<15), true)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
}

// menuItems lays out a drop-down's entries top to bottom (starting at
// the top inset) and reports the hovered one to t for its Tip.
func menuItems(gtx layout.Context, th *Theme, t *tipper, btns []widget.Clickable, items []MenuItem) layout.Dimensions {
	y := gtx.Dp(4)
	children := make([]layout.FlexChild, len(items))
	for j, it := range items {
		children[j] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := menuItem(gtx, th, &btns[j], it)
			if it.Tip != "" && btns[j].Hovered() {
				t.hover(gtx, &btns[j], Tip{Desc: it.Tip}, image.Rect(0, y, d.Size.X, y+d.Size.Y))
			}
			y += d.Size.Y
			return d
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// menuItem draws one drop-down entry, as wide as gtx.Constraints.Min.X.
func menuItem(gtx layout.Context, th *Theme, btn *widget.Clickable, it MenuItem) layout.Dimensions {
	w := gtx.Constraints.Min.X
	if it.Separator {
		h := gtx.Dp(9)
		HLine(gtx.Ops, gtx.Dp(8), w-gtx.Dp(8), h/2, 1, ChromeLine)
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
	if it.Disabled {
		gtx = gtx.Disabled()
	}
	return Hoverable(gtx, btn, colors.Transparent, HoverBG, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(28)
		gtx.Constraints = layout.Exact(image.Pt(w, h))
		fg := ChromeText
		if it.Disabled {
			fg = MutedText
		}
		return Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(32)
					if it.Checked {
						ic := gtx
						ic.Constraints = layout.Exact(image.Pt(gtx.Dp(16), gtx.Dp(16)))
						off := op.Offset(image.Pt(gtx.Dp(9), 0)).Push(gtx.Ops)
						IconCheck.Layout(ic, fg)
						off.Pop()
					}
					return layout.Dimensions{Size: image.Pt(size, gtx.Dp(16))}
				}),
				layout.Flexed(1, th.Label(13, it.Label, fg, font.Normal).Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := th.Label(12, it.Shortcut, MutedText, font.Normal)
					l.Alignment = text.End
					return layout.Inset{Left: 16, Right: 12}.Layout(gtx, l.Layout)
				}),
			)
		})
	})
}
