package ui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Data validation in the window: the Data Validation dialog (Settings,
// Input Message and Error Alert tabs), the error alert for a typed entry
// that breaks its cell's rule, a List rule's drop-down button and list
// beside the active cell, the input message box, and Circle Invalid
// Data's red circles. The rules are checked by excel/edit.

var (
	allowNames = []string{"Any value", "Whole number", "Decimal", "List", "Date", "Time", "Text length", "Custom"}
	opNames    = []string{"between", "not between", "equal to", "not equal to", "greater than", "less than",
		"greater than or equal to", "less than or equal to"}
	styleNames = []string{"Stop", "Warning", "Information"}
)

// Colors of the input message box (Excel's pale yellow tip) and the
// circles.
var (
	inputMsgBG     = colors.RGB(0xFFFFE1)
	inputMsgBorder = colors.RGB(0x767676)
	circleColor    = colors.RGB(0xFF0000)
)

// validDialog is the Data Validation dialog.
type validDialog struct {
	ranges []model.Range
	row    int // the active cell, which typed formulas are relative to
	col    int
	tab    int // 0 Settings, 1 Input Message, 2 Error Alert
	tabs   [3]widget.Clickable

	allow                 model.ValidationType
	op                    model.ValidationOp
	allowDD, dataDD       gui.DropDown
	ignoreBlank, dropDown widget.Bool
	f1, f2                widget.Editor
	showInput, showError  widget.Bool
	inTitle, inMsg        widget.Editor
	style                 model.ErrorStyle
	styleDD               gui.DropDown
	errTitle, errMsg      widget.Editor
	clearAll              widget.Clickable
}

// validationDialog opens Data Validation for the selection, showing the
// active cell's rule.
func (a *App) validationDialog() {
	if !a.commitForCommand() {
		return
	}
	d := &a.ins.dv
	sel := &a.state().view.Sel
	*d = validDialog{row: sel.Active.Row, col: sel.Active.Col}
	d.ranges = append([]model.Range{sel.Range}, sel.Extra...)
	for _, e := range []*widget.Editor{&d.f1, &d.f2, &d.inTitle, &d.errTitle} {
		e.SingleLine, e.Submit = true, true
	}
	v, ok := a.wb.Sheets[a.active].ValidationAt(d.row, d.col)
	if !ok {
		d.ignoreBlank.Value, d.dropDown.Value, d.showInput.Value, d.showError.Value = true, true, true, true
		a.showDialog(dlgValidation)
		return
	}
	d.allow, d.op, d.style = v.Type, v.Op, v.ErrorStyle
	d.ignoreBlank.Value, d.dropDown.Value = v.AllowBlank, !v.NoDropDown
	d.showInput.Value, d.showError.Value = v.ShowInput, v.ShowError
	d.inTitle.SetText(v.PromptTitle)
	d.inMsg.SetText(v.Prompt)
	d.errTitle.SetText(v.ErrorTitle)
	d.errMsg.SetText(v.Error)
	r0, c0 := v.TopLeft()
	show := func(f string) string {
		if f == "" {
			return ""
		}
		f = formula.Shift(f, d.row-r0, d.col-c0)
		return edit.ValidationBoundText(f, v.Type, a.wb.Date1904)
	}
	d.f1.SetText(show(v.Formula1))
	d.f2.SetText(show(v.Formula2))
	if v.Type == model.ValidCustom && d.f1.Text() != "" && !strings.HasPrefix(d.f1.Text(), "=") {
		d.f1.SetText("=" + d.f1.Text())
	}
	a.showDialog(dlgValidation)
}

// hasOp reports whether a rule type compares with bounds.
func hasOp(t model.ValidationType) bool {
	switch t {
	case model.ValidWhole, model.ValidDecimal, model.ValidDate, model.ValidTime, model.ValidTextLength:
		return true
	}
	return false
}

func between(op model.ValidationOp) bool {
	return op == model.ValidBetween || op == model.ValidNotBetween
}

// boundLabels are the Settings tab's names for the first and second box.
func boundLabels(t model.ValidationType, op model.ValidationOp) (string, string) {
	switch t {
	case model.ValidList:
		return "Source:", ""
	case model.ValidCustom:
		return "Formula:", ""
	}
	lo, hi, one := "Minimum:", "Maximum:", "Value:"
	switch t {
	case model.ValidDate:
		lo, hi, one = "Start date:", "End date:", "Date:"
	case model.ValidTime:
		lo, hi, one = "Start time:", "End time:", "Time:"
	case model.ValidTextLength:
		one = "Length:"
	}
	switch op {
	case model.ValidBetween, model.ValidNotBetween:
		return lo, hi
	case model.ValidGreater, model.ValidGreaterEqual:
		if t == model.ValidDate || t == model.ValidTime {
			return lo, ""
		}
		return "Minimum:", ""
	case model.ValidLess, model.ValidLessEqual:
		if t == model.ValidDate || t == model.ValidTime {
			return hi, ""
		}
		return "Maximum:", ""
	}
	return one, ""
}

// dialogRule builds the rule the dialog describes; ok is false (with
// a.ins.err set) when a box is wrong.
func (a *App) dialogRule() (model.Validation, bool) {
	d := &a.ins.dv
	v := model.Validation{Type: d.allow, Op: d.op, AllowBlank: d.ignoreBlank.Value, NoDropDown: !d.dropDown.Value,
		ShowInput: d.showInput.Value, ShowError: d.showError.Value, ErrorStyle: d.style,
		PromptTitle: strings.TrimSpace(d.inTitle.Text()), Prompt: d.inMsg.Text(),
		ErrorTitle: strings.TrimSpace(d.errTitle.Text()), Error: d.errMsg.Text()}
	if !hasOp(v.Type) {
		v.Op = model.ValidBetween
	}
	if v.Type == model.ValidAny {
		return v, true
	}
	lo, hi := boundLabels(v.Type, v.Op)
	// Typed formulas are relative to the active cell; the rule's to its
	// first range's top-left cell.
	r0, c0 := d.ranges[0].R1, d.ranges[0].C1
	read := func(ed *widget.Editor, label string) (string, bool) {
		text := strings.TrimSpace(ed.Text())
		if v.Type == model.ValidCustom && text != "" && !strings.HasPrefix(text, "=") {
			text = "=" + text
		}
		f, ok := edit.ValidationBound(text, v.Type == model.ValidList, a.wb.Date1904)
		if !ok {
			a.ins.err = "Enter a value or a formula (starting with =) for " + strings.TrimSuffix(label, ":") + "."
			if text == "" && v.Type == model.ValidList {
				a.ins.err = "The Source currently evaluates to an error: enter a list or a range."
			}
			return "", false
		}
		return formula.Shift(f, r0-d.row, c0-d.col), true
	}
	var ok bool
	if v.Formula1, ok = read(&d.f1, lo); !ok {
		return v, false
	}
	if hasOp(v.Type) && between(v.Op) {
		if v.Formula2, ok = read(&d.f2, hi); !ok {
			return v, false
		}
		x, e1 := strconv.ParseFloat(v.Formula1, 64)
		y, e2 := strconv.ParseFloat(v.Formula2, 64)
		if e1 == nil && e2 == nil && y < x {
			a.ins.err = "The " + strings.TrimSuffix(hi, ":") + " must be greater than or equal to the " + strings.TrimSuffix(lo, ":") + "."
			return v, false
		}
	}
	return v, true
}

func (a *App) layoutValidationDialog(gtx layout.Context, k int) {
	d := &a.ins.dv
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	for d.clearAll.Clicked(gtx) {
		// Excel's Clear All resets the dialog; OK then removes the rules.
		d.allow, d.op, d.style = model.ValidAny, model.ValidBetween, model.ErrorStop
		d.ignoreBlank.Value, d.dropDown.Value, d.showInput.Value, d.showError.Value = true, true, true, true
		for _, e := range []*widget.Editor{&d.f1, &d.f2, &d.inTitle, &d.inMsg, &d.errTitle, &d.errMsg} {
			e.SetText("")
		}
		a.ins.err, a.ins.focus = "", true
	}
	for i := range d.tabs {
		for d.tabs[i].Clicked(gtx) {
			d.tab = i
			a.ins.focus = true
		}
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		v, ok := a.dialogRule()
		if !ok {
			d.tab = 0
			return
		}
		a.closeDialog()
		if !a.readyToChange() {
			return
		}
		plain := v.Type == model.ValidAny && v.PromptTitle == "" && v.Prompt == "" && v.ErrorTitle == "" && v.Error == ""
		if plain {
			a.book.SetValidation(a.active, d.ranges, nil)
		} else {
			a.book.SetValidation(a.active, d.ranges, &v)
		}
		a.changed(calc.Result{})
		return
	}
	th := a.ui
	a.focusArea(gtx)
	combo := func(dd *gui.DropDown, w unit.Dp, cur int, labels []string, set func(int)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if j := dd.Layout(gtx, th, w, labels[cur], dropItems(labels)); j >= 0 {
				set(j)
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(w), gtx.Dp(24))}
		}
	}
	label := func(s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, th.Label(13, s, chromeText, font.Normal).Layout)
		})
	}
	field := func(ed *widget.Editor, lines int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return gui.TextField(gtx, a.ui, ed, 400, lines, "")
			})
		})
	}
	settings := func(gtx layout.Context) layout.Dimensions {
		lo, hi := boundLabels(d.allow, d.op)
		rows := []layout.FlexChild{
			label("Validation criteria"),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(50)
						return th.Label(13, "Allow:", chromeText, font.Normal).Layout(gtx)
					}),
					layout.Rigid(combo(&d.allowDD, 160, int(d.allow), allowNames, func(j int) {
						d.allow = model.ValidationType(j)
						a.ins.focus = true // the first box, now showing
					})),
					layout.Rigid(layout.Spacer{Width: 16}.Layout),
					layout.Rigid(a.ui.CheckBox(&d.ignoreBlank, "Ignore blank")),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
		}
		if d.allow == model.ValidList {
			rows = append(rows, layout.Rigid(a.ui.CheckBox(&d.dropDown, "In-cell dropdown")))
		}
		if hasOp(d.allow) {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(50)
							return th.Label(13, "Data:", chromeText, font.Normal).Layout(gtx)
						}),
						layout.Rigid(combo(&d.dataDD, 200, int(d.op), opNames, func(j int) { d.op = model.ValidationOp(j) })),
					)
				})
			}))
		}
		if d.allow != model.ValidAny {
			rows = append(rows, label(lo), field(&d.f1, 1))
			if hi != "" {
				rows = append(rows, label(hi), field(&d.f2, 1))
			}
		}
		rows = append(rows, a.errorLine())
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
	input := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.ui.CheckBox(&d.showInput, "Show input message when cell is selected")),
			label("When cell is selected, show this input message:"),
			label("Title:"), field(&d.inTitle, 1),
			label("Input message:"), field(&d.inMsg, 4),
		)
	}
	alert := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.ui.CheckBox(&d.showError, "Show error alert after invalid data is entered")),
			label("When user enters invalid data, show this error alert:"),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(50)
						return th.Label(13, "Style:", chromeText, font.Normal).Layout(gtx)
					}),
					layout.Rigid(combo(&d.styleDD, 140, int(d.style), styleNames, func(j int) { d.style = model.ErrorStyle(j) })),
				)
			}),
			label("Title:"), field(&d.errTitle, 1),
			label("Error message:"), field(&d.errMsg, 4),
		)
	}
	body := func(gtx layout.Context) layout.Dimensions {
		names := []string{"Settings", "Input Message", "Error Alert"}
		var tabs []layout.FlexChild
		for i, n := range names {
			tabs = append(tabs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx, th, &d.tabs[i], nil, n, d.tab == i, colors.Transparent)
			}))
		}
		page := []layout.Widget{settings, input, alert}[d.tab]
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Flex{}.Layout(gtx, tabs...) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gui.HLine(gtx.Ops, 0, gtx.Constraints.Max.X, 0, 1, chromeLine)
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(10))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(300)
				return page(gtx)
			}),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Data Validation", 460, body,
		a.plainButton(&d.clearAll, "Clear All"), a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// validationTags are what has the keyboard on each tab.
