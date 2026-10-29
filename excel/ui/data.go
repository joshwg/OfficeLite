package ui

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
)

// The Data tab: its menu and ribbon, and the AutoFilter commands (the
// drop-downs in the header cells are in filter.go). The Data Tools
// group's dialogs are in datatools.go and validation.go, the Outline
// group's commands in outline.go.

// Ribbons the Home and Data tabs show.
const (
	tabHome = iota
	tabData
)

// Data menu items.
const (
	dataSortAsc = iota
	dataSortDesc
	dataCustomSort
	_
	dataFilter
	dataClear
	dataReapply
	_
	dataTextToCols
	dataRemoveDups
	dataValidation
	dataCircle
	dataUncircle
)

// The Data Validation button's menu.
var validItems = []string{"Data Validation…", "Circle Invalid Data", "Clear Validation Circles"}

// dataRibbon is the Data ribbon's widget state.
type dataRibbon struct {
	ribbon                  gui.Ribbon
	sortAsc, sortDesc, sort widget.Clickable
	filter, clear, reapply  widget.Clickable
	textToCols, removeDups  widget.Clickable
	validDD                 gui.DropDown
	group, subtotal         widget.Clickable
	ungroupDD               gui.DropDown
	showDetail, hideDetail  widget.Clickable
	tipsSet                 bool
}

func (d *dataRibbon) setTips() {
	tip := gui.SetTip
	tip(&d.sortAsc, "Sort A to Z", "", "Sort the selection so the lowest values are at the top of the column.")
	tip(&d.sortDesc, "Sort Z to A", "", "Sort the selection so the highest values are at the top of the column.")
	tip(&d.sort, "Sort", "", "Find values quickly by sorting your data by several columns at once.")
	tip(&d.filter, "Filter", "Ctrl+Shift+L", "Turn on filtering for the selected cells. Then click the arrow in the column header to narrow down the data.")
	tip(&d.clear, "Clear", "", "Clear the filter and sort state for the current range of data.")
	tip(&d.reapply, "Reapply", "", "Reapply the filter and sort in the current range. New or modified data in the column won't be included until you click Reapply.")
	tip(&d.textToCols, "Text to Columns", "", "Split a single column of text into multiple columns. For example, separate a column of full names into first and last name columns.")
	tip(&d.removeDups, "Remove Duplicates", "", "Delete duplicate rows from a sheet. You can choose which columns should be checked for duplicate information.")
	tip(&d.validDD, "Data Validation", "", "Choose from a list of rules to limit the type of data that can be entered in a cell. For example, you can provide a list of values, like 1, 2, and 3, or only allow numbers greater than 1000 as valid entries.")
	tip(&d.group, "Group", "Shift+Alt+Right Arrow", "Tie a range of cells together so that they can be collapsed or expanded.")
	tip(&d.ungroupDD, "Ungroup", "Shift+Alt+Left Arrow", "Ungroup a range of cells that were previously grouped, or clear the whole outline.")
	tip(&d.subtotal, "Subtotal", "", "Total several rows of related data together by automatically inserting subtotals and totals for the selected cells.")
	tip(&d.showDetail, "Show Detail", "", "Expand a collapsed group of cells.")
	tip(&d.hideDetail, "Hide Detail", "", "Collapse a group of cells.")
}

// filterState reports whether the active sheet has an AutoFilter and
// whether some column of it has criteria.
func (a *App) filterState() (on, filtered bool) {
	f := a.wb.Sheets[a.active].AutoFilter
	return f != nil, f.Active()
}

