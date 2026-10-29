package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/key"

	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/model"
)

// dataTab shows the Data ribbon.
func (h *harness) dataTab() {
	h.a.tab = tabData
	h.frame()
}

// answerDialog presses button i of the message box.
func (h *harness) answerDialog(i int) {
	h.t.Helper()
	a := h.a
	if !a.dialog.Visible {
		h.t.Fatal("no message box")
	}
	answer := a.answer
	a.dialog.Hide()
	a.answer = nil
	if answer != nil {
		answer(i)
	}
	h.frame()
}

// modalArea is where a dialog w dp wide centered in the window lies.
func modalArea(w int) image.Rectangle {
	return image.Rect(500-w/2, 100, 500+w/2, 520)
}

func TestTextToColumnsUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	for i, s := range []string{"Name,Qty,Code", `"Smith, J",12,007`, "Lee,3,042"} {
		a.book.Enter(0, i, 0, s)
	}
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 2, C2: 1})
	h.dataTab()
	p := h.locate(&a.data.textToCols, ribbonArea, 6)
	h.click(p.X, p.Y)
	if !a.dialog.Visible || a.dialog.Message != edit.ErrTextToColsShape.Error() {
		t.Fatalf("two columns: dialog %v %q", a.dialog.Visible, a.dialog.Message)
	}
	h.answerDialog(0)

	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 2, C2: 0})
	h.frame()
	h.click(p.X, p.Y)
	d := &a.ins.ttc
	if a.ins.kind != dlgTextToCols || !d.comma.Value || d.tab.Value || d.dest.Text() != "=$A$1" {
		t.Fatalf("dialog %v, comma %v tab %v dest %q", a.ins.kind, d.comma.Value, d.tab.Value, d.dest.Text())
	}
	// The third column is Text: click its preview header, then Text.
	h.frame()
	d.sel = 2
	d.fmtEnum.Value = "text"
	d.setFormat(2, edit.ColText)
	h.frame()
	if d.formatOf(2) != edit.ColText {
		t.Fatalf("column 3 format %v", d.formatOf(2))
	}
	h.press(key.NameReturn, 0)
	if a.ins.kind != dlgNone {
		t.Fatalf("still open: %q", a.ins.err)
	}
	h.expect("split", map[string]string{"A1": "Name", "B1": "Qty", "C1": "Code", "A2": "Smith, J", "B2": "12", "C2": "007", "C3": "042"})
	if h.cell("B2").Kind != model.KindNumber || h.cell("C2").Kind != model.KindText {
		t.Errorf("kinds B2 %v C2 %v", h.cell("B2").Kind, h.cell("C2").Kind)
	}
	// One undo step.
	h.press("Z", key.ModShortcut)
	if h.cell("A2").Text != `"Smith, J",12,007` || h.cell("B2").Text != "" {
		t.Errorf("undo: A2 %q B2 %q", h.cell("A2").Text, h.cell("B2").Text)
	}

	// A destination over other data asks first.
	sh.Set(0, 5, model.Cell{Kind: model.KindText, Text: "keep"})
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 2, C2: 0})
	a.textToColumnsDialog()
	h.frame()
	d.dest.SetText("E1")
	h.press(key.NameReturn, 0)
	if !a.dialog.Visible || a.dialog.Message != "Do you want to replace the contents of the destination cells?" {
		t.Fatalf("overwrite question %v %q", a.dialog.Visible, a.dialog.Message)
	}
	h.answerDialog(1) // Cancel
	if h.cell("F1").Text != "keep" || h.cell("E1").Text != "" {
		t.Fatal("Cancel changed cells")
	}
	a.textToColumnsDialog()
	h.frame()
	d.dest.SetText("E1")
	h.press(key.NameReturn, 0)
	h.answerDialog(0)
	h.expect("to E1", map[string]string{"E1": "Name", "F1": "Qty", "G3": "42", "A1": "Name,Qty,Code"})

	// Fixed width: a click in the preview sets a break.
	a.book.Enter(0, 5, 0, "AB  12")
	a.selectRange(0, model.Range{R1: 5, C1: 0, R2: 5, C2: 0})
	a.textToColumnsDialog()
	d.kind.Value = "fixed"
	d.breaks = nil
	d.setBreaksText()
	h.frame()
	d.breaksEd.SetText("2")
	h.frame()
	if len(d.breaks) != 1 || d.breaks[0] != 2 {
		t.Fatalf("typed breaks %v", d.breaks)
	}
	h.press(key.NameReturn, 0)
	if h.cell("A6").Text != "AB" || h.cell("B6").Num != 12 {
		t.Errorf("fixed: %q %v", h.cell("A6").Text, h.cell("B6").Num)
	}
}

func TestRemoveDuplicatesUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.fillTable()
	a.selectRange(0, model.Range{R1: 2, C1: 1, R2: 2, C2: 1})
	h.dataTab()
	p := h.locate(&a.data.removeDups, ribbonArea, 6)
	h.click(p.X, p.Y)
	d := &a.ins.dup
	if a.ins.kind != dlgRemoveDups || d.region != (model.Range{R2: 6, C2: 2}) || !d.header.Value || len(d.cols) != 3 {
		t.Fatalf("dialog %v region %v header %v", a.ins.kind, d.region, d.header.Value)
	}
	// Unselect All, then check Kind only.
	q := h.locate(&d.none, modalArea(440), 10)
	h.click(q.X, q.Y)
	if d.cols[0].Value || d.cols[2].Value {
		t.Fatal("Unselect All")
	}
	ok := h.locate(&a.ins.ok, modalArea(440), 10)
	h.click(ok.X, ok.Y)
	if a.ins.kind != dlgRemoveDups || a.ins.err == "" {
		t.Fatal("OK with no columns")
	}
	d.cols[2].Value = true
	ok = h.locate(&a.ins.ok, modalArea(440), 10)
	h.click(ok.X, ok.Y)
	if a.ins.kind != dlgNone || a.dialog.Message != "4 duplicate values found and removed; 2 unique values remain." {
		t.Fatalf("message %q", a.dialog.Message)
	}
	h.answerDialog(0)
	h.expect("removed", map[string]string{"A2": "apple", "C2": "fruit", "A3": "bean", "C3": "veg", "A4": "", "B7": ""})
	h.press("Z", key.ModShortcut)
	h.expect("undo", map[string]string{"A4": "cherry", "A7": "endive"})
}

