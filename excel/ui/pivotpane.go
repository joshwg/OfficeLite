package ui

import (
	"image"
	"slices"
	"strconv"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
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
)

// The PivotTable Fields pane, docked at the right of the grid while the
// active cell is in a report: a search box, the checklist of the source
// fields (checking one puts it in Rows, or Values when it holds
// numbers) and the four areas, Filters, Columns, Rows and Values, whose
// field chips are dragged between and within the areas with the mouse
// (dropping one outside the areas removes it) and have a menu of their
// own.

// paneWidth is the pane's width in dp.
const paneWidth = 280

// The areas of the pane, in the order they are drawn.
const (
	areaFilters = iota
	areaColumns
	areaRows
	areaValues
)

var areaNames = [4]string{"Filters", "Columns", "Rows", "Values"}

// fieldPane is the pane's widget state.
type fieldPane struct {
	close  widget.Clickable
	search widget.Editor
	list   widget.List
	checks []widget.Clickable

	drag  int // event tag over the whole pane
	chips []chipRect
	boxes [4]image.Rectangle // the areas' boxes, in pane coordinates
	press *chipPress
	at    f32.Point // the pointer while dragging, in pane coordinates

	menu     gui.ContextMenu // a chip's menu
	menuActs []func()
}

// chipRect is where a field chip was drawn.
type chipRect struct {
	area, idx int
	r         image.Rectangle
}

// chipPress is a chip being pressed or dragged.
type chipPress struct {
	area, idx int
	from      f32.Point
	moved     bool
}

// showPane reports whether the field list shows: the active cell is in
// a report and the pane wasn't closed.
func (a *App) showPane() bool {
	if a.pv.hideList || a.back.Visible {
		return false
	}
	pt, _ := a.curPivot()
	return pt != nil
}

// areaEntries lists what area shows: fields (DataField for Σ Values) or,
// for Values, value field indexes.
func areaEntries(pt *model.PivotTable, area int) []int {
	switch area {
	case areaFilters:
		return pt.Pages
	case areaColumns:
		return pt.Cols
	case areaRows:
		return pt.Rows
	}
	out := make([]int, len(pt.Values))
	for i := range out {
		out[i] = i
	}
	return out
}

// entryLabel is a chip's text.
func entryLabel(pt *model.PivotTable, area, e int) string {
	switch {
	case area == areaValues:
		return pt.Values[e].Name
	case e == model.DataField:
		return "Σ Values"
	}
	return pt.Fields[e].Label()
}

// moveEntry moves the chip idx of area from to position to of area
// dest (-1 removes it), as dragging it does.
func moveEntry(p *model.PivotTable, src *pivot.Source, from, idx, dest, to int) {
	axis := func(area int) *[]int {
		switch area {
		case areaFilters:
			return &p.Pages
		case areaColumns:
			return &p.Cols
		}
		return &p.Rows
	}
	insert := func(list *[]int, f int) {
		to = max(0, min(to, len(*list)))
		*list = slices.Insert(*list, to, f)
	}
	if from == areaValues {
		if idx >= len(p.Values) {
			return
		}
		v := p.Values[idx]
		p.Values = slices.Delete(p.Values, idx, idx+1)
		switch dest {
		case areaValues:
			to = max(0, min(to, len(p.Values)))
			p.Values = slices.Insert(p.Values, to, v)
		case -1:
		default:
			if p.AxisOf(v.Field) == model.AxisNone || p.AxisOf(v.Field) == model.AxisValues {
				insert(axis(dest), v.Field)
			}
		}
		return
	}
	list := axis(from)
	if idx >= len(*list) {
		return
	}
	f := (*list)[idx]
	if f == model.DataField && dest != areaRows && dest != areaColumns {
		return // Σ Values stays on an axis
	}
	*list = slices.Delete(*list, idx, idx+1)
	switch dest {
	case -1:
	case areaValues:
		if src != nil && f < len(src.Fields) {
			fn := pivot.DefaultFunc(src.Fields[f])
			p.Values = append(p.Values, model.PivotValue{Field: f, Func: fn, Name: uniqueValueName(p, pivot.ValueName(fn, p.Fields[f].Name))})
		}
	default:
		if dest == from && to > idx {
			to--
		}
		insert(axis(dest), f)
	}
}

