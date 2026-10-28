package ui

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Right-click menus of the cells, the row and column headers and the
// sheet tabs, as in Excel. Each item runs the command the menus and the
// ribbon run.

// ctxMenus holds the open context menu's state.
type ctxMenus struct {
	grid     gui.ContextMenu // cells and headers, in grid coordinates
	tabs     gui.ContextMenu // sheet tabs, in window coordinates
	gridActs []func()
	tabActs  []func()
	strip    int // event tag over the sheet tab strip
}

// ctxItem is a context menu entry; one without a label is a separator.
type ctxItem struct {
	label, shortcut, tip string
	disabled             bool
	run                  func()
}

var ctxSep = ctxItem{}

// openCtx shows items in m at pos, remembering their commands.
func openCtx(m *gui.ContextMenu, acts *[]func(), pos image.Point, items []ctxItem) {
	mi := make([]gui.MenuItem, len(items))
	*acts = make([]func(), len(items))
	for i, it := range items {
		if it.label == "" {
			mi[i] = gui.MenuItem{Separator: true}
			continue
		}
		mi[i] = gui.MenuItem{Label: it.label, Shortcut: it.shortcut, Tip: it.tip, Disabled: it.disabled}
		(*acts)[i] = it.run
	}
	m.Open(pos, mi)
}

// gridContextMenu handles a right-click in the grid: a cell or header
// outside the selection is selected first, then the menu for it opens.
func (a *App) gridContextMenu(st *sheetState, pos f32.Point) {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	if a.rename.active {
		a.finishRename(true)
	}
	a.grid.focusPending = true
	v := &st.view
	hit := v.HitTest(pos.X, pos.Y)
	inside := false
	switch hit.Area {
	case render.AreaCell:
		inside = v.Sel.Contains(hit.Row, hit.Col)
	case render.AreaColHeader:
		inside = wholeCols(v.Sel.Range) && hit.Col >= v.Sel.Range.C1 && hit.Col <= v.Sel.Range.C2
	case render.AreaRowHeader:
		inside = wholeRows(v.Sel.Range) && hit.Row >= v.Sel.Range.R1 && hit.Row <= v.Sel.Range.R2
	}
	if !inside {
		a.pointAt(st, hit, false)
	}
	var items []ctxItem
	switch hit.Area {
	case render.AreaColHeader:
		items = a.headerMenu(true)
	case render.AreaRowHeader:
		items = a.headerMenu(false)
	default:
		items = a.cellMenu()
	}
	openCtx(&a.cm.grid, &a.cm.gridActs, pos.Round(), items)
}

// layoutGridMenu draws the grid's context menu and runs a chosen item.
func (a *App) layoutGridMenu(gtx layout.Context) {
	if i := a.cm.grid.Layout(gtx, a.ui); i >= 0 && i < len(a.cm.gridActs) && a.cm.gridActs[i] != nil {
		a.cm.gridActs[i]()
		a.afterCtxCommand()
	}
}

// afterCtxCommand gives the keyboard back to the grid unless the command
// opened something that wants it.
func (a *App) afterCtxCommand() {
	if !a.back.Visible && !a.ed.active && !a.rename.active && a.ins.kind == dlgNone && !a.fmtDlg.open && !a.askDlg.open {
		a.grid.focusPending = true
	}
}

// clipboardItems are Cut, Copy, Paste and the Paste Special choices.
func (a *App) clipboardItems(multi bool) []ctxItem {
	noClip := a.clip == nil || a.clip.WB != a.wb
	return []ctxItem{
		{"Cut", "Ctrl+X", "Move the selection to the Clipboard.", multi, func() { a.copy(a.gtx, true) }},
		{"Copy", "Ctrl+C", "Copy the selection to the Clipboard.", multi, func() { a.copy(a.gtx, false) }},
		{"Paste", "Ctrl+V", "Add content from the Clipboard.", multi, func() { a.requestPaste(a.gtx) }},
		{"Paste Values", "", "Paste only the values of the copied cells.", multi || noClip, func() { a.pasteSpecial(edit.PasteValues) }},
		{"Paste Formulas", "", "Paste the formulas of the copied cells, keeping these cells' formats.", multi || noClip, func() { a.pasteSpecial(edit.PasteFormulas) }},
		{"Paste Formatting", "", "Paste only the formats of the copied cells.", multi || noClip, func() { a.pasteSpecial(edit.PasteFormats) }},
	}
}

