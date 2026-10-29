// Charts in the grid: selecting, moving, sizing, nudging, deleting,
// copying and pasting them, inserting them, and editing a title in place.

package ui

import (
	"errors"
	"image"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Charts in the window: a click selects one and the part of it under
// the pointer (a frame with sizing handles; the part marked inside),
// dragging it moves it (Alt snaps it to the cells), dragging a handle
// sizes it (Shift keeps a corner's proportions), arrow keys nudge it,
// Delete deletes the selected part or the chart (undoably), Ctrl+C / X /
// V copy, cut and paste it, a double-click on the title (or typing with
// it selected) edits it in place. A chart sheet shows its chart filling
// the grid's place; commands that change cells are off there. Inserting
// charts and the Chart Design and Format tabs are in chartribbon.go, the
// dialogs in chartdlg.go.

// The default size of a new chart (Excel's 5 × 3 inches), in pixels.
const newChartW, newChartH = 480, 288

// chartUI is the window's chart state.
type chartUI struct {
	drag     *chartDrag
	clip     *model.Chart // a copied chart
	clipText string       // what was put on the clipboard with it
	clipW    float64      // its size, unzoomed pixels
	clipH    float64
	title    titleEdit
	design   chartDesignRibbon
	format   chartFormatRibbon
	charts   chartsGroup
	dlg      chartDialog
	// The last press on a chart, for double-clicks.
	lastPress time.Duration
	lastElem  render.ChartElem
}

// chartDrag is a chart being moved (handle -1) or sized by a handle
// (0..7 clockwise from the top-left corner, as render.ChartHandles).
type chartDrag struct {
	chart  *model.Chart
	idx    int
	handle int
	start  f32.Point
	box    [4]float64 // x0, y0, x1, y1 at the start, unzoomed sheet pixels
	cur    [4]float64 // where it goes now
	moved  bool
	done   bool // a Cancel came: drop it unless a Release follows
}

// onChartSheet reports whether the active sheet is a chart sheet.
func (a *App) onChartSheet() bool { return a.wb.Sheets[a.active].IsChartSheet() }

// selChart is the selected chart and its index on the active sheet (-1
// for a chart sheet's chart), or nil. A selection the workbook no longer
// has (after an undo) is dropped.
func (a *App) selChart() (*model.Chart, int) {
	v := &a.state().view
	c := v.SelChart
	if c == nil {
		return nil, 0
	}
	sh := a.wb.Sheets[a.active]
	if c == sh.ChartSheet {
		return c, -1
	}
	if i := slices.Index(sh.Charts, c); i >= 0 {
		return c, i
	}
	// Undo and redo put back copies: the chart of the same name stays
	// selected.
	if i := slices.IndexFunc(sh.Charts, func(o *model.Chart) bool { return c.Name != "" && o.Name == c.Name }); i >= 0 {
		v.SelChart = sh.Charts[i]
		return sh.Charts[i], i
	}
	if sh.ChartSheet != nil {
		v.SelChart = sh.ChartSheet
		return sh.ChartSheet, -1
	}
	v.SelChart, v.SelElem = nil, render.ChartElem{}
	return nil, 0
}

// curChart is the chart the chart commands work on: the selected one, or
// a chart sheet's.
func (a *App) curChart() (*model.Chart, int) {
	if c, i := a.selChart(); c != nil {
		return c, i
	}
	if sh := a.wb.Sheets[a.active]; sh.IsChartSheet() {
		return sh.ChartSheet, -1
	}
	return nil, 0
}

// selectChart selects chart idx of the active sheet (-1: the chart
// sheet's), keeping the selected part when it still exists.
func (a *App) selectChart(idx int, elem render.ChartElem) {
	v := &a.state().view
	sh := a.wb.Sheets[a.active]
	switch {
	case idx < 0:
		v.SelChart = sh.ChartSheet
	case idx < len(sh.Charts):
		v.SelChart = sh.Charts[idx]
	default:
		v.SelChart = nil
	}
	v.SelElem = elem
}

// chartChanged updates the window after a chart changed, selecting it.
func (a *App) chartChanged(res calc.Result, idx int) {
	elem := a.state().view.SelElem
	a.changed(res)
	a.selectChart(idx, elem)
	if c, _ := a.selChart(); c != nil && !elemExists(c, elem) {
		a.state().view.SelElem = render.ChartElem{Kind: render.ElemChartArea}
	}
}

// elemExists reports whether chart c still has part e.
func elemExists(c *model.Chart, e render.ChartElem) bool {
	switch e.Kind {
	case render.ElemTitle:
		return c.Title != nil || !c.AutoTitleDeleted && len(c.AllSeries()) == 1
	case render.ElemLegend:
		return c.Legend != nil
	case render.ElemSeries, render.ElemLabels:
		return e.Plot < len(c.Plots) && e.Series < len(c.Plots[e.Plot].Series)
	case render.ElemAxis, render.ElemGridlines, render.ElemAxisTitle:
		return e.Axis >= 0 && e.Axis < len(c.Axes)
	}
	return true
}

// errChartKind refuses to change a chart ExcelLite can't save from the
// model.
var errChartKind = errors.New("This chart has a kind of chart ExcelLite doesn't draw: it can be moved, sized, copied or deleted, but not changed")

// chartEditable reports whether the chart commands can change c: not a
// chart with groups of a kind ExcelLite doesn't draw (radar, stock ...),
// which a chart saved from the model would lose.
func chartEditable(c *model.Chart) bool {
	return c != nil && !slices.ContainsFunc(c.Plots, func(p model.ChartPlot) bool { return p.Type == model.ChartOther })
}

// changeChart changes the current chart with f (on a copy) as one undo
// step; an error from f is shown and changes nothing.
func (a *App) changeChart(f func(c *model.Chart) error) {
	c, idx := a.curChart()
	if c == nil || !a.readyToChange() {
		return
	}
	if !chartEditable(c) {
		a.showError(errChartKind)
		return
	}
	e := c.Clone()
	if err := f(e); err != nil {
		a.showError(err)
		return
	}
	a.chartChanged(a.book.SetChart(a.active, idx, e), idx)
}

// chartPress handles a press in the grid on a chart: on a selected
// chart's handle it starts sizing it, on a chart it selects it and the
// part under the pointer (a double-click on the title edits it) and
// starts moving it. A press elsewhere drops the selection. It reports
// whether the press was the charts'.
func (a *App) chartPress(v *render.View, e pointer.Event) bool {
	pos := e.Position
	if a.ch.title.active {
		a.finishTitleEdit(true)
	}
	if c, idx := a.selChart(); c != nil && idx >= 0 {
		if h := a.handleAt(v, c, pos); h >= 0 {
			a.startChartDrag(v, c, idx, h, pos)
			return true
		}
	}
	c := v.ChartAt(pos.X, pos.Y)
	if c == nil {
		v.SelChart, v.SelElem = nil, render.ChartElem{}
		return a.onChartSheet()
	}
	elem := v.ChartElementAt(c, pos.X, pos.Y)
	double := v.SelChart == c && a.ch.lastPress != 0 && e.Time-a.ch.lastPress < doubleClick && elem == a.ch.lastElem
	a.ch.lastPress, a.ch.lastElem = e.Time, elem
	v.SelChart, v.SelElem = c, elem
	if double && elem.Kind == render.ElemTitle {
		a.ch.lastPress = 0
		a.startTitleEdit("", false)
		return true
	}
	if idx := slices.Index(v.Sheet.Charts, c); idx >= 0 {
		a.startChartDrag(v, c, idx, -1, pos)
	}
	return true
}

// handleAt is the sizing handle of chart c under a point, or -1.
func (a *App) handleAt(v *render.View, c *model.Chart, pos f32.Point) int {
	rc, ok := v.ChartRect(c)
	if !ok {
		return -1
	}
	r := max(int(math.Round(6*v.Scale())), 5)
	pt := pos.Round()
	for i, h := range render.ChartHandles(rc) {
		if pt.X >= h.X-r && pt.X <= h.X+r && pt.Y >= h.Y-r && pt.Y <= h.Y+r {
			return i
		}
	}
	return -1
}

// startChartDrag starts moving chart idx (handle -1) or sizing it by a
// handle, from the press at pos.
func (a *App) startChartDrag(v *render.View, c *model.Chart, idx, handle int, pos f32.Point) {
	x0, y0, x1, y1 := render.ChartBox(c, v.Geo)
	box := [4]float64{x0, y0, x1, y1}
	a.ch.drag = &chartDrag{chart: c, idx: idx, handle: handle, start: pos, box: box, cur: box}
}

// chartDragTo follows the pointer: the chart's new box, snapped to the
// cells with Alt, its proportions kept with Shift on a corner.
func (a *App) chartDragTo(v *render.View, pos f32.Point, mods key.Modifiers) {
	d := a.ch.drag
	s := v.Scale()
	if s == 0 {
		return
	}
	dx, dy := float64(pos.X-d.start.X)/s, float64(pos.Y-d.start.Y)/s
	if !d.moved && math.Abs(dx)*s < 3 && math.Abs(dy)*s < 3 {
		return // a click, not yet a drag
	}
	d.moved = true
	g := v.Geo
	snap := func(x float64, cols bool) float64 {
		ax := g.Rows
		if cols {
			ax = g.Cols
		}
		i := ax.Index(x)
		if x-ax.Start(i) > ax.Size(i)/2 {
			i++
		}
		return ax.Start(i)
	}
	alt := mods.Contain(key.ModAlt)
	b := d.box
	if d.handle < 0 {
		w, h := b[2]-b[0], b[3]-b[1]
		x0, y0 := max(b[0]+dx, 0), max(b[1]+dy, 0)
		if alt {
			x0, y0 = snap(x0, true), snap(y0, false)
		}
		d.cur = [4]float64{x0, y0, x0 + w, y0 + h}
		return
	}
	left := d.handle == 0 || d.handle == 6 || d.handle == 7
	right := d.handle == 2 || d.handle == 3 || d.handle == 4
	top := d.handle <= 2
	bottom := d.handle >= 4 && d.handle <= 6
	c := b
	if left {
		c[0] = b[0] + dx
	}
	if right {
		c[2] = b[2] + dx
	}
	if top {
		c[1] = b[1] + dy
	}
	if bottom {
		c[3] = b[3] + dy
	}
	if alt {
		for i, on := range []bool{left, top, right, bottom} {
			if on {
				c[i] = snap(c[i], i%2 == 0)
			}
		}
	}
	const minSize = 10
	if mods.Contain(key.ModShift) && d.handle%2 == 0 {
		// A corner keeps the proportions: the larger change wins.
		w0, h0 := b[2]-b[0], b[3]-b[1]
		k := max(math.Abs(c[2]-c[0])/w0, math.Abs(c[3]-c[1])/h0)
		w, h := max(w0*k, minSize), max(h0*k, minSize)
		if left {
			c[0] = b[2] - w
		} else {
			c[2] = b[0] + w
		}
		if top {
			c[1] = b[3] - h
		} else {
			c[3] = b[1] + h
		}
	}
	// Dragged past the other side: the box turns over.
	if c[2] < c[0] {
		c[0], c[2] = c[2], c[0]
	}
	if c[3] < c[1] {
		c[1], c[3] = c[3], c[1]
	}
	c[0], c[1] = max(c[0], 0), max(c[1], 0)
	c[2], c[3] = max(c[2], c[0]+minSize), max(c[3], c[1]+minSize)
	d.cur = c
}

// chartRelease ends a drag: the chart goes where it was dragged (one
// undo step).
func (a *App) chartRelease() {
	d := a.ch.drag
	a.ch.drag = nil
	if d == nil || !d.moved || d.cur == d.box {
		return
	}
	a.placeChart(d.idx, d.cur)
}

// placeChart moves or sizes chart idx of the active sheet to a box
// (unzoomed sheet pixels).
func (a *App) placeChart(idx int, box [4]float64) {
	if !a.readyToChange() {
		return
	}
	v := &a.state().view
	c := v.Sheet.Charts[idx]
	an := render.ChartAnchor(c.Anchor, v.Geo, box[0], box[1], box[2], box[3])
	a.chartChanged(a.book.MoveChart(a.active, idx, an), idx)
}

// nudgeChart moves the selected chart a pixel (arrow keys).
func (a *App) nudgeChart(dx, dy float64) {
	c, idx := a.selChart()
	if c == nil || idx < 0 {
		return
	}
	x0, y0, x1, y1 := render.ChartBox(c, a.state().view.Geo)
	dx, dy = max(dx, -x0), max(dy, -y0)
	a.placeChart(idx, [4]float64{x0 + dx, y0 + dy, x1 + dx, y1 + dy})
}

// drawChartDrag outlines where a dragged chart would go.
func (a *App) drawChartDrag(gtx layout.Context, v *render.View) {
	d := a.ch.drag
	if d == nil || !d.moved {
		return
	}
	box := v.SheetToScreen(d.chart, d.cur[0], d.cur[1], d.cur[2], d.cur[3])
	t := max(gtx.Dp(1), 1)
	on, off := gtx.Dp(4), gtx.Dp(3)
	c := colors.RGB(0x404040)
	gui.DashedLine(gtx.Ops, box.Min.X, box.Min.Y, box.Max.X, box.Min.Y, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X, box.Max.Y-t, box.Max.X, box.Max.Y-t, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X, box.Min.Y, box.Min.X, box.Max.Y, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Max.X-t, box.Min.Y, box.Max.X-t, box.Max.Y, t, on, off, c)
}

// deleteSelectedChart deletes the selected part of the chart (title,
// legend, axis, gridlines, labels, a series) or, with the chart as a
// whole selected, the chart (Delete); it reports whether a chart was
// selected. A chart sheet's chart stays: delete the sheet.
func (a *App) deleteSelectedChart() bool {
	v := &a.state().view
	c, idx := a.selChart()
	if c == nil {
		return false
	}
	if e := v.SelElem; e.Kind > render.ElemPlotArea && chartEditable(c) {
		a.changeChart(func(c *model.Chart) error {
			deleteElem(c, e)
			return nil
		})
		v.SelElem = render.ChartElem{Kind: render.ElemChartArea}
		return true
	}
	if idx < 0 {
		return true
	}
	v.SelChart, v.SelElem = nil, render.ChartElem{}
	a.changed(a.book.DeleteChart(a.active, idx))
	return true
}

// deleteElem takes part e out of chart c.
func deleteElem(c *model.Chart, e render.ChartElem) {
	switch e.Kind {
	case render.ElemTitle:
		edit.SetChartTitle(c, false)
	case render.ElemLegend:
		edit.SetLegend(c, "")
	case render.ElemSeries:
		if e.Plot < len(c.Plots) && e.Series < len(c.Plots[e.Plot].Series) {
			c.Plots[e.Plot].Series = slices.Delete(c.Plots[e.Plot].Series, e.Series, e.Series+1)
			if len(c.Plots[e.Plot].Series) == 0 && len(c.Plots) > 1 {
				c.Plots = slices.Delete(c.Plots, e.Plot, e.Plot+1) // a combo's group that is left empty
			}
		}
	case render.ElemLabels:
		if e.Plot < len(c.Plots) {
			p := &c.Plots[e.Plot]
			if e.Series < len(p.Series) && p.Series[e.Series].Labels != nil {
				p.Series[e.Series].Labels = &model.DataLabels{Delete: true}
			} else {
				p.Labels = nil
			}
		}
	case render.ElemAxis:
		if e.Axis >= 0 && e.Axis < len(c.Axes) {
			c.Axes[e.Axis].Delete = true
		}
	case render.ElemAxisTitle:
		if e.Axis >= 0 && e.Axis < len(c.Axes) {
			c.Axes[e.Axis].Title = nil
		}
	case render.ElemGridlines:
		if e.Axis >= 0 && e.Axis < len(c.Axes) {
			c.Axes[e.Axis].MajorGrid = false
		}
	}
}

// chartKey handles a key pressed while a chart is selected; it reports
// whether the key was the chart's.
func (a *App) chartKey(e key.Event) bool {
	c, idx := a.selChart()
	if c == nil {
		return false
	}
	v := &a.state().view
	switch e.Name {
	case key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow:
		if idx >= 0 {
			step := 1.0
			if e.Modifiers.Contain(key.ModShortcut) {
				step = 0.25 // finer, as Excel's Ctrl+arrow
			}
			var dx, dy float64
			switch e.Name {
			case key.NameLeftArrow:
				dx = -step
			case key.NameRightArrow:
				dx = step
			case key.NameUpArrow:
				dy = -step
			default:
				dy = step
			}
			a.nudgeChart(dx, dy)
		}
		return true
	case key.NameDeleteForward, key.NameDeleteBackward:
		a.deleteSelectedChart()
		return true
	case key.NameF2, key.NameReturn, key.NameEnter:
		if v.SelElem.Kind == render.ElemTitle {
			a.startTitleEdit("", false)
		}
		return true
	case "Z", "Y":
		return false
	}
	return false
}

// --- the clipboard ---

// copyChart copies (or cuts) the selected chart; it reports whether one
// was selected.
func (a *App) copyChart(gtx layout.Context, cut bool) bool {
	c, idx := a.selChart()
	if c == nil {
		return false
	}
	v := &a.state().view
	a.ch.clip = c.Clone()
	if idx < 0 {
		a.ch.clipW, a.ch.clipH = newChartW, newChartH
		a.ch.clip.Anchor = model.Anchor{}
	} else {
		x0, y0, x1, y1 := render.ChartBox(c, v.Geo)
		a.ch.clipW, a.ch.clipH = x1-x0, y1-y0
	}
	a.ch.clipText = c.Name
	if a.ch.clipText == "" {
		a.ch.clipText = "Chart"
	}
	a.clip = nil
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(a.ch.clipText))})
	if cut && idx >= 0 {
		v.SelElem = render.ChartElem{}
		a.deleteSelectedChart()
	}
	return true
}

