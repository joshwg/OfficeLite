//go:build !windows

package gui

import "gioui.org/io/event"

// Files dragged onto a window aren't taken outside Windows.
func (f *FileDrop) listen(event.Event) {}
