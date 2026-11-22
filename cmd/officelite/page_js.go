// The browser side of page.go: showPage changes the address bar to the
// app on show (/word or /excel) without reloading the page, and openTab
// opens another file in a new browser tab, as web office apps do, rather
// than in a second app window stacked out of reach inside this page.

//go:build js

package main

import (
	"strings"
	"syscall/js"

	"officelite/common/doctype"
)

// showPage replaces the page's address with kind's app page. It uses
// replaceState, not pushState: going Back to the other app's address would
// reload the client and lose the open file.
func showPage(kind doctype.Kind) {
	path := "/excel"
	if kind == doctype.Document {
		path = "/word"
	}
	g := js.Global()
	if g.Get("location").Get("pathname").String() == path {
		return
	}
	g.Get("history").Call("replaceState", js.Null(), "", path)
}

// serverDir is where the user's server documents appear to the client
// (web/server/static/serverfs.js).
const serverDir = "/server/"

// localFile returns the file this tab was started with by another one,
// from the user's computer (app.js puts it in officewebLocalFile, from
// officeweb.openDataTab), or nil.
func localFile() []byte {
	g := js.Global()
	v := g.Get("officewebLocalFile")
	if !v.Truthy() {
		return nil
	}
	g.Set("officewebLocalFile", js.Undefined())
	data := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(data, v)
	return data
}

// openTab opens kind's app in a new browser tab: with the server document
// path, or a blank one for "". It reports false for a file that exists
// only in this page (one opened from the user's computer), which another
// tab can't reach; that one gets a window here as on the desktop. When
// the browser blocks the tab, the page offers a link instead
// (officeweb.openTab).
func openTab(kind doctype.Kind, path string) bool {
	g := js.Global()
	ow := g.Get("officeweb")
	if !ow.Truthy() || !ow.Get("openTab").Truthy() {
		return false
	}
	app, label := "/excel", "a new workbook"
	if kind == doctype.Document {
		app, label = "/word", "a new document"
	}
	url := app
	if path != "" {
		name, ok := strings.CutPrefix(path, serverDir)
		if !ok || name == "" || strings.Contains(name, "/") {
			return false
		}
		url += "?open=" + g.Call("encodeURIComponent", name).String()
		label = name
	}
	ow.Call("openTab", url, label)
	return true
}
