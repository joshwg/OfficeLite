// lock.go ties the window to file locking: read-only mode, the locked-file
// prompt and raising the window.

package ui

import (
	"gioui.org/layout"

	"officelite/common/gui"
	"officelite/common/lockfile"
)

// lockState is the window's side of file locking (common/lockfile): the
// open file's registration and owner file, read-only mode (in which Save
// goes to Save As) and the dialog asking about a locked file.
type lockState struct {
	doc      *lockfile.Doc
	readOnly bool
	// closeOnCancel: the window was opened for a file, so Cancel in the
	// lock dialog closes it.
	closeOnCancel bool
	dlg           gui.Dialog
	answer        func(int) // handles the dialog's button; -1 for Esc
}

// docName is the document's name for the title, marked when read-only or
// recovered.
func (d *window) docName() string {
	return gui.DocTitle(d.doc.Name, d.lk.readOnly, d.ar.recovered != nil)
}

// askLocked asks what to do with a loaded document whose file is open
// elsewhere: open it read-only, open it anyway or not at all.
func (d *window) askLocked(r loadResult) {
	d.menu.Close()
	d.back.Hide()
	d.lk.answer = gui.AskLocked(&d.lk.dlg, r.lock, r.path, func(c lockfile.Choice) {
		switch c {
		case lockfile.ReadOnly:
			d.applyLoad(r, true)
		case lockfile.Anyway:
			d.applyLoad(r, false)
			d.lk.doc.Set(r.path, true)
		default:
			d.lk.doc.Set(d.path, !d.lk.readOnly)
			if d.lk.closeOnCancel {
				d.allowClose = true
				d.close()
			}
		}
		d.v.focusPending = true
	})
}

// askSaveLocked asks whether to save over the file at path, which Save As
// chose and lockfile.Check found st about (open elsewhere), and gives
// answer the reply (gui.SaveAs.Ask).
func (d *window) askSaveLocked(st lockfile.Status, path string, answer func(ok bool)) {
	d.menu.Close()
	d.lk.answer = gui.AskSaveLocked(&d.lk.dlg, st, path, answer)
}

// lockAnswer handles button i of the lock dialog (-1 for Esc), once.
func (d *window) lockAnswer(i int) {
	if f := d.lk.answer; f != nil {
		d.lk.answer = nil
		f(i)
	}
}

// lockLayout draws the lock dialog, when it is up, and takes its clicks.
func (d *window) lockLayout(gtx layout.Context) {
	if !d.lk.dlg.Visible {
		return
	}
	if i := d.lk.dlg.Clicked(gtx); i >= 0 {
		d.lockAnswer(i)
		return
	}
	d.lk.dlg.Layout(gtx, d.th)
}
