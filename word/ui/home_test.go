package ui

import (
	"testing"

	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/word/model"
)

func TestFindReplace(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("cat dog Cat cat")
	h.press("F", key.ModCtrl)
	h.frame()
	if !d.find.open || d.find.mode != findOnly {
		t.Fatal("Ctrl+F didn't open the find bar")
	}
	h.typeText("cat")
	if d.find.what.Text() != "cat" {
		t.Fatalf("find box: %q", d.find.what.Text())
	}
	if n := len(d.findMatches()); n != 3 {
		t.Fatalf("%d matches", n)
	}
	h.press(key.NameReturn, 0)
	if a, b := d.ed.Sel.Ordered(); a.Off != 0 || b.Off != 3 {
		t.Fatalf("Enter selected %v..%v", a, b)
	}
	if i := d.currentMatch(); i != 0 {
		t.Errorf("current %d", i)
	}
	h.press(key.NameReturn, 0)
	if a, _ := d.ed.Sel.Ordered(); a.Off != 8 {
		t.Errorf("second match at %v", a)
	}
	h.press(key.NameReturn, key.ModShift)
	if a, _ := d.ed.Sel.Ordered(); a.Off != 0 {
		t.Errorf("Shift+Enter went to %v", a)
	}
	h.press(key.NameF3, 0)
	if a, _ := d.ed.Sel.Ordered(); a.Off != 8 {
		t.Errorf("F3 went to %v", a)
	}
	// Match case narrows it.
	d.find.matchCase = true
	if n := len(d.findMatches()); n != 2 {
		t.Errorf("match case: %d", n)
	}
	d.find.matchCase = false
	// Replace: the selected match, then the rest at once.
	h.press("H", key.ModCtrl)
	if d.find.mode != findReplace {
		t.Fatal("Ctrl+H")
	}
	d.find.with.SetText("bird")
	d.ed.Select(model.Pos{Off: 0}, model.Pos{Off: 3})
	d.replaceOne()
	h.frame()
	if got := h.para(0).Text(); got != "bird dog Cat cat" {
		t.Fatalf("replace one: %q", got)
	}
	d.replaceAll()
	h.frame()
	if got := h.para(0).Text(); got != "bird dog bird bird" {
		t.Fatalf("replace all: %q", got)
	}
	if d.find.msg != "Replaced 2 occurrences" {
		t.Errorf("msg %q", d.find.msg)
	}
	d.ed.Undo()
	h.frame()
	if got := h.para(0).Text(); got != "bird dog Cat cat" {
		t.Errorf("undo replace all: %q", got)
	}
	h.press(key.NameEscape, 0)
	if d.find.open {
		t.Error("Esc didn't close the bar")
	}
}

func TestHeaderFooterEditing(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Body text")
	if d.eng.Pages[0].Header != nil {
		t.Fatal("blank document has a header")
	}
	// Double-click the top margin.
	pt := h.viewPoint(0, 200, 30)
	for range 2 {
		h.mouse(pointer.Press, pt, 0)
		h.mouse(pointer.Release, pt, 0)
	}
	if !d.hf.on || d.hf.footer {
		t.Fatalf("double-click in the top margin: on=%v footer=%v", d.hf.on, d.hf.footer)
	}
	h.typeText("Report title")
	h.frame()
	if b := d.eng.Pages[0].Header; b == nil || len(b.Lines) == 0 {
		t.Fatal("header not laid out on the page")
	}
	if got := d.doc.HF.Headers[model.HFDefault].Blocks[0].(*model.Paragraph).Text(); got != "Report title" {
		t.Errorf("header text %q", got)
	}
	if h.d.bodyEd().Para(0).Text() != "Body text" {
		t.Errorf("body changed")
	}
	if !d.modified() {
		t.Error("header edit not marked modified")
	}
	// A page number in the footer.
	d.enterHF(0, true)
	d.ed.InsertText("Page ")
	d.hfInsertField(model.FieldPage)
	h.frame()
	f := d.eng.Pages[0].Footer
	if f == nil {
		t.Fatal("no footer")
	}
	got := ""
	for _, pl := range f.Lines {
		for _, it := range pl.Line.Items {
			got += it.Text
		}
	}
	if got != "Page 1" {
		t.Errorf("footer shows %q", got)
	}
	// The caret is drawn in the footer.
	c, ok := d.ceng().CaretAt(d.ed.Sel.Caret)
	if !ok || c.Y < d.eng.Pages[0].BodyBottom {
		t.Errorf("caret %+v in the footer view", c)
	}
	h.press(key.NameEscape, 0)
	if d.hf.on || d.ed != d.bodyEd() {
		t.Fatal("Esc didn't leave the footer")
	}
	// Typing goes to the body again.
	h.typeText("!")
	if got := h.para(0).Text(); got != "Body text!" {
		t.Errorf("body after leaving: %q", got)
	}
}

func TestFormatPainterAndPasteFormat(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("one two three")
	d.ed.Select(model.Pos{Off: 0}, model.Pos{Off: 3})
	h.press("B", key.ModCtrl)
	d.ed.Select(model.Pos{Off: 1}, model.Pos{Off: 1})
	d.painterClick(1)
	if !d.painter.armed {
		t.Fatal("painter not armed")
	}
	d.ed.Select(model.Pos{Off: 4}, model.Pos{Off: 7})
	d.painterRelease()
	if d.painter.armed {
		t.Error("single-click painter stayed on")
	}
	if !d.toolState().bold {
		t.Error("painted text not bold")
	}
	// Double-click: sticky until Esc.
	d.painterClick(2)
	d.ed.Select(model.Pos{Off: 8}, model.Pos{Off: 13})
	d.painterRelease()
	if !d.painter.armed {
		t.Error("sticky painter turned off")
	}
	h.press(key.NameEscape, 0)
	if d.painter.armed {
		t.Error("Esc didn't stop the painter")
	}
	// Ctrl+Shift+C / Ctrl+Shift+V.
	h.typeText(" four")
	d.ed.Select(model.Pos{Off: 0}, model.Pos{Off: 2})
	h.press("C", key.ModCtrl|key.ModShift)
	d.ed.Select(model.Pos{Off: 14}, model.Pos{Off: 18})
	h.press("V", key.ModCtrl|key.ModShift)
	p := h.para(0)
	last := p.Runs[len(p.Runs)-1]
	if last.Props.Bold != model.On {
		t.Errorf("Ctrl+Shift+V: %+v", p.Runs)
	}
}

func TestStylesPaneAndDialogs(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Hello")
	h.press("S", key.ModCtrl|key.ModAlt|key.ModShift)
	if !d.pane.open {
		t.Fatal("Ctrl+Alt+Shift+S didn't open the Styles pane")
	}
	h.frame()
	d.newStyle.openFor(d)
	h.frame()
	d.newStyle.name.SetText("Fancy")
	h.press(key.NameReturn, 0)
	if d.newStyle.open || d.doc.Styles.FindStyleByName("Fancy") == nil || h.para(0).Style != "Fancy" {
		t.Fatalf("new style: open=%v style=%q", d.newStyle.open, h.para(0).Style)
	}
	h.press("V", key.ModCtrl|key.ModAlt)
	if !d.pasteSpec.open || !d.modalOpen() {
		t.Error("Ctrl+Alt+V didn't open Paste Special")
	}
	h.frame()
	h.press(key.NameEscape, 0)
	if d.pasteSpec.open {
		t.Error("Esc didn't close Paste Special")
	}
	h.press("G", key.ModCtrl)
	if d.find.mode != findGoTo {
		t.Error("Ctrl+G")
	}
}
