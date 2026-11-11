// The desktop side of page.go: there is no address bar to keep in step.

//go:build !js

package main

import "officelite/common/doctype"

// showPage does nothing on the desktop.
func showPage(doctype.Kind) {}
