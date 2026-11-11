// lock.go ties the window to file locking: read-only mode, the locked-file prompt and raising the window.

package ui

import (
	"gioui.org/io/system"

	"officelite/common/gui"
	"officelite/common/lockfile"
)

// lockState is the window's side of file locking (common/lockfile): the
// open file's registration and owner file, and read-only mode, in which
// Save goes to Save As.
type lockState struct {
	doc      *lockfile.Doc
	readOnly bool
	// closeOnCancel: the window was opened for a file, so Cancel in the
	// lock dialog closes it.
	closeOnCancel bool
}

// docName is the workbook's name for the title, marked when read-only or
// recovered.
func (a *App) docName() string {
	return gui.DocTitle(a.wb.Name, a.lk.readOnly, a.ar.recovered != nil)
}

// askLocked asks what to do with a loaded workbook whose file is open
// elsewhere: open it read-only, open it anyway or not at all.
func (a *App) askLocked(r loadResult) {
	a.menu.Close()
	a.back.Hide()
	a.answer = gui.AskLocked(&a.dialog, r.lock, r.path, func(c lockfile.Choice) {
		switch c {
		case lockfile.ReadOnly:
			a.applyLoad(r, true)
		case lockfile.Anyway:
			a.applyLoad(r, false)
			a.lk.doc.Set(r.path, true)
		default:
			a.lk.doc.Set(a.path, !a.lk.readOnly)
			if a.lk.closeOnCancel && a.win != nil {
				a.win.Perform(system.ActionClose)
			}
		}
	})
}

// askSaveLocked asks whether to save over the file at path, which Save As
// chose and lockfile.Check found st about (open elsewhere), and gives
// answer the reply (gui.SaveAs.Ask).
func (a *App) askSaveLocked(st lockfile.Status, path string, answer func(ok bool)) {
	a.menu.Close()
	a.answer = gui.AskSaveLocked(&a.dialog, st, path, answer)
}
