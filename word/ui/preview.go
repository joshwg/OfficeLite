package ui

import (
	"os"
	"strings"
	"time"

	"gioui.org/layout"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/word/model"
)

// Preview lays out one frame of a document window without a native
// window, for the wordshot tool. page names a File page ("home", "info",
// "save as", ...) or is empty for the document; menu >= 0 opens that
// menu's drop-down; dialog "style" or "close" shows Modify Style or the
// save prompt; "marks" turns on Show/Hide ¶; "context", "colresize" and "dragtext" show the right-click
// menu, a table border being dragged and selected text being dragged;
// "undo" and "redo" make a few changes and open the Undo (or Redo) list;
// "autosave" and "autosave-saved" turn the title bar's AutoSave switch on
// (and show it just saved).
func Preview(gtx layout.Context, fonts *gui.FontLibrary, list *recent.List, doc *model.Document, path, page string, menu int, dialog string) {
	d := newWindow(nil, Config{Fonts: fonts, Recent: list})
	d.setDoc(doc, fonts.Shaper(docFonts(doc)...), docFonts(doc))
	d.path = path
	if path != "" {
		d.fileInfo, _ = os.Stat(path)
	}
	for p := gui.PageHome; p <= gui.PageClose; p++ {
		if strings.EqualFold(p.String(), page) {
			d.back.Show(p)
		}
	}
	if menu >= 0 {
		d.menu.OpenMenu(menu, len(d.menus()))
	}
	switch dialog {
	case "style":
		d.dlg.openFor(d, d.ed.ParaStyleID(0))
	case "close":
		d.confirm.show = true
	case "table", "picker", "link", "symbol", "date":
		d.previewInsert(dialog)
	case "find", "replace", "goto", "header", "footer", "styles", "newstyle", "paste", "marks":
		d.previewHome(dialog)
	case "undo", "redo":
		d.previewUndo(dialog == "redo")
	case "autosave", "autosave-saved":
		d.startAutoSave()
		d.setAutoSave(true)
		if dialog == "autosave-saved" {
			d.as.savedAt = time.Now()
		}
	case "context", "colresize", "dragtext":
		// These need the page view laid out once to place themselves.
		d.layout(gtx)
		gtx.Ops.Reset()
		d.previewMouse(dialog)
	}
	d.layout(gtx)
}

// previewUndo makes a few changes at the start of the document and opens
// the Undo list, or the Redo list after taking four of them back, with
// three entries highlighted.
func (d *window) previewUndo(redo bool) {
	ed := d.ed
	ed.SetCaret(model.Pos{}, false)
	for _, c := range "Draft " {
		ed.InsertText(string(c))
	}
	ed.Select(model.Pos{}, model.Pos{Off: 5})
	ed.ToggleBold()
	ed.SetSize(14)
	ed.SetColor("C00000")
	func() {
		defer ed.Op("Center")()
		ed.SetAlign(model.AlignCenter)
	}()
	ed.ToggleList(true)
	ed.SetCaret(model.Pos{Off: 6}, false)
	for _, c := range "copy " {
		ed.InsertText(string(c))
	}
	hl := &d.tb.undoHL
	if redo {
		ed.UndoN(4)
		hl = &d.tb.redoHL
	}
	hl.ShowOpen(3)
}
