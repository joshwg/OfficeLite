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
	"golang.org/x/exp/shiny/materialdesign/icons"

	"officelite/common/colors"
)

// A ribbon is Office's Home tab: a row of captioned groups of buttons.
// Each group's content is two rows of RibbonRowHeight controls (or one
// tall BigButton); groups sit side by side and the whole row scrolls
// sideways (wheel or Shift+wheel) when the window is narrower than it.
//
//	var r gui.Ribbon
//	r.Layout(gtx, th,
//		gui.RibbonGroup{Caption: "Font", Content: gui.RibbonStack(
//			gui.RibbonRow(fontBox, sizeBox),
//			gui.RibbonRow(boldBtn, italicBtn),
//		)},
//		...)

// Ribbon metrics.
const (
	RibbonHeight    unit.Dp = 84 // the whole bar
	RibbonRowHeight unit.Dp = 28 // one row of small controls
	ribbonCaption   unit.Dp = 17
	ribbonPad       unit.Dp = 4
)

// Icons for ribbons (Material Design, Apache 2.0).
var (
	IconFill        = mustIcon(icons.EditorFormatColorFill)
	IconBorderAll   = mustIcon(icons.EditorBorderAll)
	IconBorderNone  = mustIcon(icons.EditorBorderClear)
	IconVAlignTop   = mustIcon(icons.EditorVerticalAlignTop)
	IconVAlignMid   = mustIcon(icons.EditorVerticalAlignCenter)
	IconVAlignBot   = mustIcon(icons.EditorVerticalAlignBottom)
	IconWrap        = mustIcon(icons.EditorWrapText)
	IconMerge       = mustIcon(icons.EditorMergeType)
	IconPainter     = mustIcon(icons.EditorFormatPaint)
	IconStyles      = mustIcon(icons.ImageStyle)
	IconFunctions   = mustIcon(icons.EditorFunctions)
	IconSort        = mustIcon(icons.ContentSort)
	IconSearch      = mustIcon(icons.ActionSearch)
	IconClear       = mustIcon(icons.ContentClear)
	IconFillDown    = mustIcon(icons.NavigationArrowDownward)
	IconLauncher    = mustIcon(icons.NavigationSubdirectoryArrowRight)
	IconInsertCells = mustIcon(icons.ContentAddBox)
	IconDeleteCells = mustIcon(icons.ContentRemoveCircleOutline)
	IconFormatCells = mustIcon(icons.ActionViewModule)

	iconScrollLeft  = mustIcon(icons.NavigationChevronLeft)
	iconScrollRight = mustIcon(icons.NavigationChevronRight)
)

// Ribbon lays out groups in a sideways-scrolling row.
type Ribbon struct {
	list        layout.List
	left, right widget.Clickable // scroll buttons shown when it doesn't fit
	tips        tipper
}

// RibbonGroup is one captioned group of a ribbon.
type RibbonGroup struct {
	Caption string
	// Launcher, when set, adds the small dialog launcher in the caption's
	// right corner (Excel's Format Cells for the Font group).
	Launcher *widget.Clickable
	Content  layout.Widget // laid out in a ContentHeight-tall box
}

// RibbonContentHeight is the height available to a group's content.
func RibbonContentHeight(gtx layout.Context) int {
	return gtx.Dp(RibbonHeight - ribbonCaption - 2*ribbonPad)
}

