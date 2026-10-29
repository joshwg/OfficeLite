// lock.go ties the window to file locking: read-only mode, the locked-file prompt and raising the window.

package ui

import (
	"path/filepath"

	"gioui.org/io/system"

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

// raise brings the window to the front, when its file is opened again.
func (a *App) raise() {
	a.calls.Post(func() {
		if a.win != nil {
			a.win.Perform(system.ActionRaise)
		}
	})
}

// docName is the workbook's name for the title, marked when read-only or
// recovered.
func (a *App) docName() string {
	if a.lk.readOnly {
		return a.wb.Name + " (Read-Only)"
	}
	if a.ar.recovered != nil {
		return a.wb.Name + " [Recovered]"
	}
	return a.wb.Name
}

// askLocked asks what to do with a loaded workbook whose file is open
// elsewhere: open it read-only, open it anyway or not at all.
func (a *App) askLocked(r loadResult) {
	st := r.lock
	a.menu.Close()
	a.back.Hide()
	a.dialog.Show(st.Title(), st.Message(filepath.Base(r.path)), st.Buttons()...)
	a.answer = func(i int) {
		switch st.Choice(i) {
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
	}
}
