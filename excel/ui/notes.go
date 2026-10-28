package ui

import (
	"image"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Notes (red corner triangles, shown on hover or when the cell is
// selected) and hyperlinks (Ctrl+click to follow).

var (
	noteRed    = colors.RGB(0xC00000)
	noteBG     = colors.RGB(0xFFFFE1)
	noteBorder = colors.RGB(0x767676)
)

type noteDialog struct {
	ed              widget.Editor
	sheet, row, col int
	existing        bool
	author          string
}

// hoverNote is the cell under the pointer, whose note shows.
type hoverNote struct {
	ok       bool
	row, col int
}

// newNote adds a note to the active cell, or edits its note (Shift+F2).
func (a *App) newNote() {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	p := v.Sel.Active
	if m, ok := v.Sheet.MergeAt(p.Row, p.Col); ok {
		p = render.Pos{Row: m.R1, Col: m.C1}
	}
	d := &a.ins.note
	d.sheet, d.row, d.col = a.active, p.Row, p.Col
	n, ok := v.Sheet.NoteAt(p.Row, p.Col)
	d.existing, d.author = ok, n.Author
	if !ok {
		d.author = noteAuthor()
	}
	d.ed.SingleLine, d.ed.Submit = false, false
	d.ed.SetText(n.Text)
	d.ed.SetCaret(d.ed.Len(), d.ed.Len())
	a.showDialog(dlgNote)
}

// noteAuthor is the user's name for new notes.
func noteAuthor() string {
	for _, k := range []string{"USERNAME", "USER"} {
		if s := strings.TrimSpace(os.Getenv(k)); s != "" {
			return s
		}
	}
	return ""
}

// clearNotes deletes the notes in the selection.
func (a *App) clearNotes() {
	if !a.commitForCommand() {
		return
	}
	if a.book.ClearNotes(a.active, a.state().view.Sel.Range) {
		a.changed(calc.Result{})
	}
}

func (a *App) layoutNoteDialog(gtx layout.Context, k int) {
	d := &a.ins.note
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	del := false
	for a.ins.extra.Clicked(gtx) {
		del = true
	}
	switch {
	case k < 0:
		a.closeDialog()
		return
	case del:
		a.closeDialog()
		a.book.SetNote(d.sheet, d.row, d.col, nil)
		a.changed(calc.Result{})
		return
	case k > 0:
		a.closeDialog()
		text := strings.TrimRight(d.ed.Text(), "\n")
		if text == "" && !d.existing {
			return
		}
		a.book.SetNote(d.sheet, d.row, d.col, &model.Note{Author: d.author, Text: text})
		a.changed(calc.Result{})
		return
	}
	title := "New Note — " + model.CellName(d.row, d.col)
	if d.existing {
		title = "Edit Note — " + model.CellName(d.row, d.col)
	}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.author == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Bottom: 6}.Layout(gtx, a.ui.Label(13, d.author+":", chromeText, font.Bold).Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.textField(gtx, &d.ed, 380, 6, "Type the note")
			}),
		)
	}
	btns := []layout.Widget{a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel")}
	if d.existing {
		btns = append([]layout.Widget{a.plainButton(&a.ins.extra, "Delete Note")}, btns...)
	}
	a.modal(gtx, &a.ins.block, title, 420, body, btns...)
}

