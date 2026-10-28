package model

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func typed(t *testing.T, texts ...string) *Editor {
	t.Helper()
	d := New("t")
	e := NewEditor(d)
	for i, s := range texts {
		if i > 0 {
			e.Enter()
		}
		e.InsertText(s)
	}
	return e
}

func TestInsertTable(t *testing.T) {
	e := typed(t, "Hello world")
	e.SetCaret(Pos{0, 5}, false)
	e.InsertTable(2, 3)
	body := e.Doc.Body
	if len(body) != 3 {
		t.Fatalf("body has %d blocks", len(body))
	}
	tb, ok := body[1].(*Table)
	if !ok || len(tb.Rows) != 2 || len(tb.Rows[0].Cells) != 3 || !tb.Borders || tb.Style != "TableGrid" {
		t.Fatalf("table: %+v", body[1])
	}
	if got := body[0].(*Paragraph).Text() + "|" + body[2].(*Paragraph).Text(); got != "Hello| world" {
		t.Errorf("split: %q", got)
	}
	if tl, ok := e.CellAt(e.Sel.Caret); !ok || tl.Row != 0 || tl.Cell != 0 {
		t.Errorf("caret not in first cell: %+v", e.Sel)
	}
	w := 0.0
	for _, c := range tb.Columns {
		w += c
	}
	if w != 468 {
		t.Errorf("table width %v", w)
	}
	e.InsertText("A")
	e.NextCell(1)
	e.InsertText("B")
	if tb.Rows[0].Cells[1].Blocks[0].(*Paragraph).Text() != "B" {
		t.Error("Tab didn't move to the next cell")
	}
	// Tab in the last cell adds a row.
	e.SelectTable()
	e.Sel.Anchor = e.Sel.Caret
	e.NextCell(1)
	if len(tb.Rows) != 3 {
		t.Fatalf("rows after Tab in last cell: %d", len(tb.Rows))
	}
	if tl, _ := e.CellAt(e.Sel.Caret); tl.Row != 2 || tl.Cell != 0 {
		t.Errorf("caret at %+v", tl)
	}
	// Undo everything back to the plain paragraph.
	for len(e.Doc.Body) > 1 && e.CanUndo() {
		e.Undo()
	}
	if len(e.Doc.Body) != 1 || e.Para(0).Text() != "Hello world" {
		t.Fatalf("undo: %d blocks, %q", len(e.Doc.Body), e.Para(0).Text())
	}
}

func TestInsertTableEmptyPara(t *testing.T) {
	e := typed(t, "")
	e.InsertTable(1, 1)
	if len(e.Doc.Body) != 2 {
		t.Fatalf("%d blocks", len(e.Doc.Body))
	}
	if _, ok := e.Doc.Body[0].(*Table); !ok {
		t.Fatal("table should come first")
	}
	if _, ok := e.Doc.Body[1].(*Paragraph); !ok {
		t.Fatal("a paragraph must follow the table")
	}
}

func cellText(tb *Table, r, c int) string {
	return tb.Rows[r].Cells[c].Blocks[0].(*Paragraph).Text()
}

func TestRowsAndColumns(t *testing.T) {
	e := typed(t, "")
	e.InsertTable(2, 2)
	tb := e.Doc.Body[0].(*Table)
	for _, s := range []string{"a", "b", "c", "d"} {
		e.InsertText(s)
		if s != "d" {
			e.NextCell(1)
		}
	}
	e.SetCaret(Pos{Para: 0}, false) // cell a
	e.InsertRow(false)
	if len(tb.Rows) != 3 || cellText(tb, 0, 0) != "" || cellText(tb, 1, 0) != "a" {
		t.Fatalf("insert row above: %d rows", len(tb.Rows))
	}
	e.InsertRow(true)
	if len(tb.Rows) != 4 || cellText(tb, 2, 0) != "a" {
		t.Fatalf("insert row below")
	}
	e.DeleteRows()
	e.SetCaret(Pos{Para: 0}, false)
	e.DeleteRows()
	if len(tb.Rows) != 2 || cellText(tb, 0, 0) != "a" {
		t.Fatalf("delete rows: %d, %q", len(tb.Rows), cellText(tb, 0, 0))
	}
	// Columns.
	e.SetCaret(Pos{Para: 0}, false)
	e.InsertColumn(true)
	if n := len(tb.Rows[0].Cells); n != 3 || len(tb.Columns) != 3 || cellText(tb, 0, 2) != "b" || cellText(tb, 1, 1) != "" {
		t.Fatalf("insert column right: %d cells", n)
	}
	total := tb.Columns[0] + tb.Columns[1] + tb.Columns[2]
	if total < 467.9 || total > 468.1 {
		t.Errorf("width changed: %v", total)
	}
	if tl, _ := e.CellAt(e.Sel.Caret); tl.Cell != 1 {
		t.Errorf("caret in cell %d", tl.Cell)
	}
	e.InsertColumn(false)
	if len(tb.Rows[1].Cells) != 4 || cellText(tb, 1, 0) != "c" {
		t.Fatalf("insert column left")
	}
	e.DeleteColumns()
	e.DeleteColumns()
	if len(tb.Rows[0].Cells) != 2 || cellText(tb, 0, 1) != "b" || len(tb.Columns) != 2 {
		t.Fatalf("delete columns: %d", len(tb.Rows[0].Cells))
	}
	e.DeleteTable()
	if len(e.Doc.Body) != 1 {
		t.Fatal("delete table")
	}
	for e.CanUndo() {
		e.Undo()
	}
	if len(e.Doc.Body) != 1 {
		t.Fatal("undo all")
	}
	for e.CanRedo() {
		e.Redo()
	}
	if len(e.Doc.Body) != 1 {
		t.Fatal("redo all")
	}
}