func TestValidationUI(t *testing.T) {
	h := newHarness(t)
	a := h.a
	sh := a.wb.Sheets[0]
	a.selectRange(0, model.Range{R1: 0, C1: 0, R2: 4, C2: 0})
	h.dataTab()
	// The Data Validation button opens its menu, whose first item opens
	// the dialog.
	btn := a.data.validDD.Button()
	p := h.locate(btn, ribbonArea, 4)
	h.click(p.X, p.Y)
	if !a.data.validDD.IsOpen() {
		t.Fatal("Data Validation menu not open")
	}
	// Pointer events don't reach a ribbon drop-down's list in this
	// harness (its items never see the pointer), so the item is chosen
	// as the list would.
	a.data.validDD.Close()
	a.validMenu(0)
	h.frame()
	d := &a.ins.dv
	if a.ins.kind != dlgValidation || len(d.ranges) != 1 || d.ranges[0] != (model.Range{R2: 4}) {
		t.Fatalf("dialog %v ranges %v", a.ins.kind, d.ranges)
	}
	d.allow = model.ValidList
	h.frame()
	d.f1.SetText("Red,Green,Blue")
	d.inTitle.SetText("Color")
	d.inMsg.SetText("Pick one")
	h.press(key.NameReturn, 0)
	if a.ins.kind != dlgNone || len(sh.Validations) != 1 {
		t.Fatalf("OK: dialog %v err %q rules %d", a.ins.kind, a.ins.err, len(sh.Validations))
	}
	if v := sh.Validations[0]; v.Type != model.ValidList || v.Formula1 != `"Red,Green,Blue"` || v.PromptTitle != "Color" || !v.ShowError || !v.AllowBlank {
		t.Errorf("rule %+v", v)
	}

	// The list button beside A2 opens the list; a click picks.
	a.selectRange(0, model.Range{R1: 1, C1: 0, R2: 1, C2: 0})
	h.frame()
	b := a.vlist.btn
	if b.Empty() {
		t.Fatal("no list button")
	}
	if rc, _, _ := a.state().view.CellRect(1, 0); b.Min.X <= rc.Max.X-1 {
		t.Errorf("button %v not right of the cell %v", b, rc)
	}
	h.click(float32(b.Min.X+b.Max.X)/2, float32(gridTop+(b.Min.Y+b.Max.Y)/2))
	l := &a.vlist
	if !l.open || strings.Join(l.items, ",") != "Red,Green,Blue" {
		t.Fatalf("list open %v items %q", l.open, l.items)
	}
	row := float32(gridTop + l.at.Y + 1 + validRowH*1 + validRowH/2)
	h.click(float32(l.at.X+20), row)
	if l.open || h.cell("A2").Text != "Green" {
		t.Fatalf("pick: open %v A2 %q", l.open, h.cell("A2").Text)
	}
	// Alt+Down opens it at the current entry; Down and Enter pick the
	// next.
	h.press(key.NameDownArrow, key.ModAlt)
	if !l.open || l.hover != 1 {
		t.Fatalf("Alt+Down: open %v hover %d", l.open, l.hover)
	}
	h.press(key.NameDownArrow, 0)
	h.press(key.NameReturn, 0)
	if l.open || h.cell("A2").Text != "Blue" {
		t.Fatalf("keys: open %v A2 %q", l.open, h.cell("A2").Text)
	}
	h.press(key.NameDownArrow, key.ModAlt)
	h.press(key.NameEscape, 0)
	if l.open {
		t.Fatal("Esc left the list open")
	}

	// A Stop alert: Retry keeps editing, Cancel restores the cell.
	h.enter("A3", "Purple")
	if !a.dialog.Visible || a.dialog.Message != edit.DefaultValidationError || !a.ed.active || a.dialog.Buttons[0] != "Retry" {
		t.Fatalf("stop alert: visible %v %q editing %v", a.dialog.Visible, a.dialog.Message, a.ed.active)
	}
	h.answerDialog(0)
	if !a.ed.active || h.cell("A3").Text != "" {
		t.Fatalf("Retry: editing %v", a.ed.active)
	}
	h.press(key.NameReturn, 0)
	h.answerDialog(1)
	if a.ed.active || h.cell("A3").Text != "" {
		t.Fatalf("Cancel: editing %v A3 %q", a.ed.active, h.cell("A3").Text)
	}
	h.enter("A3", "red")
	if a.dialog.Visible || h.cell("A3").Text != "red" {
		t.Fatalf("valid entry: alert %v A3 %q", a.dialog.Visible, h.cell("A3").Text)
	}

	// Warning (Yes keeps the entry) and Information (OK keeps it).
	a.selectRange(0, model.Range{R1: 0, C1: 1, R2: 9, C2: 1})
	a.validationDialog()
	d.allow, d.op, d.style = model.ValidWhole, model.ValidBetween, model.ErrorWarning
	h.frame()
	d.f1.SetText("1")
	d.f2.SetText("10")
	d.errMsg.SetText("1 to 10 please")
	h.press(key.NameReturn, 0)
	h.enter("B2", "50")
	if !a.dialog.Visible || a.dialog.Buttons[0] != "Yes" || !strings.HasPrefix(a.dialog.Message, "1 to 10 please") {
		t.Fatalf("warning: %v %q", a.dialog.Visible, a.dialog.Message)
	}
	h.answerDialog(0)
	if a.ed.active || h.cell("B2").Num != 50 {
		t.Fatalf("Yes: editing %v B2 %v", a.ed.active, h.cell("B2").Num)
	}
	a.validationDialog()
	if d.allow != model.ValidWhole || d.f2.Text() != "10" || d.errMsg.Text() != "1 to 10 please" {
		t.Errorf("dialog shows %v %q %q", d.allow, d.f2.Text(), d.errMsg.Text())
	}
	d.style = model.ErrorInformation
	h.frame()
	h.press(key.NameReturn, 0)
	h.enter("B3", "11")
	if a.dialog.Buttons[0] != "OK" {
		t.Fatalf("information buttons %q", a.dialog.Buttons)
	}
	h.answerDialog(0)
	if h.cell("B3").Num != 11 {
		t.Errorf("OK: B3 %v", h.cell("B3").Num)
	}

	// Circle Invalid Data finds B2 and B3; fixing B2 drops its circle.
	a.circleInvalid()
	if got := a.state().circles; len(got) != 2 {
		t.Fatalf("circles %v", got)
	}
	a.book.Enter(0, 1, 1, "5")
	a.changed(calc.Result{})
	if got := a.state().circles; len(got) != 1 || got[0] != [2]int{2, 1} {
		t.Errorf("after fixing B2: %v", got)
	}
	a.clearCircles()
	if a.state().circles != nil {
		t.Error("circles not cleared")
	}

	// Clear All then OK removes the rule, in one undo step.
	a.selectRange(0, model.Range{R1: 0, C1: 1, R2: 9, C2: 1})
	a.validationDialog()
	q := h.locate(&d.clearAll, modalArea(460), 10)
	h.click(q.X, q.Y)
	if d.allow != model.ValidAny {
		t.Fatal("Clear All")
	}
	h.frame()
	h.press(key.NameReturn, 0)
	if _, ok := sh.ValidationAt(1, 1); ok {
		t.Fatal("rule left after Clear All")
	}
	h.press("Z", key.ModShortcut)
	if _, ok := sh.ValidationAt(1, 1); !ok {
		t.Fatal("undo did not bring the rule back")
	}
}