func (d *validDialog) focusTags(tag *int) []any {
	switch d.tab {
	case 1:
		return []any{&d.inTitle, tag}
	case 2:
		return []any{&d.errTitle, tag}
	}
	if d.allow == model.ValidAny {
		return []any{tag}
	}
	return []any{&d.f1, &d.f2, tag}
}

// entryAllowed checks the entry being committed against its cell's rule.
// An entry that breaks it shows the rule's error alert (when it has one)
// and keeps editing; the alert's answer commits, edits again or cancels.
func (a *App) entryAllowed(text string, dr, dc int) bool {
	ed := &a.ed
	if ed.checked {
		ed.checked = false
		return true
	}
	v := a.book.CheckEntry(ed.sheet, ed.row, ed.col, text)
	if v == nil || !v.ShowError {
		return true
	}
	title, msg := v.ErrorTitle, v.Error
	if title == "" {
		title = appName
	}
	if msg == "" {
		msg = edit.DefaultValidationError
	}
	again := func() {
		// Retry / No: back to the entry, its text selected.
		ed.focus = &ed.cell
		if ed.inBar {
			ed.focus = &ed.bar
		}
		ed.focus.SetCaret(ed.focus.Len(), 0)
	}
	commit := func() {
		ed.checked = true
		a.commitEdit(dr, dc)
	}
	switch v.ErrorStyle {
	case model.ErrorWarning:
		a.dialog.Show(title, msg+"\n\nContinue?", "Yes", "No", "Cancel")
		a.answer = func(i int) {
			switch i {
			case 0:
				commit()
			case 1:
				again()
			default:
				a.cancelEdit()
			}
		}
	case model.ErrorInformation:
		a.dialog.Show(title, msg, "OK", "Cancel")
		a.answer = func(i int) {
			if i == 0 {
				commit()
				return
			}
			a.cancelEdit()
		}
	default:
		a.dialog.Show(title, msg, "Retry", "Cancel")
		a.answer = func(i int) {
			if i == 0 {
				again()
				return
			}
			a.cancelEdit()
		}
	}
	return false
}

