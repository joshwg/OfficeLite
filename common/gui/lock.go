// A window's side of file locking (common/lockfile) shared by the apps:
// raising the window when its file is opened again, the title's read-only
// and recovered marks, and the File In Use dialog.

package gui

import (
	"path/filepath"

	"gioui.org/app"
	"gioui.org/io/system"

	"officelite/common/lockfile"
)

// Raiser is the raise function lockfile.NewDoc takes for window win: it
// brings win to the front on the UI goroutine (through calls), from any
// goroutine. A nil win (a test's window) does nothing.
func Raiser(calls *CallQueue, win *app.Window) func() {
	return func() {
		calls.Post(func() {
			if win != nil {
				win.Perform(system.ActionRaise)
			}
		})
	}
}

// DocTitle is a document's name for the title bar: marked "(Read-Only)"
// when readOnly, else "[Recovered]" when it is a recovery copy not yet
// saved.
func DocTitle(name string, readOnly, recovered bool) string {
	switch {
	case readOnly:
		return name + " (Read-Only)"
	case recovered:
		return name + " [Recovered]"
	}
	return name
}

// AskLocked shows on dlg the File In Use dialog for the file at path,
// which lockfile.Check found st about, and returns the handler of the
// dialog's buttons (-1 for Esc): it gives choose the answer.
func AskLocked(dlg *Dialog, st lockfile.Status, path string, choose func(lockfile.Choice)) func(int) {
	dlg.Show(st.Title(), st.Message(filepath.Base(path)), st.Buttons()...)
	return func(i int) { choose(st.Choice(i)) }
}

// AskSaveLocked shows on dlg the File In Use dialog for saving over the
// file at path, which lockfile.Check found st about (Save As onto a file
// open elsewhere), and returns the handler of the dialog's buttons (-1
// for Esc): it calls answer with whether to save over it anyway. A file
// open in another OfficeLite window can't be taken over (only OK), as
// Open offers no Open Anyway for it.
func AskSaveLocked(dlg *Dialog, st lockfile.Status, path string, answer func(ok bool)) func(int) {
	name := filepath.Base(path)
	buttons := []string{"Save Anyway", "Cancel"}
	var msg string
	switch {
	case st.Elsewhere:
		msg = name + " is already open in another OfficeLite window.\n\nClose it there first, or save under another name."
		buttons = []string{"OK"}
	case st.Owner != "":
		msg = name + " is locked for editing by " + st.Owner + "."
	default:
		msg = name + " is in use by another program (Word or Excel?)."
	}
	if !st.Elsewhere {
		msg += "\n\nSave over it anyway? Saving may overwrite their changes, and theirs yours."
	}
	dlg.Show(st.Title(), msg, buttons...)
	return func(i int) { answer(i == 0 && !st.Elsewhere) }
}
