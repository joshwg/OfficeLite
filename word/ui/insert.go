package ui

import (
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Insert menu items (indexes into its Items).
const (
	insTable = iota
	insTableDialog
	_
	insPicture
	insLink
	insRemoveLink
	insSymbol
	insDate
	_
	insPageBreak
	insLineBreak
	insTab
	_
	insRowAbove
	insRowBelow
	insColLeft
	insColRight
	insDelRow
	insDelCol
	insDelTable
	insSelectTable
)

// insertMenu is the Insert menu; the table commands apply to the table
// the caret is in.
func (d *window) insertMenu() []gui.MenuItem {
	inTable := d.ed.InTable()
	_, inLink := d.ed.SelectionLink()
	link := "Link…"
	if inLink {
		link = "Edit Link…"
	}
	return []gui.MenuItem{
		insTable:       {Label: "Table", Tip: "Pick a table size from a grid and insert it."},
		insTableDialog: {Label: "Insert Table…", Tip: "Insert a table with the number of rows and columns you type."},
		{Separator: true},
		insPicture:    {Label: "Picture…", Tip: "Insert a picture from a file."},
		insLink:       {Label: link, Shortcut: "Ctrl+K", Tip: "Link the selection to a web page or e-mail address."},
		insRemoveLink: {Label: "Remove Link", Disabled: !inLink, Tip: "Keep the text but remove the link."},
		insSymbol:     {Label: "Symbol…", Tip: "Insert a symbol or special character."},
		insDate:       {Label: "Date & Time…", Tip: "Insert today's date or the time in a format you pick."},
		{Separator: true},
		insPageBreak: {Label: "Page Break", Shortcut: "Ctrl+Enter", Tip: "Start the next page here."},
		insLineBreak: {Label: "Line Break", Shortcut: "Shift+Enter", Tip: "Start a new line in the same paragraph."},
		insTab:       {Label: "Tab", Shortcut: "Tab", Tip: "Insert a tab character."},
		{Separator: true},
		insRowAbove:    {Label: "Insert Row Above", Disabled: !inTable, Tip: "Add a row above the current one."},
		insRowBelow:    {Label: "Insert Row Below", Disabled: !inTable, Tip: "Add a row below the current one."},
		insColLeft:     {Label: "Insert Column Left", Disabled: !inTable, Tip: "Add a column to the left of the current one."},
		insColRight:    {Label: "Insert Column Right", Disabled: !inTable, Tip: "Add a column to the right of the current one."},
		insDelRow:      {Label: "Delete Row", Disabled: !inTable, Tip: "Delete the selected rows."},
		insDelCol:      {Label: "Delete Column", Disabled: !inTable, Tip: "Delete the selected columns."},
		insDelTable:    {Label: "Delete Table", Disabled: !inTable, Tip: "Delete the whole table."},
		insSelectTable: {Label: "Select Table", Disabled: !inTable, Tip: "Select the whole table."},
	}
}

// insertChosen runs an Insert menu item.
func (d *window) insertChosen(item int) {
	ed := d.ed
	switch item {
	case insTable:
		d.ins.show(insPicker)
	case insTableDialog:
		d.ins.show(insTableDlg)
		d.ins.rows.SetText("2")
		d.ins.cols.SetText("5")
	case insPicture:
		d.choosePicture()
	case insLink:
		d.openLinkDialog()
	case insRemoveLink:
		ed.RemoveLink()
	case insSymbol:
		d.ins.show(insSymbolDlg)
	case insDate:
		d.ins.show(insDateDlg)
		d.ins.dateSel = 0
	case insPageBreak:
		ed.InsertSpecial(model.RunPageBreak)
	case insLineBreak:
		ed.InsertSpecial(model.RunBreak)
	case insTab:
		ed.InsertSpecial(model.RunTab)
	case insRowAbove, insRowBelow:
		ed.InsertRow(item == insRowBelow)
	case insColLeft, insColRight:
		ed.InsertColumn(item == insColRight)
	case insDelRow:
		ed.DeleteRows()
	case insDelCol:
		ed.DeleteColumns()
	case insDelTable:
		ed.DeleteTable()
	case insSelectTable:
		ed.SelectTable()
	}
}

// insertKind is the Insert dialog or popup showing.
type insertKind int8

const (
	insNone insertKind = iota
	insPicker
	insTableDlg
	insLinkDlg
	insSymbolDlg
	insDateDlg
)

// insertState holds the Insert menu's dialogs and the page view's
// picture and link interactions.
type insertState struct {
	kind  insertKind
	focus *widget.Editor // editor to focus at the next frame
	tag   int            // the modal panel's event tag

	// table picker
	pickTag, panelTag  int
	pickRows, pickCols int // hovered size; 0 = none

	// dialogs
	rows, cols, text, addr widget.Editor
	ok, cancel, remove     widget.Clickable
	editing                bool // the link dialog edits an existing link
	symbols                []widget.Clickable
	dates                  []widget.Clickable
	dateSel                int
	dateTag                int

	// page view
	anchor   model.Pos // where a drag selection started
	resizing bool      // dragging a picture handle
	handle   int
	resPos   model.Pos // the picture being resized
	resBox   wl.Rect   // its box when the drag started (page points)
	resPage  int       // and its page
	resStart f32.Point // pointer position at the start
	resNow   wl.Rect   // the box being dragged
}

func (s *insertState) show(k insertKind) {
	s.kind = k
	s.pickRows, s.pickCols = 0, 0
	for _, e := range []*widget.Editor{&s.rows, &s.cols, &s.text, &s.addr} {
		e.SingleLine, e.Submit = true, true
	}
	switch k {
	case insTableDlg:
		s.focus = &s.rows
	case insLinkDlg:
		s.focus = &s.text
	}
}

// modal reports whether an Insert dialog takes the input.
func (s *insertState) modal() bool { return s.kind != insNone }

func (s *insertState) close(d *window) {
	s.kind = insNone
	d.v.focusPending = true
}

// layout draws the open Insert dialog or popup.
func (s *insertState) layout(gtx layout.Context, d *window) {
	if s.focus != nil {
		gtx.Execute(key.FocusCmd{Tag: s.focus})
		s.focus = nil
	}
	switch s.kind {
	case insPicker:
		s.pickerLayout(gtx, d)
	case insTableDlg:
		s.tableDialog(gtx, d)
	case insLinkDlg:
		s.linkDialog(gtx, d)
	case insSymbolDlg:
		s.symbolDialog(gtx, d)
	case insDateDlg:
		s.dateDialog(gtx, d)
	}
}

// Table picker: Word's grid under Insert > Table, hover to size a table
// and click to insert it.

const pickerRows, pickerCols = 8, 10

func (s *insertState) pickerLayout(gtx layout.Context, d *window) {
	cell := gtx.Dp(20)
	pad := gtx.Dp(10)
	head := gtx.Dp(26)
	w := pad*2 + cell*pickerCols
	h := head + pad + cell*pickerRows + gtx.Dp(32)
	origin := image.Pt(gtx.Dp(118), gtx.Dp(64))
	gridOrigin := origin.Add(image.Pt(pad, head))
	// Clicks anywhere else close the picker.
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &s.tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			s.close(d)
			return
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &s.pickTag, Kinds: pointer.Move | pointer.Press | pointer.Leave | pointer.Enter})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		c := int(e.Position.X)/cell + 1
		r := int(e.Position.Y)/cell + 1
		inside := e.Position.X >= 0 && e.Position.Y >= 0 && c <= pickerCols && r <= pickerRows
		switch e.Kind {
		case pointer.Leave:
			s.pickRows, s.pickCols = 0, 0
		case pointer.Move, pointer.Enter:
			if inside {
				s.pickRows, s.pickCols = r, c
			}
		case pointer.Press:
			if inside {
				s.close(d)
				d.ed.InsertTable(r, c)
				d.edited()
				return
			}
		}
	}
	size := gtx.Constraints.Max
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	area.Pop()

	box := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(w, h))}
	gui.FillRect(gtx.Ops, box.Add(image.Pt(3, 3)), colors.RGBA(0, 0, 0, 0x28))
	gui.FillRect(gtx.Ops, box.Inset(-1), gui.ChromeLine)
	gui.FillRect(gtx.Ops, box, colors.White)
	// Swallow clicks on the panel outside the grid.
	pa := clip.Rect(box).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.panelTag)
	pa.Pop()

	title := "Insert Table"
	if s.pickRows > 0 {
		title = strconv.Itoa(s.pickCols) + "x" + strconv.Itoa(s.pickRows) + " Table"
	}
	gtx2 := gtx
	gtx2.Constraints = layout.Exact(image.Pt(w-2*pad, head))
	off := op.Offset(origin.Add(image.Pt(pad, gtx.Dp(4)))).Push(gtx.Ops)
	gui.Middle(gtx2, d.th.Label(13, title, gui.ChromeText, font.SemiBold).Layout)
	off.Pop()

	for r := 0; r < pickerRows; r++ {
		for c := 0; c < pickerCols; c++ {
			cr := image.Rect(c*cell, r*cell, (c+1)*cell, (r+1)*cell).Add(gridOrigin).Inset(gtx.Dp(2))
			line := gui.ChromeLine
			bg := colors.White
			if r < s.pickRows && c < s.pickCols {
				line, bg = d.th.Accent, d.th.AccentTint
			}
			gui.FillRect(gtx.Ops, cr, line)
			gui.FillRect(gtx.Ops, cr.Inset(1), bg)
		}
	}
	grid := image.Rectangle{Min: gridOrigin, Max: gridOrigin.Add(image.Pt(cell*pickerCols, cell*pickerRows))}
	off = op.Offset(gridOrigin).Push(gtx.Ops)
	ga := clip.Rect{Max: grid.Size()}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.pickTag)
	pointer.CursorPointer.Add(gtx.Ops)
	ga.Pop()
	off.Pop()

	gtx2.Constraints = layout.Exact(image.Pt(w-2*pad, gtx.Dp(28)))
	off = op.Offset(image.Pt(origin.X+pad, grid.Max.Y+gtx.Dp(2))).Push(gtx.Ops)
	gui.Middle(gtx2, d.th.Label(12, "Larger: Insert > Insert Table…", gui.MutedText, font.Normal).Layout)
	off.Pop()
}

