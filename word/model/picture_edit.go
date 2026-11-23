// Editing a picture's layout and look, inline or floating: what Word's
// Size and Position (Layout) and Format Picture commands change. Every
// change is one undoable step, and turning an inline picture into a
// floating one (or back) keeps its image, size, border, crop and
// alternative text.

package model

// FloatDistance is Word's gap between a floating picture and the text
// wrapped around it (left, top, right, bottom, points): 0.13" at the
// sides, none above and below.
var FloatDistance = [4]float64{9, 0, 9, 0}

// PictureObjAt returns the run of the picture at caret offset p (the
// character after p), inline or floating, or nil.
func (e *Editor) PictureObjAt(p Pos) *Run {
	p = e.clamp(p)
	para := e.locs[p.Para].P
	if i := para.RunAt(p.Off); i >= 0 {
		if r := &para.Runs[i]; r.Kind == RunOpaque && isPictureObj(r.Opaque) {
			return r
		}
	}
	return nil
}

// isPictureObj reports whether o shows a picture, inline or floating.
func isPictureObj(o *Opaque) bool {
	return o != nil && (o.Picture != nil || o.Float != nil && o.Float.Picture != nil)
}

// SelectedPictureObj returns the run of the picture, inline or floating,
// when the selection is exactly that one character.
func (e *Editor) SelectedPictureObj() *Run {
	a, b := e.Sel.Ordered()
	if a.Para != b.Para || b.Off != a.Off+1 {
		return nil
	}
	return e.PictureObjAt(a)
}

// EditPicture changes the picture just after p as one undoable step named
// label: fn gets a copy of the run's content, with its own copy of a
// floating object, to change. It reports false when there is no picture
// there. The original file's XML is dropped, so the writer describes the
// picture as it now is.
func (e *Editor) EditPicture(p Pos, label string, fn func(o *Opaque)) bool {
	defer e.Op(label)()
	p = e.clamp(p) // as PictureObjAt sees it
	if e.PictureObjAt(p) == nil {
		return false
	}
	top := e.locs[p.Para].Top
	e.edit("format", top, top, false, func() {
		r := e.PictureObjAt(p)
		o := *r.Opaque
		if o.Float != nil {
			f := *o.Float
			o.Float = &f
		}
		fn(&o)
		if o.Format == "docx" {
			o.Format, o.Data = "", nil
		}
		r.Opaque = &o
		if o.Float != nil {
			r.Text = "" // as a floating object read from a file
		} else {
			r.Text = "[picture]"
		}
	})
	return true
}

// FloatPicture makes the inline picture o a floating one that text wraps
// as wrap says, at offset (x, y) from the text column and the paragraph
// (Word's defaults), keeping its size, border, crop and alternative
// text. A floating picture only changes its wrapping.
func FloatPicture(o *Opaque, wrap FloatWrap, x, y float64) {
	if o.Float != nil {
		o.Float.Wrap = wrap
		return
	}
	if o.Picture == nil {
		return
	}
	o.Float = &Floating{
		Picture: o.Picture, Look: o.Look, W: o.Width, H: o.Height,
		X: x, Y: y, RelX: RelColumn, RelY: RelParagraph,
		Wrap: wrap, Distance: FloatDistance, Line: o.Line, LineWidth: o.LineWidth,
	}
	o.Picture, o.Width, o.Height, o.Look, o.Line, o.LineWidth = nil, 0, 0, PictureLook{}, "", 0
}

// InlinePicture makes the floating picture o an inline one at its size,
// keeping its border, crop and alternative text.
func InlinePicture(o *Opaque) {
	f := o.Float
	if f == nil || f.Picture == nil {
		return
	}
	o.Picture, o.Width, o.Height, o.Look, o.Line, o.LineWidth = f.Picture, f.W, f.H, f.Look, f.Line, f.LineWidth
	o.Float = nil
}

// PictureSize returns the size of the picture o shows, inline or
// floating.
func PictureSize(o *Opaque) (float64, float64) {
	if o.Float != nil {
		return o.Float.W, o.Float.H
	}
	return o.Width, o.Height
}

// SetPictureSize sets the size of the picture o shows, inline or
// floating.
func SetPictureSize(o *Opaque, w, h float64) {
	if o.Float != nil {
		o.Float.W, o.Float.H = w, h
		return
	}
	o.Width, o.Height = w, h
}

// PictureImage returns the image o shows, inline or floating.
func PictureImage(o *Opaque) *Picture {
	if o.Float != nil {
		return o.Float.Picture
	}
	return o.Picture
}

// PictureLookOf returns a pointer to the look (crop, alternative text) of
// the picture o shows, inline or floating, for changing it.
func PictureLookOf(o *Opaque) *PictureLook {
	if o.Float != nil {
		return &o.Float.Look
	}
	return &o.Look
}

// PictureLine returns pointers to the border color and width of the
// picture o shows, inline or floating, for changing them.
func PictureLine(o *Opaque) (*string, *float64) {
	if o.Float != nil {
		return &o.Float.Line, &o.Float.LineWidth
	}
	return &o.Line, &o.LineWidth
}
