// Keeping the browser's address in step with the app on show: in the
// WebAssembly build a spreadsheet opened from WordLite (or a document
// from ExcelLite) moves the page from /word to /excel (or back), so the
// address names the app the user sees and reloading it starts that app.
// On the desktop showPage does nothing.

package main

import (
	"slices"
	"sync"

	"officelite/common/doctype"
)

// pageTracker remembers the kinds of the open windows, newest last; the
// newest is the one on show.
type pageTracker struct {
	mu    sync.Mutex
	kinds []doctype.Kind
}

// pages tracks this process's windows.
var pages pageTracker

// opened records a new window of kind, which is now on show.
func (p *pageTracker) opened(kind doctype.Kind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.kinds = append(p.kinds, kind)
	showPage(kind)
}

// closed records that a window of kind closed (the newest of that kind:
// only kinds are kept, so which one doesn't matter); the newest window
// left is on show again.
func (p *pageTracker) closed(kind doctype.Kind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.kinds) - 1; i >= 0; i-- {
		if p.kinds[i] == kind {
			p.kinds = slices.Delete(p.kinds, i, i+1)
			break
		}
	}
	if n := len(p.kinds); n > 0 {
		showPage(p.kinds[n-1])
	}
}
