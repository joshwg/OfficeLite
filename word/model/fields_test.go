package model

import (
	"reflect"
	"testing"
	"time"
)

// TestParseField checks field instruction parsing: type, quoted
// arguments, switches with and without arguments, run-on switches.
func TestParseField(t *testing.T) {
	for _, c := range []struct {
		instr string
		want  FieldCode
	}{
		{" PAGE  \\* MERGEFORMAT ", FieldCode{Name: "PAGE", Switches: []FieldSwitch{{`\*`, "MERGEFORMAT"}}}},
		{`date \@ "dddd, MMMM d, yyyy"`, FieldCode{Name: "DATE", Switches: []FieldSwitch{{`\@`, "dddd, MMMM d, yyyy"}}}},
		{`DATE \@"d MMM"`, FieldCode{Name: "DATE", Switches: []FieldSwitch{{`\@`, "d MMM"}}}},
		{`PAGE \*Roman`, FieldCode{Name: "PAGE", Switches: []FieldSwitch{{`\*`, "Roman"}}}},
		{`FILENAME \p \* Upper`, FieldCode{Name: "FILENAME", Switches: []FieldSwitch{{`\p`, ""}, {`\*`, "Upper"}}}},
		{`TOC \o "1-3" \h \z`, FieldCode{Name: "TOC", Switches: []FieldSwitch{{`\o`, "1-3"}, {`\h`, ""}, {`\z`, ""}}}},
		{`DOCPROPERTY "Company Name" \* MERGEFORMAT`, FieldCode{Name: "DOCPROPERTY", Args: []string{"Company Name"},
			Switches: []FieldSwitch{{`\*`, "MERGEFORMAT"}}}},
		{`REF "a \"quoted\" \\ name"`, FieldCode{Name: "REF", Args: []string{`a "quoted" \ name`}}},
		{`DATE \@ "unterminated`, FieldCode{Name: "DATE", Switches: []FieldSwitch{{`\@`, "unterminated"}}}},
		{"   ", FieldCode{}},
	} {
		if got := ParseField(c.instr); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseField(%q) = %+v, want %+v", c.instr, got, c.want)
		}
	}
	fc := ParseField(`FILENAME \P`)
	if _, ok := fc.Switch(`\p`); !ok {
		t.Error(`\P is not found as \p`)
	}
}

// TestEvalField checks the results of the computed fields and their
// format switches.
func TestEvalField(t *testing.T) {
	now := time.Date(2026, time.March, 4, 15, 7, 9, 0, time.UTC)
	doc := New("Report.docx")
	doc.Body = []Block{&Paragraph{Runs: []Run{{Text: "two words"}}}}
	c := &FieldContext{Page: 7, Pages: 12, Section: 2, SectionPages: 3, Now: now,
		FilePath: "/home/josh/Report.docx", Doc: doc}
	for _, tc := range []struct{ instr, want string }{
		{"PAGE", "7"},
		{` PAGE  \* MERGEFORMAT `, "7"},
		{`PAGE \* Roman`, "VII"},
		{`PAGE \* roman`, "vii"},
		{`PAGE \* alphabetic`, "g"},
		{`PAGE \* ALPHABETIC`, "G"},
		{`PAGE \* Ordinal`, "7th"},
		{`PAGE \* CardText`, "seven"},
		{`PAGE \* CardText \* Upper`, "SEVEN"},
		{`PAGE \* OrdText \* FirstCap`, "Seventh"},
		{`PAGE \# "00"`, "07"},
		{`NUMPAGES \* Hex`, "C"},
		{"NUMPAGES", "12"},
		{"SECTIONPAGES", "3"},
		{"SECTION", "2"},
		{"NUMWORDS", "2"},
		{"NUMCHARS", "9"},
		{"DATE", "3/4/2026"},
		{"TIME", "3:07 PM"},
		{`DATE \@ "dddd, MMMM d, yyyy"`, "Wednesday, March 4, 2026"},
		{`DATE \@ "dd/MM/yy"`, "04/03/26"},
		{`DATE \@ "d MMM yyyy" \* Upper`, "4 MAR 2026"},
		{`TIME \@ "HH:mm:ss"`, "15:07:09"},
		{`TIME \@ "h:mm am/pm"`, "3:07 pm"},
		{`DATE \@ "'Week of' MMM d"`, "Week of Mar 4"},
		{"FILENAME", "Report.docx"},
		{`FILENAME \p`, "/home/josh/Report.docx"},
		{`FILENAME \* Upper`, "REPORT.DOCX"},
	} {
		if got, ok := EvalField(tc.instr, c); !ok || got != tc.want {
			t.Errorf("%q = %q, %v; want %q", tc.instr, got, ok, tc.want)
		}
	}
	// A section's page number format, unless a \* switch overrides it.
	c.PageFormat = "lowerRoman"
	if got, _ := EvalField("PAGE", c); got != "vii" {
		t.Errorf("PAGE in a lowerRoman section = %q", got)
	}
	if got, _ := EvalField(`PAGE \* Arabic`, c); got != "7" {
		t.Errorf(`PAGE \* Arabic in a lowerRoman section = %q`, got)
	}
	if got, _ := EvalField(`PAGE \* MERGEFORMAT`, c); got != "vii" {
		t.Errorf(`PAGE \* MERGEFORMAT in a lowerRoman section = %q`, got)
	}
	// An unsaved document's file name is its name.
	c.FilePath = ""
	if got, _ := EvalField("FILENAME", c); got != "Report.docx" {
		t.Errorf("FILENAME unsaved = %q", got)
	}
	// Fields WordLite doesn't compute keep their result.
	for _, instr := range []string{"REF bm1", "AUTHOR", "TOC \\o", "", "SEQ Figure"} {
		if got, ok := EvalField(instr, c); ok {
			t.Errorf("%q computed as %q", instr, got)
		}
	}
	if _, ok := EvalField("NUMWORDS", &FieldContext{}); ok {
		t.Error("NUMWORDS without a document computed")
	}
}

