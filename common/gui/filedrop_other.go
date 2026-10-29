// The FileDrop stub for platforms other than Windows.

//go:build !windows

package gui

import "gioui.org/io/event"

// listen does nothing: files dragged onto a window aren't taken outside
// Windows.
func (f *FileDrop) listen(event.Event) {}