// Dialogs.

// field draws a labeled single-line text box.
func field(th *gui.Theme, label string, ed *widget.Editor, width unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(130)
					return th.Label(13, label, gui.MutedText, font.Normal).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					w := gtx.Dp(width)
					gtx.Constraints = layout.Exact(image.Pt(w, gtx.Dp(28)))
					r := image.Rectangle{Max: gtx.Constraints.Max}
					gui.FillRect(gtx.Ops, r, gui.ChromeLine)
					gui.FillRect(gtx.Ops, r.Inset(1), colors.White)
					return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							e := material.Editor(th.Theme, ed, "")
							e.TextSize = unit.Sp(13)
							e.Font = gui.UIFont
							e.Color = gui.ChromeText
							return e.Layout(gtx)
						})
					})
				}),
			)
		})
	})
}

// submitted reports whether Enter was pressed in any of the editors.
func submitted(gtx layout.Context, eds ...*widget.Editor) bool {
	sub := false
	for _, ed := range eds {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				sub = true
			}
		}
	}
	return sub
}

// buttons draws a dialog's button row: OK, then extra, then Cancel.
func (s *insertState) buttons(th *gui.Theme, okLabel string, extra layout.Widget) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			kids := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &s.ok, okLabel) }),
				layout.Rigid(layout.Spacer{Width: 8}.Layout),
			}
			if extra != nil {
				kids = append(kids, layout.Rigid(extra), layout.Rigid(layout.Spacer{Width: 8}.Layout))
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.SecondaryButton(gtx, &s.cancel, "Cancel")
			}))
			return layout.Flex{}.Layout(gtx, kids...)
		})
	})
}

