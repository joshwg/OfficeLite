// field_test.go tests listing the fields whose results span paragraphs.

package model

import "testing"

// TestFieldRangesUnclosedInner checks that a nested field that lost its
// end doesn't take the enclosing field's end.
func TestFieldRangesUnclosedInner(t *testing.T) {
	fc := func(k FieldChar, instr string) Run {
		return Run{Kind: RunOpaque, Opaque: &Opaque{FieldChar: k, Field: instr}}
	}
	d := New("t")
	d.Body = []Block{
		&Paragraph{Runs: []Run{fc(FieldBegin, ""), fc(FieldInstr, `TOC \o "1-3"`), fc(FieldSep, "")}},
		&Paragraph{Runs: []Run{{Text: "entry"}, fc(FieldBegin, ""), fc(FieldInstr, "PAGEREF _Toc1")}}, // no end
		&Paragraph{Runs: []Run{{Text: "entry 2"}, fc(FieldEnd, "")}},
	}
	got := FieldRangesIn(NewEditor(d).Locs())
	if len(got) != 1 || got[0].Instr != `TOC \o "1-3"` || got[0].To.Para != 2 {
		t.Errorf("FieldRangesIn = %+v, want the TOC ending in paragraph 2", got)
	}
}