// circleInvalid circles the cells that break their rules (Data
// Validation > Circle Invalid Data).
func (a *App) circleInvalid() {
	if !a.commitForCommand() {
		return
	}
	st := a.state()
	st.circles = a.book.InvalidCells(a.active)
	if st.circles == nil {
		st.circles = [][2]int{}
	}
	if len(st.circles) == 0 {
		a.notice = "No invalid data found"
	}
}

// clearCircles removes the circles (Clear Validation Circles).
func (a *App) clearCircles() {
	a.state().circles = nil
}

// refreshCircles drops the circles of cells that became valid, after a
// change.
func (a *App) refreshCircles() {
	for i, st := range a.states {
		if st.circles == nil || i >= len(a.wb.Sheets) {
			continue
		}
		out := st.circles[:0]
		for _, k := range st.circles {
			if !a.book.CellValid(i, k[0], k[1]) {
				out = append(out, k)
			}
		}
		st.circles = out
	}
}

// drawValidation draws the circles, the active cell's list button and
// its input message.
func (a *App) drawValidation(gtx layout.Context, v *render.View) {
	st := a.state()
	for _, k := range st.circles {
		rc, _, ok := v.CellRect(k[0], k[1])
		if !ok {
			continue
		}
		r := rc.Inset(-gtx.Dp(4))
		w := float32(max(gtx.Dp(1.5), 1))
		path := clip.Ellipse(r).Path(gtx.Ops)
		paint.FillShape(gtx.Ops, circleColor, clip.Stroke{Path: path, Width: w}.Op())
	}
	a.vlist.btn = image.Rectangle{}
	p := v.Sel.Active
	if a.ed.active && (a.ed.row != p.Row || a.ed.col != p.Col) {
		return
	}
	rule, ok := v.Sheet.ValidationAt(p.Row, p.Col)
	if !ok {
		return
	}
	rc, _, ok := v.CellRect(p.Row, p.Col)
	if !ok {
		return
	}
	if m, merged := v.Sheet.MergeAt(p.Row, p.Col); merged {
		if r2, _, ok := v.CellRect(m.R2, m.C2); ok {
			rc.Max = r2.Max
		}
	}
	if rule.Type == model.ValidList && !rule.NoDropDown {
		s := min(gtx.Dp(unit.Dp(17*a.zoom)), rc.Dy())
		b := image.Rect(rc.Max.X+1, rc.Max.Y-s, rc.Max.X+1+s, rc.Max.Y)
		a.vlist.btn = b
		bg := colors.RGB(0xF3F3F3)
		if a.vlist.open {
			bg = colors.RGB(0xDADADA)
		}
		gui.FillRect(gtx.Ops, b, colors.RGB(0xA6A6A6))
		gui.FillRect(gtx.Ops, b.Inset(1), bg)
		at := func(x, y float32) f32.Point {
			return f32.Pt(float32(b.Min.X)+x*float32(b.Dx()), float32(b.Min.Y)+y*float32(b.Dy()))
		}
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(at(0.27, 0.40))
		path.LineTo(at(0.73, 0.40))
		path.LineTo(at(0.5, 0.66))
		path.Close()
		paint.FillShape(gtx.Ops, colors.RGB(0x404040), clip.Outline{Path: path.End()}.Op())
		area := clip.Rect(b).Push(gtx.Ops)
		pointer.CursorDefault.Add(gtx.Ops)
		area.Pop()
	}
	if rule.ShowInput && (rule.PromptTitle != "" || rule.Prompt != "") && !a.vlist.open && a.ins.kind == dlgNone {
		a.inputMessage(gtx, rule, image.Pt(rc.Min.X+gtx.Dp(12), rc.Max.Y+gtx.Dp(10)))
	}
}

