package ui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// The AutoFilter drop-down that opens from a header cell's button, as
// Excel's: sorting by the column, Clear Filter, the Text / Number / Date
// Filters submenu, a search box and the checklist of the column's values.

// filterMenu is an open AutoFilter drop-down.
type filterMenu struct {
	open  bool
	col   int         // sheet column
	at    image.Point // top-left, in grid coordinates
	kind  edit.ColumnKind
	items []edit.FilterItem
	rows  []widget.Clickable // one per item

	all                      widget.Clickable // (Select All)
	sortAsc, sortDesc, clear widget.Clickable
	sub                      widget.Clickable // Text / Number / Date Filters
	ok, cancel               widget.Clickable
	search                   widget.Editor
	list                     widget.List
	subMenu                  gui.ContextMenu
	subActs                  []func()
	subY                     int // the Filters row's top, for the submenu
	catcher, body            int // event tags: clicks outside, inside
	focus                    bool
}

// Drop-down metrics, in dp.
const (
	filterMenuWidth = 280
	filterRowH      = 26
	filterListH     = 190
	filterCheckH    = 22
)

// openFilterMenu opens column col's drop-down below its button.
func (a *App) openFilterMenu(col int) {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	f := v.Sheet.AutoFilter
	if f == nil {
		return
	}
	rc, _, ok := v.CellRect(f.Range.R1, col)
	if !ok {
		return
	}
	fm := &a.filt
	*fm = filterMenu{open: true, col: col, at: image.Pt(rc.Min.X, rc.Max.Y+1), focus: true}
	fm.kind = a.book.FilterColumnKind(a.active, col)
	fm.items = a.book.FilterItems(a.active, col)
	fm.rows = make([]widget.Clickable, len(fm.items))
	fm.search.SingleLine, fm.search.Submit = true, true
	fm.list.Axis = layout.Vertical
	a.menu.Close()
	a.notice = ""
}

// closeFilterMenu closes the drop-down without applying it.
func (a *App) closeFilterMenu() {
	if !a.filt.open {
		return
	}
	a.filt.open = false
	a.filt.subMenu.Close()
	a.grid.focusPending = true
	if a.gtx.Ops != nil {
		requestFocus(a.gtx, &a.grid) // now: keys typed right after go to the grid
	}
}

// filterPress opens the drop-down whose button is at pos; it reports
// whether there was one. The buttons take a click before the cells.
func (a *App) filterPress(v *render.View, x, y float32) bool {
	col, ok := v.FilterButtonAt(x, y)
	if !ok {
		return false
	}
	if a.filt.open && a.filt.col == col {
		a.closeFilterMenu() // a second click closes it, as in Excel
		return true
	}
	a.openFilterMenu(col)
	return true
}

// filterCursors shows the arrow over the AutoFilter buttons.
func (a *App) filterCursors(gtx layout.Context, v *render.View) {
	f := v.Sheet.AutoFilter
	if f == nil {
		return
	}
	for c := f.Range.C1; c <= f.Range.C2; c++ {
		if b, ok := v.FilterButtonRect(c); ok {
			area := clip.Rect(b).Push(gtx.Ops)
			pointer.CursorDefault.Add(gtx.Ops)
			area.Pop()
		}
	}
}

// headerName is how the drop-down names its column: the header text, or
// "Column B".
func (a *App) headerName(col int) string {
	sh := a.wb.Sheets[a.active]
	if f := sh.AutoFilter; f != nil {
		if t := sh.Get(f.Range.R1, col).Text; t != "" {
			return t
		}
	}
	return "Column " + model.ColName(col)
}

// shownItems are the indexes of the checklist entries matching the
// search text.
func (fm *filterMenu) shownItems() []int {
	q := strings.ToLower(strings.TrimSpace(fm.search.Text()))
	var out []int
	for i, it := range fm.items {
		label := it.Text
		if label == "" {
			label = "(blanks)"
		}
		if q == "" || strings.Contains(strings.ToLower(label), q) {
			out = append(out, i)
		}
	}
	return out
}

// applyChecklist filters by the checked values shown (all of them shown
// and checked clears the column's criteria).
func (a *App) applyChecklist() {
	fm := &a.filt
	shown := fm.shownItems()
	all := strings.TrimSpace(fm.search.Text()) == ""
	fc := &model.FilterColumn{Kind: model.FilterValues}
	for _, i := range shown {
		it := fm.items[i]
		switch {
		case !it.Checked:
			all = false
		case it.Text == "":
			fc.Blanks = true
		default:
			fc.Values = append(fc.Values, it.Text)
		}
	}
	if !fc.Blanks && len(fc.Values) == 0 {
		return // Excel keeps OK disabled
	}
	col := fm.col
	a.closeFilterMenu()
	if all {
		a.setFilterColumn(col, nil)
		return
	}
	a.setFilterColumn(col, fc)
}

