package ui

// takeDrops opens the files dragged onto the window as File ▸ Open does:
// into this window while it holds an untouched new document, otherwise
// each in a window of its own, workbooks in ExcelLite. A drop on the page
// opens the file too (Word would insert it). While a dialog is open a
// drop does nothing.
func (d *window) takeDrops() {
	ready := func() bool { return !d.modalOpen() }
	if msg := d.drop.Take(ready, d.openFile, d.loading.Load); msg != "" {
		d.errMsg = msg
	}
}
