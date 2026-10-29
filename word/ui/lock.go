package ui

import (
	"path/filepath"

	"gioui.org/io/system"
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

// raise brings the window to the front, when its file is opened again.
func (d *window) raise() {
	d.calls.Post(func() {
		if d.win != nil {
			d.win.Perform(system.ActionRaise)
		}
	})
}

// docName is the document's name for the title, marked when read-only.
func (d *window) docName() string {
	if d.lk.readOnly {
		return d.doc.Name + " (Read-Only)"
	}
	if d.ar.recovered != nil {
		return d.doc.Name + " [Recovered]"
	}
	return d.doc.Name
}

// askLocked asks what to do with a loaded document whose file is open
// elsewhere: open it read-only, open it anyway or not at all.
func (d *window) askLocked(r loadResult) {
	st := r.lock
	d.menu.Close()
	d.back.Hide()
	d.lk.dlg.Show(st.Title(), st.Message(filepath.Base(r.path)), st.Buttons()...)
	d.lk.answer = func(i int) {
		switch st.Choice(i) {
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
	}
}

func (d *window) lockAnswer(i int) {
	if f := d.lk.answer; f != nil {
		d.lk.answer = nil
		f(i)
	}
}

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
