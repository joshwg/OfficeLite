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
	"officelite/word/fileio"
)

// savedShown is how long the title says "Saved" after an AutoSave.
const savedShown = 4 * time.Second

// autoSave is the window's AutoSave state: when the title bar's switch is
// on, the document is saved into its file a few seconds after each edit
// (common/autosave), in the background from a clone.
type autoSave struct {
	saver   *autosave.Saver
	sw      gui.AutoSaveSwitch
	seen    int    // recoveryRev last seen, to notice edits
	version int    // the body editor's version being saved
	path    string // the file being saved
	// savedAt is when the last AutoSave ended well: the title says
	// "Saved" for a moment.
	savedAt time.Time
	// elsewhere: the file was opened with Open Anyway, so it is someone
	// else's to save.
	elsewhere bool
}

// startAutoSave makes the window's AutoSave saver (off).
func (d *window) startAutoSave() {
	d.as.saver = autosave.New(d.calls.Post, d.autoSaveSnapshot, d.autoSaved)
}

// autoSaveBlocked is why AutoSave can't be on for the document (the
// switch's tooltip); "" when it can.
func (d *window) autoSaveBlocked() string {
	switch {
	case d.loading.Load() && d.path != "":
		return "Opening the document…"
	case d.ar.recovered != nil:
		return "This is a recovered document. Save it as .docx to turn AutoSave on."
	case d.lk.readOnly:
		return "This document is read-only. Save a copy as .docx to turn AutoSave on."
	case d.as.elsewhere:
		return "Someone else has this file open, so AutoSave is off: it would overwrite their changes."
	case d.path == "" || strings.ToLower(filepath.Ext(d.path)) != ".docx":
		return gui.AutoSaveWorksWith(".docx")
	}
	return ""
}

// setAutoSave turns AutoSave on (saving at once) or off for the document.
func (d *window) setAutoSave(on bool) {
	if on && d.autoSaveBlocked() != "" {
		return
	}
	d.errMsg = ""
	d.as.saver.SetOn(on)
}

// autoSaveLoaded sets AutoSave for a document just opened or saved as:
// off, or on when that is the user's default and it works with the file.
func (d *window) autoSaveLoaded() {
	if d.autoSaveBlocked() != "" {
		d.as.saver.SetOn(false)
	} else if autosave.Default() {
		d.as.saver.SetOn(true)
	}
}

// autoSaveFrame ends every frame: an edit since the last one starts
// the countdown to a save, and a document that can no longer be
// AutoSaved (read-only, saved as .doc) turns it off.
func (d *window) autoSaveFrame() {
	s := d.as.saver
	if s.On() && d.autoSaveBlocked() != "" {
		s.SetOn(false)
	}
	if rev := d.recoveryRev(); rev != d.as.seen {
		d.as.seen = rev
		if d.modified() {
			s.Edited()
		}
	}
}

// autoSaveSnapshot is the save of the document as it is now, cloned on
// the UI goroutine; nil when there is nothing to save.
func (d *window) autoSaveSnapshot() func() error {
	if !d.modified() || d.autoSaveBlocked() != "" {
		return nil
	}
	doc, path := d.doc.Clone(), d.path
	d.as.version, d.as.path = d.bodyEd().Version, path
	return func() error { return fileio.SaveFile(doc, path, fileio.DOCX) }
}

// autoSaved runs on the UI goroutine when an AutoSave ends. An error has
// turned AutoSave off; the edits are still in the window (and in the
// AutoRecover copy).
func (d *window) autoSaved(err error) {
	if err != nil {
		d.errMsg = "AutoSave turned off: " + lockfile.SaveError(d.as.path, err)
		d.as.savedAt = time.Time{}
		return
	}
	if d.as.path != d.path {
		return // saved as another file meanwhile
	}
	d.fileInfo, _ = os.Stat(d.path)
	d.as.savedAt = time.Now()
	d.saved = max(d.saved, d.as.version) // a Ctrl+S may have saved a later one
	if !d.modified() {
		d.dropRecovery() // recover.go
	}
	d.updateTitle()
}

// autoSaveStatus is what the title says after the name: "Saving…" while
// AutoSave writes, "Saved" for a moment after.
func (d *window) autoSaveStatus(gtx layout.Context) string {
	s := d.as.saver
	switch {
	case s.Busy():
		return "Saving…"
	case !s.On() || d.as.savedAt.IsZero():
		return ""
	}
	now, end := gtx.Now, d.as.savedAt.Add(savedShown)
	if now.IsZero() {
		now = time.Now() // a frame without a clock (wordshot)
	}
	if !now.Before(end) {
		return ""
	}
	gtx.Execute(op.InvalidateCmd{At: end})
	return "Saved"
}

// autoSaveSwitch is the title bar's AutoSave switch.
func (d *window) autoSaveSwitch(gtx layout.Context) layout.Dimensions {
	clicked, dims := d.as.sw.Layout(gtx, d.th, d.as.saver.On(), d.autoSaveBlocked(), accent)
	if clicked {
		d.setAutoSave(!d.as.saver.On())
	}
	return dims
}