// cellMenu is the menu of a right-clicked cell.
func (a *App) cellMenu() []ctxItem {
	v := &a.state().view
	multi := v.Sel.Multiple()
	p := v.Sel.Active
	if m, ok := v.Sheet.MergeAt(p.Row, p.Col); ok {
		p = render.Pos{Row: m.R1, Col: m.C1}
	}
	_, hasNote := v.Sheet.NoteAt(p.Row, p.Col)
	_, hasLink := v.Sheet.LinkAt(p.Row, p.Col)
	items := a.clipboardItems(multi)
	items = append(items, ctxSep,
		ctxItem{"Insert…", "Ctrl+Shift+=", "Add cells, rows or columns, shifting the others.", multi, a.insertCellsDialog},
		ctxItem{"Delete…", "Ctrl+-", "Delete cells, rows or columns, shifting the others.", multi, a.deleteCellsDialog},
		ctxItem{"Clear Contents", "Delete", "Delete the contents of the selected cells, keeping their formats.", false, a.clearSelection},
		ctxSep,
		ctxItem{"Sort A to Z", "", "Sort the selection so the lowest values are at the top.", multi, a.sortAscending},
		ctxItem{"Sort Z to A", "", "Sort the selection so the highest values are at the top.", multi, a.sortDescending},
		ctxItem{"Custom Sort…", "", "Sort by several columns or with other options.", multi, a.customSortDialog},
		ctxSep,
	)
	if hasNote {
		items = append(items,
			ctxItem{"Edit Note", "Shift+F2", "Change the note of this cell.", false, a.newNote},
			ctxItem{"Delete Note", "", "Delete the notes in the selection.", false, a.clearNotes})
	} else {
		items = append(items, ctxItem{"New Note", "Shift+F2", "Add a note to this cell.", false, a.newNote})
	}
	items = append(items, ctxItem{"Format Cells…", "Ctrl+1", "Set the number format, alignment, font, borders and fill.", false, func() { a.openFormatCells(tabNumber) }})
	if hasLink {
		items = append(items,
			ctxItem{"Edit Hyperlink…", "Ctrl+K", "Change this cell's link.", multi, a.insertHyperlinkDialog},
			ctxItem{"Remove Hyperlink", "", "Remove the links in the selection, keeping the text.", false, a.clearHyperlinks})
	} else {
		items = append(items, ctxItem{"Link…", "Ctrl+K", "Link to a web page, a file or a place in this workbook.", multi, a.insertHyperlinkDialog})
	}
	return items
}

// headerMenu is the menu of a right-clicked column (cols) or row header.
func (a *App) headerMenu(cols bool) []ctxItem {
	v := &a.state().view
	multi := v.Sel.Multiple()
	what, size, sizeTip, sizeItem := "rows", "Row Height…", "Set the height of the selected rows.", fmtRowHeight
	hide, unhide := fmtHideRows, fmtUnhideRows
	if cols {
		what, size, sizeTip, sizeItem = "columns", "Column Width…", "Set the width of the selected columns.", fmtColWidth
		hide, unhide = fmtHideCols, fmtUnhideCols
	}
	anyHidden := false
	if cols {
		for _, c := range a.selCols() {
			if w, ok := v.Sheet.ColWidth[c]; ok && w == 0 {
				anyHidden = true
				break
			}
		}
	} else {
		for _, r := range a.selRows() {
			if h, ok := v.Sheet.RowHeight[r]; ok && h == 0 {
				anyHidden = true
				break
			}
		}
	}
	items := a.clipboardItems(multi)
	items = append(items, ctxSep,
		ctxItem{"Insert", "Ctrl+Shift+=", "Insert as many " + what + " as are selected, before them.", multi, func() { a.structural(true, cols) }},
		ctxItem{"Delete", "Ctrl+-", "Delete the selected " + what + ".", multi, func() { a.structural(false, cols) }},
		ctxItem{"Clear Contents", "Delete", "Delete the contents of the selected cells, keeping their formats.", false, a.clearSelection},
		ctxSep,
		ctxItem{"Format Cells…", "Ctrl+1", "Set the number format, alignment, font, borders and fill.", false, func() { a.openFormatCells(tabNumber) }},
		ctxItem{size, "", sizeTip, multi, func() { a.formatMenu(a.gtx, sizeItem) }},
		ctxItem{"Hide", "", "Hide the selected " + what + ".", multi, func() { a.formatMenu(a.gtx, hide) }},
		ctxItem{"Unhide", "", "Show the hidden " + what + " in the selection.", multi || !anyHidden, func() { a.formatMenu(a.gtx, unhide) }},
	)
	return items
}

