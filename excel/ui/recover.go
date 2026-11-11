// recover.go is the window's AutoRecover side: periodic recovery copies and opening a recovered workbook.

package ui

import (
	"io"

	"officelite/common/autorecover"
	"officelite/common/gui"
	"officelite/excel/fileio"
	"officelite/excel/model"
)

// autoRecover is the window's AutoRecover state: every few minutes a
// workbook with unsaved changes is copied (as .xlsx) to the Recovery
// directory, for Document Recovery after a crash.
type autoRecover struct {
	saver *autorecover.Saver
	rev   int // counts edits (changed), so an unchanged workbook isn't copied again
	// recovered is the recovery copy the window was opened on, until the
	// workbook is saved: the title says [Recovered] and Save As starts
	// from the original file's folder, name and format.
	recovered *autorecover.Entry
}

// startAutoRecover starts the window's AutoRecover timer.
func (a *App) startAutoRecover() {
	a.ar.saver = autorecover.NewSaver(autorecover.Excel, a.calls.Post, a.recoverySnapshot)
	a.ar.saver.Start()
}

// recoverySnapshot is the workbook to copy, cloned on the UI goroutine
// (a few ms for 100,000 cells, where writing the .xlsx takes over 100);
// nil when there are no unsaved changes.
func (a *App) recoverySnapshot() *autorecover.Snapshot {
	if !a.modified || a.loading.Load() {
		return nil
	}
	wb := a.wb.Clone()
	path, format := gui.RecoveryOrigin(a.path, a.ar.recovered)
	return &autorecover.Snapshot{Rev: a.ar.rev, Path: path, Name: a.wb.Name, Format: format,
		Write: func(w io.Writer) error { return fileio.Write(wb, w, fileio.XLSX, 0) }}
}

// dropRecovery deletes the window's recovery copy: the workbook was saved
// or its changes thrown away.
func (a *App) dropRecovery() {
	a.ar.saver.Remove()
	a.ar.recovered = nil
}

// openRecovered loads a recovery copy (from Document Recovery) into the
// window.
func (a *App) openRecovered(e autorecover.Entry) {
	if !a.startLoad() {
		a.errMsg = busyLoading
		return
	}
	a.setWorkbook(&model.Workbook{
		Name:   e.Name,
		Sheets: []*model.Sheet{model.NewSheet("Sheet1")},
		Styles: []model.Style{model.DefaultStyle()},
	})
	go func() {
		wb, err := fileio.Open(e.Copy)
		a.finishLoadResult(loadResult{wb: wb, err: err, rec: &e})
	}()
}

// adoptRecovered makes a loaded recovery copy the window's workbook: named
// as the original, modified, with no file of its own (Save goes to Save
// As), and the copy kept up to date and deleted like the window's own.
func (a *App) adoptRecovered(e autorecover.Entry) {
	a.ar.recovered = &e
	a.wb.Name = e.Name
	a.modified = true
	a.ar.saver.Adopt(e, a.ar.rev)
	f, ok := fileio.FormatFor(e.Path)
	a.bs.saveFmt = gui.FormatIndex(saveFormats, f, ok) // else .xlsx (for .numbers, .xlsm, new workbooks)
	if a.win != nil {
		a.win.Option(windowTitle(a.docName()))
	}
}

// saveAsName is the name Save As suggests for format f: the workbook's,
// in the original file's folder for a recovered workbook.
func (a *App) saveAsName(f fileio.Format) string {
	return gui.SaveAsName(a.baseName(), f.Ext(), a.ar.recovered)
}
