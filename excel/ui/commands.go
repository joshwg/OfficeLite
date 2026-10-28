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

// Editing commands of the Home menu and their shortcuts.

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

func (a *App) undo() {
	if a.ed.active {
		a.cancelEdit()
		return
	}
	where, res, ok := a.book.Undo()
	if ok {
		a.afterUndo(where, res)
	}
}

func (a *App) redo() {
	if a.ed.active {
		return
	}
	where, res, ok := a.book.Redo()
	if ok {
		a.afterUndo(where, res)
	}
}

func (a *App) afterUndo(where edit.Where, res calc.Result) {
	a.syncSheets()
	a.changed(res)
	a.selectRange(min(where.Sheet, len(a.wb.Sheets)-1), where.Range)
}

// copy puts the selection on the clipboard: as text for other programs,
// and as cells (formulas, styles) for pasting here.
func (a *App) copy(gtx layout.Context, cut bool) {
	if a.ed.active {
		return // the editor handles its own clipboard keys
	}
	if !a.singleRange() {
		return
	}
	st := a.state()
	a.clip = a.book.Copy(a.active, st.view.Sel.Range, cut)
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
	if a.ed.active || !a.singleRange() {
		return
	}
	st := a.state()
	sel := st.view.Sel.Range
	var (
		dest model.Range
		res  calc.Result
		err  error
	)
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }
	if a.clip != nil && a.clip.WB == a.wb && norm(text) == norm(a.clipText) {
		dest, res, err = a.book.Paste(a.clip, a.active, sel)
		if err == nil && a.clip.Cut {
			a.clip = nil // a cut pastes once
		}
	} else {
		dest, res, err = a.book.PasteText(text, a.active, sel)
	}
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
	a.selectRange(a.active, dest)
}

// clearSelection empties the selected cells (every range of a multiple
// selection), keeping formats (Delete).
func (a *App) clearSelection() {
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
func (a *App) insertSheet() {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	at := a.active
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
