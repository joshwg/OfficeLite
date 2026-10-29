package ui

import (
	"image"

	"gioui.org/font"
	"slices"
	"strings"

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
	"officelite/excel/model"
	"officelite/excel/pivot"
	"officelite/excel/render"
)

// The drop-down of a report's Row Labels / Column Labels (or field
// name) and report filter buttons: Select field (for the compact form's
// headers, which stand for every field of the axis), Sort A to Z / Z to
// A, Clear Filter From, a search box and the checklist of the field's
// items, as Excel's.

// pivotMenu is an open report drop-down.
type pivotMenu struct {
	open     bool
	row, col int // the button's cell
	axis     model.PivotAxis
	fields   []int // the fields it can show
	field    int
	at       image.Point // top-left, in grid coordinates
	items    []pivotItem
	rows     []widget.Clickable

	all, sortAsc, sortDesc, clear, ok, cancel widget.Clickable
	search                                    widget.Editor
	list                                      widget.List
	fieldDD                                   gui.DropDown
	catcher, body                             int
	focus                                     bool
}

// pivotItem is a checklist entry.
type pivotItem struct {
	key, label string
	checked    bool
}

// pivotPress opens (or closes) the drop-down whose button is at pos;
// it reports whether there was one.
func (a *App) pivotPress(v *render.View, x, y float32) bool {
	row, col, ok := v.PivotButtonAt(x, y)
	if !ok {
		return false
	}
	if m := &a.pv.menu; m.open && m.row == row && m.col == col {
		a.closePivotMenu()
		return true
	}
	a.openPivotMenu(row, col)
	return true
}

// openPivotMenu opens the drop-down of the button on cell row, col.
func (a *App) openPivotMenu(row, col int) {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	pt, b, ok := v.Sheet.PivotButtonAt(row, col)
	if !ok {
		return
	}
	rc, _, ok := v.CellRect(row, col)
	if !ok {
		return
	}
	m := &a.pv.menu
	*m = pivotMenu{open: true, row: row, col: col, axis: b.Axis, at: image.Pt(rc.Min.X, rc.Max.Y+1), focus: true}
	m.fields = []int{b.Field}
	if b.Field < 0 {
		m.fields = srcFields(pt.Rows)
		if b.Axis == model.AxisCols {
			m.fields = srcFields(pt.Cols)
		}
	}
	if len(m.fields) == 0 {
		m.open = false
		return
	}
	m.search.SingleLine, m.search.Submit = true, true
	m.list.Axis = layout.Vertical
	a.loadPivotItems(pt, m.fields[0])
	a.menu.Close()
}

// loadPivotItems fills the checklist with field f's items.
func (a *App) loadPivotItems(pt *model.PivotTable, f int) {
	m := &a.pv.menu
	m.field = f
	m.items = nil
	src, err := a.book.PivotSource(a.active, pt)
	if err != nil || f >= len(src.Fields) || f >= len(pt.Fields) {
		return
	}
	sf := src.Fields[f]
	for _, i := range pivot.Order(&model.PivotField{}, sf) {
		it := sf.Items[i]
		m.items = append(m.items, pivotItem{key: it.Key, label: it.Label, checked: !pt.Fields[f].IsHidden(it.Key)})
	}
	m.rows = make([]widget.Clickable, len(m.items))
	m.search.SetText("")
}

func (a *App) closePivotMenu() {
	if !a.pv.menu.open {
		return
	}
	a.pv.menu.open = false
	a.grid.focusPending = true
	if a.gtx.Ops != nil {
		requestFocus(a.gtx, &a.grid)
	}
}

// shownItems are the checklist entries matching the search text.
func (m *pivotMenu) shownItems() []int {
	q := strings.ToLower(strings.TrimSpace(m.search.Text()))
	var out []int
	for i, it := range m.items {
		if q == "" || strings.Contains(strings.ToLower(it.label), q) {
			out = append(out, i)
		}
	}
	return out
}

// applyPivotChecklist hides the items not checked (with a search, the
// items not shown too).
func (a *App) applyPivotChecklist() {
	m := &a.pv.menu
	shown := m.shownItems()
	in := map[int]bool{}
	for _, i := range shown {
		in[i] = true
	}
	var hidden []string
	anyOn := false
	for i, it := range m.items {
		if in[i] && it.checked {
			anyOn = true
		} else {
			hidden = append(hidden, it.key)
		}
	}
	if !anyOn {
		return // Excel keeps OK disabled
	}
	f := m.field
	a.closePivotMenu()
	a.changePivot(func(p *model.PivotTable) { p.Fields[f].Hidden = hidden })
}