func TestMergedCells(t *testing.T) {
	e := typed(t, "")
	e.InsertTable(3, 3)
	tb := e.Doc.Body[0].(*Table)
	// Merge the first row into one cell spanning 3 columns.
	tb.Rows[0].Cells = tb.Rows[0].Cells[:1]
	tb.Rows[0].Cells[0].Span = 3
	e.reindex()
	e.SetCaret(Pos{Para: 2}, false) // row 1, cell 1
	e.InsertColumn(true)
	if tb.Rows[0].Cells[0].Span != 4 || len(tb.Rows[1].Cells) != 4 {
		t.Fatalf("straddled merged cell: span %d", tb.Rows[0].Cells[0].Span)
	}
	e.DeleteColumns()
	if tb.Rows[0].Cells[0].Span != 3 || len(tb.Rows[1].Cells) != 3 {
		t.Fatalf("delete column under merged cell: span %d", tb.Rows[0].Cells[0].Span)
	}
}

func TestSnapToCells(t *testing.T) {
	e := typed(t, "")
	e.InsertTable(2, 2)
	e.InsertText("abc")
	e.NextCell(1)
	e.InsertText("def")
	s := e.SnapToCells(Selection{Pos{0, 1}, Pos{1, 1}})
	if s.Anchor != (Pos{0, 0}) || s.Caret != (Pos{1, 3}) {
		t.Errorf("snap: %+v", s)
	}
	if s := e.SnapToCells(Selection{Pos{0, 1}, Pos{0, 2}}); s.Anchor != (Pos{0, 1}) {
		t.Errorf("within a cell: %+v", s)
	}
}

func testPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestInsertPicture(t *testing.T) {
	e := typed(t, "ab")
	e.SetCaret(Pos{0, 1}, false)
	pic, w, h, err := NewPicture(testPNG(1600, 800), "x.png")
	if err != nil || w != 1200 || h != 600 || pic.Ext != "png" {
		t.Fatalf("NewPicture: %v %v %v %+v", err, w, h, pic)
	}
	e.InsertPicture(pic, w, h)
	r := e.SelectedPicture()
	if r == nil {
		t.Fatal("picture not selected")
	}
	if r.Opaque.Width != 468 || r.Opaque.Height != 234 {
		t.Errorf("scaled to %v × %v", r.Opaque.Width, r.Opaque.Height)
	}
	if img, err := r.Opaque.Picture.Image(); err != nil || img.Bounds().Dx() != 1600 {
		t.Errorf("decode: %v", err)
	}
	if e.Para(0).Text() != "a￼b" {
		t.Errorf("text %q", e.Para(0).Text())
	}
	e.ResizePicture(Pos{0, 1}, 100, 50)
	if r := e.PictureAt(Pos{0, 1}); r.Opaque.Width != 100 {
		t.Error("resize")
	}
	e.Undo()
	if r := e.PictureAt(Pos{0, 1}); r.Opaque.Width != 468 {
		t.Error("undo resize")
	}
	e.Select(Pos{0, 1}, Pos{0, 2})
	e.DeleteSelection()
	if e.Para(0).Text() != "ab" {
		t.Error("delete picture")
	}
}

func TestLinks(t *testing.T) {
	e := typed(t, "see site here")
	e.Select(Pos{0, 4}, Pos{0, 8})
	e.InsertLink("site", "example.com")
	l, ok := e.LinkAt(Pos{0, 5})
	if !ok || l.Span.URL != "http://example.com" || l.Start.Off != 4 || l.End.Off != 8 || l.Text(e) != "site" {
		t.Fatalf("link: %+v %v", l, ok)
	}
	if p := e.Para(0); p.Runs[1].Style != "Hyperlink" {
		t.Errorf("style %q", p.Runs[1].Style)
	}
	// Edit: new address and text.
	e.SetCaret(Pos{0, 6}, false)
	e.InsertLink("our site", "me@x.org")
	l, ok = e.LinkAt(Pos{0, 5})
	if !ok || l.Span.URL != "mailto:me@x.org" || l.Text(e) != "our site" {
		t.Fatalf("edited: %+v", l)
	}
	if e.Para(0).Text() != "see our site here" {
		t.Errorf("text %q", e.Para(0).Text())
	}
	e.Undo()
	if l, _ := e.LinkAt(Pos{0, 5}); l.Span.URL != "http://example.com" || e.Para(0).Text() != "see site here" {
		t.Errorf("undo edit: %q", l.Span.URL)
	}
	e.SetCaret(Pos{0, 5}, false)
	e.RemoveLink()
	if _, ok := e.LinkAt(Pos{0, 5}); ok || e.Para(0).Runs[0].Style != "" {
		t.Error("remove link")
	}
	// New link text at the caret.
	e.SetCaret(Pos{0, 0}, false)
	e.InsertLink("", "https://go.dev")
	if l, ok := e.LinkAt(Pos{0, 1}); !ok || l.Text(e) != "https://go.dev" {
		t.Error("inserted link text")
	}
	// Text typed right after a link is plain.
	e.InsertText("x")
	if r := e.Para(0).Runs[1]; !strings.HasPrefix(r.Text, "x") || r.Style != "" || r.Span != nil {
		t.Errorf("typed after link: %+v", r)
	}
}

func TestLinkTarget(t *testing.T) {
	for in, want := range map[string]string{
		"a@b.com": "mailto:a@b.com", "www.x.com": "http://www.x.com", "https://x": "https://x",
		"#top": "#top", `C:\a\b.txt`: "file:///C:/a/b.txt", "x.org/p": "http://x.org/p",
	} {
		if got := LinkTarget(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
