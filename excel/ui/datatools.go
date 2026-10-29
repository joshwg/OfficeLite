package ui

import (
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

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
	"officelite/excel/edit"
	"officelite/excel/model"
)

// Text to Columns (Excel's three-step wizard as one dialog) and Remove
// Duplicates.

// Text to Columns preview size.
const (
	ttcPreviewRows = 8
	ttcPreviewH    = 150 // dp
	ttcRulerH      = 18  // dp
)

var (
	qualifierNames = []string{`"`, `'`, "{none}"}
	dateOrderNames = []string{"MDY", "DMY", "YMD", "MYD", "DYM", "YDM"}
	monoFont       = font.Font{Typeface: "Go Mono"}
)

// textToColsDialog is the Text to Columns dialog.
type textToColsDialog struct {
	src   model.Range
	lines []string // the first cells' text, for the preview

	kind                         widget.Enum // "delimited" or "fixed"
	tab, semi, comma, space, oth widget.Bool
	other                        widget.Editor
	consec                       widget.Bool
	qualDD                       gui.DropDown
	qual                         int

	breaks     []int // fixed width: positions the fields start at
	breaksEd   widget.Editor
	breaksText string // what breaksEd showed last, to see edits
	ruler      int    // event tag of the fixed-width preview

	colBtns   []widget.Clickable
	sel       int // the preview column whose format is set
	formats   []edit.ColFormat
	fmtEnum   widget.Enum // general, text, date, skip
	dateDD    gui.DropDown
	dateOrder int

	dest widget.Editor
}

// textToColumnsDialog opens Text to Columns for the selected column.
func (a *App) textToColumnsDialog() {
	if !a.commitForCommand() || !a.singleRange() {
		return
	}
	v := &a.state().view
	r := v.Sel.Range
	if r.C1 != r.C2 {
		a.dialog.Show(appName, edit.ErrTextToColsShape.Error(), "OK")
		a.answer = func(int) {}
		return
	}
	r.R2 = min(r.R2, max(v.Sheet.UsedRows()-1, r.R1))
	d := &a.ins.ttc
	*d = textToColsDialog{src: r}
	for row := r.R1; row <= r.R2 && len(d.lines) < ttcPreviewRows; row++ {
		d.lines = append(d.lines, v.Sheet.Get(row, r.C1).Text)
	}
	d.kind.Value = "delimited"
	all := strings.Join(d.lines, "\n")
	switch {
	case strings.Contains(all, "\t"):
		d.tab.Value = true
	case strings.Contains(all, ";") && !strings.Contains(all, ","):
		d.semi.Value = true
	case strings.Contains(all, ","):
		d.comma.Value = true
	default:
		d.tab.Value = true
	}
	d.breaks = guessBreaks(d.lines)
	d.setBreaksText()
	for _, e := range []*widget.Editor{&d.other, &d.breaksEd, &d.dest} {
		e.SingleLine, e.Submit = true, true
	}
	d.other.MaxLen = 1
	d.dest.SetText("=$" + model.ColName(r.C1) + "$" + strconv.Itoa(r.R1+1))
	d.fmtEnum.Value = "general"
	a.showDialog(dlgTextToCols)
}

// guessBreaks suggests fixed-width breaks: positions where every line has
// a space just before and some line text from on.
func guessBreaks(lines []string) []int {
	n := 0
	for _, l := range lines {
		n = max(n, utf8.RuneCountInString(l))
	}
	var out []int
	for p := 1; p < n; p++ {
		ok, starts := true, false
		for _, l := range lines {
			rs := []rune(l)
			if p-1 < len(rs) && rs[p-1] != ' ' {
				ok = false
				break
			}
			if p < len(rs) && rs[p] != ' ' {
				starts = true
			}
		}
		if ok && starts {
			out = append(out, p)
		}
	}
	return out
}

func (d *textToColsDialog) setBreaksText() {
	parts := make([]string, len(d.breaks))
	for i, b := range d.breaks {
		parts[i] = strconv.Itoa(b)
	}
	d.breaksText = strings.Join(parts, ", ")
	d.breaksEd.SetText(d.breaksText)
}

