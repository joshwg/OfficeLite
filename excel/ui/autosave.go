// AutoSave for the spreadsheet window: the title bar's switch, saving the
// workbook into its .xlsx in the background a few seconds after each edit
// (the shared state and rules are gui.AutoSave's).

package ui

import (
	"os"

	"gioui.org/layout"

	"officelite/common/autosave"
	"officelite/common/gui"
	"officelite/excel/fileio"
)

// startAutoSave makes the window's AutoSave saver (off).
func (a *App) startAutoSave() {
	a.as.Saver = autosave.New(a.calls.Post, a.autoSaveSnapshot, a.autoSaved)
}

// autoSaveBlocked is why AutoSave can't be on for the workbook (the
// switch's tooltip); "" when it can.
func (a *App) autoSaveBlocked() string {
	return gui.AutoSaveBlocked(gui.AutoSaveFile{
		Noun:      "workbook",
		Ext:       ".xlsx",
		Path:      a.path,
		Loading:   a.loading.Load(),
		Recovered: a.ar.recovered != nil,
		ReadOnly:  a.lk.readOnly,
		Elsewhere: a.as.Elsewhere,
		Macros:    a.wb != nil && a.wb.HasMacros,
	})
}

// setAutoSave turns AutoSave on (saving at once) or off for the workbook.
func (a *App) setAutoSave(on bool) {
	if on && a.autoSaveBlocked() != "" {
		return
	}
	if on && a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	a.errMsg = ""
	a.as.Saver.SetOn(on)
}

// autoSaveLoaded sets AutoSave for a workbook just opened or saved as:
// off, or on when that is the user's default and it works with the file.
func (a *App) autoSaveLoaded() { a.as.Loaded(a.autoSaveBlocked()) }

// autoSaveFrame ends every frame: an edit since the last one starts
// the countdown to a save, and a workbook that can no longer be
// AutoSaved (read-only, saved as .xls) turns it off.
func (a *App) autoSaveFrame() {
	a.as.Frame(a.autoSaveBlocked, a.ar.rev, a.modified)
}

// autoSaveSnapshot is the save of the workbook as it is now, cloned on
// the UI goroutine; nil when there is nothing to save.
func (a *App) autoSaveSnapshot() func() error {
	if !a.modified || a.autoSaveBlocked() != "" {
		return nil
	}
	wb, path, sheet := a.wb.Clone(), a.path, a.active
	a.as.Rev, a.as.Path = a.ar.rev, path
	return func() error { return fileio.SaveFile(wb, path, fileio.XLSX, sheet) }
}

// autoSaved runs on the UI goroutine when an AutoSave ends. An error has
// turned AutoSave off; the edits are still in the window (and in the
// AutoRecover copy).
func (a *App) autoSaved(err error) {
	msg, ok := a.as.Ended(err, a.path)
	if msg != "" {
		a.errMsg = msg
	}
	if !ok {
		return
	}
	a.fileInfo, _ = os.Stat(a.path)
	if a.ar.rev == a.as.Rev && a.modified {
		a.modified = false
		a.dropRecovery() // recover.go
		if a.win != nil {
			a.win.Option(windowTitle(a.docName()))
		}
	}
}

// askBeforeClose reports whether closing the window asks about unsaved
// changes: not after Don't Save, nor when everything is saved (as it is
// after an AutoSave, unless the workbook changed since).
func (a *App) askBeforeClose() bool { return a.modified && !a.mayClose }

// autoSaveStatus is what the title says after the name: "Saving…" while
// AutoSave writes, "Saved" for a moment after.
func (a *App) autoSaveStatus(gtx layout.Context) string { return a.as.Status(gtx) }

// autoSaveSwitch is the title bar's AutoSave switch.
func (a *App) autoSaveSwitch(gtx layout.Context) layout.Dimensions {
	clicked, d := a.as.LayoutSwitch(gtx, a.ui, a.autoSaveBlocked(), accent)
	if clicked {
		a.setAutoSave(!a.as.Saver.On())
	}
	return d
}
