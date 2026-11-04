// Tests for the backstage's recent-file time formatting and New tiles.

package gui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/recent"
)

// TestWhen checks When's today, yesterday, this-week, this-year, older
// and zero-time forms.
func TestWhen(t *testing.T) {
	now := time.Date(2026, 10, 16, 14, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 10, 16, 9, 5, 0, 0, time.Local), "Today 9:05 AM"},
		{time.Date(2026, 10, 15, 23, 59, 0, 0, time.Local), "Yesterday 11:59 PM"},
		{time.Date(2026, 10, 12, 8, 0, 0, 0, time.Local), "Mon 8:00 AM"},
		{time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local), "Mar 2"},
		{time.Date(2024, 3, 2, 8, 0, 0, 0, time.Local), "Mar 2, 2024"},
		{time.Time{}, ""},
	} {
		if got := When(c.t, now); got != c.want {
			t.Errorf("When(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

// tileRowHeight lays out n New tiles across width px (1px = 1dp) with at
// most maxRows rows and returns the height they take.
func tileRowHeight(th *Theme, n, width, maxRows int) int {
	ts := make([]Template, n)
	for i := range ts {
		ts[i] = Template{Name: "Template", Kind: recent.Document, Path: "t.dotx"}
	}
	b := &Backstage{tiles: make([]widget.Clickable, n)}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(width, 10000)}}
	return b.tileRow(th, ts, maxRows)(gtx).Size.Y
}

// TestTileRowWraps checks that New tiles wrap onto more rows when they
// don't fit across, and that maxRows caps them (the Home page's one row).
func TestTileRowWraps(t *testing.T) {
	th := NewTheme(NewFontLibrary(), ChromeText, ChromeText)
	perRow := (1000 + tileGap) / (tileWidth + tileGap) // 5 tiles
	one := tileRowHeight(th, perRow, 1000, 0)
	if one <= tileHeight {
		t.Fatalf("one row is %dpx high", one)
	}
	if got := tileRowHeight(th, perRow+1, 1000, 0); got < 2*one-tileHeight/2 {
		t.Errorf("%d tiles: %dpx high, want two rows (one is %dpx)", perRow+1, got, one)
	}
	if got := tileRowHeight(th, 3*perRow, 1000, 1); got != one {
		t.Errorf("capped at one row: %dpx high, want %dpx", got, one)
	}
	if got := tileRowHeight(th, 3, 50, 0); got < 3*one-tileHeight/2 {
		t.Errorf("narrower than a tile: %dpx high, want a row per tile", got)
	}
	if got := tileRowHeight(th, 0, 1000, 0); got != 0 {
		t.Errorf("no tiles: %dpx high", got)
	}
}