// pasteChart pastes a copied chart when text is what was put on the
// clipboard with it: offset from the selected chart, or at the active
// cell. Its references are absolute and stay as they were. It reports
// whether there was a chart to paste.
func (a *App) pasteChart(text string) bool {
	if a.ch.clip == nil || strings.TrimSpace(text) != a.ch.clipText {
		return false
	}
	if a.onChartSheet() || !a.readyToChange() {
		return true
	}
	v := &a.state().view
	var x0, y0 float64
	if c, idx := a.selChart(); c != nil && idx >= 0 {
		cx, cy, _, _ := render.ChartBox(c, v.Geo)
		x0, y0 = cx+20, cy+20
	} else {
		p := v.Sel.Active
		x0, y0 = v.Geo.Cols.Start(p.Col), v.Geo.Rows.Start(p.Row)
	}
	c := a.ch.clip.Clone()
	c.Name = ""
	base := c.Anchor
	if base.Kind == model.AnchorTwoCell && base.From == base.To {
		base = model.Anchor{}
	}
	c.Anchor = render.ChartAnchor(base, v.Geo, x0, y0, x0+a.ch.clipW, y0+a.ch.clipH)
	i, res := a.book.AddChart(a.active, c)
	a.changed(res)
	a.selectChart(i, render.ChartElem{Kind: render.ElemChartArea})
	return true
}

