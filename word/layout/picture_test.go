package layout

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"gioui.org/op"

	"officelite/word/model"
)

func TestPictureLayout(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 400, 200)))
	pic, w, h, err := model.NewPicture(buf.Bytes(), "a.png")
	if err != nil {
		t.Fatal(err)
	}
	p := para("ab")
	p.Runs = append(p.Runs, model.PictureRun(pic, w, h), model.Run{Text: "cd"})
	e := layoutDoc(docOf(p))
	page, box, ok := e.PictureRect(model.Pos{Para: 0, Off: 2})
	if !ok || page != 0 || box.X1-box.X0 != 300 || box.Y1-box.Y0 != 150 {
		t.Fatalf("PictureRect: %v %+v", ok, box)
	}
	pos, _, ok := e.PictureAt(0, (box.X0+box.X1)/2, (box.Y0+box.Y1)/2)
	if !ok || pos != (model.Pos{Para: 0, Off: 2}) {
		t.Fatalf("PictureAt: %v %+v", ok, pos)
	}
	if _, _, ok := e.PictureAt(0, box.X0-20, box.Y0+5); ok {
		t.Error("hit outside the picture")
	}
	if l := e.Pages[0].Lines[0].Line; l.Height < 150 {
		t.Errorf("line height %v", l.Height)
	}
	// Drawing (with the picture selected) mustn't fail; the image op is
	// cached per scaled size.
	var ops op.Ops
	sel := model.Selection{Anchor: pos, Caret: model.Pos{Para: 0, Off: 3}}
	e.DrawPage(&ops, e.Pages[0], DrawOptions{Scale: 1, Sel: sel})
	if c, ok := e.pics[pic]; !ok || c.bucket != 400 {
		t.Errorf("cache: %+v", c)
	}
	e.DrawPage(&ops, e.Pages[0], DrawOptions{Scale: 0.25})
	if c := e.pics[pic]; c.bucket != 128 {
		t.Errorf("scaled bucket %d", c.bucket)
	}
}

func TestCellSelectionRects(t *testing.T) {
	d := model.New("t")
	ed := model.NewEditor(d)
	ed.InsertTable(1, 2)
	ed.InsertText("x")
	e := layoutDoc(d)
	// Within one cell: just the text.
	r := e.SelectionRects(e.Pages[0], model.Pos{Para: 0}, model.Pos{Para: 0, Off: 1})
	if len(r) != 1 || r[0].X1-r[0].X0 > 20 {
		t.Fatalf("in-cell selection: %+v", r)
	}
	// Across both cells: whole cell widths.
	r = e.SelectionRects(e.Pages[0], model.Pos{Para: 0}, model.Pos{Para: 1})
	if len(r) != 2 || r[0].X1-r[0].X0 < 200 || r[1].X1-r[1].X0 < 200 {
		t.Fatalf("cross-cell selection: %+v", r)
	}
}
