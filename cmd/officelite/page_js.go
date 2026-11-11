// The browser side of page.go: showPage changes the address bar to the
// app on show (/word or /excel) without reloading the page.

//go:build js

package main

import (
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
