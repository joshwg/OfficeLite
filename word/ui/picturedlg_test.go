// Tests of the picture dialogs and floating pictures in the window: the
// right-click menu's picture commands, Size and Position turning a
// picture floating (in place) and back, Format Picture's border,
// alternative text and crop, and dragging a floating picture.

package ui

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"

	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// pictureHarness returns a window holding "x" and a 200 × 100 pixel
// (150 × 75 pt) picture, selected.
func pictureHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.typeText("x")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 200, 100))); err != nil {
		t.Fatal(err)
	}
	h.d.insertPictureData("p.png", buf.Bytes(), nil)
	h.frame()
	if h.d.ed.SelectedPicture() == nil {
		t.Fatal("no picture selected")
	}
	return h
}

// TestPictureMenu checks that the right-click menu offers Word's picture
// commands for a picture and not for text.
func TestPictureMenu(t *testing.T) {
	h := pictureHarness(t)
	d := h.d
	has := func(label string) bool {
		d.openContextMenu(image.Pt(10, 10))
		for _, it := range d.v.m.items {
			if it.Label == label {
				return true
			}
		}
		return false
	}
	for _, l := range []string{"Edit Alt Text…", "Size and Position…", "Format Picture…"} {
		if !has(l) {
			t.Errorf("no %s for a picture", l)
		}
	}
	if has("Modify Style…") {
		t.Error("Modify Style… for a picture")
	}
	d.ed.SetCaret(model.Pos{}, false)
	if has("Format Picture…") {
		t.Error("Format Picture… for text")
	}
}

// TestLayoutDialogWrapAndBack checks that choosing Square makes the
// picture float where it was, a new size applies with the aspect ratio
// kept, and In line with text brings it back.
func TestLayoutDialogWrapAndBack(t *testing.T) {
	h := pictureHarness(t)
	d := h.d
	at := model.Pos{Para: 0, Off: 1}
	pg, before, ok := d.eng.PictureRect(at)
	if !ok {
		t.Fatal("no picture box")
	}
	d.openPictureDialog(picLayoutDlg, tabSize)
	p := &d.pic
	if !p.open || p.height.Text() != `1.04"` || p.width.Text() != `2.08"` || p.scaleH.Text() != "100 %" {
		t.Fatalf("Size tab: %q × %q, %q", p.height.Text(), p.width.Text(), p.scaleH.Text())
	}
	p.wrap.Value = "square"
	p.ok.Click()
	h.frame()
	h.frame()
	r := d.ed.PictureObjAt(at)
	if r == nil || r.Opaque.Float == nil || r.Opaque.Float.Wrap != model.WrapSquare {
		t.Fatalf("not floating: %+v", r)
	}
	fpg, fb, ok := d.eng.FloatRect(at)
	if !ok || fpg != pg || math.Abs(fb.R.X0-before.X0) > 0.5 || math.Abs(fb.R.Y0-before.Y0) > 0.5 {
		t.Errorf("floated from %+v to %+v", before, fb.R)
	}
	if d.ed.SelectedPictureObj() == nil {
		t.Error("the floating picture isn't selected")
	}

	// A new height keeps the aspect ratio.
	d.openPictureDialog(picLayoutDlg, tabSize)
	p.height.SetText(`2"`)
	p.syncScales()
	p.width.SetText(gui.FormatInches(144 * 2)) // as the lock would
	p.ok.Click()
	h.frame()
	if f := d.ed.PictureObjAt(at).Opaque.Float; f.H != 144 || f.W != 288 {
		t.Errorf("size %v × %v", f.W, f.H)
	}

	d.openPictureDialog(picLayoutDlg, tabWrapping)
	p.wrap.Value = "inline"
	p.ok.Click()
	h.frame()
	if o := d.ed.PictureObjAt(at).Opaque; o.Float != nil || o.Width != 288 || o.Height != 144 {
		t.Errorf("back inline: %+v", o)
	}
	d.ed.Undo()
	if d.ed.PictureObjAt(at).Opaque.Float == nil {
		t.Error("one undo didn't make it float again")
	}
}

// TestLayoutDialogPosition checks the Position tab's alignment and an
// absolute position from the page.
func TestLayoutDialogPosition(t *testing.T) {
	h := pictureHarness(t)
	d := h.d
	at := model.Pos{Para: 0, Off: 1}
	d.openPictureDialog(picLayoutDlg, tabPosition)
	p := &d.pic
	p.wrap.Value = "front"
	p.hMode.Value, p.hAlign, p.hAlignRel = "align", 2, 1 // right of the margin
	p.vMode.Value, p.vAbsRel = "abs", 2                  // from the page
	p.vAbs.SetText(`3"`)
	p.ok.Click()
	h.frame()
	h.frame()
	f := d.ed.PictureObjAt(at).Opaque.Float
	if f == nil || f.Wrap != model.WrapNone || f.AlignX != "right" || f.RelX != model.RelMargin || f.AlignY != "" || f.Y != 216 || f.RelY != model.RelPage {
		t.Fatalf("position: %+v", f)
	}
	_, fb, ok := d.eng.FloatRect(at)
	sec := d.doc.Page
	if !ok || math.Abs(fb.R.X1-(sec.Width-sec.MarginRight)) > 0.5 || math.Abs(fb.R.Y0-216) > 0.5 {
		t.Errorf("placed at %+v", fb.R)
	}
	// OK again without changes keeps everything exactly.
	d.openPictureDialog(picLayoutDlg, tabPosition)
	p.ok.Click()
	h.frame()
	if g := d.ed.PictureObjAt(at).Opaque.Float; g.X != f.X || g.Y != f.Y || g.W != f.W || g.H != f.H || g.AlignX != f.AlignX ||
		g.RelX != f.RelX || g.RelY != f.RelY || g.Distance != f.Distance || g.Wrap != f.Wrap {
		t.Errorf("unchanged dialog changed %+v to %+v", f, g)
	}
}

