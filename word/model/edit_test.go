package model

import (
	"strings"
	"testing"
)

// texts returns every paragraph's text, table cells included.
func texts(d *Document) string {
	var out []string
	d.Paragraphs(func(p *Paragraph) bool {
		out = append(out, p.Text())
		return true
	})
	return strings.Join(out, "|")
}

func newDoc(paras ...string) (*Document, *Editor) {
	d := New("t")
	d.Body = nil
	for _, s := range paras {
		p := &Paragraph{}
		if s != "" {
			p.Runs = []Run{{Text: s}}
		}
		d.Body = append(d.Body, p)
	}
	return d, NewEditor(d)
}

func TestInsertSplitMerge(t *testing.T) {
	d, e := newDoc("Hello world")
	e.SetCaret(Pos{0, 5}, false)
	for _, c := range ", big" {
		e.InsertText(string(c))
	}
	if got := texts(d); got != "Hello, big world" {
		t.Fatalf("typed: %q", got)
	}
	if len(e.undo) != 1 {
		t.Errorf("typing should be one undo step, got %d", len(e.undo))
	}
	e.Enter()
	if got := texts(d); got != "Hello, big| world" || e.Sel.Caret != (Pos{1, 0}) {
		t.Fatalf("enter: %q caret %v", got, e.Sel.Caret)
	}
	e.Backspace()
	if got := texts(d); got != "Hello, big world" || e.Sel.Caret != (Pos{0, 10}) {
		t.Fatalf("backspace join: %q caret %v", got, e.Sel.Caret)
	}
	e.Undo()
	if got := texts(d); got != "Hello, big| world" {
		t.Fatalf("undo join: %q", got)
	}
	e.Undo()
	e.Undo()
	if got := texts(d); got != "Hello world" {
		t.Fatalf("undo all: %q", got)
	}
	e.Redo()
	if got := texts(d); got != "Hello, big world" {
		t.Fatalf("redo: %q", got)
	}
	// Delete at the end of a paragraph joins the next one.
	e.SetCaret(Pos{0, 16}, false)
	e.InsertText("\nNext")
	if got := texts(d); got != "Hello, big world|Next" {
		t.Fatalf("multi-line insert: %q", got)
	}
	e.SetCaret(Pos{0, 16}, false)
	e.Delete()
	if got := texts(d); got != "Hello, big worldNext" {
		t.Fatalf("delete join: %q", got)
	}
}

func TestDeleteAcrossParagraphs(t *testing.T) {
	d, e := newDoc("one two", "three", "four five")
	e.Select(Pos{0, 4}, Pos{2, 5})
	e.Backspace()
	if got := texts(d); got != "one five" || e.Sel.Caret != (Pos{0, 4}) {
		t.Fatalf("got %q caret %v", got, e.Sel.Caret)
	}
	e.Undo()
	if got := texts(d); got != "one two|three|four five" {
		t.Fatalf("undo: %q", got)
	}
	// Typing replaces the selection.
	e.Select(Pos{0, 0}, Pos{1, 5})
	e.InsertText("X")
	if got := texts(d); got != "X|four five" {
		t.Fatalf("replace: %q", got)
	}
}

func tableDoc() (*Document, *Editor) {
	d := New("t")
	cell := func(s string) *TableCell {
		return &TableCell{Blocks: []Block{&Paragraph{Runs: []Run{{Text: s}}}}}
	}
	tbl := &Table{Columns: []float64{100, 100}, Rows: []*TableRow{
		{Cells: []*TableCell{cell("a1"), cell("b1")}},
		{Cells: []*TableCell{cell("a2"), cell("b2")}},
	}}
	d.Body = []Block{&Paragraph{Runs: []Run{{Text: "before"}}}, tbl, &Paragraph{Runs: []Run{{Text: "after"}}}}
	return d, NewEditor(d)
}