// sortFiltered sorts the AutoFilter's range by col when the selection is
// in it (Sort A to Z with a filter on); false otherwise.
func (a *App) sortFiltered(col int, desc bool) bool {
	sh := a.wb.Sheets[a.active]
	f := sh.AutoFilter
	act := a.state().view.Sel.Active
	sel := a.state().view.Sel.Range
	if f == nil || !f.Range.Contains(act.Row, act.Col) || !f.Range.Contains(sel.R1, sel.C1) || !f.Range.Contains(sel.R2, sel.C2) {
		return false
	}
	a.sortFilterColumn(col, desc)
	return true
}

// sortFilterColumn sorts the filter's rows by col and filters again.
func (a *App) sortFilterColumn(col int, desc bool) {
	if !a.readyToChange() {
		return
	}
	res, err := a.book.SortFiltered(a.active, col, desc)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(res)
	a.grid.focusPending = true
}

// sortLabels are the drop-down's sort commands for a column kind.
func sortLabels(k edit.ColumnKind) (asc, desc string) {
	switch k {
	case edit.ColumnNumber:
		return "Sort Smallest to Largest", "Sort Largest to Smallest"
	case edit.ColumnDate:
		return "Sort Oldest to Newest", "Sort Newest to Oldest"
	}
	return "Sort A to Z", "Sort Z to A"
}

// filtersLabel names the submenu for a column kind.
func filtersLabel(k edit.ColumnKind) string {
	switch k {
	case edit.ColumnNumber:
		return "Number Filters"
	case edit.ColumnDate:
		return "Date Filters"
	}
	return "Text Filters"
}

// subItems is the Text / Number / Date Filters submenu.
func (a *App) subItems(col int, k edit.ColumnKind) []ctxItem {
	custom := func(op1, op2 int, and bool) func() {
		return func() { a.customFilterDialog(col, op1, op2, and) }
	}
	dyn := func(t string) func() {
		return func() { a.setFilterColumn(col, &model.FilterColumn{Kind: model.FilterDynamic, Dynamic: t}) }
	}
	item := func(label, tip string, run func()) ctxItem { return ctxItem{label: label, tip: tip, run: run} }
	var items []ctxItem
	switch k {
	case edit.ColumnNumber:
		items = []ctxItem{
			item("Equals…", "Show values equal to a number.", custom(opEquals, 0, true)),
			item("Does Not Equal…", "Hide values equal to a number.", custom(opNotEquals, 0, true)),
			item("Greater Than…", "Show values greater than a number.", custom(opGreater, 0, true)),
			item("Greater Than Or Equal To…", "Show values greater than or equal to a number.", custom(opGreaterEq, 0, true)),
			item("Less Than…", "Show values less than a number.", custom(opLess, 0, true)),
			item("Less Than Or Equal To…", "Show values less than or equal to a number.", custom(opLessEq, 0, true)),
			item("Between…", "Show values between two numbers.", custom(opGreaterEq, opLessEq, true)),
			item("Top 10…", "Show the largest or smallest values.", func() { a.top10Dialog(col) }),
			item("Above Average", "Show values above the column's average.", dyn("aboveAverage")),
			item("Below Average", "Show values below the column's average.", dyn("belowAverage")),
		}
	case edit.ColumnDate:
		items = []ctxItem{
			item("Equals…", "Show a date.", custom(opEquals, 0, true)),
			item("Before…", "Show dates before a date.", custom(opLess, 0, true)),
			item("After…", "Show dates after a date.", custom(opGreater, 0, true)),
			item("Between…", "Show dates between two dates.", custom(opGreaterEq, opLessEq, true)),
		}
	default:
		items = []ctxItem{
			item("Equals…", "Show text equal to a value.", custom(opEquals, 0, true)),
			item("Does Not Equal…", "Hide text equal to a value.", custom(opNotEquals, 0, true)),
			{},
			item("Begins With…", "Show text that begins with a value.", custom(opBegins, 0, true)),
			item("Ends With…", "Show text that ends with a value.", custom(opEnds, 0, true)),
			{},
			item("Contains…", "Show text that contains a value.", custom(opContains, 0, true)),
			item("Does Not Contain…", "Hide text that contains a value.", custom(opNotContains, 0, true)),
		}
	}
	return append(items, ctxItem{}, item("Custom Filter…", "Filter by one or two conditions.", custom(-1, 0, true)))
}

