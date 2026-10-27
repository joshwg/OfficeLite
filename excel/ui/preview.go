package ui

import (
	"os"
	"strings"

	"gioui.org/layout"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/model"
)

// PreviewOptions set up the window state Preview draws.
type PreviewOptions struct {
	Page string // a File page ("home", "info", "save as", ...); "" for the sheet
	Menu int    // >= 0 opens that menu's drop-down
	Edit string // text being typed into the active cell
	Ask  bool   // show the "save changes?" prompt
}

// Preview lays out one frame of a spreadsheet window without a native
// window, for the excelshot tool.
func Preview(gtx layout.Context, fonts *gui.FontLibrary, list *recent.List, wb *model.Workbook, path string, o PreviewOptions) {
	page, menu := o.Page, o.Menu
	a := &App{
		ui:     gui.NewTheme(fonts, accent, accentDark),
		fonts:  fonts,
		recent: list,
		zoom:   1,
		states: map[int]*sheetState{},
	}
	a.th = a.ui.Theme
	a.shaper = a.th.Shaper
	a.setWorkbook(wb)
	a.path = path
	if path != "" {
		a.fileInfo, _ = os.Stat(path)
	}
	for p := gui.PageHome; p <= gui.PageClose; p++ {
		if strings.EqualFold(p.String(), page) {
			a.back.Show(p)
		}
	}
	if menu >= 0 {
		a.menu.OpenMenu(menu, len(a.menus()))
	}
	if o.Edit != "" {
		a.beginEdit(o.Edit, true, false)
		a.ed.cell.SetText(o.Edit) // shown even without the focus
	}
	if o.Ask {
		a.modified = true
		a.askSaveBeforeClose()
	}
	a.layout(gtx)
}