// sheetTabsWithMenu lays out the sheet tabs and handles their right-click
// menu: a right-clicked tab is activated and gets Insert, Delete, Rename,
// Hide and Unhide.
func (a *App) sheetTabsWithMenu(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &a.cm.strip, Kinds: pointer.Press})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || !e.Buttons.Contain(pointer.ButtonSecondary) {
			continue
		}
		tab := -1
		for i := range a.tabBtns {
			if a.tabBtns[i].Hovered() {
				tab = i
			}
		}
		if tab < 0 || tab >= len(a.wb.Sheets) {
			continue
		}
		if a.ed.active && !a.commitEdit(0, 0) {
			continue
		}
		if a.rename.active {
			a.finishRename(true)
		}
		a.active = tab
		a.grid.focusPending = true
		// The strip sits above the status bar at the window's bottom.
		win := a.gtx.Constraints.Max
		top := win.Y - gtx.Dp(24) - gtx.Dp(30)
		openCtx(&a.cm.tabs, &a.cm.tabActs, e.Position.Round().Add(image.Pt(0, top)), a.tabMenu(tab))
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &a.cm.strip)
	d := a.sheetTabs(gtx)
	area.Pop()

	if a.cm.tabs.IsOpen() {
		win := a.gtx.Constraints.Max
		top := win.Y - gtx.Dp(24) - gtx.Dp(30)
		off := op.Offset(image.Pt(0, -top)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(win)
		i := a.cm.tabs.Layout(g, a.ui)
		off.Pop()
		if i >= 0 && i < len(a.cm.tabActs) && a.cm.tabActs[i] != nil {
			a.cm.tabActs[i]()
			a.afterCtxCommand()
		}
	}
	return d
}

// tabMenu is the menu of sheet tab i (the active one).
func (a *App) tabMenu(i int) []ctxItem {
	visible, hidden := 0, 0
	for _, sh := range a.wb.Sheets {
		if sh.Hidden {
			hidden++
		} else {
			visible++
		}
	}
	return []ctxItem{
		{"Insert", "Shift+F11", "Add a new sheet before this one.", false, a.insertSheet},
		{"Delete", "", "Delete this sheet.", visible < 2, a.deleteSheet},
		{"Rename", "", "Rename this sheet (or double-click its tab).", false, func() { a.startRename(i) }},
		ctxSep,
		{"Hide", "", "Hide this sheet.", visible < 2, func() { a.formatMenu(a.gtx, fmtHideSheet) }},
		{"Unhide…", "", "Show a hidden sheet.", hidden == 0, func() { a.formatMenu(a.gtx, fmtUnhideSheet) }},
	}
}

// singleRange refuses a command that can't act on a multiple selection,
// as Excel does.
func (a *App) singleRange() bool {
	if a.state().view.Sel.Multiple() {
		a.errMsg = "This action won't work on multiple selections."
		return false
	}
	return true
}

// eachRange runs f on every range of the selection (just the one,
// usually), as one undo step.
func (a *App) eachRange(f func(r model.Range)) {
	sel := &a.state().view.Sel
	if !sel.Multiple() {
		f(sel.Range)
		return
	}
	a.book.BeginGroup()
	defer a.book.EndGroup()
	for _, r := range sel.Ranges() {
		f(r)
	}
}