// TestNumberWords checks CardText and OrdText.
func TestNumberWords(t *testing.T) {
	for n, want := range map[int]string{0: "zero", 13: "thirteen", 20: "twenty", 21: "twenty-one",
		100: "one hundred", 123: "one hundred twenty-three", 1000: "one thousand",
		45_678: "forty-five thousand six hundred seventy-eight", 1_000_000: "1000000", -3: "-3"} {
		if got := CardText(n); got != want {
			t.Errorf("CardText(%d) = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{1: "first", 2: "second", 3: "third", 5: "fifth", 8: "eighth",
		9: "ninth", 12: "twelfth", 20: "twentieth", 21: "twenty-first", 40: "fortieth", 100: "one hundredth",
		111: "one hundred eleventh"} {
		if got := OrdText(n); got != want {
			t.Errorf("OrdText(%d) = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{1: "st", 2: "nd", 3: "rd", 4: "th", 11: "th", 12: "th", 13: "th", 21: "st", 112: "th"} {
		if got := ordinalSuffix(n); got != want {
			t.Errorf("ordinalSuffix(%d) = %q", n, got)
		}
	}
}

// TestFormatNumericPicture checks \# pictures.
func TestFormatNumericPicture(t *testing.T) {
	for _, c := range []struct {
		v    float64
		pic  string
		want string
	}{
		{7, "0", "7"}, {7, "00", "07"}, {7, "#", "7"}, {0, "#", ""}, {0, "0", "0"},
		{1234567, "#,##0", "1,234,567"}, {1234.5, "#,##0.00", "1,234.50"},
		{3.14159, "0.##", "3.14"}, {3, "0.##", "3"}, {12, "$#,##0.00", "$12.00"},
		{-5, "0", "-5"}, {-5, "0;(0)", "(5)"}, {-1234, "$#,##0", "-$1,234"}, {-0.001, "0.00", "0.00"}, {42, "0 'items'", "42 items"}, {9, "x", "x"},
	} {
		if got := FormatNumericPicture(c.v, c.pic); got != c.want {
			t.Errorf("FormatNumericPicture(%v, %q) = %q, want %q", c.v, c.pic, got, c.want)
		}
	}
}

// TestFormatDateTime checks \@ pictures, including midnight and noon.
func TestFormatDateTime(t *testing.T) {
	tm := time.Date(2026, time.November, 4, 0, 5, 3, 0, time.UTC)
	for pic, want := range map[string]string{
		"M/d/yyyy": "11/4/2026", "MM/dd/yy": "11/04/26", "MMM d": "Nov 4", "ddd": "Wed",
		"h:mm AM/PM": "12:05 AM", "HH:mm": "00:05", "H:m:s": "0:5:3", "yyyy-MM-dd": "2026-11-04",
		"'at' h": "at 12",
	} {
		if got := FormatDateTime(tm, pic); got != want {
			t.Errorf("FormatDateTime(%q) = %q, want %q", pic, got, want)
		}
	}
	noon := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	if got := FormatDateTime(noon, "h am/pm"); got != "12 pm" {
		t.Errorf("noon = %q", got)
	}
}

// TestUpdateFieldsCaret updates the field next to the caret (before or
// after it) and nothing when there's none, and reports what it skipped.
func TestUpdateFieldsCaret(t *testing.T) {
	doc := New("Doc.docx")
	p := &Paragraph{Runs: []Run{{Text: "ab"},
		{Kind: RunOpaque, Text: "old", Opaque: &Opaque{Field: "SECTION"}},
		{Text: "cd"},
		{Kind: RunOpaque, Text: "ref", Opaque: &Opaque{Field: "REF x"}}}}
	doc.Body = []Block{p}
	e := NewEditor(doc)
	ctx := func(Pos) FieldContext { return FieldContext{Section: 4} }
	at := func(off int) {
		e.Sel = Selection{Pos{0, off}, Pos{0, off}}
	}
	for _, off := range []int{2, 3} { // before and after the field
		p.Runs[1].Text = "old"
		at(off)
		if computed, changed, skipped := e.UpdateFields(ctx); computed != 1 || changed != 1 || skipped != nil {
			t.Errorf("caret at %d: %d computed, %d changed, skipped %v", off, computed, changed, skipped)
		}
		if p := e.Para(0); p.Runs[1].Text != "4" {
			t.Errorf("caret at %d: field shows %q", off, p.Runs[1].Text)
		}
	}
	// Up to date: nothing changes, nothing goes on the undo stack.
	undo := len(e.UndoLabels())
	if _, changed, _ := e.UpdateFields(ctx); changed != 0 || len(e.UndoLabels()) != undo {
		t.Errorf("an up-to-date field changed (%d) or was undoable", changed)
	}
	at(0)
	if computed, changed, skipped := e.UpdateFields(ctx); computed+changed+len(skipped) != 0 {
		t.Errorf("caret away from fields: %d, %d, %v", computed, changed, skipped)
	}
	at(6)
	if _, _, skipped := e.UpdateFields(ctx); len(skipped) != 1 || skipped[0] != "REF" {
		t.Errorf("skipped %v, want [REF]", skipped)
	}
	if got := e.UndoLabels(); len(got) == 0 || got[0] != "Update Field" {
		t.Errorf("undo labels %v", got)
	}
}
