package ui

import (
	"os"
	"strings"

	"gioui.org/layout"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/word/model"
)

// Preview lays out one frame of a document window without a native
// window, for the wordshot tool. page names a File page ("home", "info",
// "save as", ...) or is empty for the document; menu >= 0 opens that
// menu's drop-down; dialog "style" or "close" shows Modify Style or the
// save prompt.
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
	case "find", "replace", "goto", "header", "footer", "styles", "newstyle", "paste":
		d.previewHome(dialog)
	}
	d.layout(gtx)
}
