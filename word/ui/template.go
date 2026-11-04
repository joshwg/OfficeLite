// Templates: new documents started from a .dotx/.dot (from the New page or
// the shell), the New page's template tiles, and where Save As puts a
// template. A template opened from the Open dialog or the recent files is
// edited itself, like any document.

package ui

import (
	"os"
	"path/filepath"
	"slices"

	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/common/templates"
	"officelite/word/fileio"
	"officelite/word/model"
)

// newFromTemplate starts a new untitled document from the template at
// path, as Word does: the template's text, styles, headers and settings,
// with no file of its own (Save goes to Save As, in the format the
// template is built on). The template file is neither locked nor added to
// the recent files.
func (d *window) newFromTemplate(path string) {
	name := newDocName()
	d.loading.Store(true)
	d.setDoc(model.New(name), d.th.Shaper, nil)
	go func() {
		doc, err := fileio.Open(path)
		d.finishLoadResult(loadResult{doc: doc, err: err, template: path, name: name})
	}()
}

// adoptTemplate finishes a document just made from the template at path:
// Save As offers the document format the template is built on (.docx for
// a .dotx or .dotm, .doc for a .dot).
func (d *window) adoptTemplate(path string) {
	f, _ := fileio.FormatFor(path)
	d.bs.saveFmt = max(0, slices.Index(saveFormats[:], f.Document()))
}

// newTiles is the New page's tiles: blank documents, then the user's
// templates.
func (d *window) newTiles() []gui.Template {
	tiles := d.bs.tiles[:0]
	tiles = append(tiles, blankTiles...)
	for _, e := range d.bs.tmpl.Entries() {
		tiles = append(tiles, gui.Template{Name: e.Name, Kind: recent.Document, Path: e.Path})
	}
	d.bs.tiles = tiles
	return tiles
}

// templateNote tells where to save templates for the New page to list
// them.
func templateNote() string {
	dir := templates.Dir()
	if dir == "" {
		return ""
	}
	return "Templates saved in " + dir + " appear here. Save a document as a Word Template (*.dotx) to add one."
}

// newFromTile starts a new document from New tile t: a blank document or
// workbook in a new window, or a document from a template, in this window
// if it holds an untouched new document.
func (d *window) newFromTile(t gui.Template) {
	d.back.Hide()
	switch {
	case t.Path == "" && t.Kind == recent.Spreadsheet:
		d.cfg.Launch(doctype.Spreadsheet, "")
	case t.Path == "":
		d.cfg.Launch(doctype.Document, "")
	case d.pristine():
		d.newFromTemplate(t.Path)
	default:
		d.cfg.LaunchTemplate(t.Path)
	}
}

// templateSaveName is where Save As suggests saving a template: the
// templates folder, created if need be, so the New page lists it. Without
// one it is name alone. Called off the UI goroutine.
func templateSaveName(name string) string {
	dir := templates.Dir()
	if dir == "" || os.MkdirAll(dir, 0o755) != nil {
		return filepath.Base(name)
	}
	return filepath.Join(dir, filepath.Base(name))
}
