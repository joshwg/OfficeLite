// Keeping the keyboard on the app in the browser. Gio's WebAssembly
// backend reads keys from a hidden <textarea> that has the page's focus
// only while text input is open; Gio opens it when a text editor asks for
// the soft keyboard and closes it at the next focus change. So once the
// cell editor (or any editor) let go of the focus, the textarea lost it
// and no key reached the window: arrows, Ctrl+Z, typing into a cell all
// went nowhere. Asking for text input at the end of every frame keeps the
// textarea focused (focusing a focused element does nothing).
//
// Phone keyboards also compose each word in that textarea, and two things
// in Gio's handling of it go wrong (fixed here, see fixTextInput): at the
// end of a word it reads the whole textarea as the word, so anything else
// in it was typed again; and the apps give the keyboard no text around the
// caret (word/ui/ime.go), so with auto-capitals on every word looked like
// the start of a sentence and was capitalized.

//go:build js

package gui

import (
	"sync"
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
	fixTextInput()
}

// endWordOnce installs endWord once.
var endWordOnce sync.Once

// fixTextInput turns auto-capitals off in Gio's focused <textarea> (Gio
// turns them on for text) and makes sure the end-of-word fix is in place.
func fixTextInput() {
	endWordOnce.Do(func() {
		endWord := js.FuncOf(func(_ js.Value, args []js.Value) any {
			e := args[0]
			if t := e.Get("target"); t.Get("tagName").String() == "TEXTAREA" {
				// Runs before Gio's own listener on the textarea,
				// which takes the textarea's value as the word.
				t.Set("value", e.Get("data"))
			}
			return nil
		})
		opts := js.Global().Get("Object").New()
		opts.Set("capture", true)
		js.Global().Call("addEventListener", "compositionend", endWord, opts)
	})
	t := js.Global().Get("document").Get("activeElement")
	if t.Truthy() && t.Get("tagName").String() == "TEXTAREA" && t.Call("getAttribute", "autocapitalize").String() != "off" {
		t.Call("setAttribute", "autocapitalize", "off")
	}
}
