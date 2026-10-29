package gui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
)

// RibbonKit builds the controls of a ribbon. Did is set when one of them
// runs a command, so the window can give the keyboard back to the
// document afterwards.
type RibbonKit struct {
	Th  *Theme
	Did bool
	// Op, when set, names the undo steps a command makes after its
	// button's tooltip ("Bold", "Fill Color"), as the Undo list shows
	// them; it returns the function that ends the naming. Drop and Split
	// menus leave the naming to the command.
	Op func(label string) func()
}

// Click runs f for each click on b.
func (k *RibbonKit) Click(gtx layout.Context, b *widget.Clickable, f func()) {
	for b.Clicked(gtx) {
		k.run(b, f)
	}
}

// run runs a command of button b, naming its undo steps.
func (k *RibbonKit) run(b any, f func()) {
	if t, ok := tipFor(b); ok && k.Op != nil && t.Name != "" {
		defer k.Op(t.Name)()
	}
	f()
	k.Did = true
}

// chosen runs apply for a picked menu item (i >= 0); named, the undo
// steps take dd's tooltip name.
func (k *RibbonKit) chosen(i int, dd *DropDown, apply func(int)) {
	if i < 0 {
		return
	}
	var b any
	if dd != nil {
		b = &dd.btn
	}
	k.run(b, func() { apply(i) })
}

// Btn is a small icon button, highlighted when checked.
func (k *RibbonKit) Btn(b *widget.Clickable, ic *Icon, checked bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return ToolButton(gtx, k.Th, b, ic, "", checked, color.NRGBA{})
	}
}

// Text is a small button showing label instead of an icon.
func (k *RibbonKit) Text(b *widget.Clickable, label string, checked bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return ToolButton(gtx, k.Th, b, nil, label, checked, color.NRGBA{})
	}
}

// Glyph is a small button showing one character at icon size.
func (k *RibbonKit) Glyph(b *widget.Clickable, glyph string, checked bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return GlyphButton(gtx, k.Th, b, glyph, checked)
	}
}

// Small is a small icon button with its label beside it.
func (k *RibbonKit) Small(b *widget.Clickable, ic *Icon, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return SmallButton(gtx, k.Th, b, ic, label)
	}
}

// BigBtn is a tall button: a large icon over its label.
func (k *RibbonKit) BigBtn(b *widget.Clickable, ic *Icon, label string, checked bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return BigButton(gtx, k.Th, b, ic, label, false, checked)
	}
}

// Combo is a drop-down list width wide showing cur, its list listW wide;
// apply gets the picked item's index.
func (k *RibbonKit) Combo(dd *DropDown, width unit.Dp, cur string, items []DropItem, listW unit.Dp, apply func(int)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		dd.ListWidth = listW
		gtx.Constraints.Min.Y = gtx.Dp(RibbonRowHeight)
		var i int
		dims := Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			i = dd.Layout(gtx, k.Th, width, cur, items)
			return layout.Dimensions{Size: image.Pt(gtx.Dp(width), gtx.Dp(24))}
		})
		k.chosen(i, dd, apply)
		return dims
	}
}

// Drop is a small button that opens a menu.
func (k *RibbonKit) Drop(dd *DropDown, ic *Icon, label string, items []DropItem, listW unit.Dp, apply func(int)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		i, dims := DropButton(gtx, k.Th, dd, ic, label, color.NRGBA{}, items, listW)
		k.chosen(i, nil, apply)
		return dims
	}
}

// BigDrop is a tall button that opens a menu.
func (k *RibbonKit) BigDrop(dd *DropDown, ic *Icon, label string, items []DropItem, listW unit.Dp, apply func(int)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		i, dims := BigDropButton(gtx, k.Th, dd, ic, label, items, listW)
		k.chosen(i, dd, apply)
		return dims
	}
}

// Split is a button with a separate drop arrow for its menu; bar is the
// color strip under the icon (zero for none).
func (k *RibbonKit) Split(b *widget.Clickable, dd *DropDown, ic *Icon, bar color.NRGBA, items []DropItem, listW unit.Dp, apply func(int)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		i, dims := SplitButton(gtx, k.Th, b, dd, ic, bar, items, listW)
		k.chosen(i, nil, apply)
		return dims
	}
}

// ColorSplit is a split button with a color palette; none labels the
// "no color" choice.
func (k *RibbonKit) ColorSplit(b *widget.Clickable, cp *ColorPicker, ic *Icon, bar color.NRGBA, none string, apply func(ColorChoice)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		c, ok, dims := cp.SplitButton(gtx, k.Th, b, ic, bar, none)
		if ok {
			k.run(cp, func() { apply(c) })
		}
		return dims
	}
}

// History is an Undo or Redo split button with its list of labels (the
// steps, next first); apply gets how many to take back or repeat.
func (k *RibbonKit) History(b *widget.Clickable, h *HistoryList, ic *Icon, verb, keys string, labels []string, apply func(n int)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		n, dims := h.SplitButton(gtx, k.Th, b, ic, verb, keys, labels)
		if n > 0 {
			apply(n)
			k.Did = true
		}
		return dims
	}
}

// Gap is horizontal space between controls.
func Gap(w unit.Dp) layout.Widget { return layout.Spacer{Width: w}.Layout }

// Enabled is w, drawn disabled (and inert) unless on.
func Enabled(on bool, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !on {
			gtx = gtx.Disabled()
		}
		return w(gtx)
	}
}
