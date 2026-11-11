// comments.go is the Comments pane (Insert > Show Comments) and New
// Comment (Ctrl+Alt+M): every comment as a card with its replies, in the
// order of the text it is on, that text highlighted on the page. Clicking
// a card selects the comment's text; the active comment (clicked, or the
// one holding the caret) takes a reply, and can be resolved, reopened or
// deleted. Comments live in the body's text only (not in headers).

package ui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Comment highlight colors on the page, and the active card's border.
var (
	commentHL       = colors.RGBA(0xF4, 0xB1, 0x83, 0x50)
	commentHLActive = colors.RGBA(0xF4, 0x8B, 0x45, 0x80)
	commentBorder   = colors.RGB(0xD0D0D0)
)

// commentsPane is the pane's state.
type commentsPane struct {
	open    bool
	list    widget.List
	cards   []widget.Clickable
	active  *model.Comment // the comment whose card takes a reply
	writing bool           // a new comment is being written (on sel)
	sel     model.Selection
	text    widget.Editor // the new comment or the reply
	focus   bool          // focus text at the next frame

	post, cancel, resolve, del, closeBtn widget.Clickable

	// anchors caches the editor's comments at version.
	anchors []model.CommentAnchor
	version int
	ed      *model.Editor
	caret   model.Pos // the caret last seen, to follow it into comments
}

// comments returns the document's comments with where they are.
func (d *window) comments() []model.CommentAnchor {
	c := &d.cm
	if c.ed != d.ed || c.version != d.ed.Version || c.anchors == nil {
		c.ed, c.version = d.ed, d.ed.Version
		c.anchors = d.ed.Comments()
		if c.anchors == nil {
			c.anchors = []model.CommentAnchor{}
		}
	}
	return c.anchors
}

// resetComments sets the pane up for a newly opened document: shown when
// it has comments.
func (d *window) resetComments() {
	d.cm = commentsPane{}
	d.cm.open = len(d.comments()) > 0
}

// newComment starts writing a comment on the selection (Ctrl+Alt+M).
func (d *window) newComment() {
	if d.hf.on || d.opening() {
		return
	}
	c := &d.cm
	c.open, c.writing, c.sel, c.active = true, true, d.ed.Sel, nil
	c.text.SetText("")
	c.focus = true
}

// toggleComments shows or hides the pane.
func (d *window) toggleComments() {
	d.cm.open = !d.cm.open
	if !d.cm.open {
		d.cm.writing = false
		d.v.focusPending = true
	}
}

// replyTo opens comment c's card for a reply.
func (d *window) replyTo(c *model.Comment) {
	d.cm.open, d.cm.writing, d.cm.active = true, false, c
	d.cm.text.SetText("")
	d.cm.focus = true
}

// commentAction runs the pane's buttons.
func (d *window) commentAction(gtx layout.Context) {
	c := &d.cm
	text := strings.TrimSpace(c.text.Text())
	for c.post.Clicked(gtx) {
		switch {
		case text == "":
		case c.writing:
			d.ed.Sel = c.sel
			nc := model.NewComment(gui.UserName(), text)
			d.ed.AddComment(nc)
			c.writing, c.active = false, nc
			c.text.SetText("")
			d.edited()
		case c.active != nil:
			next := c.active.Clone()
			next.Replies = append(next.Replies, model.NewComment(gui.UserName(), text))
			next.Done = false
			d.ed.UpdateComment(c.active, next, "Reply to Comment")
			c.active = next
			c.text.SetText("")
			d.edited()
		}
	}
	for c.cancel.Clicked(gtx) {
		c.writing = false
		c.text.SetText("")
		d.v.focusPending = true
	}
	for c.resolve.Clicked(gtx) {
		if c.active != nil {
			next := c.active.Clone()
			next.Done = !next.Done
			d.ed.UpdateComment(c.active, next, either(next.Done, "Resolve Comment", "Reopen Comment"))
			c.active = next
			d.edited()
		}
	}
	for c.del.Clicked(gtx) {
		if c.active != nil {
			d.ed.DeleteComment(c.active)
			c.active = nil
			d.edited()
		}
	}
	for c.closeBtn.Clicked(gtx) {
		c.open, c.writing = false, false
		d.v.focusPending = true
	}
}

