// The window's chrome: the menu bar and its commands, the frame layout,
// window-wide shortcuts, the title bar, formula bar, scrollbars, sheet
// tabs (with renaming) and status bar.

package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

var (
	accent      = gui.ExcelAccent
	accentDark  = colors.RGB(0x7E4400)
	chromeBG    = gui.ChromeBG
	chromeLine  = gui.ChromeLine
	chromeText  = gui.ChromeText
	mutedText   = gui.MutedText
	errorText   = gui.ErrorText
	tabActiveBG = colors.White
)

// Menu bar entries.
const (
	menuFile = iota
	menuHome
	menuInsert
	menuFormulas
	menuData
	menuView
)

// Home menu items.
const (
	homeUndo = iota
	homeRedo
	_
	homeCut
	homeCopy
	homePaste
	homeDelete
	homeSelectAll
	_
	homeInsertRows
	homeInsertCols
	homeDeleteRows
	homeDeleteCols
	_
	homeInsertSheet
	homeDeleteSheet
	homeRenameSheet
	_
	homeFillDown
	homeFillRight
	_
	homeSortAsc
	homeSortDesc
	homeCustomSort
	_
	homeFind
	homeReplace
	homeGoTo
)

// menus is the menu bar: File, Home, Insert, Formulas, Data and View, then
// the contextual PivotTable and chart tabs, with commands that can't run
// now disabled.
func (a *App) menus() []gui.Menu {
	sel := a.state().view.Sel.Range
	editing := a.ed.active
	visible := 0
	for _, sh := range a.wb.Sheets {
		if !sh.Hidden {
			visible++
		}
	}
	menus := a.chartSheetMenus(append([]gui.Menu{
		menuFile: {Label: "File", Primary: true},
		menuHome: {Label: "Home", Tab: true, Selected: a.tab == tabHome, Items: []gui.MenuItem{
			homeUndo:        {Label: "Undo", Shortcut: "Ctrl+Z", Disabled: !a.book.CanUndo() && !editing, Tip: "Undo your last action."},
			homeRedo:        {Label: "Redo", Shortcut: "Ctrl+Y", Disabled: !a.book.CanRedo() || editing, Tip: "Redo your last action."},
			2:               {Separator: true},
			homeCut:         {Label: "Cut", Shortcut: "Ctrl+X", Disabled: editing, Tip: "Move the selection to the Clipboard."},
			homeCopy:        {Label: "Copy", Shortcut: "Ctrl+C", Disabled: editing, Tip: "Copy the selection to the Clipboard."},
			homePaste:       {Label: "Paste", Shortcut: "Ctrl+V", Disabled: editing, Tip: "Add content from the Clipboard."},
			homeDelete:      {Label: "Clear Contents", Shortcut: "Delete", Disabled: editing, Tip: "Delete the contents of the selected cells, keeping their formatting."},
			homeSelectAll:   {Label: "Select All", Shortcut: "Ctrl+A", Disabled: editing, Tip: "Select every cell on the sheet."},
			8:               {Separator: true},
			homeInsertRows:  {Label: "Insert Sheet Rows", Shortcut: "Ctrl+Shift+=", Disabled: wholeCols(sel), Tip: "Add rows above the selection."},
			homeInsertCols:  {Label: "Insert Sheet Columns", Disabled: wholeRows(sel), Tip: "Add columns to the left of the selection."},
			homeDeleteRows:  {Label: "Delete Sheet Rows", Shortcut: "Ctrl+-", Disabled: wholeCols(sel), Tip: "Delete the selected rows."},
			homeDeleteCols:  {Label: "Delete Sheet Columns", Disabled: wholeRows(sel), Tip: "Delete the selected columns."},
			13:              {Separator: true},
			homeInsertSheet: {Label: "Insert Sheet", Shortcut: "Shift+F11", Tip: "Add a new worksheet."},
			homeDeleteSheet: {Label: "Delete Sheet", Disabled: visible < 2, Tip: "Delete the active worksheet."},
			homeRenameSheet: {Label: "Rename Sheet", Tip: "Give the active worksheet a new name."},
			17:              {Separator: true},
			homeFillDown:    {Label: "Fill Down", Shortcut: "Ctrl+D", Disabled: editing, Tip: "Copy the top cell of the selection into the cells below."},
			homeFillRight:   {Label: "Fill Right", Shortcut: "Ctrl+R", Disabled: editing, Tip: "Copy the leftmost cell of the selection into the cells to the right."},
			20:              {Separator: true},
			homeSortAsc:     {Label: "Sort A to Z", Disabled: editing, Tip: "Sort the selection from lowest to highest."},
			homeSortDesc:    {Label: "Sort Z to A", Disabled: editing, Tip: "Sort the selection from highest to lowest."},
			homeCustomSort:  {Label: "Custom Sort…", Disabled: editing, Tip: "Sort by several columns or in a custom order."},
			24:              {Separator: true},
			homeFind:        {Label: "Find…", Shortcut: "Ctrl+F", Disabled: editing, Tip: "Find text or numbers on the sheet."},
			homeReplace:     {Label: "Replace…", Shortcut: "Ctrl+H", Disabled: editing, Tip: "Search for text and replace it with something else."},
			homeGoTo:        {Label: "Go To…", Shortcut: "Ctrl+G", Disabled: editing, Tip: "Jump to a cell or range by its reference."},
		}},
		menuInsert: a.insertMenuDef(),
		menuFormulas: {Label: "Formulas", Items: []gui.MenuItem{
			{Label: "Calculate Now", Shortcut: "F9", Tip: "Recalculate every formula in the workbook."},
		}},
		menuData: a.dataMenuDef(),
		menuView: {Label: "View", Items: []gui.MenuItem{
			{Label: "Zoom In", Shortcut: "Ctrl+Alt+=", Tip: "Make the sheet bigger on screen."},
			{Label: "Zoom Out", Shortcut: "Ctrl+Alt+-", Tip: "Make the sheet smaller on screen."},
			{Label: "100%", Shortcut: "Ctrl+0", Tip: "Zoom to the sheet's actual size."},
			{Separator: true},
			{Label: "Gridlines", Checked: a.wb.Sheets[a.active].ShowGridLines, Tip: "Show or hide the lines between cells."},
		}},
	}, append(a.pivotMenus(), a.chartMenus()...)...)) // PivotTable Analyze and Design while in a report (pivot.go), Chart Design and Format with a chart (chartribbon.go); chart sheets (chart.go)
	// Undo and Redo name the next step, as Excel's do ("Undo Typing 'x' in B5").
	home := menus[menuHome].Items
	home[homeUndo].Label = gui.HistoryLabel("Undo", a.undoLabels())
	home[homeRedo].Label = gui.HistoryLabel("Redo", a.redoLabels())
	return menus
}

