package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"officelite/common/autosave"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/excel/fileio"
)

// savedShown is how long the title says "Saved" after an AutoSave.
const savedShown = 4 * time.Second

// autoSave is the window's AutoSave state: when the title bar's switch is
// on, the workbook is saved into its file a few seconds after each edit
// (common/autosave), in the background from a clone.
type autoSave struct {
	saver *autosave.Saver
	sw    gui.AutoSaveSwitch
	seen  int    // AutoRecover's rev last seen, to notice edits
	rev   int    // the rev being saved
	path  string // the file being saved
	// savedAt is when the last AutoSave ended well: the title says
	// "Saved" for a moment.
	savedAt time.Time
	// elsewhere: the file was opened with Open Anyway, so it is someone
	// else's to save.
	elsewhere bool
}

// startAutoSave makes the window's AutoSave saver (off).
func (a *App) startAutoSave() {
	a.as.saver = autosave.New(a.calls.Post, a.autoSaveSnapshot, a.autoSaved)
}

// autoSaveBlocked is why AutoSave can't be on for the workbook (the
// switch's tooltip); "" when it can.
func (a *App) autoSaveBlocked() string {
	switch {
	case a.loading.Load():
		return "Opening the workbook…"
	case a.ar.recovered != nil:
		return "This is a recovered workbook. Save it as .xlsx to turn AutoSave on."
	case a.lk.readOnly:
		return "This workbook is read-only. Save a copy as .xlsx to turn AutoSave on."
	case a.as.elsewhere:
		return "Someone else has this file open, so AutoSave is off: it would overwrite their changes."
	case a.path == "" || strings.ToLower(filepath.Ext(a.path)) != ".xlsx":
		return gui.AutoSaveWorksWith(".xlsx")
	}
	return ""
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
	a.as.saver.SetOn(on)
}

// autoSaveLoaded sets AutoSave for a workbook just opened or saved as:
// off, or on when that is the user's default and it works with the file.
func (a *App) autoSaveLoaded() {
	if a.autoSaveBlocked() != "" {
		a.as.saver.SetOn(false)
	} else if autosave.Default() {
		a.as.saver.SetOn(true)
	}
}

// autoSaveFrame ends every frame: an edit since the last one starts
// the countdown to a save, and a workbook that can no longer be
// AutoSaved (read-only, saved as .xls) turns it off.
func (a *App) autoSaveFrame() {
	s := a.as.saver
	if s.On() && a.autoSaveBlocked() != "" {
		s.SetOn(false)
	}
	if a.ar.rev != a.as.seen {
		a.as.seen = a.ar.rev
		if a.modified {
			s.Edited()
		}
	}
}

// autoSaveSnapshot is the save of the workbook as it is now, cloned on
// the UI goroutine; nil when there is nothing to save.
func (a *App) autoSaveSnapshot() func() error {
	if !a.modified || a.autoSaveBlocked() != "" {
		return nil
	}
	wb, path, sheet := a.wb.Clone(), a.path, a.active
	a.as.rev, a.as.path = a.ar.rev, path
	return func() error { return fileio.SaveFile(wb, path, fileio.XLSX, sheet) }
}

// autoSaved runs on the UI goroutine when an AutoSave ends. An error has
// turned AutoSave off; the edits are still in the window (and in the
// AutoRecover copy).
func (a *App) autoSaved(err error) {
	if err != nil {
		a.errMsg = "AutoSave turned off: " + lockfile.SaveError(a.as.path, err)
		a.as.savedAt = time.Time{}
		return
	}
	if a.as.path != a.path {
		return // saved as another file meanwhile
	}
	a.fileInfo, _ = os.Stat(a.path)
	a.as.savedAt = time.Now()
	if a.ar.rev == a.as.rev && a.modified {
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
func (a *App) autoSaveStatus(gtx layout.Context) string {
	s := a.as.saver
	switch {
	case s.Busy():
		return "Saving…"
	case !s.On() || a.as.savedAt.IsZero():
		return ""
	}
	now, end := gtx.Now, a.as.savedAt.Add(savedShown)
	if now.IsZero() {
		now = time.Now() // a frame without a clock (excelshot)
	}
	if !now.Before(end) {
		return ""
	}
	gtx.Execute(op.InvalidateCmd{At: end})
	return "Saved"
}

// autoSaveSwitch is the title bar's AutoSave switch.
func (a *App) autoSaveSwitch(gtx layout.Context) layout.Dimensions {
	clicked, d := a.as.sw.Layout(gtx, a.ui, a.as.saver.On(), a.autoSaveBlocked(), accent)
	if clicked {
		a.setAutoSave(!a.as.saver.On())
	}
	return d
}
