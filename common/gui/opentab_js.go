// Opening a file from the user's computer in a new browser tab. The
// browser's file picker gives the file's contents but no path, and a
// window that already holds a file can't take another; a new tab can't
// be told a path either, so the contents go to it from this one
// (officeweb.openDataTab in web/server/static/serverfs.js and the
// ?local= start in static/app.js).

//go:build js

package gui

import (
	"syscall/js"

	"officelite/common/doctype"
)

// OpenDataInTab opens data, a file the Open dialog gave without a path,
// in a new tab of kind's app (ExcelLite for anything but a document). It
// reports false where there are no tabs: a page without the web server's
// scripts.
func OpenDataInTab(data []byte, kind doctype.Kind) bool {
	ow := js.Global().Get("officeweb")
	if !ow.Truthy() || !ow.Get("openDataTab").Truthy() {
		return false
	}
	app := "excel"
	if kind == doctype.Document {
		app = "word"
	}
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, data)
	ow.Call("openDataTab", app, buf)
	return true
}