// layoutFilterMenu draws the open drop-down (in grid coordinates, above
// everything else) and runs its commands.
func (a *App) layoutFilterMenu(gtx layout.Context) {
	fm := &a.filt
	if !fm.open {
		return
	}
	col := fm.col
	for fm.sortAsc.Clicked(gtx) {
		a.closeFilterMenu()
		a.sortFilterColumn(col, false)
		return
	}
	for fm.sortDesc.Clicked(gtx) {
		a.closeFilterMenu()
		a.sortFilterColumn(col, true)
		return
	}
	for fm.clear.Clicked(gtx) {
		a.closeFilterMenu()
		a.setFilterColumn(col, nil)
		return
	}
	for fm.cancel.Clicked(gtx) {
		a.closeFilterMenu()
		return
	}
	for fm.ok.Clicked(gtx) {
		a.applyChecklist()
		return
	}
	for {
		ev, ok := fm.search.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.applyChecklist()
			return
		}
	}
	shown := fm.shownItems()
	allOn := true
	for _, i := range shown {
		allOn = allOn && fm.items[i].Checked
	}
	for fm.all.Clicked(gtx) {
		for _, i := range shown {
			fm.items[i].Checked = !allOn
		}
		allOn = !allOn
	}
	for _, i := range shown {
		for fm.rows[i].Clicked(gtx) {
			fm.items[i].Checked = !fm.items[i].Checked
		}
	}
	for fm.sub.Clicked(gtx) {
		items := a.subItems(col, fm.kind)
		openCtx(&fm.subMenu, &fm.subActs, image.Pt(fm.at.X+gtx.Dp(filterMenuWidth)-gtx.Dp(4), fm.at.Y+fm.subY), items)
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &fm.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			a.closeFilterMenu()
			return
		}
	}
	if fm.focus {
		fm.focus = false
		gtx.Execute(key.FocusCmd{Tag: &fm.search})
	}

	sh := a.wb.Sheets[a.active]
	cur, filtered := sh.AutoFilter.Column(col)
	advanced := filtered && cur.Kind != model.FilterValues
	anyOn := false
	for _, i := range shown {
		anyOn = anyOn || fm.items[i].Checked
	}

	rec := op.Record(gtx.Ops)
	big := image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)
	area := clip.Rect(big).Push(gtx.Ops)
	event.Op(gtx.Ops, &fm.catcher)
	area.Pop()

	w := gtx.Dp(filterMenuWidth)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, 1<<15)}
	body := op.Record(gtx.Ops)
	asc, desc := sortLabels(fm.kind)
	y := 0 // the next row's top, for the submenu's position
	row := func(w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := w(gtx)
			y += d.Size.Y
			return d
		})
	}
	d := layout.Inset{Top: 4, Bottom: 8}.Layout(g, func(gtx layout.Context) layout.Dimensions {
		y = gtx.Dp(4)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			row(a.filterRow(&fm.sortAsc, gui.IconSortAZ, asc, false, false, false)),
			row(a.filterRow(&fm.sortDesc, gui.IconSortZA, desc, false, false, false)),
			row(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(9)
				gui.HLine(gtx.Ops, gtx.Dp(8), gtx.Constraints.Min.X-gtx.Dp(8), h/2, 1, chromeLine)
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, h)}
			}),
			row(a.filterRow(&fm.clear, gui.IconFilterClear, `Clear Filter From "`+a.headerName(col)+`"`, !filtered, false, false)),
			row(func(gtx layout.Context) layout.Dimensions {
				fm.subY = y
				return a.filterRow(&fm.sub, nil, filtersLabel(fm.kind), false, true, advanced)(gtx)
			}),
			row(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 4, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return gui.TextField(gtx, a.ui, &fm.search, unit.Dp(filterMenuWidth-16), 1, "Search")
				})
			}),
			row(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.filterChecklist(gtx, shown, allOn)
				})
			}),
			row(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, layout.Spacer{}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !anyOn {
								gtx = gtx.Disabled()
							}
							return a.accentButton(&fm.ok, "OK")(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: 8}.Layout),
						layout.Rigid(a.plainButton(&fm.cancel, "Cancel")),
					)
				})
			}),
		)
	})
	call := body.Stop()

	// Keep the drop-down inside the grid, as Excel keeps it on screen.
	size := gtx.Constraints.Max
	pos := fm.at
	pos.X = max(min(pos.X, size.X-d.Size.X-2), 0)
	pos.Y = max(min(pos.Y, size.Y-d.Size.Y-2), 0)
	off := op.Offset(pos).Push(gtx.Ops)
	sz := d.Size
	gui.FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22))
	gui.FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), chromeLine)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &fm.body)
	pointer.CursorDefault.Add(gtx.Ops)
	stop.Pop()
	call.Add(gtx.Ops)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
	if pos != fm.at {
		fm.at = pos // the submenu opens next to where it is drawn
	}
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &fm.body, Kinds: pointer.Press}); !ok {
			break
		}
	}
	// The submenu, above the drop-down.
	if i := fm.subMenu.Layout(gtx, a.ui); i >= 0 && i < len(fm.subActs) && fm.subActs[i] != nil {
		act := fm.subActs[i]
		a.closeFilterMenu()
		act()
	}
}