func TestDeleteAcrossCells(t *testing.T) {
	d, e := tableDoc()
	if e.NumParas() != 6 {
		t.Fatalf("paras = %d", e.NumParas())
	}
	// From the middle of "before" into the second cell: text cleared,
	// the table stays.
	e.Select(Pos{0, 3}, Pos{2, 1})
	e.Delete()
	if got := texts(d); got != "bef||1|a2|b2|after" {
		t.Fatalf("got %q", got)
	}
	if len(d.Body) != 3 {
		t.Fatalf("table structure changed: %d blocks", len(d.Body))
	}
	e.Undo()
	// Whole table between body paragraphs: the table goes.
	e.Select(Pos{0, 3}, Pos{5, 2})
	e.Delete()
	if got := texts(d); got != "befter" || len(d.Body) != 1 {
		t.Fatalf("whole table: %q (%d blocks)", got, len(d.Body))
	}
	e.Undo()
	if got := texts(d); got != "before|a1|b1|a2|b2|after" {
		t.Fatalf("undo: %q", got)
	}
	// Enter inside a cell adds a paragraph to the cell.
	e.SetCaret(Pos{1, 1}, false)
	e.Enter()
	if got := texts(d); got != "before|a|1|b1|a2|b2|after" {
		t.Fatalf("enter in cell: %q", got)
	}
	// Backspace at the start of a cell does nothing.
	e.SetCaret(Pos{3, 0}, false)
	e.Backspace()
	if got := texts(d); got != "before|a|1|b1|a2|b2|after" {
		t.Fatalf("backspace at cell start: %q", got)
	}
}

func TestOpaqueAtomic(t *testing.T) {
	d := New("t")
	p := &Paragraph{Runs: []Run{{Text: "ab"}, {Kind: RunOpaque, Text: "[picture]", Opaque: &Opaque{Format: "docx"}}, {Text: "cd"}}}
	d.Body = []Block{p}
	e := NewEditor(d)
	e.SetCaret(Pos{0, 2}, false)
	e.Move(1, false, false)
	if e.Sel.Caret.Off != 3 {
		t.Fatalf("caret %v", e.Sel.Caret)
	}
	e.Backspace()
	if len(p.Runs) != 1 || p.Text() != "abcd" {
		t.Fatalf("runs %+v", p.Runs)
	}
}

func TestEnterNextStyle(t *testing.T) {
	d, e := newDoc("Title text")
	e.SetParaStyle("Heading1")
	if d.Body[0].(*Paragraph).Style != "Heading1" {
		t.Fatal("style not applied")
	}
	e.SetCaret(Pos{0, 10}, false)
	e.Enter()
	e.InsertText("body")
	if p := d.Body[1].(*Paragraph); p.Style != "" || p.Text() != "body" {
		t.Fatalf("next paragraph %+v", p)
	}
	// Enter in the middle keeps the style.
	e.SetCaret(Pos{0, 5}, false)
	e.Enter()
	if p := d.Body[1].(*Paragraph); p.Style != "Heading1" {
		t.Fatalf("split style %q", p.Style)
	}
	e.SetCaret(Pos{1, 0}, false)
	e.SetParaStyle("Normal")
	if p := d.Body[1].(*Paragraph); p.Style != "" {
		t.Fatalf("Normal stored as %q", p.Style)
	}
}