// uniqueValueName is name, or name2, name3 ... when a value field has it.
func uniqueValueName(p *model.PivotTable, name string) string {
	taken := func(n string) bool {
		return slices.ContainsFunc(p.Values, func(v model.PivotValue) bool { return strings.EqualFold(v.Name, n) })
	}
	out := name
	for k := 2; taken(out); k++ {
		out = name + strconv.Itoa(k)
	}
	return out
}

// checkField adds field f to the report as Excel's field list does
// (Values for numbers, Rows otherwise) or takes it out.
func (a *App) checkField(f int, on bool) {
	pt, _ := a.curPivot()
	if pt == nil {
		return
	}
	src, err := a.book.PivotSource(a.active, pt)
	if err != nil {
		a.showError(err)
		return
	}
	a.changePivot(func(p *model.PivotTable) {
		if !on {
			removeField(p, f)
			return
		}
		sf := src.Fields[f]
		if sf.Numeric && !sf.Date {
			fn := pivot.DefaultFunc(sf)
			p.Values = append(p.Values, model.PivotValue{Field: f, Func: fn, Name: uniqueValueName(p, pivot.ValueName(fn, sf.Name))})
			return
		}
		p.Rows = append(p.Rows, f)
	})
}

// dropArea is the area whose box holds pane point pt, and the position
// among its chips; area -1 is outside every box.
func (pn *fieldPane) dropArea(pt image.Point) (area, idx int) {
	for i, b := range pn.boxes {
		if !pt.In(b) {
			continue
		}
		n := 0
		for _, c := range pn.chips {
			if c.area == i && (c.r.Min.Y+c.r.Max.Y)/2 < pt.Y {
				n = max(n, c.idx+1)
			}
		}
		return i, n
	}
	return -1, 0
}

// paneEvents handles presses, drags and releases over the chips.
func (a *App) paneEvents(gtx layout.Context, pt *model.PivotTable) {
	pn := &a.pv.pane
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &pn.drag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		p := e.Position.Round()
		switch e.Kind {
		case pointer.Press:
			pn.press = nil
			if e.Buttons != pointer.ButtonPrimary && e.Buttons != 0 {
				continue
			}
			for _, c := range pn.chips {
				if p.In(c.r) {
					pn.press = &chipPress{area: c.area, idx: c.idx, from: e.Position}
					pn.at = e.Position
				}
			}
		case pointer.Drag:
			if pn.press == nil {
				continue
			}
			pn.at = e.Position
			if d := e.Position.Sub(pn.press.from); d.X*d.X+d.Y*d.Y > 16 {
				pn.press.moved = true
			}
		case pointer.Release:
			pr := pn.press
			pn.press = nil
			if pr == nil {
				continue
			}
			if !pr.moved {
				for _, c := range pn.chips {
					if c.area == pr.area && c.idx == pr.idx {
						a.openChipMenu(pt, pr.area, pr.idx, image.Pt(c.r.Min.X, c.r.Max.Y))
					}
				}
				continue
			}
			area, idx := pn.dropArea(p)
			src, _ := a.book.PivotSource(a.active, pt)
			from, fi := pr.area, pr.idx
			a.changePivot(func(q *model.PivotTable) { moveEntry(q, src, from, fi, area, idx) })
		case pointer.Cancel:
			pn.press = nil
		}
	}
}

// openChipMenu opens the menu of chip idx of area at pos (pane
// coordinates).
func (a *App) openChipMenu(pt *model.PivotTable, area, idx int, pos image.Point) {
	n := len(areaEntries(pt, area))
	move := func(dest, to int) func() {
		return func() {
			pt, _ := a.curPivot()
			if pt == nil {
				return
			}
			src, _ := a.book.PivotSource(a.active, pt)
			a.changePivot(func(q *model.PivotTable) { moveEntry(q, src, area, idx, dest, to) })
		}
	}
	isData := area != areaValues && areaEntries(pt, area)[idx] == model.DataField
	items := []ctxItem{
		{"Move Up", "", "Move the field up in its area.", idx == 0, move(area, idx-1)},
		{"Move Down", "", "Move the field down in its area.", idx >= n-1, move(area, idx+2)},
		{"Move to Beginning", "", "Move the field to the top of its area.", idx == 0, move(area, 0)},
		{"Move to End", "", "Move the field to the bottom of its area.", idx >= n-1, move(area, n)},
		ctxSep,
		{"Move to Report Filter", "", "Filter the whole report by the field.", area == areaFilters || isData, move(areaFilters, 1<<20)},
		{"Move to Row Labels", "", "Show the field's items as rows.", area == areaRows, move(areaRows, 1<<20)},
		{"Move to Column Labels", "", "Show the field's items as columns.", area == areaColumns, move(areaColumns, 1<<20)},
		{"Move to Values", "", "Summarize the field's values.", area == areaValues || isData, move(areaValues, 1<<20)},
		ctxSep,
		{"Remove Field", "", "Take the field out of the report.", isData, move(-1, 0)},
	}
	if area == areaValues {
		items = append(items, ctxSep, ctxItem{"Value Field Settings…", "", "Change the value field's name, function and number format.", false, func() { a.valueFieldDialog(idx, false) }})
	} else if !isData {
		f := areaEntries(pt, area)[idx]
		items = append(items, ctxSep, ctxItem{"Field Settings…", "", "Change the field's name and subtotals.", false, func() { a.fieldSettingsDialog(f) }})
	}
	openCtx(&a.pv.pane.menu, &a.pv.pane.menuActs, pos, items)
}

