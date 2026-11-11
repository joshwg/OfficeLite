// FileDialog in the browser: the web server's dialog of the user's files
// on the server (web/server/static/serverfs.js, as window.officeweb). It
// returns paths under /server/, which the same script's file system
// reads and writes through the server, so the apps open, save and
// AutoSave server documents as they do files on disk. Choosing "this
// computer" in the dialog, or a page without the dialog, returns
// ErrNoFileDialog, and the callers fall back to gioui.org/x/explorer:
// the browser's file picker to open, a download to save.

//go:build js

package gui

import (
	"path"
	"syscall/js"

	"gioui.org/io/event"
)

// Replies of the page's dialogs besides a chosen path.
const (
	dialogCancel = "cancel" // closed without choosing
	dialogLocal  = "local"  // the user chose a file on this computer
)

// listen does nothing: the page's dialog has no owner window.
func (d *FileDialog) listen(event.Event) {}

// open shows the page's Open dialog of server files with filters'
// extensions.
func (d *FileDialog) open(filters []FileFilter) (string, error) {
	return ask("openDialog", js.Undefined(), filters)
}

// save shows the page's Save As dialog of server files, suggesting name
// (its last element, without a folder).
func (d *FileDialog) save(name string, filters []FileFilter) (string, error) {
	return ask("saveDialog", js.ValueOf(path.Base(name)), filters)
}

// ask calls window.officeweb[method](arg, exts) and waits for the
// promise it returns: a path, dialogCancel or dialogLocal. Without the
// page's dialog it returns ErrNoFileDialog.
func ask(method string, arg js.Value, filters []FileFilter) (string, error) {
	ow := js.Global().Get("officeweb")
	if ow.IsUndefined() || ow.Get(method).IsUndefined() {
		return "", ErrNoFileDialog
	}
	var exts []any
	for _, f := range filters {
		for _, e := range f.Exts {
			exts = append(exts, e)
		}
	}
	reply := make(chan string, 1)
	done := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 && args[0].Type() == js.TypeString {
			reply <- args[0].String()
		} else {
			reply <- dialogLocal
		}
		return nil
	})
	defer done.Release()
	fail := js.FuncOf(func(js.Value, []js.Value) any {
		reply <- dialogLocal // a broken dialog still leaves the browser's own
		return nil
	})
	defer fail.Release()
	ow.Call(method, arg, js.ValueOf(exts)).Call("then", done, fail)
	switch r := <-reply; r {
	case dialogCancel:
		return "", ErrCancelled
	case dialogLocal:
		return "", ErrNoFileDialog
	default:
		return r, nil
	}
}
