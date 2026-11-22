// Opening a file from the user's computer in a new browser tab. The
// browser's file picker gives the file's contents but no path, and a
// window that already holds a file can't take another; a new tab can't
// be told a path either, so the contents go to it from this one
// (officeweb.openDataTab in web/server/static/serverfs.js and the
// ?local= start in static/app.js).

//go:build js

package gui

import (
	"io"
	"syscall/js"

	"officelite/common/doctype"
)

// OpenInTab reads rc (a file the Open dialog gave without a path, which
// it closes) and opens it in a new tab of the app its contents call for
// (own when they don't say). It reports false, leaving rc alone, where
// there are no tabs: the desktop, or a page without the web server's
// scripts. A read failure goes to fail on calls' UI goroutine.
func OpenInTab(rc io.ReadCloser, own doctype.Kind, calls *CallQueue, fail func(error)) bool {
	ow := js.Global().Get("officeweb")
	if !ow.Truthy() || !ow.Get("openDataTab").Truthy() {
		return false
	}
	go func() {
		data, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			calls.Post(func() { fail(err) })
			return
		}
		kind := doctype.Detect("", data)
		if kind != doctype.Document && kind != doctype.Spreadsheet {
			kind = own
		}
		app := "excel"
		if kind == doctype.Document {
			app = "word"
		}
		buf := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(buf, data)
		ow.Call("openDataTab", app, buf)
	}()
	return true
}