func dialogTitle(th *gui.Theme, s string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: 14}.Layout(gtx, th.Label(18, s, gui.ChromeText, font.SemiBold).Layout)
	})
}

func (s *insertState) tableDialog(gtx layout.Context, d *window) {
	th := d.th
	if s.ok.Clicked(gtx) || submitted(gtx, &s.rows, &s.cols) {
		r, err1 := strconv.Atoi(strings.TrimSpace(s.rows.Text()))
		c, err2 := strconv.Atoi(strings.TrimSpace(s.cols.Text()))
		if err1 == nil && err2 == nil && r >= 1 && c >= 1 && r <= 32767 && c <= model.MaxTableSize {
			s.close(d)
			d.ed.InsertTable(r, c)
			d.edited()
			return
		}
		d.notice = "The number of columns must be 1 to " + strconv.Itoa(model.MaxTableSize) + " and of rows 1 to 32767."
	}
	if s.cancel.Clicked(gtx) {
		s.close(d)
		return
	}
	gui.Modal(gtx, &s.tag, 380, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			dialogTitle(th, "Insert Table"),
			field(th, "Number of columns", &s.cols, 80),
			field(th, "Number of rows", &s.rows, 80),
			s.buttons(th, "OK", nil),
		)
	})
}

// openLinkDialog shows Insert/Edit Link for the selection or the link
// at the caret.
func (d *window) openLinkDialog() {
	s := &d.ins
	s.show(insLinkDlg)
	if l, ok := d.ed.SelectionLink(); ok {
		s.editing = true
		s.text.SetText(l.Text(d.ed))
		s.addr.SetText(strings.TrimPrefix(l.Address(), "mailto:"))
		s.focus = &s.addr
		return
	}
	s.editing = false
	s.text.SetText(strings.ReplaceAll(d.ed.SelectedText(), "￼", ""))
	s.addr.SetText("")
	if s.text.Text() != "" {
		s.focus = &s.addr
	}
}

