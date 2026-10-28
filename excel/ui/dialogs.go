package ui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
)

// The dialogs of the Insert menu, Find & Select and Sort: their shared
// state, keys and drawing helpers. Each dialog lives in its own file
// (insert.go, find.go, sort.go, notes.go).

type dlgKind int

const (
	dlgNone dlgKind = iota
	dlgInsertCells
	dlgDeleteCells
	dlgFunction
	dlgSymbol
	dlgHyperlink
	dlgNote
	dlgFind
	dlgGoTo
	dlgSort
)

// insertState is the open dialog (at most one) and the fill handle and
// note hover state.
type insertState struct {
	kind  dlgKind
	focus bool // focus the dialog's first field at the next frame
	key   int  // Enter (1) or Esc (-1) pressed, from insertDialogInput
	block int  // event tag absorbing clicks behind a modal dialog
	tag   int  // focus tag of dialogs without a text field
	err   string

	ok, cancel, extra widget.Clickable

	shift widget.Enum // Insert / Delete Cells

	fn   functionDialog
	sym  symbolDialog
	link linkDialog
	note noteDialog
	find findDialog
	gt   gotoDialog
	sort sortDialog

	fill  fillDrag
	hover hoverNote
}

// showDialog shows dialog k, closing any other.
func (a *App) showDialog(k dlgKind) {
	a.menu.Close()
	a.ins.kind, a.ins.focus, a.ins.err, a.ins.key = k, true, "", 0
}

// closeDialog hides the open dialog and gives the keyboard back to the
// grid (or to the cell entry being edited).
func (a *App) closeDialog() {
	a.ins.kind = dlgNone
	if a.ed.active {
		a.ed.focus = &a.ed.cell
		if a.ed.inBar {
			a.ed.focus = &a.ed.bar
		}
		return
	}
	a.grid.focusPending = true
}

// dialogFocusTag is what has the keyboard in the open dialog.
func (a *App) dialogFocusTags() []any {
	d := &a.ins
	switch d.kind {
	case dlgFunction:
		return []any{&d.fn.search, &d.tag}
	case dlgHyperlink:
		return []any{&d.link.display, &d.link.address, &d.link.tip}
	case dlgFind:
		return []any{&d.find.what, &d.find.with, &d.tag}
	case dlgGoTo:
		return []any{&d.gt.ref, &d.tag}
	case dlgNote:
		return nil // Enter types a line break; Esc is handled by the note editor
	}
	return []any{&d.tag}
}

// insertDialogInput takes the open dialog's Enter and Esc before the
// window's shortcuts do.
func (a *App) insertDialogInput(gtx layout.Context) {
	d := &a.ins
	if d.kind == dlgNone {
		return
	}
	for _, t := range a.dialogFocusTags() {
		if k := dialogKeys(gtx, t); k != 0 {
			d.key = k
		}
	}
	if d.kind == dlgNote {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &d.note.ed, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				d.key = -1
			}
		}
	}
}

// layoutInsertDialogs draws the open dialog.
func (a *App) layoutInsertDialogs(gtx layout.Context) {
	d := &a.ins
	if d.kind == dlgNone {
		return
	}
	if d.focus {
		d.focus = false
		tags := a.dialogFocusTags()
		switch {
		case d.kind == dlgNote:
			gtx.Execute(key.FocusCmd{Tag: &d.note.ed})
		case d.kind == dlgHyperlink && d.link.focusAddr:
			gtx.Execute(key.FocusCmd{Tag: &d.link.address})
		case len(tags) > 0:
			gtx.Execute(key.FocusCmd{Tag: tags[0]})
		}
	}
	k := d.key
	d.key = 0
	switch d.kind {
	case dlgInsertCells, dlgDeleteCells:
		a.layoutCellsDialog(gtx, k)
	case dlgFunction:
		a.layoutFunctionDialog(gtx, k)
	case dlgSymbol:
		a.layoutSymbolDialog(gtx, k)
	case dlgHyperlink:
		a.layoutLinkDialog(gtx, k)
	case dlgNote:
		a.layoutNoteDialog(gtx, k)
	case dlgFind:
		a.layoutFindDialog(gtx, k)
	case dlgGoTo:
		a.layoutGoToDialog(gtx, k)
	case dlgSort:
		a.layoutSortDialog(gtx, k)
	}
}

// focusArea makes the dialog's area a key handler so that Enter and Esc
// reach it when no text field has the focus.
func (a *App) focusArea(gtx layout.Context) {
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &a.ins.tag)
	area.Pop()
}