// layoutPane draws the PivotTable Fields pane.
func (a *App) layoutPane(gtx layout.Context) layout.Dimensions {
	pt, _ := a.curPivot()
	if pt == nil || !a.showPane() {
		return layout.Dimensions{}
	}
	pn := &a.pv.pane
	pn.search.SingleLine = true
	pn.list.Axis = layout.Vertical
	for pn.close.Clicked(gtx) {
		a.pv.hideList = true
		return layout.Dimensions{}
	}
	a.paneEvents(gtx, pt)
	if len(pn.checks) != len(pt.Fields) {
		pn.checks = make([]widget.Clickable, len(pt.Fields))
	}
	for f := range pn.checks {
		for pn.checks[f].Clicked(gtx) {
			a.checkField(f, pt.AxisOf(f) == model.AxisNone)
			pt, _ = a.curPivot()
			if pt == nil {
				return layout.Dimensions{}
			}
		}
	}

	w := gtx.Dp(paneWidth)
	h := gtx.Constraints.Max.Y
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), colors.White)
	gui.VLine(gtx.Ops, 0, 0, h, 1, chromeLine)
	area := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
	event.Op(gtx.Ops, &pn.drag)
	th := a.ui
	pn.chips = pn.chips[:0]
	listH := max(gtx.Dp(120), (h-gtx.Dp(140))*2/5)
	y := 0 // the height of the rows above the areas
	rigid := func(w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := w(gtx)
			y += d.Size.Y
			return d
		})
	}
	inset := layout.Inset{Left: 10, Right: 8, Top: 8, Bottom: 8}
	inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, th.Label(15, "PivotTable Fields", chromeText, font.SemiBold).Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return gui.ToolButton(gtx, th, &pn.close, gui.IconClose, "", false, colors.Transparent)
					}),
				)
			}),
			rigid(layout.Spacer{Height: 6}.Layout),
			rigid(th.Label(12, "Choose fields to add to report:", mutedText, font.Normal).Layout),
			rigid(layout.Spacer{Height: 4}.Layout),
			rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.TextField(gtx, a.ui, &pn.search, unit.Dp(paneWidth-18), 1, "Search")
			}),
			rigid(layout.Spacer{Height: 4}.Layout),
			rigid(func(gtx layout.Context) layout.Dimensions {
				return a.paneFieldList(gtx, pt, listH)
			}),
			rigid(layout.Spacer{Height: 8}.Layout),
			rigid(th.Label(12, "Drag fields between areas below:", mutedText, font.Normal).Layout),
			rigid(layout.Spacer{Height: 4}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.paneAreas(gtx, pt, image.Pt(gtx.Dp(inset.Left), gtx.Dp(inset.Top)+y))
			}),
		)
	})
	// The chip being dragged follows the pointer.
	if pr := pn.press; pr != nil && pr.moved {
		label := entryLabel(pt, pr.area, areaEntries(pt, pr.area)[min(pr.idx, len(areaEntries(pt, pr.area))-1)])
		o := op.Offset(pn.at.Round().Add(image.Pt(-gtx.Dp(20), -gtx.Dp(12)))).Push(gtx.Ops)
		a.drawChip(gtx, label, true)
		o.Pop()
		pointer.CursorGrab.Add(gtx.Ops)
	}
	area.Pop()
	if i := pn.menu.Layout(gtx, th); i >= 0 && i < len(pn.menuActs) && pn.menuActs[i] != nil {
		pn.menuActs[i]()
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// paneFieldList is the checklist of the source fields.
func (a *App) paneFieldList(gtx layout.Context, pt *model.PivotTable, h int) layout.Dimensions {
	pn := &a.pv.pane
	w := gtx.Constraints.Max.X
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	q := strings.ToLower(strings.TrimSpace(pn.search.Text()))
	var shown []int
	for f := range pt.Fields {
		if q == "" || strings.Contains(strings.ToLower(pt.Fields[f].Name), q) {
			shown = append(shown, f)
		}
	}
	g := gtx
	g.Constraints = layout.Exact(image.Pt(w-2, h-2))
	off := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
	cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
	material.List(a.th, &pn.list).Layout(g, len(shown), func(gtx layout.Context, k int) layout.Dimensions {
		f := shown[k]
		state := 0
		if pt.AxisOf(f) != model.AxisNone {
			state = 1
		}
		return a.checkRow(gtx, &pn.checks[f], pt.Fields[f].Name, state)
	})
	cs.Pop()
	off.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// paneAreas draws the four areas, two by two, recording the boxes and
// chips for dragging.
func (a *App) paneAreas(gtx layout.Context, pt *model.PivotTable, org image.Point) layout.Dimensions {
	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	gap := gtx.Dp(8)
	cw := (w - gap) / 2
	ch := (h - gap) / 2
	for i := range 4 {
		x := (i % 2) * (cw + gap)
		y := (i / 2) * (ch + gap)
		o := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(cw, ch))
		a.paneArea(g, pt, i, org.Add(image.Pt(x, y)))
		o.Pop()
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// paneArea draws one area: its name and its chips. origin is its
// top-left in pane coordinates.
func (a *App) paneArea(gtx layout.Context, pt *model.PivotTable, area int, origin image.Point) {
	pn := &a.pv.pane
	th := a.ui
	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	name := areaNames[area]
	if area == areaValues {
		name = "Σ Values"
	}
	l := th.Label(12, name, chromeText, font.SemiBold)
	lg := gtx
	lg.Constraints.Min = image.Point{}
	ld := l.Layout(lg)
	top := ld.Size.Y + gtx.Dp(3)
	box := image.Rect(0, top, w, h)
	gui.FillRect(gtx.Ops, box, chromeLine)
	gui.FillRect(gtx.Ops, box.Inset(1), colors.RGB(0xF8F8F8))
	pn.boxes[area] = box.Add(origin)
	y := top + gtx.Dp(3)
	chipH := gtx.Dp(24)
	for i, e := range areaEntries(pt, area) {
		if y+chipH > h {
			break
		}
		r := image.Rect(gtx.Dp(3), y, w-gtx.Dp(3), y+chipH)
		o := op.Offset(r.Min).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(r.Size())
		dragged := pn.press != nil && pn.press.moved && pn.press.area == area && pn.press.idx == i
		a.drawChip(g, entryLabel(pt, area, e), dragged)
		o.Pop()
		pn.chips = append(pn.chips, chipRect{area: area, idx: i, r: r.Add(origin)})
		y += chipH + gtx.Dp(2)
	}
}

// drawChip draws a field chip: its label and a drop-down arrow.
func (a *App) drawChip(gtx layout.Context, label string, faded bool) {
	w, h := gtx.Constraints.Max.X, gtx.Dp(24)
	if w > gtx.Dp(paneWidth) || w == 0 {
		w = gtx.Dp(paneWidth/2 - 12)
	}
	bg, fg := colors.White, chromeText
	if faded {
		bg, fg = colors.RGB(0xE8F0E8), mutedText
	}
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), bg)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(w-gtx.Dp(26), h)}
	l := a.ui.Label(12, label, fg, font.Normal)
	l.MaxLines = 1
	m := op.Record(gtx.Ops)
	d := l.Layout(g)
	call := m.Stop()
	o := op.Offset(image.Pt(gtx.Dp(6), (h-d.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	o.Pop()
	is := gtx.Dp(18)
	ic := gtx
	ic.Constraints = layout.Exact(image.Pt(is, is))
	o = op.Offset(image.Pt(w-is-gtx.Dp(3), (h-is)/2)).Push(gtx.Ops)
	gui.IconDropArrow.Layout(ic, mutedText)
	o.Pop()
}
