// macros.go tells the user about a document's macros: on opening, that
// they won't run, and before a save, that the save loses them. Both use
// the lossy dialog (dialog.go).

package ui

import "officelite/common/gui"

// macroNotice tells, once the document has opened, that its macros won't
// run. Another prompt already showing wins; the warning stays on the
// Info page and in the status bar. A document made from a template has
// no macros of its own (the template keeps them), so its saves don't
// warn.
func (d *window) macroNotice(template bool) {
	if !d.doc.HasMacros {
		return
	}
	noun := "document"
	if template {
		noun = "template"
	}
	if !d.lossy.Visible {
		title, msg := gui.MacroNotice(noun)
		d.lossyGo = nil
		d.lossy.Show(title, msg, "OK")
	}
	if template {
		d.doc.DropMacros()
	}
}

// checkMacros runs save, first asking whether to go on when the document
// has macros, which the save loses. overwrite says the save replaces the
// file they came from (Save, not Save As).
func (d *window) checkMacros(overwrite bool, save func()) {
	if !d.doc.HasMacros {
		save()
		return
	}
	title, msg := gui.MacroSaveWarning("document", overwrite)
	d.lossyGo = save
	d.lossy.Show(title, msg, "Save", "Cancel")
}
