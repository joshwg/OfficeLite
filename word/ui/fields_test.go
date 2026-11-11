// Tests for fields in the window: date formats, inserting and updating
// fields, field codes and the Field dialog.

package ui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/word/model"
)

// TestDateChoices checks that the Date and Time pictures format as Word's
// dialog shows them.
func TestDateChoices(t *testing.T) {
	tm := time.Date(2026, time.November, 4, 15, 7, 9, 0, time.UTC)
	want := []string{"11/4/2026", "Wednesday, November 4, 2026", "November 4, 2026", "11/4/26",
		"2026-11-04", "4-Nov-26", "11.4.2026", "Nov. 4, 26", "4 November 2026", "November 26", "Nov-26",
		"11/4/2026 3:07 PM", "11/4/2026 3:07:09 PM", "3:07 PM", "3:07:09 PM", "15:07", "15:07:09"}
	got := DateChoices(tm)
	if len(got) != len(want) {
		t.Fatalf("%d choices, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("choice %d (%q) = %q, want %q", i, dateFormats[i], got[i], want[i])
		}
	}
	if f := dateField("h:mm AM/PM"); f != `TIME \@ "h:mm AM/PM"` {
		t.Errorf("time field %q", f)
	}
	if f := dateField("M/d/yyyy h:mm AM/PM"); f != `DATE \@ "M/d/yyyy h:mm AM/PM"` {
		t.Errorf("date field %q", f)
	}
}

// fieldAt is the run at index i of paragraph 0, which must be a field.
func fieldAt(t *testing.T, h *harness, i int) *model.Run {
	t.Helper()
	p := h.para(0)
	if i >= len(p.Runs) || p.Runs[i].Opaque == nil || p.Runs[i].Opaque.Field == "" {
		t.Fatalf("run %d of %+v is not a field", i, p.Runs)
	}
	return &p.Runs[i]
}

// TestInsertDateAutomatically inserts the date with Update automatically:
// a DATE field showing today's date that F9 keeps current.
func TestInsertDateAutomatically(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.insertChosen(insDate)
	h.frame()
	d.ins.dateSel = 4 // yyyy-MM-dd
	d.ins.dateAuto.Click()
	h.frame()
	d.ins.ok.Click()
	h.frame()
	h.frame()
	r := fieldAt(t, h, 0)
	if r.Opaque.Field != `DATE \@ "yyyy-MM-dd"` || r.Text != time.Now().Format("2006-01-02") {
		t.Fatalf("field %q showing %q", r.Opaque.Field, r.Text)
	}
	// An old result is brought up to date by F9 on the field.
	r.Text = "1999-01-01"
	d.ed.Sel = model.Selection{Anchor: model.Pos{Para: 0, Off: 1}, Caret: model.Pos{Para: 0, Off: 1}}
	h.press(key.NameF9, 0)
	if r := fieldAt(t, h, 0); r.Text != time.Now().Format("2006-01-02") {
		t.Errorf("after F9 the field shows %q", r.Text)
	}
	if !strings.Contains(d.notice, "1 field updated") {
		t.Errorf("notice %q", d.notice)
	}
	// Undo brings the old result back.
	d.ed.Undo()
	if r := fieldAt(t, h, 0); r.Text != "1999-01-01" {
		t.Errorf("after undo the field shows %q", r.Text)
	}
}