// panel draws a dialog box that doesn't block the window (Find and
// Replace), at the top right of the grid.
func (a *App) panel(gtx layout.Context, title string, width unit.Dp, body layout.Widget) {
	size := gtx.Constraints.Max
	w := min(gtx.Dp(width), size.X-gtx.Dp(16))
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, size.Y)}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(14).Layout(g, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.ui.Label(15, title, chromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 10}.Layout),
			layout.Rigid(body),
		)
	})
	call := m.Stop()
	pos := image.Pt(size.X-dims.Size.X-gtx.Dp(28), gtx.Dp(150))
	pos.Y = max(min(pos.Y, size.Y-dims.Size.Y), 0)
	off := op.Offset(pos).Push(gtx.Ops)
	box := image.Rectangle{Max: dims.Size}
	gui.FillRect(gtx.Ops, box.Add(image.Pt(3, 3)), colors.RGBA(0, 0, 0, 0x28))
	gui.FillRect(gtx.Ops, box.Inset(-1), chromeLine)
	gui.FillRect(gtx.Ops, box, colors.White)
	// The box itself takes clicks, so they don't reach the grid below.
	area := clip.Rect(box).Push(gtx.Ops)
	event.Op(gtx.Ops, &a.ins.block)
	area.Pop()
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &a.ins.block, Kinds: pointer.Press}); !ok {
			break
		}
	}
	call.Add(gtx.Ops)
	off.Pop()
}

// textField draws an editor in a box w dp wide (lines tall).
func (a *App) textField(gtx layout.Context, ed *widget.Editor, w unit.Dp, lines int, hint string) layout.Dimensions {
	pw := min(gtx.Dp(w), gtx.Constraints.Max.X)
	h := gtx.Dp(unit.Dp(8 + 18*lines))
	border := chromeLine
	if gtx.Focused(ed) {
		border = accent
	}
	gui.FillRect(gtx.Ops, image.Rect(0, 0, pw, h), border)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, pw-1, h-1), colors.White)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(pw-gtx.Dp(12), 0), Max: image.Pt(pw-gtx.Dp(12), h-gtx.Dp(8))}
	cs := clip.Rect{Max: image.Pt(pw, h)}.Push(gtx.Ops)
	off := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(4))).Push(gtx.Ops)
	e := material.Editor(a.th, ed, hint)
	e.Font = gui.UIFont
	e.TextSize = 13
	e.HintColor = mutedText
	e.Layout(g)
	off.Pop()
	cs.Pop()
	return layout.Dimensions{Size: image.Pt(pw, h)}
}

// labeled is a "label  field" row with the label in a column lw dp wide.
func (a *App) labeled(label string, lw unit.Dp, field layout.Widget) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(lw)
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					return a.ui.Label(13, label, chromeText, font.Normal).Layout(gtx)
				}),
				layout.Flexed(1, field),
			)
		})
	})
}

// dropItems turns labels into drop-down items.
func dropItems(labels []string) []gui.DropItem {
	out := make([]gui.DropItem, len(labels))
	for i, l := range labels {
		out[i] = gui.DropItem{Label: l}
	}
	return out
}

// checkBox is a check box with a label.
func (a *App) checkBox(b *widget.Bool, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		cb := material.CheckBox(a.th, b, label)
		cb.Font = gui.UIFont
		cb.TextSize = 13
		cb.Color = chromeText
		cb.IconColor = accent
		return cb.Layout(gtx)
	}
}

// radio is a radio button of group e.
func (a *App) radio(e *widget.Enum, k, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		rb := material.RadioButton(a.th, e, k, label)
		rb.Font = gui.UIFont
		rb.TextSize = 13
		rb.Color = chromeText
		rb.IconColor = accent
		return rb.Layout(gtx)
	}
}

// errorLine shows the dialog's error, if any.
func (a *App) errorLine() layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if a.ins.err == "" {
			return layout.Dimensions{}
		}
		l := a.ui.Label(12, a.ins.err, errorText, font.Normal)
		l.MaxLines = 2
		return layout.Inset{Top: 4}.Layout(gtx, l.Layout)
	})
}

// commitForCommand ends cell editing before a command that changes cells;
// false when the entry is invalid.
func (a *App) commitForCommand() bool {
	return !a.ed.active || a.commitEdit(0, 0)
}

// opOffset pushes an offset of x, y pixels.
func opOffset(gtx layout.Context, x, y int) op.TransformStack {
	return op.Offset(image.Pt(x, y)).Push(gtx.Ops)
}
