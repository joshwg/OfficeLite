// comments_test.go checks reading the comment Word saved in a .doc, and
// that saving as .doc warns it is lost while .docx keeps it.

package docfile

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"officelite/word/fileio/docxfile"
	"officelite/word/model"
)

// TestReadDocComment reads complex.doc's comment: its author, initials,
// text and the ten characters it covers.
func TestReadDocComment(t *testing.T) {
	doc := readFixture(t, "complex.doc")
	ed := model.NewEditor(doc)
	cs := ed.Comments()
	if len(cs) != 1 {
		t.Fatalf("comments = %d", len(cs))
	}
	a := cs[0]
	c := a.Comment
	if c.Author != "Joshua Goldstein" || c.Initials != "JG" || c.Text() != "A comment" {
		t.Errorf("comment = %+v text %q", c, c.Text())
	}
	if a.Start.Para != a.End.Para {
		t.Fatalf("anchor = %+v", a)
	}
	covered := strings.ReplaceAll(string([]rune(ed.Para(a.Start.Para).Text())[a.Start.Off:a.End.Off]), "￼", "")
	if len([]rune(covered)) < 5 {
		t.Errorf("covered text = %q", covered)
	}
	for _, w := range doc.Warnings {
		if strings.Contains(w, "omment") {
			t.Errorf("warning %q", w)
		}
	}

	// Saved as .doc the comment goes, with a warning; as .docx it stays.
	if !slices.Contains(Warnings(doc), WarnCommentsLost) {
		t.Errorf(".doc warnings = %q", Warnings(doc))
	}
	var buf bytes.Buffer
	if err := docxfile.Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	back, err := docxfile.Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if cs := model.NewEditor(back).Comments(); len(cs) != 1 || cs[0].Comment.Text() != "A comment" || cs[0].Comment.Author != "Joshua Goldstein" {
		t.Errorf("after saving as .docx: %+v", cs)
	}
}

// dropComments removes doc's comments, as a .doc save does.
func dropComments(doc *model.Document) {
	ed := model.NewEditor(doc)
	for _, a := range ed.Comments() {
		ed.DeleteComment(a.Comment)
	}
}
