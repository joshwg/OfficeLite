package ui

import "fmt"

// scrollHints sets the scrollbar's arrow step (about a line) and the hint
// Word shows beside a dragged thumb: the page that would come into view.
func (d *window) scrollHints() {
	b := &d.v.vbar
	b.Vertical, b.Step, b.Theme = true, 18, d.th
	b.Label = func(pos float64) string {
		n, _ := d.ceng().PageAt(pos)
		return fmt.Sprintf("Page: %d of %d", n+1, max(len(d.ceng().Pages), 1))
	}
}