// --- inserting ---

// insertChart makes a chart of kind from the selection (or the data
// around the active cell) beside the data, selects it and shows the
// Chart Design tab, as Excel does.
func (a *App) insertChart(kind edit.ChartKind) {
	if a.onChartSheet() || !a.readyToChange() || !a.singleRange() {
		return
	}
	v := &a.state().view
	c, src, err := edit.NewChart(a.wb, a.active, v.Sel.Range, kind)
	if err != nil {
		a.showError(err)
		return
	}
	// Right of the data with a column between, top-aligned; in the
	// middle of the window when that is out of sight.
	g := v.Geo
	x0, y0 := g.Cols.Start(src.Range.C2+2), g.Rows.Start(src.Range.R1)
	left, top := g.Cols.Start(v.LeftCol), g.Rows.Start(v.TopRow)
	right := g.Cols.Start(v.LeftCol + v.VisibleCols())
	bottom := g.Rows.Start(v.TopRow + v.VisibleRows())
	if x0+newChartW > right || y0 < top || y0 > bottom {
		x0 = max(left+(right-left-newChartW)/2, left)
		y0 = max(top+(bottom-top-newChartH)/2, top)
	}
	c.Anchor = render.ChartAnchor(model.Anchor{Kind: model.AnchorTwoCell}, g, x0, y0, x0+newChartW, y0+newChartH)
	i, res := a.book.AddChart(a.active, c)
	a.changed(res)
	a.selectChart(i, render.ChartElem{Kind: render.ElemChartArea})
	a.tab = tabChartDesign
	a.grid.focusPending = true
}

