// AutoSave for the document window: the title bar's switch, saving the
// document into its .docx in the background a few seconds after each edit
// (the shared state and rules are gui.AutoSave's).

package ui

import (
	"os"

	"gioui.org/layout"

	"officelite/common/autosave"
	"officelite/common/gui"
	"officelite/word/fileio"
)

// startAutoSave makes the window's AutoSave saver (off).
func (d *window) startAutoSave() {
	d.as.Saver = autosave.New(d.calls.Post, d.autoSaveSnapshot, d.autoSaved)
}

// autoSaveBlocked is why AutoSave can't be on for the document (the
// switch's tooltip); "" when it can.
func (d *window) autoSaveBlocked() string {
	return gui.AutoSaveBlocked(gui.AutoSaveFile{
		Noun:      "document",
		Ext:       ".docx",
		Path:      d.path,
		Loading:   d.loading.Load() && d.path != "",
		Recovered: d.ar.recovered != nil,
		ReadOnly:  d.lk.readOnly,
		Elsewhere: d.as.Elsewhere,
		Macros:    d.doc != nil && d.doc.HasMacros,
	})
}

// setAutoSave turns AutoSave on (saving at once) or off for the document.
func (d *window) setAutoSave(on bool) {
	if on && d.autoSaveBlocked() != "" {
		return
	}
	d.errMsg = ""
	d.as.Saver.SetOn(on)
}

// autoSaveLoaded sets AutoSave for a document just opened or saved as:
// off, or on when that is the user's default and it works with the file.
func (d *window) autoSaveLoaded() { d.as.Loaded(d.autoSaveBlocked()) }

// autoSaveFrame ends every frame: an edit since the last one starts
// the countdown to a save, and a document that can no longer be
// AutoSaved (read-only, saved as .doc) turns it off.
func (d *window) autoSaveFrame() {
	d.as.Frame(d.autoSaveBlocked, d.recoveryRev(), d.modified())
}

// autoSaveSnapshot is the save of the document as it is now, cloned on
// the UI goroutine; nil when there is nothing to save.
func (d *window) autoSaveSnapshot() func() error {
	if !d.modified() || d.autoSaveBlocked() != "" {
		return nil
	}
	doc, path := d.doc.Clone(), d.path
	d.as.Rev, d.as.Path = d.bodyEd().Version, path
	return func() error { return fileio.SaveFile(doc, path, fileio.DOCX) }
}

// autoSaved runs on the UI goroutine when an AutoSave ends. An error has
// turned AutoSave off; the edits are still in the window (and in the
// AutoRecover copy).
func (d *window) autoSaved(err error) {
	msg, ok := d.as.Ended(err, d.path)
	if msg != "" {
		d.errMsg = msg
	}
	if !ok {
		return
	}
	d.fileInfo, _ = os.Stat(d.path)
	d.saved = max(d.saved, d.as.Rev) // a Ctrl+S may have saved a later one
	if !d.modified() {
		d.dropRecovery() // recover.go
	}
	d.updateTitle()
}

// autoSaveStatus is what the title says after the name: "Saving…" while
// AutoSave writes, "Saved" for a moment after.
func (d *window) autoSaveStatus(gtx layout.Context) string { return d.as.Status(gtx) }

// autoSaveSwitch is the title bar's AutoSave switch.
func (d *window) autoSaveSwitch(gtx layout.Context) layout.Dimensions {
	clicked, dims := d.as.LayoutSwitch(gtx, d.th, d.autoSaveBlocked(), accent)
	if clicked {
		d.setAutoSave(!d.as.Saver.On())
	}
	return dims
}
