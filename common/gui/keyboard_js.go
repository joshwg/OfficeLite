// Keeping the keyboard on the app in the browser. Gio's WebAssembly
// backend reads keys from a hidden <textarea> that has the page's focus
// only while text input is open; Gio opens it when a text editor asks for
// the soft keyboard and closes it at the next focus change. So once the
// cell editor (or any editor) let go of the focus, the textarea lost it
// and no key reached the window: arrows, Ctrl+Z, typing into a cell all
// went nowhere. Asking for text input at the end of every frame keeps the
// textarea focused (focusing a focused element does nothing).

//go:build js

package gui

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

// KeepKeyboard keeps the window's keys coming in the browser; call it
// after laying out a frame, before sending it. On the desktop it does
// nothing.
func KeepKeyboard(gtx layout.Context) {
	gtx.Execute(key.SoftKeyboardCmd{Show: true})
}
