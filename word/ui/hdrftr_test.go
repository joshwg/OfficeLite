// Tests for header and footer structure: the Different First Page switch,
// Link to Previous, Page X of Y and Format Page Numbers.

package ui

import (
	"testing"

	"gioui.org/io/key"

	"officelite/word/model"
)

// footerText is the text of the footer shown on page i ("" for none).
func footerText(h *harness, i int) string {
	f := h.d.eng.Pages[i].Footer
	if f == nil {
		return ""
	}
	s := ""
	for _, pl := range f.Lines {
		for _, it := range pl.Line.Items {
			s += it.Text
		}
	}
	return s
}

// TestHeaderSwitchesUndo turns Different First Page on and removes the
// footer, then undoes both: header structure changes are on the body's
// undo stack.
func TestHeaderSwitchesUndo(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Body")
	d.enterHF(0, true)
	h.typeText("Foot")
	d.exitHF()
	h.frame()
	if got := footerText(h, 0); got != "Foot" {
		t.Fatalf("footer %q", got)
	}
	d.setHFSwitch(true, true)
	h.frame()
	if !d.doc.HF.TitlePage || footerText(h, 0) != "" {
		t.Fatalf("Different First Page: TitlePage %v, footer %q", d.doc.HF.TitlePage, footerText(h, 0))
	}
	if got := d.ed.UndoLabels()[0]; got != "Different First Page" {
		t.Errorf("undo label %q", got)
	}
	d.ed.Undo()
	d.edited()
	h.frame()
	if d.doc.HF.TitlePage || footerText(h, 0) != "Foot" {
		t.Errorf("after undo: TitlePage %v, footer %q", d.doc.HF.TitlePage, footerText(h, 0))
	}

	d.removeHF(true)
	h.frame()
	if d.doc.HF.Footers[model.HFDefault] != nil || footerText(h, 0) != "" {
		t.Fatal("footer not removed")
	}
	d.ed.Undo()
	d.edited()
	h.frame()
	if got := footerText(h, 0); got != "Foot" {
		t.Errorf("footer after undoing its removal %q", got)
	}
	d.ed.Redo()
	d.edited()
	h.frame()
	if got := footerText(h, 0); got != "" {
		t.Errorf("footer after redo %q", got)
	}
}

// TestLinkToPrevious unlinks the second section's footer (a copy it can
// change without changing the first's), links it again, and undoes that.
func TestLinkToPrevious(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("one")
	d.enterHF(0, true)
	h.typeText("Shared")
	d.exitHF()
	d.ed.InsertSectionBreak(model.BreakNextPage)
	h.typeText("two")
	d.edited()
	h.frame()
	if len(d.eng.Pages) != 2 || footerText(h, 1) != "Shared" {
		t.Fatalf("%d pages, page 2 footer %q", len(d.eng.Pages), footerText(h, 1))
	}
	if !d.doc.HeaderLinked(1, model.HFDefault, true) {
		t.Fatal("the new section's footer isn't linked")
	}
	// Editing the first section's footer can't be unlinked.
	d.enterHF(0, true)
	d.setLinkToPrevious(false)
	if d.doc.HeaderLinked(0, model.HFDefault, true) {
		t.Error("the first section was linked")
	}
	// Unlink the second's and change it.
	d.enterHF(1, true)
	d.setLinkToPrevious(false)
	if d.doc.HeaderLinked(1, model.HFDefault, true) || !d.hf.on || d.hf.page != 1 {
		t.Fatalf("unlink: linked %v, editing %v page %d", d.doc.HeaderLinked(1, model.HFDefault, true), d.hf.on, d.hf.page)
	}
	d.ed.SetCaret(model.Pos{Para: 0, Off: len("Shared")}, false)
	h.typeText(" 2")
	d.exitHF()
	h.frame()
	if a, b := footerText(h, 0), footerText(h, 1); a != "Shared" || b != "Shared 2" {
		t.Fatalf("footers after unlinking %q / %q", a, b)
	}
	// Link it again: it shows the first's.
	d.enterHF(1, true)
	d.setLinkToPrevious(true)
	d.exitHF()
	h.frame()
	if got := footerText(h, 1); got != "Shared" || !d.doc.HeaderLinked(1, model.HFDefault, true) {
		t.Fatalf("relinked footer %q", got)
	}
	d.ed.Undo()
	d.edited()
	h.frame()
	if got := footerText(h, 1); got != "Shared 2" {
		t.Errorf("after undoing the link: %q", got)
	}
}

// TestPageXofY inserts "Page X of Y" into the footer.
func TestPageXofY(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Body")
	d.hfInsertPageXofY()
	if !d.hf.on || !d.hf.footer {
		t.Fatal("not editing the footer")
	}
	d.exitHF()
	h.frame()
	if got := footerText(h, 0); got != "Page 1 of 1" {
		t.Errorf("footer %q", got)
	}
}

// TestFormatPageNumbers restarts the numbering at 4 in roman numerals,
// rejects a bad start number, and undoes the change.
func TestFormatPageNumbers(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Body")
	d.hfInsertField(model.FieldPage) // enters the footer
	d.openPageNumbers()
	p := &d.hf.nums
	if !p.open || p.format != 0 || p.restart || p.start.Text() != "1" {
		t.Fatalf("dialog: open %v format %d restart %v start %q", p.open, p.format, p.restart, p.start.Text())
	}
	p.format, p.restart = 2, true // I, II, III
	p.start.SetText("x")
	if d.applyPageNumbers() || p.err == "" {
		t.Fatal("a bad start number was taken")
	}
	p.start.SetText("4")
	if !d.applyPageNumbers() {
		t.Fatalf("apply: %s", p.err)
	}
	d.exitHF()
	h.frame()
	if s := d.doc.Section; s.PageNumFormat != "upperRoman" || !s.PageNumRestart || s.PageNumStart != 4 {
		t.Fatalf("section numbering %q %v %d", s.PageNumFormat, s.PageNumRestart, s.PageNumStart)
	}
	if got := footerText(h, 0); got != "IV" {
		t.Errorf("footer %q, want IV", got)
	}
	d.ed.Undo()
	d.edited()
	h.frame()
	if got := footerText(h, 0); got != "1" {
		t.Errorf("after undo the footer shows %q", got)
	}
}

// TestEscapeClosesPageNumbers closes the Format Page Numbers dialog with
// Esc, leaving the footer it was opened over still being edited.
func TestEscapeClosesPageNumbers(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.hfInsertField(model.FieldPage) // enters the footer
	d.openPageNumbers()
	h.frame()
	h.press(key.NameEscape, 0)
	if d.hf.nums.open {
		t.Fatal("Esc left the dialog open")
	}
	if !d.hf.on {
		t.Error("Esc left the footer under the dialog")
	}
}
