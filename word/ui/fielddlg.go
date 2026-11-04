// The Insert Field dialog (Insert > Field…, Ctrl+F9): pick a field type
// or type a field code with its switches, see its result, and insert it.

package ui

import (
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// fieldChoice is a field type the dialog lists: its type, the code a
// click puts in the box, and what it shows.
type fieldChoice struct {
	name, code, desc string
}

// fieldChoices are the fields WordLite computes, as the dialog lists them.
var fieldChoices = []fieldChoice{
	{model.FieldPage, model.FieldPage, "The number of the page"},
	{model.FieldNumPages, model.FieldNumPages, "The number of pages in the document"},
	{model.FieldSectionPages, model.FieldSectionPages, "The number of pages in the section"},
	{model.FieldSection, model.FieldSection, "The number of the section"},
	{model.FieldDate, `DATE \@ "M/d/yyyy"`, "Today's date"},
	{model.FieldTime, `TIME \@ "h:mm AM/PM"`, "The current time"},
	{model.FieldFileName, model.FieldFileName, "The document's file name (\\p for the full path)"},
	{model.FieldNumWords, model.FieldNumWords, "The number of words in the document"},
	{model.FieldNumChars, model.FieldNumChars, "The number of characters in the document"},
}

// fieldDialog is the Insert Field dialog's state.
type fieldDialog struct {
	choices []widget.Clickable
	code    widget.Editor
	sel     int // the choice last clicked; -1 when the code was typed
}

// openFieldDialog shows the Insert Field dialog, starting on PAGE.
func (d *window) openFieldDialog() {
	d.ins.show(insFieldDlg)
	f := &d.ins.field
	f.code.SingleLine, f.code.Submit = true, true
	f.sel = 0
	f.code.SetText(fieldChoices[0].code)
	d.ins.focus = &f.code
}

// fieldPreview is what the field code shows where the caret is, and
// whether WordLite computes it.
func (d *window) fieldPreview(code string) (string, bool) {
	c := d.fieldContext(d.ed.Sel.Caret)
	return model.EvalField(code, &c)
}

// insertFieldCode inserts the field code at the caret, showing its result,
// or its type in brackets for a field WordLite doesn't compute (which
// keeps that text until Word updates it). It reports whether there was a
// code to insert.
func (d *window) insertFieldCode(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	result, ok := d.fieldPreview(code)
	if !ok {
		result = "[" + model.FieldName(code) + "]"
	}
	d.ed.InsertSimpleField(code, result)
	d.edited()
	return true
}

// fieldDialogLayout draws the Insert Field dialog and handles its input.
func (s *insertState) fieldDialogLayout(gtx layout.Context, d *window) {
	th := d.th
	f := &s.field
	if len(f.choices) != len(fieldChoices) {
		f.choices = make([]widget.Clickable, len(fieldChoices))
	}
	for i := range f.choices {
		for f.choices[i].Clicked(gtx) {
			f.sel = i
			f.code.SetText(fieldChoices[i].code)
			s.focus = &f.code
		}
	}
	insert := s.ok.Clicked(gtx)
	for {
		ev, ok := f.code.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			insert = true
		case widget.ChangeEvent:
			f.sel = -1
			for i, c := range fieldChoices {
				if strings.EqualFold(strings.TrimSpace(f.code.Text()), c.code) {
					f.sel = i
				}
			}
		}
	}
	if insert {
		code := f.code.Text()
		if strings.TrimSpace(code) == "" {
			s.focus = &f.code
		} else {
			s.close(d)
			d.insertFieldCode(code)
			return
		}
	}
	if s.cancel.Clicked(gtx) {
		s.close(d)
		return
	}
	preview, computed := d.fieldPreview(f.code.Text())
	switch {
	case strings.TrimSpace(f.code.Text()) == "":
		preview = "Type a field code, such as PAGE \\* roman or DATE \\@ \"MMMM d, yyyy\"."
	case !computed:
		preview = "WordLite doesn't compute " + model.FieldName(f.code.Text()) +
			" fields: it is kept, and Word shows its result when it updates fields."
	default:
		preview = "Result: " + preview
	}
	gui.Modal(gtx, &s.tag, 520, func(gtx layout.Context) layout.Dimensions {
		kids := []layout.FlexChild{dialogTitle(th, "Field"),
			layout.Rigid(th.Label(13, "Field names:", gui.MutedText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 6}.Layout)}
		for i, c := range fieldChoices {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				bg := colors.Transparent
				if i == f.sel {
					bg = th.AccentTint
				}
				return gui.Hoverable(gtx, &f.choices[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: 3, Bottom: 3, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(130)
								return th.Label(13, c.name, gui.ChromeText, font.SemiBold).Layout(gtx)
							}),
							layout.Flexed(1, th.Label(13, c.desc, gui.MutedText, font.Normal).Layout))
					})
				})
			}))
		}
		kids = append(kids, layout.Rigid(layout.Spacer{Height: 10}.Layout),
			field(th, "Field code", &f.code, 360),
			layout.Rigid(th.Label(12, preview, gui.MutedText, font.Normal).Layout),
			layout.Rigid(th.Label(12, `Switches: \* Upper, \* roman, \* CardText, \# "0.00", \@ "dddd, MMMM d"`, gui.MutedText, font.Normal).Layout),
			s.buttons(th, "Insert", nil))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}
