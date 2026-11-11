// macros.go records whether a document came from a file with VBA macros,
// which WordLite never runs and never writes.

package model

import "slices"

// MacroWarning is the load warning of a document with macros.
const MacroWarning = "This document contains macros. WordLite never runs them, and they are lost when the document is saved."

// MarkMacros records that the file being read has macros, with the load
// warning.
func (d *Document) MarkMacros() {
	if !d.HasMacros {
		d.HasMacros = true
		d.Warnings = append(d.Warnings, MacroWarning)
	}
}

// DropMacros records that d's file no longer has macros: it was saved
// without them, or d was made from a template that keeps them.
func (d *Document) DropMacros() {
	d.HasMacros = false
	d.Warnings = slices.DeleteFunc(d.Warnings, func(w string) bool { return w == MacroWarning })
}
