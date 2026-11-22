// The desktop opens every file in a window of its own (see opentab_js.go
// for the browser).

//go:build !js

package gui

import (
	"io"

	"officelite/common/doctype"
)

// OpenInTab reports false on the desktop, leaving rc alone.
func OpenInTab(io.ReadCloser, doctype.Kind, *CallQueue, func(error)) bool { return false }
