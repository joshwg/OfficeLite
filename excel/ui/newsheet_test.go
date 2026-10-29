// newsheet_test.go tests the + button that adds a sheet after the active one.

package ui

import (
	"image"
	"testing"
)

// TestNewSheetButton: the + after the sheet tabs adds a sheet after the active one and
// switches to it, as Excel's does.
func TestNewSheetButton(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.frame()
	bottomBar := image.Rect(0, 640, 600, 680)
	before := len(a.wb.Sheets)
	p := h.locate(&a.newSheetBtn, bottomBar, 4)
	h.click(p.X, p.Y)
	if len(a.wb.Sheets) != before+1 || a.active != 1 {
		t.Fatalf("after +: %d sheets, active %d; want %d, 1", len(a.wb.Sheets), a.active, before+1)
	}
	// From the first sheet the new one goes second, before the other.
	a.active = 0
	h.frame()
	p = h.locate(&a.newSheetBtn, bottomBar, 4)
	h.click(p.X, p.Y)
	if len(a.wb.Sheets) != before+2 || a.active != 1 {
		t.Fatalf("after + from the first sheet: %d sheets, active %d", len(a.wb.Sheets), a.active)
	}
}