func (a *App) dataMenuDef() gui.Menu {
	editing := a.ed.active
	on, filtered := a.filterState()
	return gui.Menu{Label: "Data", Tab: true, Selected: a.tab == tabData, Items: []gui.MenuItem{
		dataSortAsc:    {Label: "Sort A to Z", Disabled: editing, Tip: "Sort the selection so the lowest values are at the top of the column."},
		dataSortDesc:   {Label: "Sort Z to A", Disabled: editing, Tip: "Sort the selection so the highest values are at the top of the column."},
		dataCustomSort: {Label: "Sort…", Disabled: editing, Tip: "Sort by several columns at once."},
		3:              {Separator: true},
		dataFilter:     {Label: "Filter", Shortcut: "Ctrl+Shift+L", Checked: on, Disabled: editing, Tip: "Turn filtering on or off for the data around the selection."},
		dataClear:      {Label: "Clear", Disabled: editing || !filtered, Tip: "Show every row the filter hides."},
		dataReapply:    {Label: "Reapply", Disabled: editing || !filtered, Tip: "Filter again after the data changed."},
		7:              {Separator: true},
		dataTextToCols: {Label: "Text to Columns…", Disabled: editing, Tip: "Split a column of text into several columns."},
		dataRemoveDups: {Label: "Remove Duplicates…", Disabled: editing, Tip: "Delete rows that repeat values in the columns you choose."},
		dataValidation: {Label: "Data Validation…", Disabled: editing, Tip: "Limit what can be typed in the selected cells."},
		dataCircle:     {Label: "Circle Invalid Data", Disabled: editing, Tip: "Circle the cells whose values break their validation rules."},
		dataUncircle:   {Label: "Clear Validation Circles", Disabled: a.state().circles == nil, Tip: "Remove the circles."},
	}}
}

func (a *App) dataChosen(item int) {
	switch item {
	case dataSortAsc:
		a.sortAscending()
	case dataSortDesc:
		a.sortDescending()
	case dataCustomSort:
		a.customSortDialog()
	case dataFilter:
		a.toggleFilter()
	case dataClear:
		a.clearFilter()
	case dataReapply:
		a.reapplyFilter()
	case dataTextToCols:
		a.textToColumnsDialog()
	case dataRemoveDups:
		a.removeDuplicatesDialog()
	case dataValidation:
		a.validationDialog()
	case dataCircle:
		a.circleInvalid()
	case dataUncircle:
		a.clearCircles()
	}
}

// validMenu runs an item of the Data Validation button's menu.
func (a *App) validMenu(i int) { a.dataChosen(dataValidation + i) }

// dataKeys handles Ctrl+Shift+L (Filter), Alt+Down (the active header
// cell's filter drop-down, or its validation list) and Shift+Alt+Right /
// Left (Group, Ungroup) while the grid has the focus.
func (a *App) dataKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &a.grid, Name: "L", Required: key.ModShortcut | key.ModShift},
			key.Filter{Focus: &a.grid, Name: key.NameDownArrow, Required: key.ModAlt},
			key.Filter{Focus: &a.grid, Name: key.NameRightArrow, Required: key.ModAlt | key.ModShift},
			key.Filter{Focus: &a.grid, Name: key.NameLeftArrow, Required: key.ModAlt | key.ModShift},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch e.Name {
		case "L":
			a.toggleFilter()
			continue
		case key.NameRightArrow:
			a.groupCmd()
			continue
		case key.NameLeftArrow:
			a.ungroupCmd()
			continue
		}
		act := a.state().view.Sel.Active
		if f := a.wb.Sheets[a.active].AutoFilter; f != nil && act.Row == f.Range.R1 && act.Col >= f.Range.C1 && act.Col <= f.Range.C2 {
			a.openFilterMenu(act.Col)
			continue
		}
		a.openValidList()
	}
}

