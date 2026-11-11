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
	"syscall/js"

	"gioui.org/io/key"
	"gioui.org/layout"
)

// modalFlag is the page global the web server's file dialog
// (web/server/static/serverfs.js) sets while it is open over the app.
const modalFlag = "officewebModalOpen"

// KeepKeyboard keeps the window's keys coming in the browser; call it
// after laying out a frame, before sending it. It holds off while the
// page shows its own dialog, whose text box would otherwise lose the
// focus at every frame. On the desktop it does nothing.
func KeepKeyboard(gtx layout.Context) {
	if js.Global().Get(modalFlag).Truthy() {
		return
	}
	gtx.Execute(key.SoftKeyboardCmd{Show: true})
}