func TestFormatting(t *testing.T) {
	d, e := newDoc("one two three")
	e.Select(Pos{0, 4}, Pos{0, 7})
	e.ToggleBold()
	p := d.Body[0].(*Paragraph)
	if len(p.Runs) != 3 || p.Runs[1].Text != "two" || p.Runs[1].Props.Bold != On {
		t.Fatalf("runs %+v", p.Runs)
	}
	if !e.AllSelected(func(r RunProps) bool { return r.Bold.Bool(false) }) {
		t.Error("selection should read as bold")
	}
	e.ToggleBold()
	if len(p.Runs) != 3 || p.Runs[1].Props.Bold != Off {
		t.Fatalf("toggle off: %+v", p.Runs)
	}
	e.Undo()
	e.Undo()
	if p = d.Body[0].(*Paragraph); len(p.Runs) != 1 {
		t.Fatalf("undo: %+v", d.Body[0].(*Paragraph).Runs)
	}
	// Caret inside a word formats the word.
	e.SetCaret(Pos{0, 9}, false)
	e.ToggleItalic()
	if p.Runs[1].Text != "three" || p.Runs[1].Props.Italic != On {
		t.Fatalf("word format: %+v", p.Runs)
	}
	// At the end, formatting is pending for the next typed text.
	e.SetCaret(Pos{0, 13}, false)
	e.ToggleItalic() // italic text before the caret: turn it off
	e.InsertText("!")
	last := p.Runs[len(p.Runs)-1]
	if last.Text != "!" || last.Props.Italic != Off {
		t.Fatalf("pending: %+v", p.Runs)
	}
	// Character style and clear formatting.
	e.Select(Pos{0, 0}, Pos{0, 3})
	e.SetSize(20)
	e.SetCharStyle("Strong")
	if r := d.ResolveRun(p, &p.Runs[0]); !r.Bold.Bool(false) || r.Size != 20 {
		t.Fatalf("strong: %+v", r)
	}
	e.ClearCharFormatting()
	if r := d.ResolveRun(p, &p.Runs[0]); r.Bold.Bool(false) || r.Size != 11 {
		t.Fatalf("cleared: %+v", r)
	}
}

func TestParagraphFormatting(t *testing.T) {
	d, e := newDoc("a", "b", "c")
	e.Select(Pos{0, 0}, Pos{1, 1})
	e.SetAlign(AlignCenter)
	e.SetLineSpacing(2)
	ps := func(i int) *Paragraph { return d.Body[i].(*Paragraph) }
	if ps(0).Props.Align != AlignCenter || ps(1).Props.Align != AlignCenter || ps(2).Props.Align != AlignUnset {
		t.Fatal("align")
	}
	if d.ResolvePara(ps(1)).Line != 2 {
		t.Fatal("line spacing")
	}
	e.Indent(1)
	e.Indent(1)
	if v := ps(0).Props.IndentLeft; v == nil || *v != 72 {
		t.Fatalf("indent %v", v)
	}
	e.Indent(-1)
	if v := ps(0).Props.IndentLeft; *v != 36 {
		t.Fatalf("outdent %v", *v)
	}
	// Bullets: on, continue, off.
	e.ToggleList(true)
	if e.ListKind(ps(0)) != "bullet" || e.ListKind(ps(1)) != "bullet" || len(d.Lists) != 1 {
		t.Fatalf("bullets: %q %q lists %d", e.ListKind(ps(0)), e.ListKind(ps(1)), len(d.Lists))
	}
	e.SetCaret(Pos{2, 0}, false)
	e.ToggleList(true)
	if ps(2).Props.List.NumID != ps(0).Props.List.NumID {
		t.Error("list not continued")
	}
	e.Indent(1)
	if ps(2).Props.List.Level != 1 {
		t.Error("list level")
	}
	e.ToggleList(false)
	if e.ListKind(ps(2)) != "decimal" && e.ListKind(ps(2)) != "lowerLetter" {
		t.Errorf("numbering kind %q", e.ListKind(ps(2)))
	}
	e.Select(Pos{0, 0}, Pos{1, 1})
	e.ToggleList(true)
	if e.ListKind(ps(0)) != "" {
		t.Error("bullets not removed")
	}
	// Enter in an empty list paragraph ends the list.
	e.SetCaret(Pos{2, 1}, false)
	e.Enter()
	if e.ListKind(ps(3)) == "" {
		t.Fatal("new paragraph should stay in the list")
	}
	e.Enter()
	if e.ListKind(ps(3)) != "" || len(d.Body) != 4 {
		t.Fatalf("empty list paragraph: kind %q, %d paragraphs", e.ListKind(ps(3)), len(d.Body))
	}
}