// layoutDataRibbon draws the Data ribbon and runs its commands.
func (a *App) layoutDataRibbon(gtx layout.Context) layout.Dimensions {
	d := &a.data
	if !d.tipsSet {
		d.setTips()
		d.tipsSet = true
	}
	th := a.ui
	k := &gui.RibbonKit{Th: th}
	click := func(b *widget.Clickable, f func()) { k.Click(gtx, b, f) }
	click(&d.sortAsc, a.sortAscending)
	click(&d.sortDesc, a.sortDescending)
	click(&d.sort, a.customSortDialog)
	click(&d.filter, a.toggleFilter)
	click(&d.clear, a.clearFilter)
	click(&d.reapply, a.reapplyFilter)
	click(&d.textToCols, a.textToColumnsDialog)
	click(&d.removeDups, a.removeDuplicatesDialog)
	click(&d.group, a.groupCmd)
	click(&d.subtotal, a.subtotalDialog)
	click(&d.showDetail, a.showDetail)
	click(&d.hideDetail, a.hideDetail)

	on, filtered := a.filterState()
	sh := a.wb.Sheets[a.active]
	outlined := sh.MaxLevel(true) > 0 || sh.MaxLevel(false) > 0
	small := func(b *widget.Clickable, ic *gui.Icon, label string, enabled bool) layout.Widget {
		return gui.Enabled(enabled, k.Small(b, ic, label))
	}
	big := k.BigBtn
	row, stack := gui.RibbonRow, gui.RibbonStack
	groups := []gui.RibbonGroup{
		{Caption: "Sort & Filter", Content: gui.RibbonSide(2,
			stack(
				row(k.Btn(&d.sortAsc, gui.IconSortAZ, false)),
				row(k.Btn(&d.sortDesc, gui.IconSortZA, false)),
			),
			big(&d.sort, gui.IconSortCustom, "Sort", false),
			big(&d.filter, gui.IconFilter, "Filter", on),
			stack(
				row(small(&d.clear, gui.IconFilterClear, "Clear", filtered)),
				row(small(&d.reapply, gui.IconReapply, "Reapply", filtered)),
			),
		)},
		{Caption: "Data Tools", Content: gui.RibbonSide(2,
			big(&d.textToCols, gui.IconTextToCols, "Text to Columns", false),
			stack(
				row(small(&d.removeDups, gui.IconRemoveDups, "Remove Duplicates", true)),
				row(k.Drop(&d.validDD, gui.IconValidation, "Data Validation", gui.MenuItems(validItems), 190, a.validMenu)),
			),
		)},
		{Caption: "Outline", Content: gui.RibbonSide(2,
			big(&d.group, gui.IconGroup, "Group", false),
			k.BigDrop(&d.ungroupDD, gui.IconUngroup, "Ungroup", gui.MenuItems(ungroupItems), 160, a.ungroupMenu),
			big(&d.subtotal, gui.IconSubtotal, "Subtotal", false),
			stack(
				row(small(&d.showDetail, gui.IconShowDetail, "Show Detail", outlined)),
				row(small(&d.hideDetail, gui.IconHideDetail, "Hide Detail", outlined)),
			),
		)},
		a.chartsRibbonGroup(gtx, k), // Insert > Charts (chartribbon.go)
	}
	dims := d.ribbon.Layout(gtx, th, groups...)
	if k.Did && !a.ed.active && !a.rename.active && a.ins.kind == dlgNone {
		a.grid.focusPending = true
	}
	return dims
}

// toggleFilter turns the AutoFilter on for the selection (or the data
// around the active cell), or off (Data > Filter, Ctrl+Shift+L).
func (a *App) toggleFilter() {
	if !a.readyToChange() {
		return
	}
	a.menu.Close()
	sh := a.wb.Sheets[a.active]
	if sh.AutoFilter != nil {
		a.closeFilterMenu()
		a.book.SetAutoFilter(a.active, nil)
		a.afterFilter(false)
		return
	}
	if !a.singleRange() {
		return
	}
	r, err := edit.FilterRange(sh, a.state().view.Sel.Range)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.book.SetAutoFilter(a.active, &r)
	a.afterFilter(false)
}

// clearFilter shows every row the filter hides (Data > Clear).
func (a *App) clearFilter() {
	if _, filtered := a.filterState(); !filtered || !a.readyToChange() {
		return
	}
	a.book.ClearFilters(a.active)
	a.afterFilter(false)
}

// reapplyFilter filters again after the data changed (Data > Reapply).
func (a *App) reapplyFilter() {
	if _, filtered := a.filterState(); !filtered || !a.readyToChange() {
		return
	}
	a.book.ReapplyFilter(a.active)
	a.afterFilter(true)
}

// setFilterColumn applies a column's criteria (nil clears them).
func (a *App) setFilterColumn(col int, fc *model.FilterColumn) {
	if !a.readyToChange() {
		return
	}
	a.book.FilterColumn(a.active, col, fc)
	a.afterFilter(true)
}

// afterFilter updates the view after the filter changed: the rows it
// hides, and with count, Excel's "N of M records found".
func (a *App) afterFilter(count bool) {
	a.syncSheets()
	a.changed(calc.Result{})
	sh := a.wb.Sheets[a.active]
	if count && sh.AutoFilter.Active() {
		n, m := edit.FilterCount(sh)
		a.notice = fmt.Sprintf("%d of %d records found", n, m)
	}
	a.grid.focusPending = true
}
