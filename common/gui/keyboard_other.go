// The desktop side of keyboard_js.go: a native window gets its keys
// without any text input being open.

//go:build !js

package gui

import "gioui.org/layout"

// KeepKeyboard does nothing on the desktop (see keyboard_js.go).
func KeepKeyboard(layout.Context) {}