// menuChosen runs the menu item or switches to the tab ev names.
func (a *App) menuChosen(ev gui.MenuEvent) {
	if ev.Menu > menuView {
		i := ev.Menu - menuView - 1
		if n := len(a.pivotMenus()); i >= n {
			a.chartMenuChosen(i - n)
		} else {
			a.pivotMenuChosen(i)
		}
		return
	}
	if ev.Item == gui.MenuTab {
		// Home and Data also switch the ribbon, as Excel's tabs do.
		switch ev.Menu {
		case menuHome:
			a.tab = tabHome
		case menuData:
			a.tab = tabData
		}
		return
	}
	switch ev.Menu {
	case menuFile:
		a.back.Show(gui.PageHome)
	case menuHome:
		v := &a.state().view
		switch ev.Item {
		case homeUndo:
			a.undo()
		case homeRedo:
			a.redo()
		case homeCut, homeCopy:
			a.copy(a.gtx, ev.Item == homeCut)
		case homePaste:
			a.requestPaste(a.gtx)
		case homeDelete:
			a.clearSelection()
		case homeSelectAll:
			selectAll(v)
		case homeInsertRows, homeInsertCols, homeDeleteRows, homeDeleteCols:
			a.structural(ev.Item == homeInsertRows || ev.Item == homeInsertCols, ev.Item == homeInsertCols || ev.Item == homeDeleteCols)
		case homeInsertSheet:
			a.insertSheet()
		case homeDeleteSheet:
			a.deleteSheet()
		case homeRenameSheet:
			a.startRename(a.active)
		case homeFillDown:
			a.fillDown()
		case homeFillRight:
			a.fillRight()
		case homeSortAsc:
			a.sortAscending()
		case homeSortDesc:
			a.sortDescending()
		case homeCustomSort:
			a.customSortDialog()
		case homeFind:
			a.showFind()
		case homeReplace:
			a.showReplace()
		case homeGoTo:
			a.showGoTo()
		}
	case menuInsert:
		a.insertChosen(ev.Item)
	case menuFormulas:
		a.recalculate()
	case menuData:
		a.dataChosen(ev.Item)
	case menuView:
		switch ev.Item {
		case 0:
			a.setZoom(a.zoom + 0.1)
		case 1:
			a.setZoom(a.zoom - 0.1)
		case 2:
			a.setZoom(1)
		case 4:
			sh := a.wb.Sheets[a.active]
			sh.ShowGridLines = !sh.ShowGridLines
		}
	}
	if !a.back.Visible && !a.ed.active && !a.rename.active && a.ins.kind == dlgNone {
		a.grid.focusPending = true
	}
}

