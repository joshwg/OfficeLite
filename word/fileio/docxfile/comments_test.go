// comments_test.go checks reading Word's comments, and that comments,
// replies and the resolved state survive a save and reopen, both in a new
// package and in a copy of the source.

package docxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"officelite/word/model"
)

// partsOf unzips a package into its parts' text.
func partsOf(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

// commentText is the text a comment covers.
func commentText(ed *model.Editor, a model.CommentAnchor) string {
	var b strings.Builder
	for i := a.Start.Para; i <= a.End.Para; i++ {
		rs := []rune(ed.Para(i).Text())
		from, to := 0, len(rs)
		if i == a.Start.Para {
			from = a.Start.Off
		}
		if i == a.End.Para {
			to = a.End.Off
		}
		for _, r := range rs[from:to] {
			if r != '￼' {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// TestReadWordComment reads the comment Word saved in word.docx.
func TestReadWordComment(t *testing.T) {
	doc, _ := readFile(t, "word.docx")
	ed := model.NewEditor(doc)
	cs := ed.Comments()
	if len(cs) != 1 {
		t.Fatalf("comments = %d", len(cs))
	}
	c := cs[0].Comment
	if c.Author != "Joshua Goldstein" || c.Initials != "JG" || c.Text() != "A comment" || c.Date.IsZero() || c.Done {
		t.Errorf("comment = %+v text %q", c, c.Text())
	}
	if commentText(ed, cs[0]) == "" {
		t.Errorf("comment covers no text: %+v", cs[0])
	}
	for _, w := range doc.Warnings {
		if strings.Contains(w, "omment") {
			t.Errorf("warning %q", w)
		}
	}
}

// TestCommentsRoundTrip adds a comment with a reply, resolves it, and
// checks a new package and a rewrite of it keep everything.
func TestCommentsRoundTrip(t *testing.T) {
	doc := model.New("c")
	ed := model.NewEditor(doc)
	ed.InsertText("Hello brave world")
	ed.Sel = model.Selection{Anchor: model.Pos{Para: 0, Off: 6}, Caret: model.Pos{Para: 0, Off: 11}}
	c := model.NewComment("Ann Lee", "Is this brave?\nSecond line")
	ed.AddComment(c)
	if got := ed.SelectedText(); got != "brave" {
		t.Errorf("selection after adding = %q", got)
	}
	next := c.Clone()
	next.Replies = append(next.Replies, model.NewComment("Bob", "Yes & <no>"))
	next.Done = true
	ed.UpdateComment(c, next, "Reply")

	data := write(t, doc)
	parts := partsOf(t, data)
	for _, want := range []string{"word/comments.xml", "word/commentsExtended.xml"} {
		if _, ok := parts[want]; !ok {
			t.Fatalf("no %s", want)
		}
	}
	if !strings.Contains(parts["word/commentsExtended.xml"], "paraIdParent") || !strings.Contains(parts["[Content_Types].xml"], "commentsExtended+xml") {
		t.Errorf("commentsExtended: %s", parts["word/commentsExtended.xml"])
	}
	if n := strings.Count(parts["word/document.xml"], "<w:commentRangeStart"); n != 2 {
		t.Errorf("range starts = %d, want 2 (the comment's and the reply's)", n)
	}

	check := func(doc *model.Document) {
		t.Helper()
		ed := model.NewEditor(doc)
		cs := ed.Comments()
		if len(cs) != 1 {
			t.Fatalf("comments = %d", len(cs))
		}
		c := cs[0].Comment
		if c.Author != "Ann Lee" || c.Initials != "AL" || c.Text() != "Is this brave?\nSecond line" || !c.Done {
			t.Errorf("comment = %+v text %q", c, c.Text())
		}
		if got := commentText(ed, cs[0]); got != "brave" {
			t.Errorf("commented text = %q", got)
		}
		if len(c.Replies) != 1 || c.Replies[0].Author != "Bob" || c.Replies[0].Text() != "Yes & <no>" {
			t.Errorf("replies = %+v", c.Replies)
		}
		if got := doc.Body[0].(*model.Paragraph).Text(); strings.ReplaceAll(got, "￼", "") != "Hello brave world" {
			t.Errorf("text = %q", got)
		}
	}
	back, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	check(back)
	again, err := Read(write(t, back)) // written into its source package
	if err != nil {
		t.Fatal(err)
	}
	check(again)
}

// TestRewriteWordComments replies to Word's comment and deletes it again,
// checking the comment id lists Word adds are dropped and the text stays.
func TestRewriteWordComments(t *testing.T) {
	doc, _ := readFile(t, "word.docx")
	ed := model.NewEditor(doc)
	c := ed.Comments()[0].Comment
	next := c.Clone()
	next.Replies = append(next.Replies, model.NewComment("Me", "reply"))
	ed.UpdateComment(c, next, "Reply")
	data := write(t, doc)
	parts := partsOf(t, data)
	for name := range parts {
		if strings.Contains(name, "commentsIds") || strings.Contains(name, "commentsExtensible") {
			t.Errorf("%s kept", name)
		}
	}
	if strings.Contains(parts["[Content_Types].xml"], "commentsIds") || strings.Contains(parts["word/_rels/document.xml.rels"], "commentsIds") {
		t.Error("commentsIds still listed")
	}
	back, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	cs := model.NewEditor(back).Comments()
	if len(cs) != 1 || cs[0].Comment.Text() != "A comment" || len(cs[0].Comment.Replies) != 1 || cs[0].Comment.Replies[0].Text() != "reply" {
		t.Fatalf("after reply: %+v", cs)
	}

	ed = model.NewEditor(back)
	before := paras(back)[0].Text()
	ed.DeleteComment(ed.Comments()[0].Comment)
	gone, err := Read(write(t, back))
	if err != nil {
		t.Fatal(err)
	}
	if cs := model.NewEditor(gone).Comments(); len(cs) != 0 {
		t.Errorf("deleted comment came back: %+v", cs)
	}
	ed.Undo()
	if len(ed.Comments()) != 1 || paras(back)[0].Text() != before {
		t.Error("undo didn't restore the comment")
	}
}
