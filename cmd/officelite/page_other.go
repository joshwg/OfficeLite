// The desktop side of page.go: there is no address bar to keep in step.

//go:build !js

package main

import "officelite/common/doctype"

// showPage does nothing on the desktop.
func showPage(doctype.Kind) {}

// localFile is nil on the desktop: files come by path.
func localFile() []byte { return nil }

// openTab reports false on the desktop: every file gets a window.
func openTab(doctype.Kind, string) bool { return false }