// spec is the split the dialog describes.
func (d *textToColsDialog) spec() edit.TextToColsSpec {
	s := edit.TextToColsSpec{Fixed: d.kind.Value == "fixed", Consecutive: d.consec.Value, Breaks: d.breaks, Formats: d.formats}
	for _, x := range []struct {
		on bool
		c  string
	}{{d.tab.Value, "\t"}, {d.semi.Value, ";"}, {d.comma.Value, ","}, {d.space.Value, " "}, {d.oth.Value, d.other.Text()}} {
		if x.on {
			s.Delims += x.c
		}
	}
	switch d.qual {
	case 0:
		s.Qualifier = '"'
	case 1:
		s.Qualifier = '\''
	}
	return s
}

// formatOf is the preview column's format.
func (d *textToColsDialog) formatOf(i int) edit.ColFormat {
	if i < len(d.formats) {
		return d.formats[i]
	}
	return edit.ColGeneral
}

func (d *textToColsDialog) setFormat(i int, f edit.ColFormat) {
	for len(d.formats) <= i {
		d.formats = append(d.formats, edit.ColGeneral)
	}
	d.formats[i] = f
}

// syncFormatControls shows the selected column's format in the radio
// buttons, or applies a change made with them.
func (d *textToColsDialog) syncFormatControls(gtx layout.Context) {
	if d.fmtEnum.Update(gtx) {
		f := edit.ColGeneral
		switch d.fmtEnum.Value {
		case "text":
			f = edit.ColText
		case "date":
			f = edit.ColDateMDY + edit.ColFormat(d.dateOrder)
		case "skip":
			f = edit.ColSkip
		}
		d.setFormat(d.sel, f)
	}
	switch f := d.formatOf(d.sel); {
	case f == edit.ColText:
		d.fmtEnum.Value = "text"
	case f == edit.ColSkip:
		d.fmtEnum.Value = "skip"
	case f >= edit.ColDateMDY:
		d.fmtEnum.Value = "date"
		d.dateOrder = int(f - edit.ColDateMDY)
	default:
		d.fmtEnum.Value = "general"
	}
}

func (a *App) layoutTextToColsDialog(gtx layout.Context, k int) {
	d := &a.ins.ttc
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
	// Breaks typed in the list box.
	if t := d.breaksEd.Text(); t != d.breaksText {
		d.breaksText = t
		var bs []int
		for _, f := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
			if n, err := strconv.Atoi(f); err == nil && n > 0 {
				bs = append(bs, n)
			}
		}
		slices.Sort(bs)
		d.breaks = slices.Compact(bs)
	}
	if k > 0 {
		a.runTextToColumns()
		return
	}
	spec := d.spec()
	var rows [][]string
	ncols := 1
	for _, l := range d.lines {
		f := edit.SplitFields(l, spec)
		rows = append(rows, f)
		ncols = max(ncols, len(f))
	}
	if len(d.colBtns) < ncols {
		d.colBtns = append(d.colBtns, make([]widget.Clickable, ncols-len(d.colBtns))...)
	}
	for i := range d.colBtns[:ncols] {
		for d.colBtns[i].Clicked(gtx) {
			d.sel = i
		}
	}
	d.sel = min(d.sel, ncols-1)
	d.syncFormatControls(gtx)

	th := a.ui
	label := func(s string) layout.Widget { return th.Label(13, s, chromeText, font.Normal).Layout }
	gap := func(h unit.Dp) layout.FlexChild { return layout.Rigid(layout.Spacer{Height: h}.Layout) }
	fixed := d.kind.Value == "fixed"
	delimRow := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.ui.CheckBox(&d.tab, "Tab")),
			layout.Rigid(a.ui.CheckBox(&d.semi, "Semicolon")),
			layout.Rigid(a.ui.CheckBox(&d.comma, "Comma")),
			layout.Rigid(a.ui.CheckBox(&d.space, "Space")),
			layout.Rigid(a.ui.CheckBox(&d.oth, "Other:")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return gui.TextField(gtx, a.ui, &d.other, 30, 1, "") }),
		)
	}
	optsRow := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.ui.CheckBox(&d.consec, "Treat consecutive delimiters as one")),
			layout.Rigid(layout.Spacer{Width: 24}.Layout),
			layout.Rigid(label("Text qualifier: ")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if j := d.qualDD.Layout(gtx, th, 80, qualifierNames[d.qual], dropItems(qualifierNames)); j >= 0 {
					d.qual = j
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(80), gtx.Dp(24))}
			}),
		)
	}
	breaksRow := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(label("Break lines at characters: ")),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return gui.TextField(gtx, a.ui, &d.breaksEd, 300, 1, "e.g. 10, 25")
			}),
		)
	}
	formatRow := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(label(fmt.Sprintf("Column %d data format: ", d.sel+1))),
			layout.Rigid(a.ui.Radio(&d.fmtEnum, "general", "General")),
			layout.Rigid(a.ui.Radio(&d.fmtEnum, "text", "Text")),
			layout.Rigid(a.ui.Radio(&d.fmtEnum, "date", "Date:")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if j := d.dateDD.Layout(gtx, th, 70, dateOrderNames[d.dateOrder], dropItems(dateOrderNames)); j >= 0 {
					d.dateOrder = j
					d.setFormat(d.sel, edit.ColDateMDY+edit.ColFormat(j))
				}
				return layout.Dimensions{Size: image.Pt(gtx.Dp(70), gtx.Dp(24))}
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(a.ui.Radio(&d.fmtEnum, "skip", "Do not import (skip)")),
		)
	}
	body := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(label("Text to Columns " + rangeName(d.src) + ": choose how to split the text")),
			gap(6),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(a.ui.Radio(&d.kind, "delimited", "Delimited: characters such as commas or tabs separate each field")),
				)
			}),
			layout.Rigid(a.ui.Radio(&d.kind, "fixed", "Fixed width: fields are aligned in columns with spaces between")),
			gap(6),
		}
		if fixed {
			children = append(children,
				layout.Rigid(label("Click in the preview to set a break, click a break to remove it.")),
				gap(4),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.fixedPreview(gtx) }),
				gap(6),
				layout.Rigid(breaksRow),
			)
		} else {
			children = append(children, layout.Rigid(delimRow), layout.Rigid(optsRow))
		}
		children = append(children,
			gap(8),
			layout.Rigid(label("Data preview (click a column to set its format):")),
			gap(4),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.ttcPreview(gtx, rows, ncols) }),
			gap(6),
			layout.Rigid(formatRow),
			gap(6),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(label("Destination: ")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return gui.TextField(gtx, a.ui, &d.dest, 140, 1, "") }),
				)
			}),
			a.errorLine(),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Convert Text to Columns", 660, body, a.accentButton(&a.ins.ok, "Finish"), a.plainButton(&a.ins.cancel, "Cancel"))
}

