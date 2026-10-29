package gui

import (
	"image"
	"sync"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"officelite/common/colors"
)

// TestIconConcurrent draws the shared icons from several goroutines (as
// windows do) at two sizes and colors; run with -race.
func TestIconConcurrent(t *testing.T) {
	fresh := mustIcon(icons.EditorFormatBold)
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ops op.Ops
			for i := range 50 {
				ops.Reset()
				for _, sz := range []int{18, 26} {
					gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
						Constraints: layout.Exact(image.Pt(sz, sz))}
					c := ChromeText
					if (g+i)%2 == 0 {
						c = colors.White
					}
					for _, ic := range []*Icon{fresh, IconBold, IconDropArrow} {
						if d := ic.Layout(gtx, c); d.Size != image.Pt(sz, sz) {
							t.Errorf("size %v, want %d", d.Size, sz)
							return
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	// Two sizes and two colors: four renderings each, kept.
	fresh.mu.Lock()
	n := len(fresh.images)
	fresh.mu.Unlock()
	if n != 4 {
		t.Errorf("the icon holds %d images, want 4", n)
	}
}