// filterRow is one command row of the drop-down: an icon, a label, and
// for the Filters submenu a check (when a custom filter is on) and an
// arrow.
func (a *App) filterRow(btn *widget.Clickable, icon *gui.Icon, label string, disabled, sub, checked bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Min.X
		if disabled {
			gtx = gtx.Disabled()
		}
		return gui.Hoverable(gtx, btn, colors.Transparent, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(filterRowH)
			gtx.Constraints = layout.Exact(image.Pt(w, h))
			fg := chromeText
			if disabled {
				fg = mutedText
			}
			is := gtx.Dp(16)
			ic := gtx
			ic.Constraints = layout.Exact(image.Pt(is, is))
			if checked {
				icon = gui.IconCheck
			}
			if icon != nil {
				o := op.Offset(image.Pt(gtx.Dp(9), (h-is)/2)).Push(gtx.Ops)
				icon.Layout(ic, fg)
				o.Pop()
			}
			l := a.ui.Label(13, label, fg, font.Normal)
			lg := gtx
			lg.Constraints = layout.Constraints{Max: image.Pt(w-gtx.Dp(56), h)}
			m := op.Record(gtx.Ops)
			ld := l.Layout(lg)
			call := m.Stop()
			o := op.Offset(image.Pt(gtx.Dp(34), (h-ld.Size.Y)/2)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			o.Pop()
			if sub {
				// ▸
				x, cy, s := w-gtx.Dp(16), h/2, gtx.Dp(4)
				for i := 0; i <= s; i++ {
					gui.VLine(gtx.Ops, x+i, cy-s+i, cy+s-i, 1, fg)
				}
			}
			return layout.Dimensions{Size: image.Pt(w, h)}
		})
	}
}

// filterChecklist is the scrolling list of the column's values with
// (Select All) first.
func (a *App) filterChecklist(gtx layout.Context, shown []int, allOn bool) layout.Dimensions {
	fm := &a.filt
	w, h := gtx.Constraints.Max.X, gtx.Dp(filterListH)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	g := gtx
	g.Constraints = layout.Exact(image.Pt(w-2, h-2))
	off := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
	cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
	someOn := false
	for _, i := range shown {
		someOn = someOn || fm.items[i].Checked
	}
	allLabel := "(Select All)"
	if strings.TrimSpace(fm.search.Text()) != "" {
		allLabel = "(Select All Search Results)"
	}
	material.List(a.th, &fm.list).Layout(g, len(shown)+1, func(gtx layout.Context, k int) layout.Dimensions {
		if k == 0 {
			state := 0
			switch {
			case allOn && len(shown) > 0:
				state = 1
			case someOn:
				state = 2
			}
			return a.checkRow(gtx, &fm.all, allLabel, state)
		}
		it := fm.items[shown[k-1]]
		label := it.Text
		if label == "" {
			label = "(Blanks)"
		}
		state := 0
		if it.Checked {
			state = 1
		}
		return a.checkRow(gtx, &fm.rows[shown[k-1]], label, state)
	})
	cs.Pop()
	off.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// checkRow is a checklist entry: a box (0 clear, 1 checked, 2 some of
// the entries checked) and its label.
func (a *App) checkRow(gtx layout.Context, btn *widget.Clickable, label string, state int) layout.Dimensions {
	w := gtx.Constraints.Max.X
	return gui.Hoverable(gtx, btn, colors.Transparent, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(filterCheckH)
		s := gtx.Dp(13)
		x0, y0 := gtx.Dp(6), (h-s)/2
		box := image.Rect(x0, y0, x0+s, y0+s)
		switch state {
		case 1:
			gui.FillRect(gtx.Ops, box, accent)
			ic := gtx
			ic.Constraints = layout.Exact(image.Pt(s, s))
			o := op.Offset(box.Min).Push(gtx.Ops)
			gui.IconCheck.Layout(ic, colors.White)
			o.Pop()
		default:
			gui.FillRect(gtx.Ops, box, mutedText)
			gui.FillRect(gtx.Ops, box.Inset(1), colors.White)
			if state == 2 {
				gui.FillRect(gtx.Ops, box.Inset(gtx.Dp(3)), accent)
			}
		}
		l := a.ui.Label(13, label, chromeText, font.Normal)
		lg := gtx
		lg.Constraints = layout.Constraints{Max: image.Pt(w-x0-s-gtx.Dp(14), h)}
		m := op.Record(gtx.Ops)
		ld := l.Layout(lg)
		call := m.Stop()
		o := op.Offset(image.Pt(x0+s+gtx.Dp(8), (h-ld.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		o.Pop()
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}