// Layout draws the ribbon across the full width.
func (r *Ribbon) Layout(gtx layout.Context, th *Theme, groups ...RibbonGroup) layout.Dimensions {
	w, h := gtx.Constraints.Max.X, gtx.Dp(RibbonHeight)
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	FillRect(gtx.Ops, image.Rect(0, 0, w, h), ChromeBG)
	HLine(gtx.Ops, 0, w, h-1, 1, ChromeLine)
	r.list.Axis = layout.Horizontal
	cs := clip.Rect{Max: image.Pt(w, h-1)}.Push(gtx.Ops)
	// The mouse wheel scrolls the row sideways when it doesn't fit.
	event.Op(gtx.Ops, r)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: r, Kinds: pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			r.list.Position.Offset += int(e.Scroll.Y)
		}
	}
	for r.left.Clicked(gtx) {
		r.list.Position.Offset -= w / 2
	}
	for r.right.Clicked(gtx) {
		r.list.Position.Offset += w / 2
	}
	gtx = withTipper(gtx, &r.tips)
	r.tips.track(gtx)
	r.list.Layout(gtx, len(groups), func(gtx layout.Context, i int) layout.Dimensions {
		return r.group(gtx, th, groups[i])
	})
	// Scroll buttons at the ends that have more groups beyond them.
	arrow := func(btn *widget.Clickable, ic *widget.Icon, x int) {
		bw := gtx.Dp(18)
		o := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(bw, h-1))
		Hoverable(g, btn, ChromeBG, HoverBG, func(gtx layout.Context) layout.Dimensions {
			VLine(gtx.Ops, 0, 0, h-1, 1, ChromeLine)
			VLine(gtx.Ops, bw-1, 0, h-1, 1, ChromeLine)
			is := gtx.Dp(18)
			ig := gtx
			ig.Constraints = layout.Exact(image.Pt(is, is))
			o := op.Offset(image.Pt((bw-is)/2, (h-is)/2)).Push(gtx.Ops)
			ic.Layout(ig, ChromeText)
			o.Pop()
			return layout.Dimensions{Size: image.Pt(bw, h-1)}
		})
		o.Pop()
	}
	if p := r.list.Position; p.First > 0 || p.Offset > 0 {
		arrow(&r.left, iconScrollLeft, 0)
	}
	if r.list.Position.BeforeEnd {
		arrow(&r.right, iconScrollRight, w-gtx.Dp(18))
	}
	cs.Pop()
	// The hovered button's tooltip, above everything else.
	m := op.Record(gtx.Ops)
	r.tips.layout(gtx, th, image.Rect(0, 0, w, 1<<15), false)
	op.Defer(gtx.Ops, m.Stop())
	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (r *Ribbon) group(gtx layout.Context, th *Theme, g RibbonGroup) layout.Dimensions {
	h := gtx.Dp(RibbonHeight) - 1
	capH := gtx.Dp(ribbonCaption)
	gc := gtx
	ch := RibbonContentHeight(gtx)
	gc.Constraints = layout.Constraints{Min: image.Pt(0, ch), Max: image.Pt(1<<20, ch)}
	off := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(ribbonPad))).Push(gtx.Ops)
	dims := g.Content(gc)
	off.Pop()

	// The caption, centered under the content; the launcher at its right.
	m := op.Record(gtx.Ops)
	gl := gtx
	gl.Constraints = layout.Constraints{Max: image.Pt(1<<20, capH)}
	cd := th.Label(11, g.Caption, MutedText, font.Normal).Layout(gl)
	caption := m.Stop()
	launch := 0
	if g.Launcher != nil {
		launch = gtx.Dp(16)
	}
	w := max(dims.Size.X, cd.Size.X+2*launch) + gtx.Dp(12)
	o := op.Offset(image.Pt((w-cd.Size.X)/2, h-capH+(capH-cd.Size.Y)/2-1)).Push(gtx.Ops)
	caption.Add(gtx.Ops)
	o.Pop()
	if g.Launcher != nil {
		s := gtx.Dp(14)
		o := op.Offset(image.Pt(w-s-gtx.Dp(4), h-capH+(capH-s)/2)).Push(gtx.Ops)
		gb := gtx
		gb.Constraints = layout.Exact(image.Pt(s, s))
		Hoverable(gb, g.Launcher, colors.Transparent, HoverBG, func(gtx layout.Context) layout.Dimensions {
			ic := gtx
			ic.Constraints = layout.Exact(image.Pt(s-2, s-2))
			o := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
			IconLauncher.Layout(ic, MutedText)
			o.Pop()
			tipButton(gtx, g.Launcher, g.Launcher.Hovered(), image.Pt(s, s))
			return layout.Dimensions{Size: image.Pt(s, s)}
		})
		o.Pop()
	}
	VLine(gtx.Ops, w-1, gtx.Dp(6), h-gtx.Dp(6), 1, ChromeLine)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// RibbonStack lays rows out top to bottom, spread over the group's height.