func (s *insertState) linkDialog(gtx layout.Context, d *window) {
	th := d.th
	if s.ok.Clicked(gtx) || submitted(gtx, &s.text, &s.addr) {
		addr := strings.TrimSpace(s.addr.Text())
		if addr == "" {
			s.focus = &s.addr
		} else {
			s.close(d)
			d.ed.InsertLink(s.text.Text(), addr)
			d.edited()
			return
		}
	}
	if s.remove.Clicked(gtx) {
		s.close(d)
		d.ed.RemoveLink()
		d.edited()
		return
	}
	if s.cancel.Clicked(gtx) {
		s.close(d)
		return
	}
	title := "Insert Hyperlink"
	var extra layout.Widget
	if s.editing {
		title = "Edit Hyperlink"
		extra = func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &s.remove, "Remove Link") }
	}
	gui.Modal(gtx, &s.tag, 520, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			dialogTitle(th, title),
			field(th, "Text to display", &s.text, 340),
			field(th, "Address", &s.addr, 340),
			layout.Rigid(th.Label(12, "A web address (www.example.com), an e-mail address or #bookmark", gui.MutedText, font.Normal).Layout),
			s.buttons(th, "OK", extra),
		)
	})
}

// symbolSet is the Symbol dialog's grid: Word's recently used defaults,
// typography, currency, math, arrows, Greek and a few shapes.
var symbolSet = []rune(
	"©®™§¶†‡•·–—…‰′″‹›«»‘’‚“”„¡¿" +
		"€£¥¢¤ƒ₹₽₩₪" +
		"°±×÷≤≥≠≈∞√∑∏∫∂∆∇∈∉∩∪⊂⊃∧∨¬∀∃∅∝∠⊥½⅓⅔¼¾⅛¹²³µ" +
		"←↑→↓↔↕⇐⇒⇔↵" +
		"ΑΒΓΔΘΛΞΠΣΦΨΩαβγδεζηθικλμνξπρστυφχψω" +
		"✓✔✗✘★☆♠♣♥♦●○■□▲►▼◄◆◊☺☎✉♪")

func (s *insertState) symbolDialog(gtx layout.Context, d *window) {
	th := d.th
	if len(s.symbols) != len(symbolSet) {
		s.symbols = make([]widget.Clickable, len(symbolSet))
	}
	for i := range s.symbols {
		if s.symbols[i].Clicked(gtx) {
			s.close(d)
			d.ed.InsertText(string(symbolSet[i]))
			d.edited()
			return
		}
	}
	if s.cancel.Clicked(gtx) {
		s.close(d)
		return
	}
	const perRow = 16
	gui.Modal(gtx, &s.tag, unit.Dp(perRow*32+40), func(gtx layout.Context) layout.Dimensions {
		var rows []layout.FlexChild
		rows = append(rows, dialogTitle(th, "Symbol"))
		for start := 0; start < len(symbolSet); start += perRow {
			var cells []layout.FlexChild
			for i := start; i < min(start+perRow, len(symbolSet)); i++ {
				cells = append(cells, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return gui.Hoverable(gtx, &s.symbols[i], colors.Transparent, th.AccentTint, func(gtx layout.Context) layout.Dimensions {
						sz := gtx.Dp(32)
						gtx.Constraints = layout.Exact(image.Pt(sz, sz))
						gui.HLine(gtx.Ops, 0, sz, sz-1, 1, gui.ChromeLine)
						gui.VLine(gtx.Ops, sz-1, 0, sz, 1, gui.ChromeLine)
						l := th.Label(17, string(symbolSet[i]), gui.ChromeText, font.Normal)
						l.Alignment = text.Middle
						return layout.Center.Layout(gtx, l.Layout)
					})
				}))
			}
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx, cells...)
			}))
		}
		rows = append(rows,
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(th.Label(12, "Click a symbol to insert it at the cursor.", gui.MutedText, font.Normal).Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.SecondaryButton(gtx, &s.cancel, "Close")
				})
			}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

// dateFormats are Word's Date and Time formats (en-US), as Go layouts.
var dateFormats = []string{
	"1/2/2006",
	"Monday, January 2, 2006",
	"January 2, 2006",
	"1/2/06",
	"2006-01-02",
	"2-Jan-06",
	"1.2.2006",
	"Jan. 2, 06",
	"2 January 2006",
	"January 06",
	"Jan-06",
	"1/2/2006 3:04 PM",
	"1/2/2006 3:04:05 PM",
	"3:04 PM",
	"3:04:05 PM",
	"15:04",
	"15:04:05",
}