// inputMessage draws a rule's input message box at pt (moved to stay in
// the grid).
func (a *App) inputMessage(gtx layout.Context, rule *model.Validation, pt image.Point) {
	th := a.ui
	w := gtx.Dp(210)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(w, gtx.Dp(300))}
	m := op.Record(gtx.Ops)
	dims := layout.UniformInset(6).Layout(g, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if rule.PromptTitle == "" {
					return layout.Dimensions{}
				}
				return th.Label(12, rule.PromptTitle, chromeText, font.Bold).Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := th.Label(12, rule.Prompt, chromeText, font.Normal)
				l.MaxLines = 14
				return l.Layout(gtx)
			}),
		)
	})
	call := m.Stop()
	size := gtx.Constraints.Max
	pt.X = max(min(pt.X, size.X-dims.Size.X-2), 0)
	if pt.Y+dims.Size.Y > size.Y {
		pt.Y = max(size.Y-dims.Size.Y-2, 0)
	}
	off := op.Offset(pt).Push(gtx.Ops)
	box := image.Rectangle{Max: dims.Size}
	gui.FillRect(gtx.Ops, box.Inset(-1), inputMsgBorder)
	gui.FillRect(gtx.Ops, box, inputMsgBG)
	call.Add(gtx.Ops)
	off.Pop()
}

