// Editing commands of the Home menu and their shortcuts: undo and redo,
// the clipboard, clearing, inserting and deleting rows, columns and
// sheets, and renaming sheets.

package ui

import (
	"io"
	"slices"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
	"officelite/excel/render"
)

// selectRange makes r (on sheet) the selection, with the active cell at
// its top-left corner, switching sheets if needed.
func (a *App) selectRange(sheet int, r model.Range) {
	if sheet >= 0 && sheet < len(a.wb.Sheets) && sheet != a.active {
		a.active = sheet
	}
	st := a.state()
	p := render.Pos{Row: r.R1, Col: r.C1}
	st.view.Sel = render.Selection{Active: p, Range: expandMerges(st.view.Sheet, r)}
	st.selEnd = render.Pos{Row: r.R2, Col: r.C2}
	st.view.EnsureVisible(p.Row, p.Col)
	a.grid.focusPending = true
}

// undo and redo take back or redo one step (Ctrl+Z, Ctrl+Y).
func (a *App) undo() { a.undoN(1) }
func (a *App) redo() { a.redoN(1) }

// undoN takes back n steps of the Undo list (undoLabels): an entry being
// typed is the first, which Undo cancels.
func (a *App) undoN(n int) {
	if a.ed.active {
		a.cancelEdit()
		n--
	}
	if n <= 0 {
		return
	}
	where, res, ok := a.book.UndoN(n)
	if ok {
		a.afterUndo(where, res)
	}
}

// redoN redoes n steps of the Redo list; nothing while an entry is typed.
func (a *App) redoN(n int) {
	if a.ed.active {
		return
	}
	where, res, ok := a.book.RedoN(n)
	if ok {
		a.afterUndo(where, res)
	}
}

// undoLabels is the Undo list, the next step first: "Typing" for an entry
// being typed.
func (a *App) undoLabels() []string {
	if a.ed.active {
		return append([]string{"Typing"}, a.book.UndoLabels()...)
	}
	return a.book.UndoLabels()
}

// redoLabels is the Redo list, the next step first; empty while an entry
// is typed.
func (a *App) redoLabels() []string {
	if a.ed.active {
		return nil
	}
	return a.book.RedoLabels()
}

// afterUndo updates the window after an undo or redo and selects where
// the step happened.
func (a *App) afterUndo(where edit.Where, res calc.Result) {
	a.syncSheets()
	a.changed(res)
	a.selectRange(min(where.Sheet, len(a.wb.Sheets)-1), where.Range)
	if a.wb.Sheets[a.active].Hidden {
		a.switchSheet(-1)
	}
}

// copy puts the selection on the clipboard: as text for other programs,
// and as cells (formulas, styles) for pasting here.
func (a *App) copy(gtx layout.Context, cut bool) {
	if a.ed.active {
		return // the editor handles its own clipboard keys
	}
	if a.copyChart(gtx, cut) {
		return // a selected chart (chart.go)
	}
	if !a.singleRange() {
		return
	}
	st := a.state()
	if cut && a.pivotProtected(st.view.Sel.Range) {
		return
	}
	a.clip, a.ch.clip = a.book.Copy(a.active, st.view.Sel.Range, cut), nil
	a.clipText = edit.CopyText(st.view.Sheet, st.view.Sel.Range)
	a.notice = ""
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(a.clipText))})
}

// requestPaste asks for the clipboard's text; paste runs when it arrives.
func (a *App) requestPaste(gtx layout.Context) {
	gtx.Execute(clipboard.ReadCmd{Tag: &a.grid})
}

// paste pastes clipboard text into the selection: the copied cells when
// the text is what this window copied, otherwise the text parsed as
// typed input.
func (a *App) paste(text string) {
	if !a.ed.active && a.pasteChart(text) {
		return // a copied chart (chart.go)
	}
	if a.ed.active || !a.singleRange() {
		return
	}
	st := a.state()
	sel := st.view.Sel.Range
	if a.pivotProtected(sel) {
		return
	}
	var (
		dest model.Range
		res  calc.Result
		err  error
	)
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }
	var keep *edit.Clip
	if a.clip != nil && a.clip.WB == a.wb && norm(text) == norm(a.clipText) {
		dest, res, err = a.book.Paste(a.clip, a.active, sel)
		if !a.clip.Cut {
			keep = a.clip // a copy pastes again; a cut pastes once
		}
	} else {
		dest, res, err = a.book.PasteText(text, a.active, sel)
	}
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
	a.clip = keep
	a.selectRange(a.active, dest)
}

// clearSelection empties the selected cells (every range of a multiple
// selection), keeping formats (Delete).
func (a *App) clearSelection() {
	if a.deleteSelectedChart() || a.onChartSheet() {
		return // chart.go
	}
	if a.pivotDelete() {
		return // pivot.go
	}
	var res calc.Result
	a.eachRange(func(r model.Range) { res = joinResult(res, a.book.Clear(a.active, r)) })
	a.changed(res)
}