// layout handles the frame's input and draws the window: the backstage,
// or the title bar, menus, ribbon, formula bar, grid, tabs and status
// bar, with any open dialog on top.
func (a *App) layout(gtx layout.Context) {
	a.gtx = gtx
	a.takeDrops() // drop.go
	if a.dialog.Visible {
		if i := a.dialog.Clicked(gtx); i >= 0 && a.answer != nil {
			answer := a.answer
			a.answer = nil
			answer(i)
		}
	}
	defer a.dialog.Layout(gtx, a.ui)
	defer a.layoutPivotDialog(gtx) // pivotdlg.go
	defer a.layoutChartDialog(gtx) // chartdlg.go
	defer a.layoutDialogs(gtx)
	defer a.layoutInsertDialogs(gtx)
	a.dialogInput(gtx) // before the shortcuts: Esc and Enter go to an open dialog
	a.insertDialogInput(gtx)
	a.pivotDialogInput(gtx)
	a.chartDialogInput(gtx)
	a.insertKeys(gtx)
	a.globalKeys(gtx)
	a.editKeys(gtx)
	defer a.autoSaveFrame() // after this frame's edits (autosave.go)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, chromeBG)
	if a.back.Visible {
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.titleBar),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.back.Layout(gtx, a.ui, a.backstageConfig(), a.handleBackstage)
			}),
		)
		return
	}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.titleBar),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.menu.Layout(gtx, a.ui, a.menus(), a.menuChosen)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.onChartSheet() && a.tab < tabChartDesign {
				gtx = gtx.Disabled() // no cells to format (chart.go)
			}
			return a.layoutRibbon(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.onChartSheet() {
				gtx = gtx.Disabled()
			}
			return a.formulaBar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, a.layoutGrid),
				layout.Rigid(a.vScrollbar),
				layout.Rigid(a.layoutPane), // PivotTable Fields (pivotpane.go)
			)
		}),
		layout.Rigid(a.bottomBar),
		layout.Rigid(a.statusBar),
	)
}