// insertChartSheet makes the default chart from the selection on a chart
// sheet of its own before the active sheet (F11).
func (a *App) insertChartSheet() {
	if a.onChartSheet() || !a.readyToChange() || !a.singleRange() {
		return
	}
	v := &a.state().view
	c, _, err := edit.NewChart(a.wb, a.active, v.Sel.Range, edit.ColumnClustered)
	if err != nil {
		a.showError(err)
		return
	}
	at := a.active
	if err := a.book.InsertChartSheet(at, edit.NextChartSheetName(a.wb), c); err != nil {
		a.showError(err)
		return
	}
	a.syncSheets()
	a.changed(calc.Result{})
	a.active = at
	a.tab = tabChartDesign
	a.grid.focusPending = true
}

// --- the title edited in place ---

// titleEdit is the chart title being typed over.
type titleEdit struct {
	active bool
	ed     widget.Editor
	focus  bool // to take the keyboard
	had    bool // it had the keyboard: losing it ends the edit
	chart  *model.Chart
}

// startTitleEdit edits the current chart's title in place: its text
// selected, or replaced by text (typing with the title selected).
func (a *App) startTitleEdit(text string, replace bool) {
	c, _ := a.curChart()
	if c == nil || !chartEditable(c) {
		return
	}
	t := &a.ch.title
	t.active, t.focus, t.had, t.chart = true, true, false, c
	t.ed.SingleLine, t.ed.Submit = true, true
	cur := ""
	switch {
	case c.Title != nil && c.Title.Text != "":
		cur = c.Title.Text
	case len(c.AllSeries()) == 1:
		sr := c.AllSeries()[0]
		cur = a.wb.SeriesName(&c.Plots[sr.Plot].Series[sr.Series], a.wb.Sheets[a.active].Name, 0)
	default:
		cur = "Chart Title"
	}
	cur = strings.ReplaceAll(cur, "\n", " ")
	if replace {
		t.ed.SetText(text)
		t.ed.SetCaret(t.ed.Len(), t.ed.Len())
		return
	}
	t.ed.SetText(cur)
	t.ed.SetCaret(t.ed.Len(), 0)
}

