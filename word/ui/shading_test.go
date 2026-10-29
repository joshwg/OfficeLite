package ui

import (
	"image"
	"testing"

	"gioui.org/io/key"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// pickColor opens cp's menu from the ribbon and clicks swatch i (or No
// Color for i < 0).
func (h *harness) pickColor(cp *gui.ColorPicker, i int) {
	h.t.Helper()
	ribbon := image.Rect(0, ribbonArea.Min.Y, 1100, ribbonArea.Max.Y)
	p := h.locate(cp.Arrow(), ribbon, 4)
	h.click(p, 0)
	if !cp.IsOpen() {
		h.t.Fatal("the arrow didn't open the color menu")
	}
	btn := cp.NoColor()
	if i >= 0 {
		btn = cp.Swatch(i)
	}
	menu := image.Rect(int(p.X)-40, int(p.Y)+6, int(p.X)+250, int(p.Y)+250)
	h.click(h.locate(btn, menu, 4), 0)
	if cp.IsOpen() {
		h.t.Fatal("the color menu stayed open")
	}
}

func TestShadingButton(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Hello world")
	h.press(key.NameReturn, 0)
	h.typeText("Second")
	orange := colors.Hex(d.tb.shadeCP.SwatchColor(62)) // Standard Colors: Orange
	if orange != "FFC000" {
		t.Fatalf("swatch 62 is %s", orange)
	}

	// Part of a paragraph: the menu shades the selected text.
	d.ed.Select(model.Pos{Para: 0, Off: 6}, model.Pos{Para: 0, Off: 11})
	h.frame()
	h.pickColor(&d.tb.shadeCP, 62)
	p0 := h.para(0)
	if len(p0.Runs) != 2 || p0.Runs[1].Text != "world" || p0.Runs[1].Props.Shading != orange || p0.Runs[0].Props.Shading != "" || p0.Props.Shading != "" {
		t.Fatalf("text shading: %+v / %q", p0.Runs, p0.Props.Shading)
	}
	if d.tb.lastShade != orange {
		t.Errorf("the button's color is %q, want the last one picked", d.tb.lastShade)
	}

	// The button part repeats the last color on the caret's paragraph.
	d.ed.SetCaret(model.Pos{Para: 1, Off: 2}, false)
	h.frame()
	h.click(h.locate(&d.tb.shade, image.Rect(0, ribbonArea.Min.Y, 1100, ribbonArea.Max.Y), 4), 0)
	if got := h.para(1).Props.Shading; got != orange {
		t.Errorf("paragraph shading %q", got)
	}

	// Whole cells: cell shading; No Color takes it off again.
	d.ed.SetCaret(model.Pos{Para: 1, Off: 6}, false)
	d.ed.Enter()
	d.ed.InsertTable(1, 2)
	d.ed.SelectTable()
	d.edited()
	h.frame()
	h.pickColor(&d.tb.shadeCP, 63) // Yellow
	var tb *model.Table
	for _, b := range d.doc.Body {
		if x, ok := b.(*model.Table); ok {
			tb = x
		}
	}
	if tb == nil {
		t.Fatal("no table")
	}
	for c, cell := range tb.Rows[0].Cells {
		if cell.Shading != "FFFF00" {
			t.Errorf("cell %d shading %q", c, cell.Shading)
		}
	}
	d.ed.SelectTable()
	h.frame()
	h.pickColor(&d.tb.shadeCP, -1)
	if tb.Rows[0].Cells[0].Shading != "" {
		t.Errorf("No Color left %q", tb.Rows[0].Cells[0].Shading)
	}
	if got := d.ed.UndoLabels(); len(got) < 2 || got[0] != "Shading" || got[1] != "Shading" {
		t.Errorf("undo labels %q", got)
	}
}
