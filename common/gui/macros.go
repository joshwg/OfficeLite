// macros.go words the notices about macros: OfficeLite never runs them,
// and a save leaves them out.

package gui

// MacroNotice is the title and message shown when a file with macros
// opens. noun is "workbook", "document" or "template".
func MacroNotice(noun string) (title, message string) {
	message = "This " + noun + " contains macros. OfficeLite never runs macros, so buttons, " +
		"forms and anything else that depends on them won't work."
	if noun == "template" {
		message += "\n\nThe template keeps its macros; documents made from it don't have them."
	} else {
		message += "\n\nIf you save the " + noun + ", all of its macros will be lost."
	}
	return "Macros won't run", message
}

// MacroSaveWarning is the title and message asking whether to save a
// noun ("workbook" or "document") that contains macros, which the save
// loses. overwrite says the save replaces the file the macros came from.
func MacroSaveWarning(noun string, overwrite bool) (title, message string) {
	message = "This " + noun + " contains macros, and OfficeLite can't save them: " +
		"all macros will be lost from the saved file."
	if overwrite {
		message += "\n\nTo keep them, cancel and use Save As to save a copy under a new name."
	}
	return "Save without macros?", message
}
