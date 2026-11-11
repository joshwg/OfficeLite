// Opening a file the user picked (File ▸ Open, a recent file, a drop):
// routing it to a window that has it already, to this window when it is
// pristine, to a new window, or to the other app.

package gui

import (
	"log"
	"os"
	"path/filepath"

	"officelite/common/doctype"
	"officelite/common/lockfile"
	"officelite/common/recent"
)

// OpenRequest is a file to open and what the window opening it offers.
type OpenRequest struct {
	Path   string
	Recent *recent.List // a missing file is dropped from it
	// Own is the kind of file this app opens; Takes reports whether a
	// detected kind is this app's (a kind it doesn't take goes to the
	// other app, through Launch).
	Own   doctype.Kind
	Takes func(kind doctype.Kind) bool
	// Pristine reports whether the window holds an untouched new file,
	// which the opened one replaces (OpenHere) instead of going to a new
	// window.
	Pristine func() bool
	OpenHere func(path string)
	Launch   func(kind doctype.Kind, path string)
	// Activate raises a window that has the file open already and reports
	// whether there was one; nil uses lockfile.Activate.
	Activate func(path string) bool
}

// OpenFile opens r.Path where it belongs: nowhere when another window has
// it (that window comes to the front), in this window when it is pristine
// and the file is this app's, otherwise in a new window of the app the
// file is for. A file that can't be found is dropped from the recent
// files; the message to show is returned ("" when all went well).
func OpenFile(r OpenRequest) (errMsg string) {
	if _, err := os.Stat(r.Path); err != nil {
		if r.Recent != nil {
			if removeErr := r.Recent.Remove(r.Path); removeErr != nil {
				log.Printf("remove %s from recent files: %v", r.Path, removeErr)
			}
		}
		return "Can't open " + filepath.Base(r.Path) + ": " + err.Error()
	}
	activate := r.Activate
	if activate == nil {
		activate = lockfile.Activate
	}
	if activate(r.Path) {
		return "" // open in a window already, which comes to the front
	}
	kind, _ := doctype.DetectFile(r.Path)
	switch {
	case !r.Takes(kind):
		r.Launch(kind, r.Path)
	case r.Pristine():
		r.OpenHere(r.Path)
	default:
		r.Launch(r.Own, r.Path)
	}
	return ""
}