// drawAnnots draws note markers, the note under the pointer or of the
// active cell, the fill handle's drag outline, and the hand cursor over
// links. It runs right after the grid is drawn, in grid coordinates.
func (a *App) drawAnnots(gtx layout.Context, v *render.View) {
	sh := v.Sheet
	tri := max(gtx.Dp(6), 4)
	for k := range sh.Notes {
		rc, pane, ok := v.CellRect(k[0], k[1])
		if !ok || !rc.Overlaps(pane) {
			continue
		}
		cs := clip.Rect(pane).Push(gtx.Ops)
		for i := 0; i < tri; i++ {
			gui.HLine(gtx.Ops, rc.Max.X-1-tri+i, rc.Max.X-1, rc.Min.Y+i, 1, noteRed)
		}
		cs.Pop()
	}
	for k := range sh.Links {
		rc, pane, ok := v.CellRect(k[0], k[1])
		if !ok {
			continue
		}
		area := clip.Rect(rc.Intersect(pane)).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		area.Pop()
	}
	a.drawFillDrag(gtx, v)

	// The note shown: under the pointer, else the active cell's.
	var at render.Pos
	show := false
	if h := a.ins.hover; h.ok {
		if _, ok := sh.NoteAt(h.row, h.col); ok {
			at, show = render.Pos{Row: h.row, Col: h.col}, true
		}
	}
	if !show && !a.ed.active && !a.grid.dragging && !a.ins.fill.active {
		p := v.Sel.Active
		if _, ok := sh.NoteAt(p.Row, p.Col); ok {
			at, show = p, true
		}
	}
	if !show || a.ins.kind == dlgNote {
		return
	}
	n, _ := sh.NoteAt(at.Row, at.Col)
	rc, _, ok := v.CellRect(at.Row, at.Col)
	if !ok {
		return
	}
	a.notePopup(gtx, n, image.Pt(rc.Max.X+gtx.Dp(8), rc.Min.Y))
}

// notePopup draws a note's box with its top-left corner at pt (moved
// to stay inside the grid).
func (a *App) notePopup(gtx layout.Context, n model.Note, pt image.Point) {
	th := a.ui
	w := gtx.Dp(200)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, gtx.Dp(300))}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(6).Layout(g, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if n.Author == "" {
					return layout.Dimensions{}
				}
				return th.Label(12, n.Author+":", chromeText, font.Bold).Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := th.Label(12, n.Text, chromeText, font.Normal)
				l.MaxLines = 14
				return l.Layout(gtx)
			}),
		)
	})
	call := m.Stop()
	size := gtx.Constraints.Max
	if pt.X+dims.Size.X > size.X {
		pt.X = max(size.X-dims.Size.X-2, 0)
	}
	if pt.Y+dims.Size.Y > size.Y {
		pt.Y = max(size.Y-dims.Size.Y-2, 0)
	}
	off := op.Offset(pt).Push(gtx.Ops)
	box := image.Rectangle{Max: dims.Size}
	gui.FillRect(gtx.Ops, box.Add(image.Pt(2, 2)), colors.RGBA(0, 0, 0, 0x30))
	gui.FillRect(gtx.Ops, box.Inset(-1), noteBorder)
	gui.FillRect(gtx.Ops, box, noteBG)
	call.Add(gtx.Ops)
	off.Pop()
}

// gridHover tracks the cell under the pointer for notes.
func (a *App) gridHover(v *render.View, pos image.Point) {
	hit := v.HitTest(float32(pos.X), float32(pos.Y))
	h := hoverNote{}
	if hit.Area == render.AreaCell {
		if _, ok := v.Sheet.NoteAt(hit.Row, hit.Col); ok {
			h = hoverNote{true, hit.Row, hit.Col}
		}
	}
	if h != a.ins.hover {
		a.ins.hover = h
		if a.win != nil {
			a.win.Invalidate()
		}
	}
}

// followLinkAt follows the hyperlink of a Ctrl+clicked cell; false when
// the cell has none.
func (a *App) followLinkAt(v *render.View, hit render.Hit) bool {
	if hit.Area != render.AreaCell {
		return false
	}
	r, c := hit.Row, hit.Col
	if m, ok := v.Sheet.MergeAt(r, c); ok {
		r, c = m.R1, m.C1
	}
	h, ok := v.Sheet.LinkAt(r, c)
	if !ok {
		return false
	}
	if h.Target == "" {
		sheet, rng, err := a.book.ResolveRef(h.Location, a.active)
		if err != nil {
			a.errMsg = "Reference isn't valid: " + h.Location
			return true
		}
		a.selectRange(sheet, rng)
		return true
	}
	target := h.Target
	if h.Location != "" {
		target += "#" + h.Location
	}
	if err := openURL(target); err != nil {
		a.errMsg = "Cannot open " + target + ": " + err.Error()
	} else {
		a.notice = "Opening " + target
	}
	return true
}

// openURL opens a link with the system's handler.
func openURL(s string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", s)
	case "darwin":
		cmd = exec.Command("open", s)
	default:
		cmd = exec.Command("xdg-open", s)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
