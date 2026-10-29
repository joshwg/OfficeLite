package ui

// takeDrops opens the files dragged onto the window as File ▸ Open does:
// into this window while it holds an untouched new workbook, otherwise
// each in a window of its own, documents in WordLite. While a dialog is
// open a drop does nothing; a cell being edited is entered first.
func (a *App) takeDrops() {
	ready := func() bool { return !a.modalOpen() && (!a.ed.active || a.commitEdit(0, 0)) }
	if msg := a.drop.Take(ready, a.openFile, a.loading.Load); msg != "" {
		a.errMsg = msg
	}
}
