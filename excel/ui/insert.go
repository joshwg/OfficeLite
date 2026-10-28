package ui

import (
	"image"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

// The Insert menu: Insert/Delete Cells, sheet rows, columns and sheets,
// Function, Symbol, Hyperlink and notes.

// Insert menu items.
const (
	insCells = iota
	insDeleteCells
	_
	insRows
	insCols
	insSheet
	_
	insFunction
	insSymbol
	_
	insHyperlink
	insRemoveLink
	_
	insNote
	insDeleteNote
)

func (a *App) insertMenuDef() gui.Menu {
	v := &a.state().view
	sel := v.Sel.Range
	editing := a.ed.active
	hasLink, hasNote := false, false
	for k := range v.Sheet.Links {
		if sel.Contains(k[0], k[1]) {
			hasLink = true
			break
		}
	}
	_, noteHere := v.Sheet.NoteAt(v.Sel.Active.Row, v.Sel.Active.Col)
	for k := range v.Sheet.Notes {
		if sel.Contains(k[0], k[1]) {
			hasNote = true
			break
		}
	}
	noteLabel := "New Note"
	if noteHere {
		noteLabel = "Edit Note"
	}
	return gui.Menu{Label: "Insert", Items: []gui.MenuItem{
		insCells:       {Label: "Insert Cells…", Disabled: editing},
		insDeleteCells: {Label: "Delete Cells…", Disabled: editing},
		2:              {Separator: true},
		insRows:        {Label: "Insert Sheet Rows", Disabled: wholeCols(sel)},
		insCols:        {Label: "Insert Sheet Columns", Disabled: wholeRows(sel)},
		insSheet:       {Label: "Insert Sheet", Shortcut: "Shift+F11"},
		6:              {Separator: true},
		insFunction:    {Label: "Function…", Shortcut: "Shift+F3"},
		insSymbol:      {Label: "Symbol…"},
		9:              {Separator: true},
		insHyperlink:   {Label: "Hyperlink…", Shortcut: "Ctrl+K", Disabled: editing},
		insRemoveLink:  {Label: "Remove Hyperlinks", Disabled: editing || !hasLink},
		12:             {Separator: true},
		insNote:        {Label: noteLabel, Shortcut: "Shift+F2", Disabled: editing},
		insDeleteNote:  {Label: "Delete Notes", Disabled: editing || !hasNote},
	}}
}

func (a *App) insertChosen(item int) {
	switch item {
	case insCells:
		a.insertCellsDialog()
	case insDeleteCells:
		a.deleteCellsDialog()
	case insRows, insCols:
		a.structural(true, item == insCols)
	case insSheet:
		a.insertSheet()
	case insFunction:
		a.insertFunctionDialog()
	case insSymbol:
		a.insertSymbolDialog()
	case insHyperlink:
		a.insertHyperlinkDialog()
	case insRemoveLink:
		a.clearHyperlinks()
	case insNote:
		a.newNote()
	case insDeleteNote:
		a.clearNotes()
	}
}

// --- Insert / Delete Cells ---

// insertCellsDialog asks how to insert cells in place of the selection
// (Excel's Insert dialog). Whole rows or columns are inserted at once.
func (a *App) insertCellsDialog() { a.cellsDialog(true) }

// deleteCellsDialog asks how to delete the selected cells.
func (a *App) deleteCellsDialog() { a.cellsDialog(false) }

func (a *App) cellsDialog(insert bool) {
	if !a.commitForCommand() {
		return
	}
	r := a.state().view.Sel.Range
	if wholeRows(r) || wholeCols(r) {
		a.insertDelete(insert) // whole rows or columns: no question
		return
	}
	if insert {
		a.showDialog(dlgInsertCells)
		a.ins.shift.Value = "down"
		if r.R2-r.R1 > r.C2-r.C1 {
			a.ins.shift.Value = "right"
		}
	} else {
		a.showDialog(dlgDeleteCells)
		a.ins.shift.Value = "up"
		if r.R2-r.R1 > r.C2-r.C1 {
			a.ins.shift.Value = "left"
		}
	}
}

func (a *App) layoutCellsDialog(gtx layout.Context, k int) {
	d := &a.ins
	insert := d.kind == dlgInsertCells
	for d.ok.Clicked(gtx) {
		k = 1
	}
	for d.cancel.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		how := map[string]edit.Shift{"right": edit.ShiftRight, "down": edit.ShiftDown, "left": edit.ShiftLeft,
			"up": edit.ShiftUp, "row": edit.EntireRow, "col": edit.EntireCol}[d.shift.Value]
		a.closeDialog()
		a.shiftCells(insert, how)
		return
	}
	title, opts := "Insert", [][2]string{{"right", "Shift cells right"}, {"down", "Shift cells down"}, {"row", "Entire row"}, {"col", "Entire column"}}
	if !insert {
		title, opts = "Delete", [][2]string{{"left", "Shift cells left"}, {"up", "Shift cells up"}, {"row", "Entire row"}, {"col", "Entire column"}}
	}
	a.focusArea(gtx)
	body := func(gtx layout.Context) layout.Dimensions {
		var rows []layout.FlexChild
		for _, o := range opts {
			rows = append(rows, layout.Rigid(a.radio(&d.shift, o[0], o[1])))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	a.modal(gtx, &d.block, title, 260, body, a.accentButton(&d.ok, "OK"), a.plainButton(&d.cancel, "Cancel"))
}

// shiftCells inserts or deletes the selection's cells.
func (a *App) shiftCells(insert bool, how edit.Shift) {
	r := a.state().view.Sel.Range
	var (
		res calc.Result
		err error
	)
	if insert {
		res, err = a.book.InsertCells(a.active, r, how)
	} else {
		res, err = a.book.DeleteCells(a.active, r, how)
	}
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.syncSheets()
	a.changed(res)
	switch how {
	case edit.EntireRow:
		r.C1, r.C2 = 0, model.MaxCols-1
	case edit.EntireCol:
		r.R1, r.R2 = 0, model.MaxRows-1
	}
	a.selectRange(a.active, r)
}

// --- Function ---

type functionDialog struct {
	search  widget.Editor
	cat     gui.DropDown
	catIdx  int // 0 = all
	cats    []functions.Category
	list    widget.List
	btns    []widget.Clickable
	shown   []string
	sel     string
	lastHit string
}

// insertFunctionDialog lists the implemented functions by category with
// a search box; the chosen one is inserted as =NAME( into the entry.
func (a *App) insertFunctionDialog() {
	d := &a.ins.fn
	if d.cats == nil {
		d.cats = functions.Categories()
	}
	d.search.SingleLine, d.search.Submit = true, true
	d.search.SetText("")
	d.sel = ""
	a.showDialog(dlgFunction)
}

func (d *functionDialog) filter() []string {
	q := strings.ToUpper(strings.TrimSpace(d.search.Text()))
	var out []string
	for i, c := range d.cats {
		if d.catIdx != 0 && d.catIdx != i+1 {
			continue
		}
		for _, n := range c.Funcs {
			if q == "" || strings.Contains(n, q) {
				out = append(out, n)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (a *App) layoutFunctionDialog(gtx layout.Context, k int) {
	d := &a.ins.fn
	for {
		if _, ok := d.search.Update(gtx); !ok {
			break
		}
	}
	d.shown = d.filter()
	if !slices.Contains(d.shown, d.sel) {
		d.sel = ""
		if len(d.shown) > 0 {
			d.sel = d.shown[0]
		}
	}
	if len(d.btns) < len(d.shown) {
		d.btns = make([]widget.Clickable, len(d.shown))
	}
	for i, n := range d.shown {
		for {
			c, ok := d.btns[i].Update(gtx)
			if !ok {
				break
			}
			d.sel = n
			if c.NumClicks >= 2 {
				k = 1
			}
		}
	}
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 && d.sel != "" {
		a.closeDialog()
		a.insertIntoEntry("="+d.sel+"(", true)
		return
	}
	names := []string{"All"}
	for _, c := range d.cats {
		names = append(names, c.Name)
	}
	th := a.ui
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			a.labeled("Search:", 90, func(gtx layout.Context) layout.Dimensions {
				return a.textField(gtx, &d.search, 300, 1, "Type part of a function name")
			}),
			a.labeled("Category:", 90, func(gtx layout.Context) layout.Dimensions {
				if i := d.cat.Layout(gtx, th, 220, names[d.catIdx], dropItems(names)); i >= 0 {
					d.catIdx = i
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(220), gtx.Dp(24))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(260)
				w := gtx.Constraints.Max.X
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
				gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
				g := gtx
				g.Constraints = layout.Exact(image.Pt(w-2, h-2))
				d.list.Axis = layout.Vertical
				defer opOffset(gtx, 1, 1).Pop()
				material.List(a.th, &d.list).Layout(g, len(d.shown), func(gtx layout.Context, i int) layout.Dimensions {
					n := d.shown[i]
					bg := colors.Transparent
					if n == d.sel {
						bg = th.AccentTint
					}
					return gui.Hoverable(gtx, &d.btns[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Top: 3, Bottom: 3, Left: 8}.Layout(gtx, th.Label(13, n, chromeText, font.Normal).Layout)
					})
				})
				return layout.Dimensions{Size: image.Pt(w, h)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := ""
				if d.sel != "" {
					s = d.sel + "(…)  — double-click or OK to insert"
				}
				return layout.Inset{Top: 6}.Layout(gtx, th.Label(12, s, mutedText, font.Normal).Layout)
			}),
		)
	}
	a.modal(gtx, &a.ins.block, "Insert Function", 440, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// insertIntoEntry types s into the cell entry at the caret, starting an
// entry when none is being edited (replacing the cell's content when
// replace is set, else editing it).
func (a *App) insertIntoEntry(s string, replace bool) {
	if !a.ed.active {
		if replace {
			a.beginEdit("", false, false)
		} else {
			st := a.state()
			a.beginEdit(editText(st.view.Sheet, st.view.Sel.Active), false, false)
		}
	}
	ed := &a.ed.cell
	if a.ed.inBar {
		ed = &a.ed.bar
	}
	ed.Insert(s)
	a.ed.enter = false // arrow keys move the caret, as after a typed function
	a.syncEditors()
	a.ed.focus = ed
}

// --- Symbol ---

type symbolDialog struct {
	btns []widget.Clickable
	sel  int
}

// symbols are the characters offered by Insert Symbol.
var symbols = []string{
	"©", "®", "™", "°", "±", "×", "÷", "≠", "≤", "≥", "≈", "∞", "√", "∑", "∏", "∂",
	"π", "µ", "Ω", "α", "β", "γ", "δ", "ε", "θ", "λ", "σ", "φ", "ω", "Δ", "Σ", "Φ",
	"€", "£", "¥", "¢", "₹", "₩", "₽", "¤", "§", "¶", "•", "·", "…", "–", "—", "†",
	"‘", "’", "“", "”", "«", "»", "‹", "›", "¿", "¡", "‰", "′", "″", "½", "¼", "¾",
	"¹", "²", "³", "⁴", "ⁿ", "←", "→", "↑", "↓", "↔", "⇒", "⇔", "∈", "∩", "∪", "∀",
	"✓", "✗", "★", "☆", "♠", "♣", "♥", "♦", "■", "□", "▲", "▼", "●", "○", "◆", "◇",
}

// insertSymbolDialog shows a grid of symbols to type into the entry.
func (a *App) insertSymbolDialog() {
	a.showDialog(dlgSymbol)
}

func (a *App) layoutSymbolDialog(gtx layout.Context, k int) {
	d := &a.ins.sym
	if len(d.btns) != len(symbols) {
		d.btns = make([]widget.Clickable, len(symbols))
	}
	for i := range symbols {
		for {
			c, ok := d.btns[i].Update(gtx)
			if !ok {
				break
			}
			d.sel = i
			if c.NumClicks >= 2 {
				k = 1
			}
		}
	}
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		// Insert keeps the dialog open, as in Excel.
		a.insertIntoEntry(symbols[d.sel], false)
	}
	th := a.ui
	a.focusArea(gtx)
	const perRow = 16
	body := func(gtx layout.Context) layout.Dimensions {
		var rows []layout.FlexChild
		for r := 0; r*perRow < len(symbols); r++ {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var cells []layout.FlexChild
				for i := r * perRow; i < min((r+1)*perRow, len(symbols)); i++ {
					cells = append(cells, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						bg := colors.White
						if i == d.sel {
							bg = th.AccentTint
						}
						return gui.Hoverable(gtx, &d.btns[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
							s := gtx.Dp(30)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							gui.HLine(gtx.Ops, 0, s, s-1, 1, chromeLine)
							gui.VLine(gtx.Ops, s-1, 0, s, 1, chromeLine)
							return layout.Center.Layout(gtx, th.Label(16, symbols[i], chromeText, font.Normal).Layout)
						})
					}))
				}
				return layout.Flex{}.Layout(gtx, cells...)
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8}.Layout(gtx, th.Label(12, "Insert types the symbol into the active cell's entry.", mutedText, font.Normal).Layout)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	a.modal(gtx, &a.ins.block, "Symbol", unit.Dp(30*perRow+40), body, a.accentButton(&a.ins.ok, "Insert"), a.plainButton(&a.ins.cancel, "Close"))
}

// --- Hyperlink ---

type linkDialog struct {
	display, address, tip widget.Editor
	sheet, row, col       int
	existing              bool
	focusAddr             bool // the cell has text: start in Address
}

// insertHyperlinkDialog edits the active cell's hyperlink (Ctrl+K): text
// to display, address (a URL, mailto:, file, or #Sheet!A1 for a place in
// the workbook) and ScreenTip.
func (a *App) insertHyperlinkDialog() {
	if !a.commitForCommand() {
		return
	}
	v := &a.state().view
	p := v.Sel.Active
	d := &a.ins.link
	d.sheet, d.row, d.col = a.active, p.Row, p.Col
	h, ok := v.Sheet.LinkAt(p.Row, p.Col)
	d.existing = ok
	for _, e := range []*widget.Editor{&d.display, &d.address, &d.tip} {
		e.SingleLine, e.Submit = true, true
	}
	d.display.SetText(v.Sheet.Get(p.Row, p.Col).Text)
	addr := ""
	if ok {
		addr = h.Address()
	}
	d.address.SetText(addr)
	d.tip.SetText(h.Tooltip)
	d.focusAddr = d.display.Text() != ""
	a.showDialog(dlgHyperlink)
}

// parseAddress splits a typed address into target and location.
func parseAddress(s string) model.Hyperlink {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		return model.Hyperlink{Location: s[1:]}
	}
	target, loc, _ := strings.Cut(s, "#")
	if !strings.Contains(target, ":") && !strings.HasPrefix(target, `\\`) && strings.Contains(target, ".") &&
		!strings.ContainsAny(target, `\/`) && !strings.Contains(target, "@") {
		// www.example.com, as Excel completes it.
		target = "http://" + target
	} else if !strings.Contains(target, ":") && strings.Contains(target, "@") && !strings.ContainsAny(target, `\/`) {
		target = "mailto:" + target
	}
	return model.Hyperlink{Target: target, Location: loc}
}

func (a *App) layoutLinkDialog(gtx layout.Context, k int) {
	d := &a.ins.link
	for _, e := range []*widget.Editor{&d.display, &d.address, &d.tip} {
		for {
			ev, ok := e.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				k = 1
			}
		}
	}
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	remove := false
	for a.ins.extra.Clicked(gtx) {
		remove = true
	}
	switch {
	case k < 0:
		a.closeDialog()
		return
	case remove:
		a.closeDialog()
		r := model.Range{R1: d.row, C1: d.col, R2: d.row, C2: d.col}
		if a.book.ClearHyperlinks(d.sheet, r) {
			a.changed(calc.Result{})
		}
		return
	case k > 0:
		addr := strings.TrimSpace(d.address.Text())
		if addr == "" {
			a.ins.err = "Type an address"
			break
		}
		h := parseAddress(addr)
		h.Tooltip = strings.TrimSpace(d.tip.Text())
		if h.Target == "" {
			if _, _, err := a.book.ResolveRef(h.Location, d.sheet); err != nil {
				a.ins.err = "Reference isn't valid: " + h.Location
				break
			}
		}
		a.closeDialog()
		res, err := a.book.SetHyperlink(d.sheet, d.row, d.col, d.display.Text(), h)
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.changed(res)
		return
	}
	body := func(gtx layout.Context) layout.Dimensions {
		field := func(e *widget.Editor, hint string) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions { return a.textField(gtx, e, 360, 1, hint) }
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			a.labeled("Text to display:", 120, field(&d.display, "")),
			a.labeled("Address:", 120, field(&d.address, "https://…, mailto:…, a file, or #Sheet2!A1")),
			a.labeled("ScreenTip:", 120, field(&d.tip, "")),
			layout.Rigid(a.ui.Label(12, "Ctrl+click the cell to follow the link.", mutedText, font.Normal).Layout),
			a.errorLine(),
		)
	}
	btns := []layout.Widget{a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel")}
	if d.existing {
		btns = append([]layout.Widget{a.plainButton(&a.ins.extra, "Remove Link")}, btns...)
	}
	a.modal(gtx, &a.ins.block, "Insert Hyperlink", 540, body, btns...)
}

// clearHyperlinks removes the selection's hyperlinks, keeping the text
// (Excel's Clear Hyperlinks / Remove Hyperlinks).
func (a *App) clearHyperlinks() {
	if !a.commitForCommand() {
		return
	}
	if a.book.ClearHyperlinks(a.active, a.state().view.Sel.Range) {
		a.changed(calc.Result{})
	}
}