// finishTitleEdit sets the title typed (ok) or leaves it.
func (a *App) finishTitleEdit(ok bool) {
	t := &a.ch.title
	if !t.active {
		return
	}
	t.active = false
	a.grid.focusPending = true
	if c, _ := a.curChart(); !ok || c != t.chart {
		return
	}
	text := strings.TrimSpace(t.ed.Text())
	a.changeChart(func(c *model.Chart) error {
		edit.SetChartTitleText(c, text)
		return nil
	})
	a.state().view.SelElem = render.ChartElem{Kind: render.ElemTitle}
}

// layoutTitleEdit draws the title editor over the title and handles
// Enter and the focus going elsewhere.
func (a *App) layoutTitleEdit(gtx layout.Context, v *render.View) {
	t := &a.ch.title
	if !t.active {
		return
	}
	if c, _ := a.curChart(); c != t.chart {
		t.active = false
		return
	}
	switch {
	case t.focus:
		requestFocus(gtx, &t.ed)
		t.focus = false
	case gtx.Focused(&t.ed):
		t.had = true
	case t.had:
		a.finishTitleEdit(true) // a click elsewhere
		return
	}
	for {
		ev, ok := t.ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.finishTitleEdit(true)
			return
		}
	}
	rc, ok := v.ChartRect(t.chart)
	if !ok {
		return
	}
	s := float32(v.Scale())
	size := unit.Sp(14 * 96 / 72 * s / gtx.Metric.PxPerSp)
	if tr, _, ok := v.ChartElementRect(t.chart, render.ChartElem{Kind: render.ElemTitle}); ok {
		rc = image.Rect(rc.Min.X, tr.Min.Y-2, rc.Max.X, tr.Max.Y+2)
	} else {
		rc = image.Rect(rc.Min.X, rc.Min.Y+gtx.Dp(6), rc.Max.X, rc.Min.Y+gtx.Dp(6)+gtx.Sp(size)*3/2)
	}
	w := min(max(rc.Dx()*3/4, gtx.Dp(160)), rc.Dx())
	box := image.Rect(rc.Min.X+(rc.Dx()-w)/2, rc.Min.Y, rc.Min.X+(rc.Dx()+w)/2, rc.Max.Y)
	gui.FillRect(gtx.Ops, box.Inset(-1), accent)
	gui.FillRect(gtx.Ops, box, colors.White)
	g := gtx
	g.Constraints = layout.Constraints{Min: image.Pt(box.Dx()-8, 0), Max: image.Pt(box.Dx()-8, box.Dy())}
	cs := clip.Rect(box).Push(gtx.Ops)
	off := op.Offset(box.Min.Add(image.Pt(4, 2))).Push(gtx.Ops)
	t.ed.Alignment = text.Middle // centered, as the title
	t.ed.Layout(g, a.th.Shaper, gui.UIFont, size, gui.Material(gtx.Ops, chromeText), gui.Material(gtx.Ops, editSelection))
	off.Pop()
	cs.Pop()
}