func RibbonStack(rows ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2*len(rows))
		for i, row := range rows {
			if i > 0 {
				children = append(children, layout.Flexed(1, layout.Spacer{}.Layout))
			}
			children = append(children, layout.Rigid(row))
		}
		gtx.Constraints.Min.X = 0
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
}

// RibbonRow lays controls out left to right, centered vertically in a
// RibbonRowHeight row.
func RibbonRow(items ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(items))
		for i, it := range items {
			children[i] = layout.Rigid(it)
		}
		gtx.Constraints.Min = image.Pt(0, gtx.Dp(RibbonRowHeight))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

// RibbonSide lays groups of a ribbon group side by side (a big button
// next to a stack of small rows), separated by gap.
func RibbonSide(gap unit.Dp, items ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for i, it := range items {
			if i > 0 {
				children = append(children, layout.Rigid(layout.Spacer{Width: gap}.Layout))
			}
			children = append(children, layout.Rigid(it))
		}
		return layout.Flex{}.Layout(gtx, children...)
	}
}

// BigButton is a tall ribbon button: a large icon over its label, with a
// drop arrow under the label when arrow is set. It fills the group's
// height.
func BigButton(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *widget.Icon, label string, arrow, checked bool) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return bigFace(gtx, th, btn, icon, label, arrow, checked)
	})
}

// bigFace draws a BigButton for btn (which handles the clicks).
func bigFace(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *widget.Icon, label string, arrow, checked bool) layout.Dimensions {
	h := gtx.Constraints.Min.Y
	if h == 0 {
		h = RibbonContentHeight(gtx)
	}
	fg := ChromeText
	if !gtx.Enabled() {
		fg = colors.Mix(ChromeText, ChromeBG, 0.6)
	}
	m := op.Record(gtx.Ops)
	lg := gtx
	lg.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(90), h)}
	ld := th.Label(11, label, fg, font.Normal).Layout(lg)
	lcall := m.Stop()
	is := gtx.Dp(26)
	w := max(ld.Size.X+gtx.Dp(10), gtx.Dp(44))
	paintFace(gtx, th, btn, image.Pt(w, h), checked)
	ic := gtx
	ic.Constraints = layout.Exact(image.Pt(is, is))
	o := op.Offset(image.Pt((w-is)/2, gtx.Dp(3))).Push(gtx.Ops)
	icon.Layout(ic, fg)
	o.Pop()
	y := gtx.Dp(3) + is + gtx.Dp(2)
	o = op.Offset(image.Pt((w-ld.Size.X)/2, y)).Push(gtx.Ops)
	lcall.Add(gtx.Ops)
	o.Pop()
	if arrow {
		s := gtx.Dp(14)
		ag := gtx
		ag.Constraints = layout.Exact(image.Pt(s, s))
		o = op.Offset(image.Pt((w-s)/2, min(y+ld.Size.Y-gtx.Dp(2), h-s))).Push(gtx.Ops)
		IconDropArrow.Layout(ag, MutedText)
		o.Pop()
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// paintFace paints a button's background (checked or hovered), sets
// the hand cursor and reports the button for its tooltip.
func paintFace(gtx layout.Context, th *Theme, btn *widget.Clickable, size image.Point, checked bool) {
	bg := colors.Transparent
	switch {
	case checked:
		bg = th.AccentTint
	case btn.Hovered() && gtx.Enabled():
		bg = HoverBG
	}
	if bg.A > 0 {
		FillRect(gtx.Ops, image.Rectangle{Max: size}, bg)
	}
	if gtx.Enabled() {
		area := clip.Rect{Max: size}.Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
	}
	tipButton(gtx, btn, btn.Hovered(), size)
}

// DropButton is a small ribbon button (icon and/or label plus a drop
// arrow) that opens dd's list. It returns the item chosen (or -1) and
// the button's size.
func DropButton(gtx layout.Context, th *Theme, dd *DropDown, icon *widget.Icon, label string, bar color.NRGBA, items []DropItem, listWidth unit.Dp) (int, layout.Dimensions) {
	i := dd.LayoutButton(gtx, th, listWidth, items, func(gtx layout.Context) layout.Dimensions {
		return dropFace(gtx, th, &dd.btn, icon, label, bar, dd.IsOpen(), false)
	})
	return i, layout.Dimensions{Size: dd.size}
}

// BigDropButton is BigButton opening dd's list.
func BigDropButton(gtx layout.Context, th *Theme, dd *DropDown, icon *widget.Icon, label string, items []DropItem, listWidth unit.Dp) (int, layout.Dimensions) {
	i := dd.LayoutButton(gtx, th, listWidth, items, func(gtx layout.Context) layout.Dimensions {
		return bigFace(gtx, th, &dd.btn, icon, label, true, dd.IsOpen())
	})
	return i, layout.Dimensions{Size: dd.size}
}

// SplitButton is a small button whose main part runs an action (btn) and
// whose arrow opens dd's list, like Excel's Borders and Font Color
// buttons. It returns the item chosen (or -1) and the button's size.
func SplitButton(gtx layout.Context, th *Theme, btn *widget.Clickable, dd *DropDown, icon *widget.Icon, bar color.NRGBA, items []DropItem, listWidth unit.Dp) (int, layout.Dimensions) {
	chosen := -1
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ToolButton(gtx, th, btn, icon, "", false, bar)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			chosen = dd.LayoutButton(gtx, th, listWidth, items, func(gtx layout.Context) layout.Dimensions {
				return dropFace(gtx, th, &dd.btn, nil, "", color.NRGBA{}, dd.IsOpen(), true)
			})
			return layout.Dimensions{Size: dd.size}
		}),
	)
	return chosen, dims
}

