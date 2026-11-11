// recover.go is the window's AutoRecover side: periodic recovery copies
// and opening a recovered document.

package ui

import (
	"io"

	"officelite/common/autorecover"
	"officelite/common/gui"
	"officelite/word/fileio"
	"officelite/word/model"
)

// autoRecover is the window's AutoRecover state: every few minutes a
// document with unsaved changes is copied (as .docx) to the Recovery
// directory, for Document Recovery after a crash.
type autoRecover struct {
	saver *autorecover.Saver
	// recovered is the recovery copy the window was opened on, until the
	// document is saved: the title says [Recovered] and Save As starts
	// from the original file's folder, name and format.
	recovered *autorecover.Entry
}

// startAutoRecover starts the window's AutoRecover timer.
func (d *window) startAutoRecover() {
	d.ar.saver = autorecover.NewSaver(autorecover.Word, d.calls.Post, d.recoverySnapshot)
	d.ar.saver.Start()
}

// recoveryRev changes with every edit, of the body or of a header being
// edited (both editors' versions only grow).
func (d *window) recoveryRev() int {
	if d.ed == nil {
		return 0
	}
	if b := d.bodyEd(); b != d.ed {
		return b.Version + d.ed.Version
	}
	return d.ed.Version
}

// recoverySnapshot is the document to copy, cloned on the UI goroutine;
// nil when there are no unsaved changes.
func (d *window) recoverySnapshot() *autorecover.Snapshot {
	if !d.modified() || d.loading.Load() {
		return nil
	}
	doc := d.doc.Clone()
	path, format := gui.RecoveryOrigin(d.path, d.ar.recovered)
	return &autorecover.Snapshot{Rev: d.recoveryRev(), Path: path, Name: d.doc.Name, Format: format,
		Write: func(w io.Writer) error { return fileio.Write(doc, w, fileio.DOCX) }}
}

// dropRecovery deletes the window's recovery copy: the document was saved
// or its changes thrown away.
func (d *window) dropRecovery() {
	d.ar.saver.Remove()
	if d.ar.recovered != nil {
		d.ar.recovered = nil
		d.updateTitle()
	}
}

// openRecovered loads a recovery copy (from Document Recovery) into the
// window.
func (d *window) openRecovered(e autorecover.Entry) {
	if !d.startLoad() {
		d.errMsg = busyLoading
		return
	}
	d.setDoc(model.New(e.Name), d.th.Shaper, nil)
	go func() {
		doc, err := fileio.Open(e.Copy)
		d.finishLoadResult(loadResult{doc: doc, err: err, rec: &e})
	}()
}

// adoptRecovered makes a loaded recovery copy the window's document: named
// as the original, modified, with no file of its own (Save goes to Save
// As), and the copy kept up to date and deleted like the window's own.
func (d *window) adoptRecovered(e autorecover.Entry) {
	d.ar.recovered = &e
	d.doc.Name = e.Name
	d.saved = -1 // modified
	d.ar.saver.Adopt(e, d.recoveryRev())
	f, ok := fileio.FormatFor(e.Path)
	d.bs.saveFmt = gui.FormatIndex(saveFormats[:], f, ok) // else .docx (for .pages, .docm, new documents)
	d.updateTitle()
}

// saveAsName is the name Save As suggests for format f: the document's,
// in the original file's folder for a recovered document.
func (d *window) saveAsName(f fileio.Format) string {
	return gui.SaveAsName(d.baseName(), f.Ext(), d.ar.recovered)
}
