// The desktop opens every file in a window of its own (see opentab_js.go
// for the browser).

//go:build !js

package gui

import "officelite/common/doctype"

// OpenDataInTab reports false: the desktop has no tabs.
func OpenDataInTab([]byte, doctype.Kind) bool { return false }