// joinResult adds up the recalculations of several changes.
func joinResult(a, b calc.Result) calc.Result {
	a.Calculated += b.Calculated
	a.Circular = append(a.Circular, b.Circular...)
	return a
}

// wholeCols and wholeRows report whether the selection spans entire
// columns or rows.
func wholeCols(r model.Range) bool { return r.R1 == 0 && r.R2 == model.MaxRows-1 }
func wholeRows(r model.Range) bool { return r.C1 == 0 && r.C2 == model.MaxCols-1 }

// insertDelete inserts or deletes the selection's rows (or its columns)
// for Ctrl++ and Ctrl+-: columns when whole columns are selected,
// otherwise rows.
func (a *App) insertDelete(insert bool) {
	r := a.state().view.Sel.Range
	cols := wholeCols(r) && !wholeRows(r)
	a.structural(insert, cols)
}

// structural inserts or deletes the rows (cols: columns) of the
// selection.
func (a *App) structural(insert, cols bool) {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	r := a.state().view.Sel.Range
	var (
		res calc.Result
		err error
	)
	switch {
	case insert && cols:
		res, err = a.book.InsertCols(a.active, r.C1, r.C2-r.C1+1)
	case insert:
		res, err = a.book.InsertRows(a.active, r.R1, r.R2-r.R1+1)
	case cols:
		res, err = a.book.DeleteCols(a.active, r.C1, r.C2-r.C1+1)
	default:
		res, err = a.book.DeleteRows(a.active, r.R1, r.R2-r.R1+1)
	}
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(res)
	if cols {
		a.selectRange(a.active, model.Range{C1: r.C1, C2: r.C2, R2: model.MaxRows - 1})
	} else {
		a.selectRange(a.active, model.Range{R1: r.R1, R2: r.R2, C2: model.MaxCols - 1})
	}
}

// insertSheet adds a sheet before the active one and activates it.
func (a *App) insertSheet() { a.addSheet(a.active) }

// newSheet adds a sheet after the active one, as Excel's + button beside
// the sheet tabs does, and activates it.
func (a *App) newSheet() { a.addSheet(a.active + 1) }

// addSheet inserts a sheet at index at and activates it.
func (a *App) addSheet(at int) {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	if _, err := a.book.InsertSheet(at); err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(calc.Result{})
	a.active = at
	a.grid.focusPending = true
}

// deleteSheet deletes the active sheet.
func (a *App) deleteSheet() {
	if a.ed.active {
		a.cancelEdit()
	}
	res, err := a.book.DeleteSheet(a.active)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(res)
	a.active = min(a.active, len(a.wb.Sheets)-1)
	if a.wb.Sheets[a.active].Hidden {
		a.switchSheet(-1)
	}
	a.grid.focusPending = true
}

// startRename edits sheet i's tab name in place.
func (a *App) startRename(i int) {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	a.rename.sheet = i
	a.rename.active = true
	a.rename.ed.SingleLine = true
	a.rename.ed.Submit = true
	a.rename.ed.SetText(a.wb.Sheets[i].Name)
	a.rename.ed.SetCaret(a.rename.ed.Len(), 0)
	a.rename.focus = true
}

// finishRename applies (or with ok false, abandons) a tab rename.
func (a *App) finishRename(ok bool) {
	rn := &a.rename
	if !rn.active {
		return
	}
	if ok {
		res, err := a.book.RenameSheet(rn.sheet, strings.TrimSpace(rn.ed.Text()))
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.changed(res)
	}
	rn.active = false
	a.grid.focusPending = true
}

// syncSheets brings per-sheet state in line with the workbook after a
// change that may have added, removed or reshaped sheets: view state
// follows its sheet, and row and column positions are recomputed.
func (a *App) syncSheets() {
	old := a.states
	a.states = map[int]*sheetState{}
	for _, st := range old {
		i := slices.Index(a.wb.Sheets, st.view.Sheet)
		if i < 0 {
			continue
		}
		st.view.Geo = render.NewGeometry(st.view.Sheet, st.view.DigitWidth)
		a.states[i] = st
	}
	if len(a.tabBtns) != len(a.wb.Sheets) {
		a.tabBtns = make([]widget.Clickable, len(a.wb.Sheets))
	}
	a.active = min(a.active, len(a.wb.Sheets)-1)
	// Clips refer to sheets by index.
	if a.clip != nil && (a.clip.Sheet >= len(a.wb.Sheets)) {
		a.clip = nil
	}
}

// clipNotice is the status bar's prompt while cells are on the clipboard.
func (a *App) clipNotice() string {
	if a.clip.Cut {
		return "Select destination and press Ctrl+V to move the cells"
	}
	return "Select destination and press Ctrl+V to paste"
}

// renameState is a sheet tab being renamed in place.
type renameState struct {
	active bool
	sheet  int
	ed     widget.Editor
	focus  bool
}