// either is yes when b is set, else no.
func either(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// commentsPaneLayout lays out the pane at the right of the page view.
func (d *window) commentsPaneLayout(gtx layout.Context) layout.Dimensions {
	c := &d.cm
	if !c.open {
		return layout.Dimensions{}
	}
	d.commentAction(gtx)
	if !c.open {
		return layout.Dimensions{}
	}
	anchors := d.comments()
	// The comment holding the caret becomes the active one when the caret
	// moves there.
	if caret := d.ed.Sel.Caret; caret != c.caret {
		c.caret = caret
		if cm, ok := d.ed.CommentAt(caret); ok && !c.writing {
			c.active = cm
		}
	}
	if len(c.cards) < len(anchors) {
		c.cards = make([]widget.Clickable, len(anchors)+8)
	}
	for i, a := range anchors {
		for c.cards[i].Clicked(gtx) {
			if c.active != a.Comment {
				c.text.SetText("")
			}
			c.active, c.writing = a.Comment, false
			d.ed.Sel = model.Selection{Anchor: a.Start, Caret: a.End}
			c.caret = a.End
			d.v.ensureCaret = true
			d.relayout()
		}
	}
	if c.focus {
		c.focus = false
		gtx.Execute(key.FocusCmd{Tag: &c.text})
	}

	th := d.th
	w := gtx.Dp(290)
	h := gtx.Constraints.Max.Y
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), gui.ChromeBG)
	gui.VLine(gtx.Ops, 0, 0, h, 1, gui.ChromeLine)

	btn := func(b *widget.Clickable, s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.ToolButton(gtx, th, b, nil, s, false, color.NRGBA{})
		})
	}
	editor := func(hint string, buttons ...layout.FlexChild) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return gui.TextField(gtx, th, &c.text, unit.Dp(gtx.Metric.PxToDp(gtx.Constraints.Max.X)), 3, hint)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx, buttons...)
				}),
			)
		}
	}
	card := func(gtx layout.Context, active bool, w layout.Widget) layout.Dimensions {
		m := opRecord(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(8).Layout(gtx, w)
		})
		size := m.dims.Size
		border := commentBorder
		if active {
			border = th.Accent
		}
		gui.FillRect(gtx.Ops, image.Rectangle{Max: size}.Inset(-1), border)
		gui.FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.White)
		m.call.Add(gtx.Ops)
		return m.dims
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 10, Right: 4, Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, th.Label(15, "Comments", gui.ChromeText, font.SemiBold).Layout),
					btn(&c.closeBtn, "✕"),
				)
			})
		}),
	}
	if c.writing {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 8, Right: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return card(gtx, true, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(th.Label(13, nameOr(gui.UserName()), gui.ChromeText, font.Bold).Layout),
						layout.Rigid(editor("Start a conversation", btn(&c.post, "Post"), btn(&c.cancel, "Cancel"))),
					)
				})
			})
		}))
	}
	if len(anchors) == 0 && !c.writing {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 10, Right: 10, Top: 4}.Layout(gtx,
				th.Label(13, "No comments. Select text and choose Insert > New Comment (Ctrl+Alt+M).", gui.MutedText, font.Normal).Layout)
		}))
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return paneList(gtx, th, &c.list, len(anchors), func(gtx layout.Context, i int) layout.Dimensions {
			a := anchors[i]
			active := a.Comment == c.active && !c.writing
			return layout.Inset{Left: 8, Right: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return card(gtx, active, func(gtx layout.Context) layout.Dimensions {
					rows := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.cards[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return d.commentPosts(gtx, a.Comment)
							})
						}),
					}
					if active {
						label := "Resolve"
						if a.Comment.Done {
							label = "Reopen"
						}
						rows = append(rows, layout.Rigid(editor("Reply", btn(&c.post, "Reply"), btn(&c.resolve, label), btn(&c.del, "Delete"))))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
				})
			})
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// nameOr is a name, or "You" when unknown.
func nameOr(s string) string {
	if s == "" {
		return "You"
	}
	return s
}

// commentPosts lays out a comment and its replies: each author (with the
// date) over the text; a resolved thread is marked and dimmed.
func (d *window) commentPosts(gtx layout.Context, c *model.Comment) layout.Dimensions {
	th := d.th
	text := gui.ChromeText
	if c.Done {
		text = gui.MutedText
	}
	var rows []layout.FlexChild
	for i, x := range append([]*model.Comment{c}, c.Replies...) {
		head := nameOr(x.Author)
		if x.Author == "" {
			head = "Unknown author"
		}
		if !x.Date.IsZero() {
			head += " · " + x.Date.Local().Format("Jan 2, 2006 3:04 PM")
		}
		var left, top unit.Dp
		if i > 0 {
			left, top = 12, 8
		}
		body := x.Text()
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: left, Top: top}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(th.Label(12, head, text, font.Bold).Layout),
					layout.Rigid(th.Label(13, body, text, font.Normal).Layout),
				)
			})
		}))
	}
	if c.Done {
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6}.Layout(gtx, th.Label(11, "✓ Resolved", gui.MutedText, font.Normal).Layout)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// drawComments highlights the commented text on page cpg of eng, the
// active comment's more strongly.
func (d *window) drawComments(gtx layout.Context, eng *wl.Engine, cpg *wl.Page, rect func(wl.Rect) image.Rectangle) {
	if d.hf.on || len(cpg.Lines) == 0 {
		return
	}
	anchors := d.comments()
	if len(anchors) == 0 {
		return
	}
	lo, hi := cpg.Lines[0].Para, cpg.Lines[0].Para
	for _, pl := range cpg.Lines {
		lo, hi = min(lo, pl.Para), max(hi, pl.Para)
	}
	for _, a := range anchors {
		if a.Start == a.End || a.End.Para < lo || a.Start.Para > hi {
			continue
		}
		c := commentHL
		if a.Comment == d.cm.active && d.cm.open {
			c = commentHLActive
		}
		for _, r := range eng.SelectionRects(cpg, a.Start, a.End) {
			gui.FillRect(gtx.Ops, rect(r), c)
		}
	}
}

// recorded is a widget laid out into a macro, to draw behind it.
type recorded struct {
	call op.CallOp
	dims layout.Dimensions
}

// opRecord lays out w into a macro.
func opRecord(gtx layout.Context, w layout.Widget) recorded {
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	return recorded{m.Stop(), dims}
}
