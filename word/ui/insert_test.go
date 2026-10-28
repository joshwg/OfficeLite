package ui

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	wl "officelite/word/layout"
	"officelite/word/model"
)

// viewPoint is the window position of page point (x, y) on page pg.
func (h *harness) viewPoint(pg int, x, y float64) f32.Point {
	d := h.d
	viewTop := 36 + 30 + ribbonHeight // title, menu bar, ribbon
	return f32.Pt(float32(d.pageX())+float32(x*d.v.scale),
		float32(viewTop)+float32((d.eng.Pages[pg].Top+y-d.v.scrollY)*d.v.scale))
}

func (h *harness) mouse(kind pointer.Kind, pt f32.Point, mods key.Modifiers) {
	btn := pointer.ButtonPrimary
	if kind == pointer.Release {
		btn = 0
	}
	if kind == pointer.Drag {
		kind = pointer.Move // the router turns a move with a button down into a drag
	}
	h.r.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Buttons: btn, Position: pt, Modifiers: mods})
	h.frame()
}

func TestInsertMenuTable(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Intro")
	// The grid picker: hover 3 columns × 2 rows and click.
	d.insertChosen(insTable)
	h.frame()
	if d.ins.kind != insPicker {
		t.Fatal("picker not open")
	}
	cell := 20
	pt := f32.Pt(float32(118+10+2*cell+cell/2), float32(64+26+1*cell+cell/2))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pt})
	h.frame()
	if d.ins.pickRows != 2 || d.ins.pickCols != 3 {
		t.Fatalf("hover %d×%d", d.ins.pickRows, d.ins.pickCols)
	}
	h.mouse(pointer.Press, pt, 0)
	h.mouse(pointer.Release, pt, 0)
	tb, ok := d.doc.Body[1].(*model.Table)
	if !ok || len(tb.Rows) != 2 || len(tb.Rows[0].Cells) != 3 || d.ins.kind != insNone {
		t.Fatalf("table not inserted: %T", d.doc.Body[1])
	}
	// Type into cells with Tab; Tab in the last cell adds a row.
	for i := range 6 {
		h.typeText(string(rune('a' + i)))
		h.press(key.NameTab, 0)
	}
	if len(tb.Rows) != 3 {
		t.Fatalf("rows %d", len(tb.Rows))
	}
	h.press(key.NameTab, key.ModShift)
	if a, b := d.ed.Sel.Ordered(); b.Off-a.Off != 1 || d.ed.Para(a.Para).Text() != "f" {
		t.Errorf("Shift+Tab selected %v..%v", a, b)
	}
	// Table commands from the menu.
	d.insertChosen(insDelRow)
	if len(tb.Rows) != 2 {
		t.Errorf("delete row: %d", len(tb.Rows))
	}
	d.insertChosen(insColRight)
	if len(tb.Rows[0].Cells) != 4 {
		t.Errorf("insert column: %d", len(tb.Rows[0].Cells))
	}
	if items := d.insertMenu(); items[insDelTable].Disabled {
		t.Error("table commands disabled in a table")
	}
	d.insertChosen(insDelTable)
	if len(d.doc.Body) != 2 {
		t.Errorf("delete table: %d blocks", len(d.doc.Body))
	}
	if items := d.insertMenu(); !items[insDelTable].Disabled {
		t.Error("table commands enabled outside a table")
	}

	// Insert Table… dialog.
	d.insertChosen(insTableDialog)
	h.frame()
	d.ins.rows.SetText("4")
	d.ins.cols.SetText("2")
	h.press(key.NameReturn, 0)
	if d.ins.kind != insNone {
		t.Fatal("dialog still open")
	}
	found := false
	for _, b := range d.doc.Body {
		if tb, ok := b.(*model.Table); ok && len(tb.Rows) == 4 && len(tb.Rows[0].Cells) == 2 {
			found = true
		}
	}
	if !found {
		t.Error("Insert Table dialog didn't insert 4×2")
	}
}

func TestDragSelectsCells(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.ed.InsertTable(1, 2)
	h.typeText("left text")
	h.press(key.NameTab, 0)
	h.typeText("right text")
	h.frame()
	c0, _ := d.eng.CaretAt(model.Pos{Para: 0, Off: 2})
	c1, _ := d.eng.CaretAt(model.Pos{Para: 1, Off: 3})
	h.mouse(pointer.Press, h.viewPoint(0, c0.X, c0.Y+c0.H/2), 0)
	h.mouse(pointer.Drag, h.viewPoint(0, c1.X, c1.Y+c1.H/2), 0)
	h.mouse(pointer.Release, h.viewPoint(0, c1.X, c1.Y+c1.H/2), 0)
	a, b := d.ed.Sel.Ordered()
	if a != (model.Pos{Para: 0}) || b != (model.Pos{Para: 1, Off: 10}) {
		t.Errorf("drag across cells selected %v..%v", a, b)
	}
}

