package model

import "testing"

func TestFindAll(t *testing.T) {
	_, e := newDoc("The cat sat on the mat.", "Catalog THE end")
	ms := e.FindAll("the", FindOptions{})
	if len(ms) != 3 || ms[0].A != (Pos{0, 0}) || ms[1].A != (Pos{0, 15}) || ms[2].A != (Pos{1, 8}) {
		t.Fatalf("case-insensitive: %v", ms)
	}
	if ms := e.FindAll("the", FindOptions{MatchCase: true}); len(ms) != 1 || ms[0].A != (Pos{0, 15}) {
		t.Fatalf("match case: %v", ms)
	}
	if ms := e.FindAll("cat", FindOptions{WholeWords: true}); len(ms) != 1 || ms[0].A.Para != 0 {
		t.Fatalf("whole words: %v", ms)
	}
	if ms := e.FindAll("cat", FindOptions{}); len(ms) != 2 {
		t.Fatalf("parts of words: %v", ms)
	}
	if i := MatchIndex(ms, Pos{0, 5}, false); i != 1 {
		t.Errorf("next from 0:5 = %d", i)
	}
	if i := MatchIndex(ms, Pos{0, 5}, true); i != 0 {
		t.Errorf("prev from 0:5 = %d", i)
	}
	if i := MatchIndex(ms, Pos{1, 9}, false); i != 0 {
		t.Errorf("next wraps: %d", i)
	}
}

func TestFindAcrossRuns(t *testing.T) {
	d, e := newDoc("")
	p := d.Body[0].(*Paragraph)
	p.Runs = []Run{{Text: "Hel", Props: RunProps{Bold: On}}, {Text: "lo wor", Props: RunProps{Italic: On}}, {Text: "ld"}}
	e.reindex()
	ms := e.FindAll("lo world", FindOptions{})
	if len(ms) != 1 || ms[0].A != (Pos{0, 3}) || ms[0].B != (Pos{0, 11}) {
		t.Fatalf("spanning runs: %v", ms)
	}
	ms = e.FindAll("llo w", FindOptions{})
	e.Replace(ms[0], "XY")
	if got := p.Text(); got != "HeXYorld" {
		t.Fatalf("replace: %q", got)
	}
	// The replacement takes the first matched character's formatting (bold).
	if p.Runs[0].Text != "HeXY" || p.Runs[0].Props.Bold != On {
		t.Errorf("replacement formatting: %+v", p.Runs)
	}
	if a, b := e.Sel.Ordered(); a != (Pos{0, 2}) || b != (Pos{0, 4}) {
		t.Errorf("selection after replace: %v %v", a, b)
	}
	e.Undo()
	if got := d.Body[0].(*Paragraph).Text(); got != "Hello world" {
		t.Errorf("undo: %q", got)
	}
}

func TestReplaceAllOneStep(t *testing.T) {
	d, e := newDoc("a b a", "x", "a")
	n := e.ReplaceAll("a", "zz", FindOptions{WholeWords: true})
	if n != 3 || texts(d) != "zz b zz|x|zz" {
		t.Fatalf("replace all: %d %q", n, texts(d))
	}
	if len(e.undo) != 1 {
		t.Errorf("replace all should be one undo step, got %d", len(e.undo))
	}
	e.Undo()
	if texts(d) != "a b a|x|a" {
		t.Errorf("undo: %q", texts(d))
	}
	if n := e.ReplaceAll("nothing", "x", FindOptions{}); n != 0 || len(e.redo) != 1 {
		t.Errorf("no matches: %d", n)
	}
}

func TestFormatPainter(t *testing.T) {
	d, e := newDoc("bold text here", "plain words", "")
	p0 := d.Body[0].(*Paragraph)
	p0.Runs = []Run{{Text: "bold", Props: RunProps{Bold: On, Color: "FF0000"}}, {Text: " text here"}}
	p0.Style = "Heading1"
	p0.Props.Align = AlignCenter
	e.reindex()
	e.Select(Pos{0, 0}, Pos{0, 2})
	f := e.CopyFormat()
	if f.Props.Bold != On || f.Para != nil {
		t.Fatalf("copy: %+v", f)
	}
	// A caret in a word paints the word.
	e.SetCaret(Pos{1, 2}, false)
	e.PasteFormat(f)
	p1 := d.Body[1].(*Paragraph)
	if p1.Runs[0].Text != "plain" || p1.Runs[0].Props.Bold != On || p1.Runs[1].Props.Bold == On {
		t.Fatalf("paint word: %+v", p1.Runs)
	}
	if len(e.undo) != 1 || e.Sel.Caret != (Pos{1, 2}) {
		t.Errorf("one step, caret kept: %d %v", len(e.undo), e.Sel.Caret)
	}
	// A whole paragraph copies paragraph formatting too.
	e.Select(Pos{0, 0}, Pos{0, p0.Len()})
	f = e.CopyFormat()
	if f.Para == nil || f.Para.Style != "Heading1" {
		t.Fatalf("whole paragraph copy: %+v", f)
	}
	e.Select(Pos{1, 0}, Pos{1, p1.Len()})
	e.PasteFormat(f)
	if p1.Style != "Heading1" || p1.Props.Align != AlignCenter {
		t.Errorf("paragraph formatting not painted: %q %v", p1.Style, p1.Props.Align)
	}
	// Painting part of a paragraph leaves its paragraph formatting.
	e.Undo()
	e.Select(Pos{1, 0}, Pos{1, 3})
	e.PasteFormat(f)
	if p1 := d.Body[1].(*Paragraph); p1.Style != "" {
		t.Errorf("partial paint changed the paragraph style")
	}
}

// Invisible markers (a bookmark inside a word, a comment range) don't
// stop a match or count as word boundaries, and replacing keeps them.
func TestFindAcrossMarkers(t *testing.T) {
	d, e := newDoc("")
	p := d.Body[0].(*Paragraph)
	mark := func() Run { return Run{Kind: RunOpaque, Opaque: &Opaque{Format: "docx"}} }
	p.Runs = []Run{{Text: "the ca"}, mark(), {Text: "t sat"}, mark()}
	e.reindex()
	ms := e.FindAll("cat", FindOptions{WholeWords: true})
	if len(ms) != 1 || ms[0].A != (Pos{0, 4}) || ms[0].B != (Pos{0, 8}) {
		t.Fatalf("across a marker: %v", ms)
	}
	if ms := e.FindAll("ca", FindOptions{WholeWords: true}); len(ms) != 0 {
		t.Fatalf("marker taken as a word boundary: %v", ms)
	}
	if ms := e.FindAll("sat", FindOptions{WholeWords: true}); len(ms) != 1 || ms[0].B != (Pos{0, 12}) {
		t.Fatalf("before a trailing marker: %v", ms)
	}
	if n := e.ReplaceAll("cat", "dog", FindOptions{}); n != 1 {
		t.Fatalf("replaced %d", n)
	}
	markers := 0
	for _, r := range p.Runs {
		if r.Kind == RunOpaque {
			markers++
		}
	}
	if got := p.Text(); got != "the dog￼ sat￼" || markers != 2 {
		t.Fatalf("replace: %q, %d markers", got, markers)
	}
}
