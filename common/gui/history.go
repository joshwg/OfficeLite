package gui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// HistoryList is an Undo or Redo split button with its drop-down list,
// as in Office: the button takes back (or repeats) one step; the arrow
// lists the steps, the next one first. Pointing at an entry highlights
// it and every entry above it, which go with it, and the footer counts
// them ("Undo 3 Actions", "Cancel" when none is pointed at); clicking
// takes them all back as one.
type HistoryList struct {
	dd    DropDown
	items []widget.Clickable
	list  widget.List
	foot  widget.Clickable
	// hover is how many entries are highlighted (the pointed-at one and
	// those above it).
	hover int
	tip   string // the button's tooltip name, last set
}

// maxHistory is the most entries a HistoryList shows.
const maxHistory = 100

// IsOpen reports whether the list is showing.
func (h *HistoryList) IsOpen() bool { return h.dd.IsOpen() }

// Arrow is the button that opens the list (for tests that click it).
func (h *HistoryList) Arrow() *widget.Clickable { return &h.dd.btn }

// Hovered is how many entries are highlighted, for tests.
func (h *HistoryList) Hovered() int { return h.hover }

// Footer is the list's footer text: "Undo 3 Actions", or "Cancel".
func (h *HistoryList) Footer(verb string) string { return footer(verb, h.hover) }

func footer(verb string, n int) string {
	switch n {
	case 0:
		return "Cancel"
	case 1:
		return verb + " 1 Action"
	}
	return verb + " " + strconv.Itoa(n) + " Actions"
}

// ShowOpen opens the list with its first n entries highlighted, for
// preview tools.
func (h *HistoryList) ShowOpen(n int) { h.dd.open, h.hover = true, n }

// SplitButton draws btn with icon and the list's arrow; verb ("Undo")
// and keys ("Ctrl+Z") make the tooltip, with the next step's name
// ("Undo Typing 'x'"), and labels are the steps, the next first. Both
// are disabled while labels is empty. It returns how many steps were
// chosen: 1 for a click on the button, the entries taken for a click in
// the list, 0 for none.
func (h *HistoryList) SplitButton(gtx layout.Context, th *Theme, btn *widget.Clickable, icon *Icon, verb, keys string, labels []string) (int, layout.Dimensions) {
	if len(labels) == 0 {
		gtx = gtx.Disabled()
		h.dd.Close()
	}
	if tip := HistoryLabel(verb, labels); tip != h.tip {
		h.tip = tip
		SetTip(btn, tip, keys, verb+" your last action.")
		SetTip(&h.dd, verb, "", "Choose the actions to "+lower(verb)+".")
	}
	n := 0
	for btn.Clicked(gtx) {
		n = 1
	}
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ToolButton(gtx, th, btn, icon, "", false, color.NRGBA{})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h.dd.LayoutPanel(gtx, func(gtx layout.Context) layout.Dimensions {
				return dropFace(gtx, th, &h.dd.btn, nil, "", color.NRGBA{}, h.dd.IsOpen(), true)
			}, func(gtx layout.Context) (layout.Dimensions, bool) {
				d, k, done := h.panel(gtx, th, verb, labels)
				if k > 0 {
					n = k
				}
				return d, done
			})
			return layout.Dimensions{Size: h.dd.size}
		}),
	)
	if !h.dd.IsOpen() {
		h.hover = 0
	}
	return n, dims
}

// HistoryLabel names the next step for a menu item or tooltip: "Undo
// Typing 'x'", or just verb when there is none.
func HistoryLabel(verb string, labels []string) string {
	if len(labels) == 0 {
		return verb
	}
	return verb + " " + labels[0]
}

func lower(s string) string {
	b := []byte(s)
	if len(b) > 0 && b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}

// panel draws the open list: the entries over the footer. It returns how
// many entries were chosen and whether the list is done.
func (h *HistoryList) panel(gtx layout.Context, th *Theme, verb string, labels []string) (layout.Dimensions, int, bool) {
	labels = labels[:min(len(labels), maxHistory)]
	if len(h.items) != len(labels) {
		h.items = make([]widget.Clickable, len(labels))
	}
	chosen, done := 0, false
	for i := range h.items {
		for h.items[i].Clicked(gtx) {
			chosen, done = i+1, true
		}
	}
	for h.foot.Clicked(gtx) {
		chosen, done = h.hover, true
	}
	w, rowH, footH := gtx.Dp(240), gtx.Dp(22), gtx.Dp(26)
	listH := rowH * min(len(labels), 12)
	pad := gtx.Dp(2)

	// The highlight and footer follow the entries the pointer was over
	// last frame.
	shown, seen := h.hover, 0
	g := gtx
	g.Constraints = layout.Exact(image.Pt(w, listH))
	off := op.Offset(image.Pt(0, pad)).Push(gtx.Ops)
	h.list.Axis = layout.Vertical
	material.List(th.Theme, &h.list).Layout(g, len(labels), func(gtx layout.Context, i int) layout.Dimensions {
		if h.items[i].Hovered() {
			seen = i + 1
		}
		bg := color.NRGBA{}
		if i < shown {
			bg = th.AccentTint
		}
		return Hoverable(gtx, &h.items[i], bg, bg, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, rowH))
			return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := th.Label(13, labels[i], ChromeText, font.Normal)
				l.MaxLines = 1
				return Middle(gtx, l.Layout)
			})
		})
	})
	off.Pop()
	if seen != h.hover && !done {
		h.hover = seen
		gtx.Execute(op.InvalidateCmd{})
	}

	// The footer, under a line.
	y := pad + listH + pad
	HLine(gtx.Ops, 0, w, y, 1, ChromeLine)
	off = op.Offset(image.Pt(0, y+1)).Push(gtx.Ops)
	g.Constraints = layout.Exact(image.Pt(w, footH))
	Hoverable(g, &h.foot, color.NRGBA{}, HoverBG, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, th.Label(12, footer(verb, shown), ChromeText, font.Normal).Layout)
	})
	off.Pop()
	return layout.Dimensions{Size: image.Pt(w, y+1+footH)}, chosen, done
}
