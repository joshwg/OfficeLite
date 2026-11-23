// Tests of editing pictures' layout and look: inline to floating and
// back, one undo step each, and copies that keep a picture a picture.

package model

import "testing"

// pictureDoc returns an editor on a document holding "a", a 40 × 20
// picture and "b".
func pictureDoc(t *testing.T) *Editor {
	t.Helper()
	e := NewEditor(New("t"))
	e.InsertText("a")
	e.InsertPicture(&Picture{Data: []byte("x"), Ext: "png"}, 40, 20)
	e.SetCaret(e.Sel.Caret, false)
	e.InsertText("b")
	return e
}

// TestFloatPictureAndBack checks that an inline picture becomes a
// floating one and back keeping its size, border and look, with each
// change one undo step.
func TestFloatPictureAndBack(t *testing.T) {
	e := pictureDoc(t)
	at := Pos{Para: 0, Off: 1}
	look := PictureLook{Crop: [4]float64{0.1, 0, 0, 0}, Alt: "logo"}
	e.EditPicture(at, "Format Picture", func(o *Opaque) { o.Look, o.Line, o.LineWidth = look, "000000", 1 })
	e.EditPicture(at, "Wrap Text", func(o *Opaque) { FloatPicture(o, WrapSquare, 10, 5) })
	r := e.PictureObjAt(at)
	f := r.Opaque.Float
	if f == nil || f.Picture == nil || f.W != 40 || f.H != 20 || f.X != 10 || f.Y != 5 || f.Look != look ||
		f.Line != "000000" || f.Wrap != WrapSquare || f.Distance != FloatDistance || r.Opaque.Picture != nil || r.Text != "" || f.IsTextBox() {
		t.Fatalf("floated: %+v %+v", r.Opaque, f)
	}
	e.Select(at, Pos{Para: 0, Off: 2})
	if e.SelectedPictureObj() == nil || e.SelectedPicture() != nil {
		t.Error("a selected floating picture is found as one, and not as an inline picture")
	}
	e.EditPicture(at, "Wrap Text", func(o *Opaque) { InlinePicture(o) })
	r = e.PictureObjAt(at)
	if o := r.Opaque; o.Float != nil || o.Picture == nil || o.Width != 40 || o.Look != look || o.Line != "000000" || r.Text != "[picture]" {
		t.Fatalf("back inline: %+v", o)
	}
	e.Undo()
	if e.PictureObjAt(at).Opaque.Float == nil {
		t.Error("undo didn't make it floating again")
	}
	e.Undo()
	if o := e.PictureObjAt(at).Opaque; o.Float != nil || o.Look != look {
		t.Errorf("second undo: %+v", o)
	}
	if e.EditPicture(Pos{Para: 0, Off: 0}, "x", func(*Opaque) {}) {
		t.Error("edited a letter as a picture")
	}
}

// TestCopyKeepsShapes checks that copying a floating shape or picture
// (as undo does) doesn't make it an empty text box.
func TestCopyKeepsShapes(t *testing.T) {
	o := &Opaque{Float: &Floating{Shape: "ellipse"}}
	if c := copyOpaque(o); c.Float.IsTextBox() {
		t.Error("a shape became a text box")
	}
	tb := &Opaque{Float: &Floating{Blocks: []Block{&Paragraph{}}}}
	if c := copyOpaque(tb); !c.Float.IsTextBox() || &c.Float.Blocks[0] == &tb.Float.Blocks[0] {
		t.Error("a text box's text isn't copied")
	}
}