// validList is a List rule's open drop-down.
type validList struct {
	btn             image.Rectangle // the active cell's button, from the last frame
	open            bool
	sheet, row, col int
	at              image.Point // top-left, grid coordinates
	width           int         // px
	items           []string
	rows            []widget.Clickable
	hover           int // the highlighted entry
	mouse           int // 1 + the entry last under the pointer, 0 for none
	list            widget.List
	catcher, body   int // event tags
	focus           bool
}

// List metrics, in dp.
const (
	validRowH     = 20
	validMaxShown = 8
)

// validPress opens or closes the list when its button is at pos; it
// reports whether it was.
func (a *App) validPress(v *render.View, pos f32.Point) bool {
	if !image.Pt(int(pos.X), int(pos.Y)).In(a.vlist.btn) {
		return false
	}
	if a.vlist.open {
		a.closeValidList()
		return true
	}
	a.openValidList()
	return true
}

// openValidList opens the active cell's list (its button, or Alt+Down).
func (a *App) openValidList() bool {
	st := a.state()
	v := &st.view
	p := v.Sel.Active
	items := a.book.ListItems(a.active, p.Row, p.Col)
	if items == nil || !a.commitForCommand() {
		return false
	}
	rc, _, ok := v.CellRect(p.Row, p.Col)
	if !ok {
		return false
	}
	l := &a.vlist
	cur := v.Sheet.Get(p.Row, p.Col).Text
	*l = validList{btn: l.btn, open: true, sheet: a.active, row: p.Row, col: p.Col, items: items, focus: true,
		at: image.Pt(rc.Min.X, rc.Max.Y+1), width: max(rc.Dx()+l.btn.Dx()+1, 100)}
	l.rows = make([]widget.Clickable, len(items))
	l.list.Axis = layout.Vertical
	for i, it := range items {
		if strings.EqualFold(it, cur) {
			l.hover = i
			l.list.Position.First = max(i-validMaxShown+1, 0)
		}
	}
	a.menu.Close()
	return true
}

// closeValidList closes the list without choosing.
func (a *App) closeValidList() {
	if !a.vlist.open {
		return
	}
	a.vlist.open = false
	a.focusGridNow()
}

// focusGridNow gives the keyboard back to the grid in this frame, so
// that keys pressed right after go to it.
func (a *App) focusGridNow() {
	if a.gtx.Ops != nil {
		requestFocus(a.gtx, &a.grid)
		return
	}
	a.grid.focusPending = true
}

