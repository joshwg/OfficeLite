package model

// Floating objects: text boxes, shapes and pictures positioned on the
// page instead of in a line. Each is anchored to a paragraph by an opaque
// run (Opaque.Float) that takes no room in its line; its position is an
// offset from a reference (the page, the margins, the column or the
// anchor paragraph) and it is drawn over or behind the text, or pushes
// the text around it.

// FloatRel is what a floating object's position is measured from.
type FloatRel int8

const (
	RelColumn    FloatRel = iota // the text column (Word's default horizontally)
	RelParagraph                 // the anchor paragraph's top (Word's default vertically)
	RelPage                      // the page's edge
	RelMargin                    // the margin (the text area's edge)
	RelLine                      // the anchor line's top
	RelCharacter                 // the anchor character
)

// FloatRelNames are the .docx names of the references (wp:positionH
// relativeFrom).
var FloatRelNames = [...]string{"column", "paragraph", "page", "margin", "line", "character"}

// FloatWrap is how text flows around a floating object.
type FloatWrap int8

const (
	WrapNone      FloatWrap = iota // in front of the text (the text ignores it)
	WrapBehind                     // behind the text
	WrapSquare                     // the text wraps around its box
	WrapTight                      // around its outline (treated as square)
	WrapTopBottom                  // the text stays above and below it
)

// Floating is a floating object.
type Floating struct {
	// Shape is the preset shape ("rect", "roundRect", "ellipse", "line",
	// "rightArrow"...; "" = a rectangle).
	Shape string
	// Blocks is a text box's content (nil for a plain shape or a picture).
	Blocks []Block
	// Picture is a floating picture's image.
	Picture *Picture
	// X and Y are the offset of the object's top-left corner from its
	// references; AlignX ("left", "center", "right", "inside", "outside")
	// and AlignY ("top", "center", "bottom") replace them when set.
	X, Y           float64
	RelX, RelY     FloatRel
	AlignX, AlignY string
	W, H           float64
	Wrap           FloatWrap
	// Distance is the gap kept between the object and wrapped text
	// (left, top, right, bottom), points.
	Distance [4]float64
	// Fill and Line are the colors of the interior and the outline
	// ("" = none); LineWidth is in points.
	Fill, Line string
	LineWidth  float64
	// Inset is a text box's internal margin (left, top, right, bottom).
	Inset [4]float64
	// FlipH and FlipV mirror a line or arrow.
	FlipH, FlipV bool
	// Name is the object's name ("Text Box 1").
	Name string
}

// DefaultInset is Word's text box internal margin: 0.1" left and right,
// 0.05" top and bottom.
var DefaultInset = [4]float64{7.2, 3.6, 7.2, 3.6}

// IsTextBox reports whether the object holds text.
func (f *Floating) IsTextBox() bool { return f.Blocks != nil }

// Floats lists the floating objects anchored in blocks with the
// paragraphs anchoring them.
func Floats(blocks []Block, fn func(p *Paragraph, run int, f *Floating)) {
	WalkBlocks(blocks, func(p *Paragraph) bool {
		for i := range p.Runs {
			if o := p.Runs[i].Opaque; o != nil && o.Float != nil {
				fn(p, i, o.Float)
			}
		}
		return true
	})
}
