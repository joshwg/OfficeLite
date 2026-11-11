// notes.go handles cell notes and Excel 365 comment threads (their
// dialog, markers and hover popups) and following hyperlinks.

package ui

import (
	"image"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Notes (red corner triangles, shown on hover or when the cell is
// selected), comment threads (purple corners, shown the same way) and
// hyperlinks (Ctrl+click to follow).

var (
	noteRed       = colors.RGB(0xC00000)
	commentPurple = colors.RGB(0x7030A0)
	noteBG        = colors.RGB(0xFFFFE1)
	noteBorder    = colors.RGB(0x767676)
)

// noteDialog is the state of the New / Edit Note dialog, which is also
// the comment dialog: the text, the cell it belongs to, whether the cell
// already had a note, and the note's author. For a comment (thread set)
// the text is the new post, and base the thread being replied to.
type noteDialog struct {
	ed              widget.Editor
	sheet, row, col int
	existing        bool
	author          string
	thread          bool
	base            model.Note
}

// hoverNote is the cell under the pointer, whose note shows.
type hoverNote struct {
	ok       bool
	row, col int
}

// newNote adds a note to the active cell, or edits its note (Shift+F2).
// On a cell with a comment it opens the comment to reply.
func (a *App) newNote() { a.openNoteDialog(false) }

// newComment starts a comment thread on the active cell, or replies to
// its thread. A cell with a note can't also have a comment.
func (a *App) newComment() { a.openNoteDialog(true) }

// openNoteDialog opens the note dialog (thread false) or the comment
// dialog for the active cell; what the cell has decides on a cell that
// has either.
func (a *App) openNoteDialog(thread bool) {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	p := activeCell(v)
	d := &a.ins.note
	d.sheet, d.row, d.col = a.active, p.Row, p.Col
	n, ok := v.Sheet.NoteAt(p.Row, p.Col)
	if ok {
		thread = n.Threaded
	}
	d.existing, d.author, d.thread, d.base = ok, n.Author, thread, n
	if !ok || thread {
		d.author = noteAuthor()
	}
	d.ed.SingleLine, d.ed.Submit = false, false
	if thread {
		d.ed.SetText("")
	} else {
		d.ed.SetText(n.Text)
	}
	d.ed.SetCaret(d.ed.Len(), d.ed.Len())
	a.showDialog(dlgNote)
}

// activeCell is the active cell, or the top-left cell of the merged
// cell it is in.
func activeCell(v *render.View) render.Pos {
	p := v.Sel.Active
	if m, ok := v.Sheet.MergeAt(p.Row, p.Col); ok {
		p = render.Pos{Row: m.R1, Col: m.C1}
	}
	return p
}

// toggleResolved marks the active cell's comment thread resolved, or
// reopens it.
func (a *App) toggleResolved() {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	p := activeCell(v)
	n, ok := v.Sheet.NoteAt(p.Row, p.Col)
	if !ok || !n.Threaded {
		return
	}
	n.Resolved = !n.Resolved
	a.book.SetNote(a.active, p.Row, p.Col, &n)
	a.changed(calc.Result{})
}

// noteAuthor is the user's name for new notes and comments.
func noteAuthor() string { return gui.UserName() }

// clearNotes deletes the notes in the selection.
func (a *App) clearNotes() {
	if !a.commitForCommand() {
		return
	}
	done := false
	a.eachRange(func(r model.Range) { done = a.book.ClearNotes(a.active, r) || done })
	if done {
		a.changed(calc.Result{})
	}
}

// layoutNoteDialog lays out the note dialog and applies it: OK sets the note
// (an empty new note adds nothing), Delete Note removes it, Cancel or Esc
// closes. k is the dialog key: 1 for Enter, -1 for Esc, 0 for none.
func (a *App) layoutNoteDialog(gtx layout.Context, k int) {
	d := &a.ins.note
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	del, resolve := false, false
	for a.ins.extra.Clicked(gtx) {
		del = true
	}
	for a.ins.extra2.Clicked(gtx) {
		resolve = true
	}
	if d.thread {
		a.layoutCommentDialog(gtx, k, del, resolve)
		return
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
				return gui.TextField(gtx, a.ui, &d.ed, 380, 6, "Type the note")
			}),
		)
	}
	btns := []layout.Widget{a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel")}
	if d.existing {
		btns = append([]layout.Widget{a.plainButton(&a.ins.extra, "Delete Note")}, btns...)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, title, 420, body, btns...)
}