func TestModifyStyleUndo(t *testing.T) {
	d, e := newDoc("x")
	e.ModifyStyle("Normal", func(st *Style) { st.Run.Size = 14 })
	p := d.Body[0].(*Paragraph)
	if d.ResolveRun(p, &p.Runs[0]).Size != 14 {
		t.Fatal("modify")
	}
	if _, all := e.TakeDirty(); !all {
		t.Error("style change should relayout everything")
	}
	e.Undo()
	p = d.Body[0].(*Paragraph)
	if d.ResolveRun(p, &p.Runs[0]).Size != 11 {
		t.Fatal("undo modify")
	}
}

func TestClipboard(t *testing.T) {
	d, e := newDoc("alpha beta", "gamma")
	e.Select(Pos{0, 6}, Pos{0, 10})
	e.ToggleBold()
	e.Select(Pos{0, 6}, Pos{1, 2})
	f := e.Copy()
	if f.Text() != "beta\r\nga" {
		t.Fatalf("text %q", f.Text())
	}
	e.SetCaret(Pos{1, 5}, false)
	e.Paste(f)
	if got := texts(d); got != "alpha beta|gammabeta|ga" {
		t.Fatalf("paste: %q", got)
	}
	p := d.Body[1].(*Paragraph)
	if p.Runs[len(p.Runs)-1].Props.Bold != On {
		t.Errorf("pasted formatting lost: %+v", p.Runs)
	}
	e.PasteText("x\r\ny")
	if got := texts(d); got != "alpha beta|gammabeta|gax|y" {
		t.Fatalf("paste text: %q", got)
	}
}

func TestWordMovement(t *testing.T) {
	_, e := newDoc("Hello, big world")
	p := Pos{0, 0}
	var stops []int
	for range 5 {
		p = e.WordRight(p)
		stops = append(stops, p.Off)
	}
	if got := stops; got[0] != 5 || got[1] != 7 || got[2] != 11 || got[3] != 16 {
		t.Errorf("word right %v", got)
	}
	if q := e.WordLeft(Pos{0, 16}); q.Off != 11 {
		t.Errorf("word left %v", q)
	}
	e.SelectWord(Pos{0, 8})
	if a, b := e.Sel.Ordered(); a.Off != 7 || b.Off != 11 {
		t.Errorf("word select %v %v", a, b)
	}
}

func TestMarkersAndSpans(t *testing.T) {
	d := New("t")
	link := &Span{Kind: SpanLink, URL: "https://example.com"}
	p := &Paragraph{Runs: []Run{{Text: "ab"}, {Kind: RunOpaque, Opaque: &Opaque{Format: "docx"}}, {Text: "cd"},
		{Text: "link", Span: link}, {Text: "!"}}}
	d.Body = []Block{p}
	e := NewEditor(d)
	// The invisible bookmark takes no extra caret step.
	e.SetCaret(Pos{0, 2}, false)
	e.Move(1, false, false)
	if e.Sel.Caret.Off != 4 {
		t.Fatalf("right over marker: %v", e.Sel.Caret)
	}
	e.Move(-1, false, false)
	if e.Sel.Caret.Off != 3 { // before "c", after the marker
		t.Fatalf("left over marker: %v", e.Sel.Caret)
	}
	// Backspace after the marker deletes the visible character before it.
	e.SetCaret(Pos{0, 3}, false)
	e.Backspace()
	if p.Text() != "a￼cdlink!" {
		t.Fatalf("backspace: %q", p.Text())
	}
	// Typing inside a link stays in the link; at its end it does not.
	e.SetCaret(Pos{0, 6}, false)
	e.InsertText("X")
	e.SetCaret(Pos{0, 9}, false)
	e.InsertText("Y")
	var inLink string
	for _, r := range p.Runs {
		if r.Span == link {
			inLink += r.Text
		}
	}
	if inLink != "liXnk" {
		t.Fatalf("link text %q (%s)", inLink, p.Text())
	}
}