// TestFormatPictureDialog checks the border, the alternative text (gone
// when marked decorative) and a crop that keeps the scale.
func TestFormatPictureDialog(t *testing.T) {
	h := pictureHarness(t)
	d := h.d
	at := model.Pos{Para: 0, Off: 1}
	d.openPictureDialog(picFormatDlg, tabFillLine)
	p := &d.pic
	p.line.Value = "solid"
	p.lineWidth.SetText("3 pt")
	p.alt.SetText("  A gray box  ")
	p.cropL.SetText(gui.FormatInches(37.5)) // a quarter of the 150 pt width
	p.ok.Click()
	h.frame()
	o := d.ed.PictureObjAt(at).Opaque
	if o.Line != "000000" || o.LineWidth != 3 || o.Look.Alt != "A gray box" || math.Abs(o.Look.Crop[0]-0.25) > 0.01 ||
		math.Abs(o.Width-112.5) > 0.5 || o.Height != 75 {
		t.Fatalf("formatted: %+v %v × %v line %q %v", o.Look, o.Width, o.Height, o.Line, o.LineWidth)
	}
	d.openPictureDialog(picFormatDlg, tabProperties)
	if p.alt.Text() != "A gray box" || p.line.Value != "solid" {
		t.Errorf("reopened: alt %q line %q", p.alt.Text(), p.line.Value)
	}
	p.decorative.Value = true
	p.line.Value = "none"
	p.ok.Click()
	h.frame()
	o = d.ed.PictureObjAt(at).Opaque
	if !o.Look.Decorative || o.Look.Alt != "" || o.Line != "" || math.Abs(o.Look.Crop[0]-0.25) > 0.01 {
		t.Errorf("decorative: %+v line %q", o.Look, o.Line)
	}
	d.openPictureDialog(picFormatDlg, tabPicture)
	p.cropR.SetText(`5"`)
	p.ok.Click()
	h.frame()
	if p.err == "" || !p.open {
		t.Error("a crop of everything was taken")
	}
}

// TestDragFloatingPicture checks that a floating picture is selected by
// a click, moves with a drag (one undo step) and resizes by a handle.
func TestDragFloatingPicture(t *testing.T) {
	h := pictureHarness(t)
	d := h.d
	at := model.Pos{Para: 0, Off: 1}
	d.ed.EditPicture(at, "Wrap Text", func(o *model.Opaque) { model.FloatPicture(o, model.WrapNone, 40, 30) })
	d.edited()
	h.frame()
	d.ed.SetCaret(model.Pos{}, false)
	pg, fb, ok := d.eng.FloatRect(at)
	if !ok {
		t.Fatal("no floating picture")
	}
	mid := h.viewPoint(pg, (fb.R.X0+fb.R.X1)/2, (fb.R.Y0+fb.R.Y1)/2)
	h.hover(mid)
	if !d.overFloat(d.v.m.hover) {
		t.Error("not over the floating picture")
	}
	step := float32(20 * d.v.scale)
	h.mouse(pointer.Press, mid, 0)
	if d.ed.SelectedPictureObj() == nil || !d.ins.moving {
		t.Fatal("the press didn't select it and start a move")
	}
	h.mouse(pointer.Drag, mid.Add(f32.Pt(step, step)), 0)
	h.mouse(pointer.Release, mid.Add(f32.Pt(step, step)), 0)
	f := d.ed.PictureObjAt(at).Opaque.Float
	if math.Abs(f.X-60) > 1 || math.Abs(f.Y-50) > 1 {
		t.Errorf("moved to %v, %v", f.X, f.Y)
	}
	// The bottom-right handle makes it larger, keeping its corner.
	_, fb, _ = d.eng.FloatRect(at)
	hd := wl.PictureHandles(fb.R)[4]
	from := h.viewPoint(pg, (hd.X0+hd.X1)/2, (hd.Y0+hd.Y1)/2)
	h.mouse(pointer.Press, from, 0)
	h.mouse(pointer.Drag, from.Add(f32.Pt(step, 0)), 0)
	h.mouse(pointer.Release, from.Add(f32.Pt(step, 0)), 0)
	f = d.ed.PictureObjAt(at).Opaque.Float
	if f.W < 165 || f.W > 175 || math.Abs(f.X-60) > 1 {
		t.Errorf("resized: %v wide at %v", f.W, f.X)
	}
	d.ed.Undo()
	d.ed.Undo()
	if f := d.ed.PictureObjAt(at).Opaque.Float; f.X != 40 || f.Y != 30 {
		t.Errorf("after two undos at %v, %v", f.X, f.Y)
	}
}