func TestInsertLinkDialog(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Read the docs please")
	d.ed.Select(model.Pos{Para: 0, Off: 9}, model.Pos{Para: 0, Off: 13})
	h.press("K", key.ModShortcut)
	if d.ins.kind != insLinkDlg || d.ins.text.Text() != "docs" {
		t.Fatalf("dialog %d text %q", d.ins.kind, d.ins.text.Text())
	}
	d.ins.addr.SetText("go.dev/doc")
	h.press(key.NameReturn, 0)
	l, ok := d.ed.LinkAt(model.Pos{Para: 0, Off: 10})
	if !ok || l.Span.URL != "http://go.dev/doc" || l.Text(d.ed) != "docs" {
		t.Fatalf("link %+v", l)
	}
	// In the link the dialog edits it; Remove Link removes it.
	d.ed.SetCaret(model.Pos{Para: 0, Off: 10}, false)
	if items := d.insertMenu(); items[insRemoveLink].Disabled || items[insLink].Label != "Edit Link…" {
		t.Error("menu doesn't offer editing the link")
	}
	d.openLinkDialog()
	h.frame()
	if !d.ins.editing || d.ins.addr.Text() != "http://go.dev/doc" {
		t.Errorf("edit dialog: %v %q", d.ins.editing, d.ins.addr.Text())
	}
	d.ins.remove.Click()
	h.frame()
	h.frame()
	if _, ok := d.ed.LinkAt(model.Pos{Para: 0, Off: 10}); ok {
		t.Error("link not removed")
	}
	if d.ins.kind != insNone {
		t.Error("dialog still open")
	}
	// Escape cancels.
	d.openLinkDialog()
	h.frame()
	h.press(key.NameEscape, 0)
	if d.ins.kind != insNone {
		t.Error("Esc didn't close")
	}
}

func TestSymbolAndDate(t *testing.T) {
	h := newHarness(t)
	d := h.d
	d.insertChosen(insSymbol)
	h.frame()
	d.ins.symbols[0].Click()
	h.frame()
	h.frame()
	if got := h.para(0).Text(); got != "©" {
		t.Errorf("symbol: %q", got)
	}
	d.insertChosen(insDate)
	h.frame()
	d.ins.dateSel = 4
	d.ins.ok.Click()
	h.frame()
	h.frame()
	if got, want := h.para(0).Text(), "©"+time.Now().Format("2006-01-02"); got != want {
		t.Errorf("date: %q, want %q", got, want)
	}
}

func TestInsertPictureAndClick(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("x")
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 200, 100)))
	d.insertPictureData("C:\\pics\\p.png", buf.Bytes(), nil)
	h.frame()
	r := d.ed.SelectedPicture()
	if r == nil || r.Opaque.Width != 150 {
		t.Fatalf("picture: %+v", r)
	}
	d.insertPictureData("bad.png", []byte("nope"), nil)
	if d.errMsg == "" {
		t.Error("no error for a bad picture")
	}
	// Click elsewhere, then on the picture: it is selected.
	d.ed.SetCaret(model.Pos{}, false)
	pg, box, ok := d.eng.PictureRect(model.Pos{Para: 0, Off: 1})
	if !ok {
		t.Fatal("no picture box")
	}
	mid := h.viewPoint(pg, (box.X0+box.X1)/2, (box.Y0+box.Y1)/2)
	h.mouse(pointer.Press, mid, 0)
	h.mouse(pointer.Release, mid, 0)
	if d.ed.SelectedPicture() == nil {
		t.Fatal("click didn't select the picture")
	}
	// Drag the bottom-right handle to make it larger.
	hd := wl.PictureHandles(box)[4]
	from := h.viewPoint(pg, (hd.X0+hd.X1)/2, (hd.Y0+hd.Y1)/2)
	h.mouse(pointer.Press, from, 0)
	if !d.ins.resizing {
		t.Fatal("handle press didn't start a resize")
	}
	h.mouse(pointer.Drag, from.Add(f32.Pt(float32(30*d.v.scale), 0)), 0)
	h.mouse(pointer.Release, from.Add(f32.Pt(float32(30*d.v.scale), 0)), 0)
	r = d.ed.PictureAt(model.Pos{Para: 0, Off: 1})
	if r.Opaque.Width < 175 || r.Opaque.Width > 185 || r.Opaque.Height < 87 || r.Opaque.Height > 93 {
		t.Errorf("resized to %v × %v", r.Opaque.Width, r.Opaque.Height)
	}
	d.ed.Undo()
	if r := d.ed.PictureAt(model.Pos{Para: 0, Off: 1}); r.Opaque.Width != 150 {
		t.Error("undo resize")
	}
	// Delete removes the selected picture.
	d.ed.Select(model.Pos{Para: 0, Off: 1}, model.Pos{Para: 0, Off: 2})
	h.press(key.NameDeleteForward, 0)
	if h.para(0).Text() != "x" {
		t.Errorf("delete: %q", h.para(0).Text())
	}
}
