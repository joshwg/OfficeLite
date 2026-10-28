package ui

import (
	"fmt"
	"image/color"
	"slices"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Commands of the Home ribbon and their shortcuts: cell formatting,
// Paste Special, the Format Painter, the Cells group and AutoSum.

// activeStyle is the format of the active cell (of its merged area's
// top-left cell), which the ribbon shows.
func (a *App) activeStyle() model.Style {
	v := &a.state().view
	p := v.Sel.Active
	if m, ok := v.Sheet.MergeAt(p.Row, p.Col); ok {
		p = render.Pos{Row: m.R1, Col: m.C1}
	}
	i := v.Sheet.Get(p.Row, p.Col).Style
	if int(i) < len(a.wb.Styles) {
		return a.wb.Styles[i]
	}
	return a.wb.Styles[0]
}

// readyToChange ends editing before a command (false if the entry is
// wrong) and refuses while the workbook is being saved.
func (a *App) readyToChange() bool {
	if a.saving.Load() {
		return false
	}
	return !a.ed.active || a.commitEdit(0, 0)
}

// format changes the style of every selected cell with f, as one undo
// step.
func (a *App) format(f func(st *model.Style)) {
	a.formatAt(func(st *model.Style, _, _ int) { f(st) })
}

// formatGrow is format for changes that can make text taller (font,
// size, wrap): rows grow to fit, in the same undo step.
func (a *App) formatGrow(f func(st *model.Style)) {
	last := a.book.LastChange()
	a.format(f)
	if a.book.LastChange() != last {
		a.growRows()
	}
}

func (a *App) formatAt(f func(st *model.Style, row, col int)) {
	if !a.readyToChange() {
		return
	}
	a.changed(a.book.Format(a.active, a.state().view.Sel.Range, f))
}

func (a *App) borders(kind edit.BorderKind) {
	if !a.readyToChange() {
		return
	}
	a.changed(a.book.Borders(a.active, a.state().view.Sel.Range, kind))
}

// setFontColor sets the font color; zero is Automatic (the Normal
// style's color).
func (a *App) setFontColor(c color.NRGBA) {
	if c.A == 0 {
		c = a.wb.Styles[0].Font.Color
	}
	a.format(func(s *model.Style) { s.Font.Color = c })
}

// setHAlign sets the horizontal alignment; choosing the active cell's
// alignment again goes back to General, as Excel's toggles do.
func (a *App) setHAlign(cur model.Style, h model.HAlign) {
	if cur.HAlign == h {
		h = model.HAlignGeneral
	}
	a.format(func(s *model.Style) { s.HAlign = h })
}

// indent changes the indent by delta levels; an indented cell that isn't
// right-aligned becomes left-aligned, as in Excel.
func (a *App) indent(delta int) {
	a.format(func(s *model.Style) {
		s.Indent = min(max(s.Indent+delta, 0), 15)
		if delta > 0 && s.HAlign != model.HAlignLeft && s.HAlign != model.HAlignRight && s.HAlign != model.HAlignDistributed {
			s.HAlign = model.HAlignLeft
		}
	})
}

func (a *App) merge(kind edit.MergeKind) {
	if !a.readyToChange() {
		return
	}
	st := a.state()
	res, lost := a.book.Merge(a.active, st.view.Sel.Range, kind)
	a.changed(res)
	if lost {
		a.notice = "Merging cells only keeps the upper-left value"
	}
	st.view.Sel.Range = expandMerges(st.view.Sheet, st.view.Sel.Range)
}

func (a *App) setNumFmt(id int, code string) {
	a.format(func(s *model.Style) { s.NumFmtID, s.NumFmtCode = id, code })
}

// stepDecimals shows one more or one fewer decimal: the active cell's
// format with a decimal added or removed goes to the whole selection.
func (a *App) stepDecimals(delta int) {
	st := a.activeStyle()
	v := &a.state().view
	shown := v.Sheet.Get(v.Sel.Active.Row, v.Sel.Active.Col).Text
	code, ok := edit.StepDecimals(st.NumFmtID, st.NumFmtCode, shown, delta)
	if ok {
		a.setNumFmt(0, code)
	}
}

// setFont sets the font family, loading it first.
func (a *App) setFont(name string) {
	a.ensureFont(name)
	a.formatGrow(func(s *model.Style) { s.Font.Family = name })
}

// ensureFont makes the shaper able to draw a font family no cell used
// so far.
func (a *App) ensureFont(name string) {
	h := &a.home
	if h.fams == nil {
		h.fams = []string{"Segoe UI", "Tahoma"}
		for _, st := range a.wb.Styles {
			if !slices.Contains(h.fams, st.Font.Family) {
				h.fams = append(h.fams, st.Font.Family)
			}
		}
	}
	if name == "" || slices.Contains(h.fams, name) || a.fonts == nil {
		return
	}
	h.fams = append(h.fams, name)
	a.shaper = a.fonts.Shaper(h.fams...)
	a.th.Shaper = a.shaper
}

// formatKeys handles the formatting shortcuts while the grid has the
// keyboard: Ctrl+B/I/U/5, Ctrl+1 and Excel's Ctrl+Shift number formats.
func (a *App) formatKeys(gtx layout.Context) {
	tag := &a.grid
	filters := []event.Filter{
		key.Filter{Focus: tag, Name: "B", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "I", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "U", Required: key.ModShortcut},
		key.Filter{Focus: tag, Name: "1", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Focus: tag, Name: "2", Required: key.ModShortcut | key.ModShift},
		key.Filter{Focus: tag, Name: "3", Required: key.ModShortcut | key.ModShift},
		key.Filter{Focus: tag, Name: "4", Required: key.ModShortcut | key.ModShift},
		key.Filter{Focus: tag, Name: "5", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Focus: tag, Name: "`", Required: key.ModShortcut | key.ModShift},
	}
	if a.home.painterClip != nil {
		filters = append(filters, key.Filter{Focus: tag, Name: key.NameEscape})
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		st := a.activeStyle()
		shift := e.Modifiers.Contain(key.ModShift)
		switch {
		case e.Name == key.NameEscape:
			a.stopPainter()
		case e.Name == "B":
			a.format(func(s *model.Style) { s.Font.Bold = !st.Font.Bold })
		case e.Name == "I":
			a.format(func(s *model.Style) { s.Font.Italic = !st.Font.Italic })
		case e.Name == "U":
			a.format(func(s *model.Style) { s.Font.Underline = !st.Font.Underline })
		case e.Name == "5" && !shift:
			a.format(func(s *model.Style) { s.Font.Strike = !st.Font.Strike })
		case e.Name == "1" && !shift:
			a.openFormatCells(tabNumber)
		case e.Name == "`": // Ctrl+Shift+~
			a.setNumFmt(0, "")
		case e.Name == "4": // $
			a.setNumFmt(0, `"$"#,##0.00_);\("$"#,##0.00\)`)
		case e.Name == "5": // %
			a.setNumFmt(9, "")
		case e.Name == "3": // #
			a.setNumFmt(0, "d-mmm-yy")
		case e.Name == "2": // @
			a.setNumFmt(0, "h:mm AM/PM")
		case e.Name == "1": // !
			a.setNumFmt(0, "#,##0.00")
		}
	}
}

// Paste and the Format Painter.

// pasteSpecial pastes the cells copied in this window: all of them, or
// only their values, formulas or formats.
func (a *App) pasteSpecial(mode edit.PasteMode) {
	if mode == edit.PasteAll {
		a.requestPaste(a.gtx)
		return
	}
	if !a.readyToChange() {
		return
	}
	if a.clip == nil || a.clip.WB != a.wb {
		a.errMsg = "Copy cells in this window first"
		return
	}
	dest, res, err := a.book.PasteSpecial(a.clip, a.active, a.state().view.Sel.Range, mode)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
	a.selectRange(a.active, dest)
}

// startPainter picks up the selection's formats; the next selection made
// with the mouse gets them. sticky (a double-click) keeps painting until
// Esc or another click on the button.
func (a *App) startPainter(sticky bool) {
	h := &a.home
	if h.painterClip != nil && !sticky {
		a.stopPainter()
		return
	}
	if !a.readyToChange() {
		return
	}
	h.painterClip = a.book.CopyFormats(a.active, a.state().view.Sel.Range)
	h.painterSticky = sticky
	h.painterDrag = false
	a.notice = "Click or drag over cells to paint the format"
}

func (a *App) stopPainter() {
	a.home.painterClip = nil
	a.home.painterDrag = false
	a.notice = ""
}

// painterUpdate paints when a selection drag that started while the
// painter was on ends.
func (a *App) painterUpdate(gtx layout.Context) {
	h := &a.home
	if h.painterClip == nil {
		return
	}
	if a.grid.dragging {
		h.painterDrag = true
		// The release ends the drag in the grid, laid out after the
		// ribbon: look again in the next frame.
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if !h.painterDrag {
		return
	}
	h.painterDrag = false
	clip := h.painterClip
	if !h.painterSticky {
		h.painterClip = nil
		a.notice = ""
	}
	dest, res, err := a.book.PasteSpecial(clip, a.active, a.state().view.Sel.Range, edit.PasteFormats)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
	if h.painterClip != nil {
		a.notice = "Click or drag over cells to paint the format"
	}
	a.selectRange(a.active, dest)
}

// The Cells group.

// selRows and selCols are the selection's rows and columns, limited to
// the used area when whole columns (rows) are selected.
func (a *App) selRows() []int {
	r := a.state().view.Sel.Range
	if wholeCols(r) {
		r.R2 = min(r.R2, max(a.wb.Sheets[a.active].UsedRows()-1, r.R1))
	}
	return span(r.R1, r.R2)
}

func (a *App) selCols() []int {
	r := a.state().view.Sel.Range
	if wholeRows(r) {
		r.C2 = min(r.C2, max(a.wb.Sheets[a.active].UsedCols()-1, r.C1))
	}
	return span(r.C1, r.C2)
}

func (a *App) insertMenu(i int) {
	switch i {
	case 0:
		a.insertCellsDialog()
	case 1, 2:
		a.structural(true, i == 2)
	case 3:
		a.insertSheet()
	}
}

func (a *App) deleteMenu(i int) {
	switch i {
	case 0:
		a.deleteCellsDialog()
	case 1, 2:
		a.structural(false, i == 2)
	case 3:
		a.deleteSheet()
	}
}

// Format menu items (formatItems).
const (
	fmtRowHeight = iota
	fmtFitRows
	fmtColWidth
	fmtFitCols
	fmtDefaultWidth
	_
	fmtHideRows
	fmtHideCols
	fmtHideSheet
	fmtUnhideRows
	fmtUnhideCols
	fmtUnhideSheet
	_
	fmtRename
	fmtCells
)

func (a *App) formatMenu(gtx layout.Context, i int) {
	if !a.readyToChange() {
		return
	}
	st := a.state()
	v := &st.view
	sh := v.Sheet
	mdw := v.DigitWidth
	switch i {
	case fmtRowHeight:
		pt := float64(sh.RowHeightPx(v.Sel.Active.Row)) * 72 / 96
		a.ask("Row Height", "Row height (points):", trimFloat(pt), func(x float64) bool {
			if x < 0 || x > 409 {
				return false
			}
			a.book.ResizeRows(a.active, a.selRows(), model.PointsToPx(x))
			a.afterResize()
			return true
		})
	case fmtFitRows:
		a.autoFit(gtx, st, &resizeDrag{targets: a.selRows()})
	case fmtColWidth:
		px := float64(sh.ColWidthPx(v.Sel.Active.Col, mdw))
		a.ask("Column Width", "Column width (characters):", trimFloat(shownWidth(px, mdw)), func(x float64) bool {
			if x < 0 || x > 255 {
				return false
			}
			a.book.ResizeCols(a.active, a.selCols(), widthChars(x, mdw))
			a.afterResize()
			return true
		})
	case fmtFitCols:
		a.autoFit(gtx, st, &resizeDrag{cols: true, targets: a.selCols()})
	case fmtDefaultWidth:
		cur := float64(sh.BaseColWidth)
		if sh.DefaultColWidth > 0 {
			cur = shownWidth(float64(sh.DefaultColWidthPx(mdw)), mdw)
		}
		a.ask("Standard Width", "Standard column width (characters):", trimFloat(cur), func(x float64) bool {
			if x < 0 || x > 255 {
				return false
			}
			a.book.SetStandardWidth(a.active, float32(x))
			a.afterResize()
			return true
		})
	case fmtHideRows:
		a.book.ResizeRows(a.active, a.selRows(), 0)
		a.afterResize()
	case fmtHideCols:
		a.book.ResizeCols(a.active, a.selCols(), 0)
		a.afterResize()
	case fmtUnhideRows:
		var idx []int
		for _, r := range a.selRows() {
			if h, ok := sh.RowHeight[r]; ok && h == 0 {
				idx = append(idx, r)
			}
		}
		if len(idx) > 0 {
			a.book.ResizeRows(a.active, idx, sh.DefaultRowHeight)
			a.afterResize()
		}
	case fmtUnhideCols:
		var idx []int
		for _, c := range a.selCols() {
			if w, ok := sh.ColWidth[c]; ok && w == 0 {
				idx = append(idx, c)
			}
		}
		if len(idx) > 0 {
			a.book.ResizeCols(a.active, idx, model.PxToColWidth(float64(sh.DefaultColWidthPx(mdw)), mdw))
			a.afterResize()
		}
	case fmtHideSheet:
		visible := 0
		for _, s := range a.wb.Sheets {
			if !s.Hidden {
				visible++
			}
		}
		if visible < 2 {
			a.errMsg = "A workbook must contain at least one visible worksheet"
			return
		}
		hidden := a.active
		a.switchSheet(1)
		a.book.HideSheet(hidden, true, edit.Where{Sheet: a.active, Range: a.state().view.Sel.Range})
		a.changed(calc.Result{})
	case fmtUnhideSheet:
		var names []string
		var idx []int
		for i, s := range a.wb.Sheets {
			if s.Hidden {
				names, idx = append(names, s.Name), append(idx, i)
			}
		}
		if len(names) == 0 {
			a.notice = "No sheets are hidden"
			return
		}
		a.choose("Unhide", "Unhide sheet:", names, func(i int) {
			a.book.HideSheet(idx[i], false, edit.Where{Sheet: idx[i]})
			a.changed(calc.Result{})
			a.active = idx[i]
			a.grid.focusPending = true
		})
	case fmtRename:
		a.startRename(a.active)
	case fmtCells:
		a.openFormatCells(tabNumber)
	}
}

// afterResize redoes the sheet's geometry after sizes changed.
func (a *App) afterResize() {
	a.changed(calc.Result{})
	v := &a.state().view
	v.Geo = render.NewGeometry(v.Sheet, v.DigitWidth)
}

// shownWidth is a column width in characters as Excel shows it (cell
// padding excluded) for px pixels; widthChars is the stored width for
// a shown one.
func shownWidth(px, mdw float64) float64 {
	return float64(int(max(px-5, 0)/mdw*100+0.5)) / 100
}

func widthChars(shown, mdw float64) float32 {
	if shown <= 0 {
		return 0
	}
	return model.PxToColWidth(float64(int(shown*mdw+0.5))+5, mdw)
}

func trimFloat(x float64) string {
	return fmt.Sprintf("%g", float64(int(x*100+0.5))/100)
}

// The Editing group.

// sumFuncs are the AutoSum functions (sumItems).
var sumFuncs = []string{"SUM", "AVERAGE", "COUNT", "MAX", "MIN"}

// autoSum starts a formula of function i over the numbers above the
// active cell (or else to its left), as Excel guesses; a selected range
// gets the formula in the cell below it.
func (a *App) autoSum(i int) {
	if !a.readyToChange() {
		return
	}
	st := a.state()
	v := &st.view
	sh := v.Sheet
	sel := v.Sel.Range
	fn := sumFuncs[i]
	if sel.R1 != sel.R2 || sel.C1 != sel.C2 {
		if sel.R2+1 >= model.MaxRows || wholeCols(sel) || wholeRows(sel) {
			return
		}
		text := fmt.Sprintf("=%s(%s:%s)", fn, model.CellName(sel.R1, sel.C1), model.CellName(sel.R2, sel.C2))
		res, err := a.book.Enter(a.active, sel.R2+1, sel.C1, text)
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.changed(res)
		a.selectRange(a.active, model.Range{R1: sel.R2 + 1, C1: sel.C1, R2: sel.R2 + 1, C2: sel.C1})
		return
	}
	p := v.Sel.Active
	arg := ""
	if r, ok := numberRun(sh, p.Row, p.Col, -1, 0); ok {
		arg = r
	} else if r, ok := numberRun(sh, p.Row, p.Col, 0, -1); ok {
		arg = r
	}
	a.beginEdit("="+fn+"("+arg+")", false, false)
}

// numberRun is the block of cells holding numbers next to row, col in
// direction dr, dc (up or left), as a range reference.
func numberRun(sh *model.Sheet, row, col, dr, dc int) (string, bool) {
	isNum := func(r, c int) bool {
		cell := sh.Get(r, c)
		return cell.Kind == model.KindNumber || cell.Formula != "" && cell.Kind == model.KindEmpty
	}
	r, c := row+dr, col+dc
	if r < 0 || c < 0 || !isNum(r, c) {
		return "", false
	}
	for r+dr >= 0 && c+dc >= 0 && isNum(r+dr, c+dc) {
		r, c = r+dr, c+dc
	}
	return model.CellName(r, c) + ":" + model.CellName(row+dr, col+dc), true
}

func (a *App) fillMenu(i int) {
	if i == 0 {
		a.fillDown()
	} else {
		a.fillRight()
	}
}

func (a *App) clearMenu(i int) {
	if !a.readyToChange() {
		return
	}
	r := a.state().view.Sel.Range
	switch i {
	case 0, 1:
		a.changed(a.book.ClearFormats(a.active, r, i == 0))
	case 2:
		a.clearSelection()
	case 3:
		a.clearNotes()
	case 4:
		a.clearHyperlinks()
	}
}

func (a *App) sortMenu(i int) {
	switch i {
	case 0:
		a.sortAscending()
	case 1:
		a.sortDescending()
	case 2:
		a.customSortDialog()
	}
}

func (a *App) findMenu(i int) {
	switch i {
	case 0:
		a.showFind()
	case 1:
		a.showReplace()
	case 2:
		a.showGoTo()
	}
}

// growRows makes the selected rows that have no height of their own
// tall enough for their text after a font or wrap change, as part of
// that change (Excel's automatic row height; rows never shrink here).
func (a *App) growRows() {
	v := &a.state().view
	sh := v.Sheet
	r, ok := edit.FormatArea(sh, v.Sel.Range)
	if !ok || r.R2-r.R1 > 2000 {
		return
	}
	var rows []int
	var heights []float32
	for row := r.R1; row <= r.R2; row++ {
		if _, set := sh.RowHeight[row]; set {
			continue
		}
		if px, ok := v.FitRowHeight(a.gtx, row); ok && float32(px) > sh.DefaultRowHeight {
			rows, heights = append(rows, row), append(heights, float32(px))
		}
	}
	if len(rows) > 0 {
		a.book.GrowRows(a.active, rows, heights)
		v.Geo = render.NewGeometry(sh, v.DigitWidth)
	}
}