// dropFace draws a drop button: [icon] [label] ▾. arrowOnly draws just a
// narrow arrow (a split button's second half).
func dropFace(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *widget.Icon, label string, bar color.NRGBA, open, arrowOnly bool) layout.Dimensions {
	size := gtx.Dp(RibbonRowHeight)
	fg := ChromeText
	m := op.Record(gtx.Ops)
	x := gtx.Dp(4)
	if arrowOnly {
		x = 0
	}
	if icon != nil {
		is := gtx.Dp(18)
		ic := gtx
		ic.Constraints = layout.Exact(image.Pt(is, is))
		dy := (size - is) / 2
		if bar.A > 0 {
			dy -= gtx.Dp(2)
		}
		o := op.Offset(image.Pt(x, dy)).Push(gtx.Ops)
		icon.Layout(ic, fg)
		o.Pop()
		if bar.A > 0 {
			FillRect(gtx.Ops, image.Rect(x, dy+is, x+is, dy+is+gtx.Dp(4)), bar)
		}
		x += is + gtx.Dp(3)
	}
	if label != "" {
		lg := gtx
		lg.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(140), size)}
		mm := op.Record(gtx.Ops)
		d := th.Label(12, label, fg, font.Normal).Layout(lg)
		call := mm.Stop()
		o := op.Offset(image.Pt(x, (size-d.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		x += d.Size.X
	}
	as := gtx.Dp(14)
	ag := gtx
	ag.Constraints = layout.Exact(image.Pt(as, as))
	ax := x
	if arrowOnly {
		ax = -gtx.Dp(1)
	}
	o := op.Offset(image.Pt(ax, (size-as)/2)).Push(gtx.Ops)
	IconDropArrow.Layout(ag, MutedText)
	o.Pop()
	w := ax + as + gtx.Dp(2)
	if arrowOnly {
		w = gtx.Dp(12)
	}
	call := m.Stop()
	paintFace(gtx, th, btn, image.Pt(w, size), open)
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(w, size)}
}