// layoutPivotMenu draws the open drop-down (in grid coordinates) and
// runs its commands.
func (a *App) layoutPivotMenu(gtx layout.Context) {
	m := &a.pv.menu
	if !m.open {
		return
	}
	pt, _ := a.wb.Sheets[a.active].PivotAt(m.row, m.col)
	if pt == nil {
		m.open = false
		return
	}
	f := m.field
	for m.sortAsc.Clicked(gtx) {
		a.closePivotMenu()
		a.sortPivotField(f, model.SortAscending)
		return
	}
	for m.sortDesc.Clicked(gtx) {
		a.closePivotMenu()
		a.sortPivotField(f, model.SortDescending)
		return
	}
	for m.clear.Clicked(gtx) {
		a.closePivotMenu()
		a.changePivot(func(p *model.PivotTable) { p.Fields[f].Hidden = nil })
		return
	}
	for m.cancel.Clicked(gtx) {
		a.closePivotMenu()
		return
	}
	for m.ok.Clicked(gtx) {
		a.applyPivotChecklist()
		return
	}
	for {
		ev, ok := m.search.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.applyPivotChecklist()
			return
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &m.search, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			a.closePivotMenu()
			return
		}
	}
	shown := m.shownItems()
	allOn := len(shown) > 0
	for _, i := range shown {
		allOn = allOn && m.items[i].checked
	}
	for m.all.Clicked(gtx) {
		for _, i := range shown {
			m.items[i].checked = !allOn
		}
		allOn = !allOn
	}
	for _, i := range shown {
		for m.rows[i].Clicked(gtx) {
			m.items[i].checked = !m.items[i].checked
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &m.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			a.closePivotMenu()
			return
		}
	}
	if m.focus {
		m.focus = false
		gtx.Execute(key.FocusCmd{Tag: &m.search})
	}
	anyOn := false
	for _, i := range shown {
		anyOn = anyOn || m.items[i].checked
	}
	filtered := len(pt.Fields[f].Hidden) > 0

	rec := op.Record(gtx.Ops)
	area := clip.Rect(image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)).Push(gtx.Ops)
	event.Op(gtx.Ops, &m.catcher)
	area.Pop()
	w := gtx.Dp(filterMenuWidth)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, 1<<15)}
	body := op.Record(gtx.Ops)
	var children []layout.FlexChild
	row := func(w layout.Widget) { children = append(children, layout.Rigid(w)) }
	if len(m.fields) > 1 {
		row(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 2, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(a.ui.Label(13, "Select field:", chromeText, font.Normal).Layout),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						labels := make([]string, len(m.fields))
						cur := 0
						for i, x := range m.fields {
							labels[i] = pt.Fields[x].Label()
							if x == m.field {
								cur = i
							}
						}
						if i := m.fieldDD.Layout(gtx, a.ui, 170, labels[cur], dropItems(labels)); i >= 0 {
							a.loadPivotItems(pt, m.fields[i])
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(170), gtx.Dp(24))}
					}),
				)
			})
		})
	}
	if m.axis != model.AxisPages {
		row(a.filterRow(&m.sortAsc, gui.IconSortAZ, "Sort A to Z", false, false, pt.Fields[f].Sort == model.SortAscending))
		row(a.filterRow(&m.sortDesc, gui.IconSortZA, "Sort Z to A", false, false, pt.Fields[f].Sort == model.SortDescending))
		row(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(9)
			gui.HLine(gtx.Ops, gtx.Dp(8), gtx.Constraints.Min.X-gtx.Dp(8), h/2, 1, chromeLine)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, h)}
		})
	}
	row(a.filterRow(&m.clear, gui.IconFilterClear, `Clear Filter From "`+pt.Fields[f].Label()+`"`, !filtered, false, false))
	row(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 4, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return gui.TextField(gtx, a.ui, &m.search, unit.Dp(filterMenuWidth-16), 1, "Search")
		})
	})
	row(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.pivotChecklist(gtx, shown, allOn)
		})
	})
	row(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !anyOn {
						gtx = gtx.Disabled()
					}
					return a.accentButton(&m.ok, "OK")(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: 8}.Layout),
				layout.Rigid(a.plainButton(&m.cancel, "Cancel")),
			)
		})
	})
	d := layout.Inset{Top: 4, Bottom: 8}.Layout(g, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := body.Stop()
	size := gtx.Constraints.Max
	pos := m.at
	pos.X = max(min(pos.X, size.X-d.Size.X-2), 0)
	pos.Y = max(min(pos.Y, size.Y-d.Size.Y-2), 0)
	m.at = pos
	off := op.Offset(pos).Push(gtx.Ops)
	sz := d.Size
	gui.FillRect(gtx.Ops, image.Rect(2, 2, sz.X+3, sz.Y+3), colors.RGBA(0, 0, 0, 0x22))
	gui.FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), chromeLine)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &m.body)
	pointer.CursorDefault.Add(gtx.Ops)
	stop.Pop()
	call.Add(gtx.Ops)
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &m.body, Kinds: pointer.Press}); !ok {
			break
		}
	}
}

// pivotChecklist is the scrolling list of the field's items with
// (Select All) first.
func (a *App) pivotChecklist(gtx layout.Context, shown []int, allOn bool) layout.Dimensions {
	m := &a.pv.menu
	w, h := gtx.Constraints.Max.X, gtx.Dp(filterListH)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	g := gtx
	g.Constraints = layout.Exact(image.Pt(w-2, h-2))
	off := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
	cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
	someOn := slices.ContainsFunc(shown, func(i int) bool { return m.items[i].checked })
	allLabel := "(Select All)"
	if strings.TrimSpace(m.search.Text()) != "" {
		allLabel = "(Select All Search Results)"
	}
	material.List(a.th, &m.list).Layout(g, len(shown)+1, func(gtx layout.Context, k int) layout.Dimensions {
		if k == 0 {
			state := 0
			switch {
			case allOn:
				state = 1
			case someOn:
				state = 2
			}
			return a.checkRow(gtx, &m.all, allLabel, state)
		}
		i := shown[k-1]
		state := 0
		if m.items[i].checked {
			state = 1
		}
		return a.checkRow(gtx, &m.rows[i], m.items[i].label, state)
	})
	cs.Pop()
	off.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// pivotCursors shows the arrow over the report buttons.
func (a *App) pivotCursors(gtx layout.Context, v *render.View) {
	for _, p := range v.Sheet.Pivots {
		for _, b := range p.Out.Buttons {
			if r, ok := v.PivotButtonRect(b.Row, b.Col); ok {
				area := clip.Rect(r).Push(gtx.Ops)
				pointer.CursorDefault.Add(gtx.Ops)
				area.Pop()
			}
		}
	}
}
