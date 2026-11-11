// comments_test.go drives the Comments pane: Ctrl+Alt+M on a selection,
// posting, replying, resolving and deleting, with undo.

package ui

import (
	"testing"

	"gioui.org/io/key"

	"officelite/word/model"
)

// TestCommentsPane writes a comment on a selection and works it through
// the pane's buttons.
func TestCommentsPane(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Hello brave world")
	d.ed.Sel = model.Selection{Anchor: model.Pos{Para: 0, Off: 6}, Caret: model.Pos{Para: 0, Off: 11}}
	h.frame()
	h.press("M", key.ModShortcut|key.ModAlt)
	if !d.cm.open || !d.cm.writing {
		t.Fatalf("Ctrl+Alt+M: open %v writing %v", d.cm.open, d.cm.writing)
	}
	click := func(text string, btn interface{ Click() }) {
		t.Helper()
		d.cm.text.SetText(text)
		h.frame()
		btn.Click()
		h.frame()
		h.frame()
	}
	click("Why brave?", &d.cm.post)
	cs := d.ed.Comments()
	if len(cs) != 1 || cs[0].Comment.Text() != "Why brave?" || d.ed.SelectedText() != "brave" {
		t.Fatalf("after posting: %+v, selection %q", cs, d.ed.SelectedText())
	}
	if d.cm.active != cs[0].Comment {
		t.Error("the new comment isn't the active one")
	}
	click("Because.", &d.cm.post) // a reply
	click("", &d.cm.resolve)
	c := d.ed.Comments()[0].Comment
	if len(c.Replies) != 1 || c.Replies[0].Text() != "Because." || !c.Done {
		t.Fatalf("after reply and resolve: %+v", c)
	}
	d.ed.Undo()
	if c := d.ed.Comments()[0].Comment; c.Done || len(c.Replies) != 1 {
		t.Errorf("undo of resolve: %+v", c)
	}
	d.cm.active = d.ed.Comments()[0].Comment
	click("", &d.cm.del)
	if len(d.ed.Comments()) != 0 {
		t.Fatal("comment not deleted")
	}
	if got := d.ed.Para(0).Text(); got != "Hello brave world" {
		t.Errorf("text after delete = %q", got)
	}
}