// fixedPreview draws the lines in a fixed-pitch font under a ruler, with
// the breaks as lines; a click sets or removes a break.
func (a *App) fixedPreview(gtx layout.Context) layout.Dimensions {
	d := &a.ins.ttc
	w, h := gtx.Constraints.Max.X, gtx.Dp(ttcPreviewH)
	size := unit.Sp(13)
	// The pitch: a run of digits measured once per frame.
	mg := gtx
	mg.Constraints = layout.Constraints{Max: image.Pt(1<<15, 1<<15)}
	m := op.Record(gtx.Ops)
	cw := float32(widget.Label{}.Layout(mg, a.shaper, monoFont, size, "0000000000", op.CallOp{}).Size.X) / 10
	m.Stop()
	if cw <= 0 {
		cw = float32(gtx.Dp(8))
	}
	pad := gtx.Dp(4)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &d.ruler, Kinds: pointer.Press})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		p := int((e.Position.X-float32(pad))/cw + 0.5)
		if p <= 0 {
			continue
		}
		if i := slices.Index(d.breaks, p); i >= 0 {
			d.breaks = slices.Delete(d.breaks, i, i+1)
		} else {
			d.breaks = append(d.breaks, p)
			slices.Sort(d.breaks)
		}
		d.setBreaksText()
	}
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	cs := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
	defer cs.Pop()
	area := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
	event.Op(gtx.Ops, &d.ruler)
	pointer.CursorPointer.Add(gtx.Ops)
	area.Pop()
	rh := gtx.Dp(ttcRulerH)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, rh), chromeBG)
	for i := 0; float32(pad)+float32(i)*cw < float32(w); i++ {
		x := pad + int(float32(i)*cw)
		th := rh / 4
		if i%5 == 0 {
			th = rh / 2
		}
		gui.VLine(gtx.Ops, x, rh-th, rh, 1, mutedText)
		if i%10 == 0 && i > 0 {
			l := a.ui.Label(10, strconv.Itoa(i), mutedText, font.Normal)
			o := op.Offset(image.Pt(x+2, 0)).Push(gtx.Ops)
			l.Layout(gtx)
			o.Pop()
		}
	}
	lineH := gtx.Sp(16)
	for i, l := range d.lines {
		o := op.Offset(image.Pt(pad, rh+2+i*lineH)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Constraints{Max: image.Pt(1<<15, lineH)}
		widget.Label{MaxLines: 1}.Layout(g, a.shaper, monoFont, size, l, gui.Material(gtx.Ops, chromeText))
		o.Pop()
	}
	for _, b := range d.breaks {
		x := pad + int(float32(b)*cw)
		gui.VLine(gtx.Ops, x, 2, h-2, max(1, gtx.Dp(1)), chromeText)
		gui.FillRect(gtx.Ops, image.Rect(x-gtx.Dp(3), rh-gtx.Dp(5), x+gtx.Dp(4), rh), chromeText)
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// ttcPreview is the grid of the first rows as they will be split: a
// header per column naming its format (the selected one highlighted).
func (a *App) ttcPreview(gtx layout.Context, rows [][]string, ncols int) layout.Dimensions {
	d := &a.ins.ttc
	w, h := gtx.Constraints.Max.X, gtx.Dp(ttcPreviewH)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
	gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
	cs := clip.Rect{Min: image.Pt(1, 1), Max: image.Pt(w-1, h-1)}.Push(gtx.Ops)
	defer cs.Pop()
	rowH := gtx.Dp(17)
	x := 1
	for c := 0; c < ncols && x < w; c++ {
		n := 6
		for _, r := range rows {
			if c < len(r) {
				n = max(n, utf8.RuneCountInString(r[c]))
			}
		}
		f := d.formatOf(c)
		name := []string{"General", "Text"}[min(int(f), 1)]
		switch {
		case f == edit.ColSkip:
			name = "Skip column"
		case f >= edit.ColDateMDY:
			name = dateOrderNames[f-edit.ColDateMDY]
		}
		n = max(n, len(name))
		cw := min(max(gtx.Dp(unit.Dp(7*n+12)), gtx.Dp(50)), gtx.Dp(200))
		hdr := image.Rect(x, 1, x+cw, 1+rowH)
		bg, fg := chromeBG, chromeText
		if c == d.sel {
			bg, fg = chromeText, colors.White
		}
		o := op.Offset(hdr.Min).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(hdr.Size())
		d.colBtns[c].Layout(g, func(gtx layout.Context) layout.Dimensions {
			gui.FillRect(gtx.Ops, image.Rectangle{Max: hdr.Size()}, bg)
			l := a.ui.Label(11, name, fg, font.Normal)
			t := op.Offset(image.Pt(gtx.Dp(3), gtx.Dp(1))).Push(gtx.Ops)
			l.Layout(gtx)
			t.Pop()
			return layout.Dimensions{Size: hdr.Size()}
		})
		o.Pop()
		for i, r := range rows {
			if c >= len(r) {
				continue
			}
			cell := image.Rect(x, 1+(i+1)*rowH, x+cw, 1+(i+2)*rowH)
			if c == d.sel {
				gui.FillRect(gtx.Ops, cell, colors.RGB(0xDDEBF7))
			}
			col := chromeText
			if f == edit.ColSkip {
				col = mutedText
			}
			l := a.ui.Label(12, r[c], col, font.Normal)
			t := op.Offset(cell.Min.Add(image.Pt(gtx.Dp(3), gtx.Dp(1)))).Push(gtx.Ops)
			g := gtx
			g.Constraints = layout.Constraints{Max: image.Pt(cw-gtx.Dp(4), rowH)}
			l.Layout(g)
			t.Pop()
		}
		gui.VLine(gtx.Ops, x+cw-1, 1, h, 1, chromeLine)
		x += cw
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// runTextToColumns checks the destination and splits, asking first when
// cells would be overwritten.
func (a *App) runTextToColumns() {
	d := &a.ins.ttc
	sheet, dest, err := a.book.ResolveRef(strings.TrimPrefix(strings.TrimSpace(d.dest.Text()), "="), a.active)
	if err != nil || sheet != a.active {
		a.ins.err = "The destination must be a cell on this sheet."
		return
	}
	spec := d.spec()
	if !spec.Fixed && spec.Delims == "" {
		spec.Delims = "\t"
	}
	src := d.src
	a.closeDialog()
	run := func() {
		if !a.readyToChange() {
			return
		}
		res, err := a.book.TextToColumns(a.active, src, dest.R1, dest.C1, spec)
		if err != nil {
			a.errMsg = err.Error()
			return
		}
		a.changed(res)
	}
	if a.book.TextToColumnsOverwrites(a.active, src, dest.R1, dest.C1, spec) {
		a.dialog.Show(appName, "Do you want to replace the contents of the destination cells?", "OK", "Cancel")
		a.answer = func(i int) {
			if i == 0 {
				run()
			}
		}
		return
	}
	run()
}

// dupsDialog is Remove Duplicates.
type dupsDialog struct {
	region    model.Range
	header    widget.Bool
	cols      []widget.Bool
	all, none widget.Clickable
	list      widget.List
}

// removeDuplicatesDialog opens Remove Duplicates for the selection (or
// the data around the active cell).
func (a *App) removeDuplicatesDialog() {
	if !a.commitForCommand() || !a.singleRange() {
		return
	}
	r := a.sortRegion()
	sh := a.wb.Sheets[a.active]
	if r.R1 == r.R2 && r.C1 == r.C2 && sh.Get(r.R1, r.C1).IsBlank() {
		a.dialog.Show(appName, "The command could not be completed by using the range specified. Select a single cell within the range and try the command again.", "OK")
		a.answer = func(int) {}
		return
	}
	d := &a.ins.dup
	*d = dupsDialog{region: r}
	d.header.Value = a.book.GuessHeader(a.active, r)
	d.cols = make([]widget.Bool, r.C2-r.C1+1)
	for i := range d.cols {
		d.cols[i].Value = true
	}
	d.list.Axis = layout.Vertical
	a.selectRange(a.active, r)
	a.showDialog(dlgRemoveDups)
}

func (a *App) layoutRemoveDupsDialog(gtx layout.Context, k int) {
	d := &a.ins.dup
	for a.ins.ok.Clicked(gtx) {
		k = 1
	}
	for a.ins.cancel.Clicked(gtx) {
		k = -1
	}
	for d.all.Clicked(gtx) {
		for i := range d.cols {
			d.cols[i].Value = true
		}
	}
	for d.none.Clicked(gtx) {
		for i := range d.cols {
			d.cols[i].Value = false
		}
	}
	if k < 0 {
		a.closeDialog()
		return
	}
	if k > 0 {
		var cols []int
		for i := range d.cols {
			if d.cols[i].Value {
				cols = append(cols, d.region.C1+i)
			}
		}
		if len(cols) == 0 {
			a.ins.err = "Select one or more columns."
			return
		}
		a.closeDialog()
		if !a.readyToChange() {
			return
		}
		removed, remain, res := a.book.RemoveDuplicates(a.active, d.region, cols, d.header.Value)
		if removed > 0 {
			a.changed(res)
		}
		msg := fmt.Sprintf("%d duplicate values found and removed; %d unique values remain.", removed, remain)
		if removed == 0 {
			msg = "No duplicate values found."
		}
		a.dialog.Show(appName, msg, "OK")
		a.answer = func(int) {}
		return
	}
	a.focusArea(gtx)
	th := a.ui
	sh := a.wb.Sheets[a.active]
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Paragraph("To delete duplicate values, select one or more columns that contain duplicates.", chromeText)),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(a.plainButton(&d.all, "Select All")),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(a.plainButton(&d.none, "Unselect All")),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(a.ui.CheckBox(&d.header, "My data has headers")),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w, h := gtx.Constraints.Max.X, gtx.Dp(180)
				gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), chromeLine)
				gui.FillRect(gtx.Ops, image.Rect(1, 1, w-1, h-1), colors.White)
				g := gtx
				g.Constraints = layout.Exact(image.Pt(w-8, h-4))
				o := op.Offset(image.Pt(4, 2)).Push(gtx.Ops)
				cs := clip.Rect{Max: g.Constraints.Max}.Push(gtx.Ops)
				material.List(a.th, &d.list).Layout(g, len(d.cols), func(gtx layout.Context, i int) layout.Dimensions {
					c := d.region.C1 + i
					name := "Column " + model.ColName(c)
					if d.header.Value {
						if t := sh.Get(d.region.R1, c).Text; t != "" {
							name = t
						}
					}
					return a.ui.CheckBox(&d.cols[i], name)(gtx)
				})
				cs.Pop()
				o.Pop()
				return layout.Dimensions{Size: image.Pt(w, h)}
			}),
			a.errorLine(),
		)
	}
	a.ui.ModalDialog(gtx, &a.ins.block, "Remove Duplicates", 440, body, a.accentButton(&a.ins.ok, "OK"), a.plainButton(&a.ins.cancel, "Cancel"))
}