// chartCursors shows the arrow over charts, which aren't cells, the
// move cursor over the selected one and the sizing cursors over its
// handles.
func (a *App) chartCursors(gtx layout.Context, v *render.View) {
	if a.onChartSheet() {
		pointer.CursorDefault.Add(gtx.Ops)
		return
	}
	for _, c := range v.Sheet.Charts {
		if rc, ok := v.ChartRect(c); ok {
			st := clip.Rect(rc).Push(gtx.Ops)
			if c == v.SelChart {
				pointer.CursorAllScroll.Add(gtx.Ops)
			} else {
				pointer.CursorDefault.Add(gtx.Ops)
			}
			st.Pop()
		}
	}
	c, idx := a.selChart()
	if c == nil || idx < 0 {
		return
	}
	rc, ok := v.ChartRect(c)
	if !ok {
		return
	}
	r := max(int(math.Round(6*v.Scale())), 5)
	for i, h := range render.ChartHandles(rc) {
		st := clip.Rect(image.Rect(h.X-r, h.Y-r, h.X+r+1, h.Y+r+1)).Push(gtx.Ops)
		handleCursors[i].Add(gtx.Ops)
		st.Pop()
	}
}

// handleCursors are the sizing handles' cursors, clockwise from the
// top-left corner.
var handleCursors = [8]pointer.Cursor{
	pointer.CursorNorthWestSouthEastResize, pointer.CursorNorthSouthResize, pointer.CursorNorthEastSouthWestResize,
	pointer.CursorEastWestResize, pointer.CursorNorthWestSouthEastResize, pointer.CursorNorthSouthResize,
	pointer.CursorNorthEastSouthWestResize, pointer.CursorEastWestResize,
}

// dragCursor is the cursor while a chart is dragged.
func (a *App) dragCursor() pointer.Cursor {
	if h := a.ch.drag.handle; h >= 0 {
		return handleCursors[h]
	}
	return pointer.CursorAllScroll
}

// chartSheetMenus turns off the menu commands that work on cells while a
// chart sheet is active: only undo, redo, the sheet and chart commands
// stay.
func (a *App) chartSheetMenus(menus []gui.Menu) []gui.Menu {
	if !a.onChartSheet() {
		return menus
	}
	keep := []string{"Undo", "Redo", "Insert Sheet", "Delete Sheet", "Rename Sheet", "Calculate Now",
		"Zoom In", "Zoom Out", "100%"}
	for i := range menus {
		if i == menuFile || i > menuView {
			continue
		}
		items := slices.Clone(menus[i].Items)
		for k := range items {
			if !slices.Contains(keep, items[k].Label) {
				items[k].Disabled = true
			}
		}
		menus[i].Items = items
	}
	return menus
}