// layoutCommentDialog lays out the comment dialog and applies it: Post
// starts the thread or adds the typed reply (which reopens a resolved
// thread), Resolve / Reopen marks it (keeping a typed reply), Delete
// Thread removes it, Cancel or Esc closes. k is as for the note dialog.
func (a *App) layoutCommentDialog(gtx layout.Context, k int, del, resolve bool) {
	d := &a.ins.note
	text := strings.TrimSpace(d.ed.Text())
	post := func(n model.Note) model.Note {
		if text == "" {
			return n
		}
		p := model.Post{Author: d.author, Text: text, Time: time.Now()}
		if !d.existing {
			return model.Note{Threaded: true, Author: p.Author, Text: p.Text, Time: p.Time}
		}
		n = n.WithReply(p)
		n.Resolved = false
		return n
	}
	switch {
	case k < 0:
		a.closeDialog()
		return
	case del && d.existing:
		a.closeDialog()
		a.book.SetNote(d.sheet, d.row, d.col, nil)
		a.changed(calc.Result{})
		return
	case resolve && d.existing:
		a.closeDialog()
		n := post(d.base)
		n.Resolved = !d.base.Resolved
		a.book.SetNote(d.sheet, d.row, d.col, &n)
		a.changed(calc.Result{})
		return
	case k > 0:
		a.closeDialog()
		if text == "" {
			return
		}
		n := post(d.base)
		a.book.SetNote(d.sheet, d.row, d.col, &n)
		a.changed(calc.Result{})
		return
	}
	title := "New Comment — " + model.CellName(d.row, d.col)
	hint := "Start a conversation"
	if d.existing {
		title = "Comment — " + model.CellName(d.row, d.col)
		if d.base.Resolved {
			title += " (resolved)"
		}
		hint = "Reply"
	}
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !d.existing {
					return layout.Dimensions{}
				}
				gtx.Constraints.Max.Y = gtx.Dp(260)
				return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.threadPosts(gtx, d.base, 12)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.TextField(gtx, a.ui, &d.ed, 380, 4, hint)
			}),
		)
	}
	btns := []layout.Widget{a.accentButton(&a.ins.ok, "Post"), a.plainButton(&a.ins.cancel, "Cancel")}
	if d.existing {
		label := "Resolve Thread"
		if d.base.Resolved {
			label = "Reopen Thread"
		}
		btns = append([]layout.Widget{a.plainButton(&a.ins.extra, "Delete Thread"), a.plainButton(&a.ins.extra2, label)}, btns...)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, title, 420, body, btns...)
}

// threadPosts lists a thread's posts: each author (with the post's date)
// over its text, the most recent last; maxLines caps each post's text.
func (a *App) threadPosts(gtx layout.Context, n model.Note, maxLines int) layout.Dimensions {
	th := a.ui
	posts := n.Posts()
	children := make([]layout.FlexChild, 0, 2*len(posts)+1)
	for i, p := range posts {
		head := p.Author
		if head == "" {
			head = "Unknown"
		}
		if !p.Time.IsZero() {
			head += " · " + p.Time.Local().Format("Jan 2, 2006 3:04 PM")
		}
		var top unit.Dp
		if i > 0 {
			top = 6
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: top}.Layout(gtx, th.Label(12, head, chromeText, font.Bold).Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := th.Label(12, p.Text, chromeText, font.Normal)
				l.MaxLines = maxLines
				return l.Layout(gtx)
			}))
	}
	if n.Resolved {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6}.Layout(gtx, th.Label(11, "Resolved", mutedText, font.Normal).Layout)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// drawAnnots draws note markers, the note under the pointer or of the
// active cell, the fill handle's drag outline, and the hand cursor over
// links. It runs right after the grid is drawn, in grid coordinates.
func (a *App) drawAnnots(gtx layout.Context, v *render.View) {
	sh := v.Sheet
	tri := max(gtx.Dp(6), 4)
	for k, n := range sh.Notes {
		rc, pane, ok := v.CellRect(k[0], k[1])
		if !ok || !rc.Overlaps(pane) {
			continue
		}
		c := noteRed
		if n.Threaded {
			c = commentPurple
		}
		cs := clip.Rect(pane).Push(gtx.Ops)
		for i := 0; i < tri; i++ {
			gui.HLine(gtx.Ops, rc.Max.X-1-tri+i, rc.Max.X-1, rc.Min.Y+i, 1, c)
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
	if n.Threaded {
		w = gtx.Dp(260)
	}
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, gtx.Dp(300))}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(6).Layout(g, func(gtx layout.Context) layout.Dimensions {
		if n.Threaded {
			return a.threadPosts(gtx, n, 6)
		}
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
	if err := gui.OpenURL(target); err != nil {
		a.errMsg = "Cannot open " + target + ": " + err.Error()
	} else {
		a.notice = "Opening " + target
	}
	return true
}