// TestUpdateFieldsSelection updates every field in a selection (Ctrl+A,
// F9), drops the stale file bytes of a changed field, and names the
// fields it can't compute.
func TestUpdateFieldsSelection(t *testing.T) {
	h := newHarness(t)
	d := h.d
	p := h.para(0)
	p.Runs = []model.Run{
		{Text: "Page "},
		{Kind: model.RunOpaque, Text: "9", Opaque: &model.Opaque{Field: "PAGE", Format: "docx", Data: []byte("<w:r/>")}},
		{Text: " of "},
		{Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: `NUMPAGES \* MERGEFORMAT`}},
		{Text: " in "},
		{Kind: model.RunOpaque, Text: "x", Opaque: &model.Opaque{Field: "FILENAME"}},
		{Text: ", see "},
		{Kind: model.RunOpaque, Text: "Figure 1", Opaque: &model.Opaque{Field: "REF fig1 \\h"}},
	}
	d.eng.Invalidate(nil, true)
	d.laidOut = -1
	d.edited()
	h.press("A", key.ModShortcut)
	h.press(key.NameF9, 0)
	var text strings.Builder
	for _, r := range h.para(0).Runs {
		text.WriteString(r.Text)
	}
	if got := text.String(); got != "Page 1 of 1 in Test, see Figure 1" {
		t.Errorf("text after F9 = %q", got)
	}
	if o := fieldAt(t, h, 1).Opaque; o.Format != "" || o.Data != nil || o.Field != "PAGE" {
		t.Errorf("changed PAGE field keeps its file bytes: %+v", o)
	}
	if !strings.Contains(d.notice, "3 fields updated") || !strings.Contains(d.notice, "REF") {
		t.Errorf("notice %q", d.notice)
	}
}

// TestFieldCodesToggle shows field codes with Alt+F9 and results again.
func TestFieldCodesToggle(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.ed.InsertSimpleField(`PAGE \* roman`, "i")
	d.edited()
	shown := func() string {
		var b strings.Builder
		for _, pl := range d.eng.Pages[0].Lines {
			for _, it := range pl.Line.Items {
				b.WriteString(it.Text)
			}
		}
		return b.String()
	}
	if got := shown(); got != "i" {
		t.Fatalf("result shown %q", got)
	}
	h.press(key.NameF9, key.ModAlt)
	if !d.showCodes {
		t.Fatal("Alt+F9 didn't turn field codes on")
	}
	d.relayout()
	if got := shown(); got != `{ PAGE \* roman }` {
		t.Errorf("codes shown %q", got)
	}
	d.toggleFieldCodes()
	d.relayout()
	if got := shown(); got != "i" {
		t.Errorf("result shown again %q", got)
	}
}

// TestInsertFieldDialog inserts fields from the Field dialog: a listed
// one, a typed code with switches, and one WordLite doesn't compute; an
// empty code keeps the dialog open.
func TestInsertFieldDialog(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.press(key.NameF9, key.ModShortcut)
	if d.ins.kind != insFieldDlg {
		t.Fatal("Ctrl+F9 didn't open the Field dialog")
	}
	h.frame()
	d.ins.field.choices[1].Click() // NUMPAGES
	h.frame()
	if got := d.ins.field.code.Text(); got != model.FieldNumPages {
		t.Fatalf("code after a click = %q", got)
	}
	d.ins.ok.Click()
	h.frame()
	h.frame()
	if r := fieldAt(t, h, 0); r.Opaque.Field != "NUMPAGES" || r.Text != "1" {
		t.Errorf("inserted %q showing %q", r.Opaque.Field, r.Text)
	}

	d.insertChosen(insField)
	h.frame()
	d.ins.field.code.SetText(`PAGE \* CardText \* Upper`)
	d.ins.ok.Click()
	h.frame()
	h.frame()
	if r := fieldAt(t, h, 1); r.Text != "ONE" {
		t.Errorf("typed code shows %q", r.Text)
	}

	d.insertChosen(insField)
	h.frame()
	d.ins.field.code.SetText("   ")
	d.ins.ok.Click()
	h.frame()
	if d.ins.kind != insFieldDlg {
		t.Error("an empty code closed the dialog")
	}
	d.ins.field.code.SetText("REF figure1")
	d.ins.ok.Click()
	h.frame()
	h.frame()
	if r := fieldAt(t, h, 2); r.Opaque.Field != "REF figure1" || r.Text != "[REF]" {
		t.Errorf("uncomputed field %q shows %q", r.Opaque.Field, r.Text)
	}
}