// globalKeys handles shortcuts that work regardless of focus.
func (a *App) globalKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "O", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "P", Required: key.ModShortcut},
			key.Filter{Name: "W", Required: key.ModShortcut},
			key.Filter{Name: key.NameF12, Optional: key.ModShortcut},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "+", Required: key.ModShortcut | key.ModAlt, Optional: key.ModShift},
			key.Filter{Name: "=", Required: key.ModShortcut | key.ModAlt, Optional: key.ModShift},
			key.Filter{Name: "-", Required: key.ModShortcut | key.ModAlt},
			key.Filter{Name: "0", Required: key.ModShortcut},
			key.Filter{Name: key.NameF9},
			key.Filter{Name: key.NameF11, Optional: key.ModShift},
			key.Filter{Name: key.NameF1, Required: key.ModAlt},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		if e.Name != key.NameEscape {
			if a.modalOpen() {
				continue // the dialog has the keyboard; Esc closes it below
			}
			switch e.Name {
			case key.NameF9, key.NameF11, key.NameF1, "+", "=", "-", "0":
				if a.back.Visible {
					continue // the File page has no sheet to act on
				}
			}
			if a.rename.active {
				// A tab being renamed takes its new name first, as when it
				// loses the focus; a name that is refused keeps it open.
				if a.finishRename(true); a.rename.active {
					continue
				}
			}
		}
		switch e.Name {
		case "N":
			a.launch(doctype.Spreadsheet, "")
		case "O":
			a.menu.Close()
			a.back.Show(gui.PageOpen)
		case "S":
			a.menu.Close()
			a.save()
		case "P":
			a.menu.Close()
			a.back.Show(gui.PagePrint)
		case "W":
			a.win.Perform(system.ActionClose)
		case key.NameF12:
			a.menu.Close()
			if e.Modifiers.Contain(key.ModShortcut) {
				a.openDialog() // Ctrl+F12, as in Excel
			} else {
				a.back.Show(gui.PageSaveAs)
			}
		case key.NameEscape:
			switch {
			case a.dialog.Visible:
				a.dialog.Hide()
				if answer := a.answer; answer != nil {
					a.answer = nil
					answer(len(a.dialog.Buttons) - 1) // Cancel
				}
				continue
			case a.menu.IsOpen():
				a.menu.Close()
				continue
			case a.filt.open:
				a.closeFilterMenu() // a filter drop-down (filter.go)
				continue
			case a.vlist.open:
				a.closeValidList() // a validation list (validation.go)
				continue
			case a.cm.grid.IsOpen() || a.cm.tabs.IsOpen():
				a.cm.grid.Close() // right-click menus (ctxmenu.go)
				a.cm.tabs.Close()
				continue
			case a.ins.kind != dlgNone:
				a.closeDialog()
				continue
			case a.ch.dlg.kind != cdNone:
				a.closeChartDialog() // chartdlg.go
				continue
			case a.fmtDlg.open:
				a.fmtDlg.key = -1 // formatdialog.go closes it on the next layout
				continue
			case a.askDlg.open:
				a.askDlg.key = -1
				continue
			case a.pv.dlg.open:
				a.closePivotDialog() // pivotdlg.go
				continue
			case a.pv.menu.open:
				a.closePivotMenu() // pivotmenu.go
				continue
			case a.pv.ctx.IsOpen() || a.pv.pane.menu.IsOpen():
				a.pv.ctx.Close() // pivot menus (pivot.go, pivotpane.go)
				a.pv.pane.menu.Close()
				continue
			case a.back.Visible:
				a.back.Hide()
				a.closing = false
			case a.rename.active:
				a.finishRename(false)
			case a.ch.title.active:
				a.finishTitleEdit(false) // chart.go
			case a.ed.active:
				a.cancelEdit()
			case a.home.painterClip != nil:
				a.stopPainter() // Format Painter (format.go)
				continue
			case a.state().view.SelElem.Kind > render.ElemChartArea && a.state().view.SelChart != nil:
				a.state().view.SelElem = render.ChartElem{Kind: render.ElemChartArea} // a chart's part, then the chart
			default:
				a.clip, a.ch.clip = nil, nil
				a.state().view.SelChart, a.state().view.SelElem = nil, render.ChartElem{}
			}
			a.grid.focusPending = true
		case "+", "=":
			a.setZoom(a.zoom + 0.1)
		case "-":
			a.setZoom(a.zoom - 0.1)
		case "0":
			a.setZoom(1)
		case key.NameF9:
			a.recalculate()
		case key.NameF11:
			if e.Modifiers.Contain(key.ModShift) {
				a.insertSheet()
			} else {
				a.insertChartSheet() // chart.go
			}
		case key.NameF1:
			a.insertChart(edit.ColumnClustered) // Alt+F1: the default chart beside the data
		}
	}
}

// label is a one-line UI-font label of s in color c, bold or not.
func (a *App) label(size float32, s string, c color.NRGBA, bold bool) material.LabelStyle {
	l := material.Label(a.th, unit.Sp(size), s)
	l.Color = c
	l.Font = gui.UIFont
	if bold {
		l.Font.Weight = font.Bold
	}
	l.MaxLines = 1
	return l
}

// titleBar draws the app name, the AutoSave switch and the workbook's
// name with its modified mark and AutoSave status.
func (a *App) titleBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(36)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, gtx.Constraints.Max.X, h), accent)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	name := a.docName()
	if a.modified {
		name += " •"
	}
	if st := a.autoSaveStatus(gtx); st != "" {
		name += " – " + st
	}
	return layout.Inset{Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(a.label(15, appName, colors.White, true).Layout),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(a.autoSaveSwitch), // autosave.go
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Flexed(1, a.label(13, name, colors.White, false).Layout),
			)
		})
	})
}

// zoomControl is the status bar's "−  100%  +".
func (a *App) zoomControl(gtx layout.Context) layout.Dimensions {
	for a.zoomInBtn.Clicked(gtx) {
		a.setZoom(a.zoom + 0.1)
	}
	for a.zoomOutBtn.Clicked(gtx) {
		a.setZoom(a.zoom - 0.1)
	}
	for a.zoomBtn.Clicked(gtx) {
		a.setZoom(1)
	}
	btn := func(c *widget.Clickable, s string, w int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.Hoverable(gtx, c, colors.Transparent, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(w)), gtx.Constraints.Max.Y))
				return layout.Center.Layout(gtx, a.label(12, s, chromeText, false).Layout)
			})
		})
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		btn(&a.zoomOutBtn, "−", 22),
		btn(&a.zoomBtn, fmt.Sprintf("%d%%", int(a.zoom*100+0.5)), 44),
		btn(&a.zoomInBtn, "+", 22),
	)
}

