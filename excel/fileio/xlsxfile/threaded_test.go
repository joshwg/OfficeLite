// threaded_test.go checks that Excel 365 threaded comments survive a
// save and reopen, next to plain notes, and that the parts Excel needs
// are written.

package xlsxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"officelite/excel/model"
)

// TestThreadedCommentsRoundTrip writes a thread with replies (resolved),
// one without, and a plain note, and reads them back.
func TestThreadedCommentsRoundTrip(t *testing.T) {
	t0 := time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)
	sh := model.NewSheet("Sheet1")
	sh2 := model.NewSheet("Two")
	thread := model.Note{Threaded: true, Author: "Josh", Text: "Is this right?\nline 2", Time: t0, Resolved: true}
	thread = thread.WithReply(model.Post{Author: "Ann <QA>", Text: "Yes & no", Time: t0.Add(time.Hour)})
	thread = thread.WithReply(model.Post{Author: "Josh", Text: "Fixed", Time: t0.Add(2 * time.Hour)})
	sh.SetNote(1, 1, &thread)
	sh.SetNote(0, 0, &model.Note{Author: "Old", Text: "plain note"})
	sh2.SetNote(4, 2, &model.Note{Threaded: true, Author: "Bob", Text: "single", Time: t0})
	wb := &model.Workbook{Sheets: []*model.Sheet{sh, sh2}, Styles: []model.Style{model.DefaultStyle()}}
	got, data := roundTrip(t, wb)

	n, ok := got.Sheets[0].NoteAt(1, 1)
	if !ok || !n.Threaded || !n.Resolved || n.Author != "Josh" || n.Text != thread.Text || !n.Time.Equal(t0) {
		t.Fatalf("thread = %+v", n)
	}
	if len(n.Replies) != 2 || n.Replies[0].Author != "Ann <QA>" || n.Replies[0].Text != "Yes & no" ||
		!n.Replies[0].Time.Equal(t0.Add(time.Hour)) || n.Replies[1].Text != "Fixed" || n.Replies[1].Author != "Josh" {
		t.Errorf("replies = %+v", n.Replies)
	}
	if p, _ := got.Sheets[0].NoteAt(0, 0); p.Threaded || p.Text != "plain note" || p.Author != "Old" {
		t.Errorf("plain note = %+v", p)
	}
	if s, _ := got.Sheets[1].NoteAt(4, 2); !s.Threaded || s.Resolved || s.Text != "single" || s.Author != "Bob" || len(s.Replies) != 0 {
		t.Errorf("second sheet thread = %+v", s)
	}

	// The legacy comment older versions show carries the whole thread.
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}
	if !strings.Contains(parts["xl/persons/person.xml"], `displayName="Ann &lt;QA&gt;"`) {
		t.Errorf("persons = %s", parts["xl/persons/person.xml"])
	}
	for _, want := range []string{"threadedComment", "person+xml"} {
		if !strings.Contains(parts["[Content_Types].xml"], want) {
			t.Errorf("content types lack %s", want)
		}
	}
	legacy := ""
	for name, s := range parts {
		if strings.HasPrefix(name, "xl/comments") {
			legacy += s
		}
	}
	for _, want := range []string{"tc={", "[Threaded comment]", "Reply:", "Fixed"} {
		if !strings.Contains(legacy, want) {
			t.Errorf("legacy comments lack %q", want)
		}
	}
}

// TestThreadPlainText checks the flattened text formats that only hold
// notes get.
func TestThreadPlainText(t *testing.T) {
	n := model.Note{Threaded: true, Author: "A", Text: "one"}.WithReply(model.Post{Author: "B", Text: "two"})
	if got := n.PlainText(); got != "A: one\nB: two" {
		t.Errorf("PlainText = %q", got)
	}
}