// pickValid enters entry i of the open list in its cell.
func (a *App) pickValid(i int) {
	l := &a.vlist
	l.open = false
	a.focusGridNow()
	if i < 0 || i >= len(l.items) || !a.readyToChange() {
		return
	}
	res, err := a.book.Enter(l.sheet, l.row, l.col, l.items[i])
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
}

// layoutValidList draws the open list (in grid coordinates).
func (a *App) layoutValidList(gtx layout.Context) {
	l := &a.vlist
	if !l.open {
		return
	}
	if l.sheet != a.active {
		l.open = false
		return
	}
	for i := range l.rows {
		for l.rows[i].Clicked(gtx) {
			a.pickValid(i)
			return
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &l.catcher, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			a.closeValidList()
			return
		}
	}
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: &l.body},
			key.Filter{Focus: &l.body, Name: key.NameUpArrow},
			key.Filter{Focus: &l.body, Name: key.NameDownArrow, Optional: key.ModAlt},
			key.Filter{Focus: &l.body, Name: key.NameReturn},
			key.Filter{Focus: &l.body, Name: key.NameEnter},
			key.Filter{Focus: &l.body, Name: key.NameTab},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch e.Name {
		case key.NameUpArrow:
			l.hover = max(l.hover-1, 0)
		case key.NameDownArrow:
			l.hover = min(l.hover+1, len(l.items)-1)
		default:
			a.pickValid(l.hover)
			return
		}
		if l.hover < l.list.Position.First {
			l.list.Position.First = l.hover
		} else if l.hover >= l.list.Position.First+validMaxShown {
			l.list.Position.First = l.hover - validMaxShown + 1
		}
		l.list.Position.Offset = 0
	}
	if l.focus {
		l.focus = false
		gtx.Execute(key.FocusCmd{Tag: &l.body})
	}

	rec := op.Record(gtx.Ops)
	big := image.Rect(-1<<15, -1<<15, 1<<15, 1<<15)
	area := clip.Rect(big).Push(gtx.Ops)
	event.Op(gtx.Ops, &l.catcher)
	area.Pop()
	rowH := gtx.Dp(validRowH)
	sz := image.Pt(l.width, rowH*min(max(len(l.items), 1), validMaxShown)+2)
	size := gtx.Constraints.Max
	pos := l.at
	pos.X = max(min(pos.X, size.X-sz.X-2), 0)
	if pos.Y+sz.Y > size.Y {
		pos.Y = max(l.at.Y-sz.Y-rowH-2, 0) // above the cell when there is no room below
	}
	off := op.Offset(pos).Push(gtx.Ops)
	gui.FillRect(gtx.Ops, image.Rect(2, 2, sz.X+2, sz.Y+2), colors.RGBA(0, 0, 0, 0x22))
	gui.FillRect(gtx.Ops, image.Rectangle{Max: sz}, chromeLine)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: sz}.Inset(1), colors.White)
	stop := clip.Rect{Max: sz}.Push(gtx.Ops)
	event.Op(gtx.Ops, &l.body)
	pointer.CursorDefault.Add(gtx.Ops)
	stop.Pop()
	g := gtx
	g.Constraints = layout.Exact(sz.Sub(image.Pt(2, 2)))
	o2 := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
	cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
	material.List(a.th, &l.list).Layout(g, len(l.items), func(gtx layout.Context, i int) layout.Dimensions {
		w := gtx.Constraints.Max.X
		return l.rows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// The pointer moving onto a row highlights it; the keys
			// move the highlight from there.
			if l.rows[i].Hovered() && l.mouse != i+1 {
				l.hover, l.mouse = i, i+1
			}
			fg := chromeText
			if i == l.hover {
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, rowH), accent)
				fg = colors.White
			}
			lg := gtx
			lg.Constraints = layout.Constraints{Max: image.Pt(w-gtx.Dp(8), rowH)}
			m := op.Record(gtx.Ops)
			d := a.ui.Label(13, l.items[i], fg, font.Normal).Layout(lg)
			call := m.Stop()
			t := op.Offset(image.Pt(gtx.Dp(4), (rowH-d.Size.Y)/2)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			t.Pop()
			return layout.Dimensions{Size: image.Pt(w, rowH)}
		})
	})
	cs.Pop()
	o2.Pop()
	off.Pop()
	op.Defer(gtx.Ops, rec.Stop())
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &l.body, Kinds: pointer.Press}); !ok {
			break
		}
	}
}