// formulaBar draws the Name Box (the active cell, or the size of a range
// being dragged), fx and the editor of the active cell's content.
func (a *App) formulaBar(gtx layout.Context) layout.Dimensions {
	st := a.state()
	sel := st.view.Sel
	name := model.CellName(sel.Active.Row, sel.Active.Col)
	if r := sel.Range; a.grid.dragging && (r.R1 != r.R2 || r.C1 != r.C2) {
		name = fmt.Sprintf("%dR x %dC", r.R2-r.R1+1, r.C2-r.C1+1)
	}
	content := editText(st.view.Sheet, sel.Active)

	h := gtx.Dp(30)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	w := gtx.Constraints.Max.X
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), colors.White)
	gui.HLine(gtx.Ops, 0, w, h-1, 1, chromeLine)

	box := gtx.Dp(100)
	gui.VLine(gtx.Ops, box, 0, h, 1, chromeLine)
	gui.VLine(gtx.Ops, box+gtx.Dp(32), gtx.Dp(6), h-gtx.Dp(6), 1, chromeLine)
	place := func(x int, l material.LabelStyle) {
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(max(w-x-gtx.Dp(8), 0), h)}
		m := op.Record(gtx.Ops)
		d := l.Layout(g)
		call := m.Stop()
		off := op.Offset(image.Pt(x, (h-d.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
	}
	place(gtx.Dp(8), a.label(13, name, chromeText, false))
	fx := a.label(13, "fx", mutedText, false)
	fx.Font.Style = font.Italic
	place(box+gtx.Dp(10), fx)
	x := box + gtx.Dp(42)
	off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	a.layoutBarEditor(gtx, content, max(w-x-gtx.Dp(8), 1), h-1)
	off.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// looksNumeric reports whether formatted text is a plain number (as opposed
// to a date or time), in which case the raw value is more useful.
func looksNumeric(s string) bool {
	s = strings.NewReplacer(",", "", "$", "", "%", "", "(", "", ")", "", " ", "").Replace(s)
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// vScrollbar draws the grid's vertical scrollbar and scrolls the rows
// below the frozen ones as it is dragged.
func (a *App) vScrollbar(gtx layout.Context) layout.Dimensions {
	st := a.state()
	v := &st.view
	w := gtx.Dp(14)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	fr := v.Sheet.FreezeRows
	visible := float64(v.VisibleRows())
	rows, _ := a.dataExtent(st)
	total := float64(max(rows, v.TopRow, st.selEnd.Row+1)-fr) + visible
	a.scrollHints()
	pos := a.vbar.Layout(gtx, float64(v.TopRow-fr), visible, total)
	if top := fr + int(pos+0.5); top != v.TopRow {
		v.TopRow = min(top, model.MaxRows-1)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// bottomBar draws the sheet tabs and the horizontal scrollbar.
func (a *App) bottomBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(30)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	gui.HLine(gtx.Ops, 0, gtx.Constraints.Max.X, 0, 1, chromeLine)
	barW := gtx.Constraints.Max.X * 2 / 5
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(1, a.sheetTabsWithMenu), // ctxmenu.go
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(barW, h))
			st := a.state()
			v := &st.view
			fc := v.Sheet.FreezeCols
			visible := float64(v.VisibleCols())
			_, cols := a.dataExtent(st)
			total := float64(max(cols, v.LeftCol, st.selEnd.Col+1)-fc) + visible
			ins := layout.Inset{Top: 8, Bottom: 8, Right: 14}
			return ins.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				pos := a.hbar.Layout(gtx, float64(v.LeftCol-fc), visible, total)
				if left := fc + int(pos+0.5); left != v.LeftCol {
					v.LeftCol = min(left, model.MaxCols-1)
					gtx.Execute(op.InvalidateCmd{})
				}
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
	)
}

// sheetTabs handles clicks on the sheet tabs (a double-click renames
// one) and the + button, and draws them.
func (a *App) sheetTabs(gtx layout.Context) layout.Dimensions {
	for i := range a.tabBtns {
		for {
			click, ok := a.tabBtns[i].Update(gtx)
			if !ok {
				break
			}
			if a.ed.active && !a.commitEdit(0, 0) {
				continue
			}
			if a.rename.active && a.rename.sheet != i {
				a.finishRename(true)
			}
			a.active = i
			a.grid.focusPending = true
			if click.NumClicks >= 2 {
				a.startRename(i) // double-click a tab to rename it
			}
		}
	}
	a.renameEvents(gtx)
	for a.newSheetBtn.Clicked(gtx) {
		a.newSheet()
	}
	a.tabList.Axis = layout.Horizontal
	return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// The tabs, then the + that adds a sheet right after them.
		btnW := gtx.Dp(30)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = max(gtx.Constraints.Max.X-btnW, 0)
				return a.tabStrip(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return gui.ToolButton(gtx, a.ui, &a.newSheetBtn, gui.IconNew, "", false, color.NRGBA{})
				})
			}),
		)
	})
}

