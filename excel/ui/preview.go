package ui

import (
	"image"
	"os"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/model"
	"officelite/excel/render"
)

// PreviewOptions set up the window state Preview draws.
type PreviewOptions struct {
	Page string // a File page ("home", "info", "save as", ...); "" for the sheet
	Menu int    // >= 0 opens that menu's drop-down
	Edit string // text being typed into the active cell
	Ask  bool   // show the "save changes?" prompt
	// Select selects ranges ("B2:C4,E6": a multiple selection).
	Select string
	// Context opens a right-click menu: "cell" (at the active cell),
	// "row", "col" (their headers) or "tab".
	Context string
	// Drag shows the selection being dragged by its border to the cell
	// named, e.g. "E6".
	Drag string
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
	if o.Ask {
		a.modified = true
		a.askSaveBeforeClose()
	}
	if o.Select != "" {
		var rs []model.Range
		for _, part := range strings.Split(o.Select, ",") {
			if _, r, err := a.book.ResolveRef(strings.TrimSpace(part), a.active); err == nil {
				rs = append(rs, r)
			}
		}
		if len(rs) > 0 {
			a.selectRange(a.active, rs[len(rs)-1])
			a.state().view.Sel.Extra = rs[:len(rs)-1]
		}
	}
	if o.Edit != "" {
		a.beginEdit(o.Edit, true, false)
		a.ed.cell.SetText(o.Edit) // shown even without the focus
	}
	if o.Context == "" && o.Drag == "" {
		a.layout(gtx)
		return
	}
	// Lay out once to place the grid, then again with the menu or drag.
	a.layout(gtx)
	gtx.Ops.Reset()
	st := a.state()
	v := &st.view
	p := v.Sel.Active
	rc, _, _ := v.CellRect(p.Row, p.Col)
	at := rc.Min.Add(rc.Max).Div(2)
	hw, hh := v.HeaderSize()
	switch o.Context {
	case "cell":
		a.gridContextMenu(st, f32.Pt(float32(at.X), float32(at.Y)))
	case "row":
		a.gridContextMenu(st, f32.Pt(float32(hw/2), float32(at.Y)))
	case "col":
		a.gridContextMenu(st, f32.Pt(float32(at.X), float32(hh/2)))
	case "tab":
		y := gtx.Constraints.Max.Y - gtx.Dp(24) - gtx.Dp(15)
		openCtx(&a.cm.tabs, &a.cm.tabActs, image.Pt(gtx.Dp(40), y), a.tabMenu(a.active))
	}
	if o.Drag != "" {
		if _, r, err := a.book.ResolveRef(o.Drag, a.active); err == nil {
			src := v.Sel.Range
			a.grid.move = &moveDrag{src: src, grab: render.Pos{Row: src.R1, Col: src.C1}, dRow: r.R1 - src.R1, dCol: r.C1 - src.C1}
		}
	}
	a.layout(gtx)
}