// DateChoices formats t in each of Word's date and time formats.
func DateChoices(t time.Time) []string {
	out := make([]string, len(dateFormats))
	for i, f := range dateFormats {
		out[i] = t.Format(f)
	}
	return out
}

func (s *insertState) dateDialog(gtx layout.Context, d *window) {
	th := d.th
	choices := DateChoices(time.Now())
	if len(s.dates) != len(choices) {
		s.dates = make([]widget.Clickable, len(choices))
	}
	insert := s.ok.Clicked(gtx)
	for i := range s.dates {
		for {
			c, ok := s.dates[i].Update(gtx)
			if !ok {
				break
			}
			s.dateSel = i
			insert = insert || c.NumClicks >= 2 // a double click inserts
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &s.dateTag, Name: key.NameReturn}, key.Filter{Focus: &s.dateTag, Name: key.NameUpArrow},
			key.Filter{Focus: &s.dateTag, Name: key.NameDownArrow})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameReturn:
				insert = true
			case key.NameUpArrow:
				s.dateSel = max(s.dateSel-1, 0)
			case key.NameDownArrow:
				s.dateSel = min(s.dateSel+1, len(choices)-1)
			}
		}
	}
	if insert {
		s.close(d)
		d.ed.InsertText(choices[s.dateSel])
		d.edited()
		return
	}
	if s.cancel.Clicked(gtx) {
		s.close(d)
		return
	}
	gui.Modal(gtx, &s.tag, 360, func(gtx layout.Context) layout.Dimensions {
		event.Op(gtx.Ops, &s.dateTag)
		gtx.Execute(key.FocusCmd{Tag: &s.dateTag})
		kids := []layout.FlexChild{dialogTitle(th, "Date and Time"),
			layout.Rigid(th.Label(13, "Available formats:", gui.MutedText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 6}.Layout)}
		for i, c := range choices {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				bg := colors.Transparent
				if i == s.dateSel {
					bg = th.AccentTint
				}
				return gui.Hoverable(gtx, &s.dates[i], bg, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: 3, Bottom: 3, Left: 8}.Layout(gtx, th.Label(13, c, gui.ChromeText, font.Normal).Layout)
				})
			}))
		}
		kids = append(kids, s.buttons(th, "Insert", nil))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// Pictures.

// pictureExts are the picture files Insert > Picture offers.
var pictureExts = gui.ImageFilter.Exts

// choosePicture asks for an image file and inserts it at the caret.
func (d *window) choosePicture() {
	if d.expl == nil {
		return
	}
	go func() {
		var data []byte
		path, err := d.files.Open(gui.ImageFilter, gui.AllFilter)
		switch {
		case err == nil:
			data, err = os.ReadFile(path)
		case errors.Is(err, gui.ErrCancelled):
			return
		default:
			rc, err2 := d.expl.ChooseFile(pictureExts...)
			if errors.Is(err2, explorer.ErrUserDecline) {
				return
			}
			err = err2
			if err == nil {
				if f, ok := rc.(*os.File); ok {
					path = f.Name()
				}
				data, err = io.ReadAll(rc)
				rc.Close()
			}
		}
		d.calls.Post(func() { d.insertPictureData(path, data, err) })
	}()
}

// insertPictureData inserts a picture file's contents (on the UI
// goroutine).
func (d *window) insertPictureData(path string, data []byte, err error) {
	if err == nil {
		var pic *model.Picture
		var w, h float64
		pic, w, h, err = model.NewPicture(data, path)
		if err == nil {
			d.ed.InsertPicture(pic, w, h)
			d.errMsg = ""
			d.edited()
			return
		}
		err = errors.New("not a picture WordLite can show (PNG, JPEG, GIF, BMP, TIFF or WebP)")
	}
	d.errMsg = "Can't insert " + gui.OrNone(filepath.Base(path), "the picture") + ": " + err.Error()
}

// previewInsert opens an Insert dialog for the wordshot tool.
func (d *window) previewInsert(name string) {
	switch name {
	case "table":
		d.insertChosen(insTableDialog)
	case "picker":
		d.insertChosen(insTable)
		d.ins.pickRows, d.ins.pickCols = 3, 4
	case "link":
		d.openLinkDialog()
	case "symbol":
		d.insertChosen(insSymbol)
	case "date":
		d.insertChosen(insDate)
	}
}