// tabStrip lays out the sheet tabs.
func (a *App) tabStrip(gtx layout.Context) layout.Dimensions {
	return a.tabList.Layout(gtx, len(a.wb.Sheets), func(gtx layout.Context, i int) layout.Dimensions {
		sh := a.wb.Sheets[i]
		if sh.Hidden {
			return layout.Dimensions{}
		}
		activeTab := i == a.active
		if a.rename.active && a.rename.sheet == i {
			return a.renameBox(gtx)
		}
		return a.tabBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			m := op.Record(gtx.Ops)
			c := chromeText
			if activeTab {
				c = accent
			}
			l := a.label(13, sh.Name, c, activeTab)
			d := layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, l.Layout)
			call := m.Stop()
			if activeTab {
				gui.FillRect(gtx.Ops, image.Rectangle{Max: d.Size}, tabActiveBG)
				gui.HLine(gtx.Ops, gtx.Dp(8), d.Size.X-gtx.Dp(8), d.Size.Y-gtx.Dp(3), gtx.Dp(2), accent)
			}
			call.Add(gtx.Ops)
			gui.VLine(gtx.Ops, d.Size.X-1, gtx.Dp(6), d.Size.Y-gtx.Dp(6), 1, chromeLine)
			return d
		})
	})
}

// statusBar draws the mode or message on the left, the selection's
// statistics and the zoom control on the right.
func (a *App) statusBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(24)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	w := gtx.Constraints.Max.X
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeBG)
	gui.HLine(gtx.Ops, 0, w, 0, 1, chromeLine)

	left, leftColor := "Ready", mutedText
	switch {
	case a.loading.Load():
		left = "Loading…"
	case a.errMsg != "":
		left, leftColor = a.errMsg, errorText
	case a.notice != "":
		left = a.notice
	case a.clip != nil:
		left = a.clipNotice()
	case a.ed.active && a.ed.enter:
		left = "Enter"
	case a.ed.active:
		left = "Edit"
	case len(a.wb.Warnings) > 0:
		left = a.wb.Warnings[0]
		if n := len(a.wb.Warnings); n > 1 {
			left += fmt.Sprintf(" (+%d more)", n-1)
		}
	}
	st := a.state()
	right := a.stats.text(st.view.Sheet, st.view.Sel.Range)
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, a.label(12, left, leftColor, false).Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := a.label(12, right, chromeText, false)
					l.Alignment = text.End
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: 16}.Layout),
				layout.Rigid(a.zoomControl),
			)
		})
	})
}

// renameEvents finishes a tab rename on Enter or when the editor loses
// the focus.
func (a *App) renameEvents(gtx layout.Context) {
	rn := &a.rename
	if !rn.active {
		return
	}
	if rn.focus {
		requestFocus(gtx, &rn.ed)
		rn.focus = false
		return
	}
	for {
		ev, ok := rn.ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.finishRename(true)
			return
		}
	}
	if !gtx.Focused(&rn.ed) {
		a.finishRename(true)
	}
}

// renameBox is the editor that replaces a tab being renamed.
func (a *App) renameBox(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(120)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
		m := op.Record(gtx.Ops)
		d := layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.rename.ed.Layout(gtx, a.th.Shaper, gui.UIFont, unit.Sp(13), gui.Material(gtx.Ops, chromeText), gui.Material(gtx.Ops, editSelection))
		})
		call := m.Stop()
		gui.FillRect(gtx.Ops, image.Rectangle{Max: d.Size}.Inset(-1), accent)
		gui.FillRect(gtx.Ops, image.Rectangle{Max: d.Size}, tabActiveBG)
		call.Add(gtx.Ops)
		return d
	})
}
