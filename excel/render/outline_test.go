// Tests for the outline gutters: sizes, hit-testing and buttons.

package render

import (
	"image"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"officelite/excel/model"
)

// drawn lays out v once in an 800×600 frame.
func drawn(v *View) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 600))}
	v.Draw(gtx)
}

// TestOutlineGutters checks gutter sizes, hit-testing beside them with
// frozen panes, and the level and group buttons.
func TestOutlineGutters(t *testing.T) {
	sh := model.NewSheet("S")
	for r := 1; r <= 30; r++ {
		sh.SetLevel(true, r, 1)
	}
	for r := 2; r <= 3; r++ {
		sh.SetLevel(true, r, 2)
	}
	sh.SetLevel(false, 2, 1)
	sh.FreezeRows, sh.FreezeCols = 2, 1
	v := &View{Sheet: sh, Styles: []model.Style{model.DefaultStyle()}, Geo: NewGeometry(sh, 7), Zoom: 1, DigitWidth: 7,
		Shaper: text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))}
	v.TopRow, v.LeftCol = 2, 1
	drawn(v)
	gw, gh := v.Gutters()
	if gw != outlinePad+outlineLane*3 || gh != outlinePad+outlineLane*2 {
		t.Fatalf("gutters %d×%d", gw, gh)
	}
	// Cells line up with hit-testing in every pane, the gutters aside.
	for _, c := range [][2]int{{0, 0}, {1, 0}, {0, 3}, {5, 0}, {5, 4}} {
		rc, _, ok := v.CellRect(c[0], c[1])
		if !ok {
			t.Fatalf("%v not shown", c)
		}
		m := rc.Min.Add(rc.Max).Div(2)
		if hit := v.HitTest(float32(m.X), float32(m.Y)); hit.Area != AreaCell || hit.Row != c[0] || hit.Col != c[1] {
			t.Errorf("%v: hit %+v", c, hit)
		}
	}
	hw, hh := v.HeaderSize()
	if hit := v.HitTest(float32(gw+2), float32(hh+3)); hit.Area != AreaRowHeader || hit.Row != 0 {
		t.Errorf("row header %+v", hit)
	}
	if hit := v.HitTest(float32(hw+3), float32(gh+2)); hit.Area != AreaColHeader || hit.Col != 0 {
		t.Errorf("column header %+v", hit)
	}
	if hit := v.HitTest(2, 300); hit.Area != AreaOutline {
		t.Errorf("gutter %+v", hit)
	}
	// Buttons: three row levels, two column levels, the - of the inner
	// row group (summary row 5) and the outer (31, off screen), the
	// column group's at D.
	var levels, groups int
	for _, b := range v.OutlineButtons() {
		if b.Level > 0 {
			levels++
			continue
		}
		groups++
		got, ok := v.OutlineButtonAt(float32(b.Rect.Min.X+1), float32(b.Rect.Min.Y+1))
		if !ok || got.Group != b.Group {
			t.Errorf("button %+v not found at its place", b.Group)
		}
		if b.Group.Rows && b.Group.Summary != 4 || !b.Group.Rows && b.Group.Summary != 3 {
			t.Errorf("group button %+v", b.Group)
		}
	}
	if levels != 5 || groups != 2 {
		t.Errorf("%d level buttons, %d group buttons", levels, groups)
	}
}
