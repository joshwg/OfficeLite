// macros.go tells the user about a workbook's macros: on opening, that
// they won't run, and before a save, that the save loses them.

package ui

import "officelite/common/gui"

// macroNotice tells, once the workbook has opened, that its macros won't
// run. Another message already showing wins; the warning stays on the
// Info page and in the status bar.
func (a *App) macroNotice() {
	if !a.wb.HasMacros || a.dialog.Visible {
		return
	}
	title, msg := gui.MacroNotice("workbook")
	a.dialog.Show(title, msg, "OK")
	a.answer = nil
}

// confirmMacros runs save, first asking whether to go on when the
// workbook has macros, which the save loses. overwrite says the save
// replaces the file they came from (Save, not Save As).
func (a *App) confirmMacros(overwrite bool, save func()) {
	if !a.wb.HasMacros {
		save()
		return
	}
	title, msg := gui.MacroSaveWarning("workbook", overwrite)
	a.dialog.Show(title, msg, "Save", "Cancel")
	a.answer = func(i int) {
		if i != 0 {
			a.closing = false
			return
		}
		save()
	}
}
