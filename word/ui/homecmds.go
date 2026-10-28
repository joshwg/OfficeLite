package ui

import (
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/word/model"
)

// Home commands added to the Home menu after Modify Style: find and
// replace, paste options, the Format Painter, style management and
// headers and footers.

// Home menu items after homeModify (indexes into its Items).
const (
	_ = homeModify + 1 + iota
	homeNewStyle
	homeUpdateStyle
	homeStylesPane
	_
	homeFind
	homeReplace
	homeGoTo
	_
	homePasteSpecial
	homePastePlain
	homeCopyFormat
	homePasteFormat
	_
	homeEditHeader
	homeEditFooter
	homePageNumber
	homeRemoveHeader
	homeRemoveFooter
)

func (d *window) homeExtraItems() []gui.MenuItem {
	styleName := styleName(d.doc.Styles.Find(d.ed.ParaStyleID(d.ed.Sel.Caret.Para)))
	return []gui.MenuItem{
		{Separator: true},
		{Label: "New Style…"},
		{Label: "Update " + styleName + " to Match Selection"},
		{Label: "Styles Pane", Shortcut: "Ctrl+Alt+Shift+S", Checked: d.pane.open},
		{Separator: true},
		{Label: "Find…", Shortcut: "Ctrl+F"},
		{Label: "Replace…", Shortcut: "Ctrl+H"},
		{Label: "Go To…", Shortcut: "Ctrl+G"},
		{Separator: true},
		{Label: "Paste Special…", Shortcut: "Ctrl+Alt+V"},
		{Label: "Paste as Plain Text"},
		{Label: "Copy Formatting", Shortcut: "Ctrl+Shift+C"},
		{Label: "Paste Formatting", Shortcut: "Ctrl+Shift+V", Disabled: d.fmtClip == nil},
		{Separator: true},
		{Label: "Edit Header"},
		{Label: "Edit Footer"},
		{Label: "Page Number (in Footer)"},
		{Label: "Remove Header"},
		{Label: "Remove Footer"},
	}
}

// homeExtra runs a Home menu item after homeModify.
func (d *window) homeExtra(item int) {
	switch item {
	case homeNewStyle:
		d.newStyle.openFor(d)
	case homeUpdateStyle:
		d.ed.UpdateStyleToMatch(d.ed.ParaStyleID(d.ed.Sel.Caret.Para))
	case homeStylesPane:
		d.pane.open = !d.pane.open
	case homeFind:
		d.openFind(findOnly)
	case homeReplace:
		d.openFind(findReplace)
	case homeGoTo:
		d.openFind(findGoTo)
	case homePasteSpecial:
		d.pasteSpec.open = true
	case homePastePlain:
		d.pastePlainText(d.gtx)
	case homeCopyFormat:
		d.fmtClip = d.ed.CopyFormat()
	case homePasteFormat:
		d.ed.PasteFormat(d.fmtClip)
	case homeEditHeader, homeEditFooter:
		d.enterHF(d.eng.PageOfPos(d.bodyEd().Sel.Caret), item == homeEditFooter)
	case homePageNumber:
		d.hfInsertField(model.FieldPage)
	case homeRemoveHeader, homeRemoveFooter:
		d.removeHF(item == homeRemoveFooter)
	}
}

// extraKeys handles the shortcuts of these commands.
func (d *window) extraKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "F", Required: key.ModShortcut},
			key.Filter{Name: "H", Required: key.ModShortcut},
			key.Filter{Name: "G", Required: key.ModShortcut},
			key.Filter{Name: key.NameF3, Optional: key.ModShift},
			key.Filter{Name: "C", Required: key.ModShortcut | key.ModShift},
			key.Filter{Name: "V", Required: key.ModShortcut | key.ModShift},
			key.Filter{Name: "V", Required: key.ModShortcut | key.ModAlt},
			key.Filter{Name: "S", Required: key.ModShortcut | key.ModAlt | key.ModShift},
			key.Filter{Name: ".", Required: key.ModShortcut | key.ModShift},
			key.Filter{Name: ",", Required: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press || d.modalOpen() || d.back.Visible {
			continue
		}
		shift := e.Modifiers.Contain(key.ModShift)
		alt := e.Modifiers.Contain(key.ModAlt)
		switch e.Name {
		case "F":
			d.openFind(findOnly)
		case "H":
			d.openFind(findReplace)
		case "G":
			d.openFind(findGoTo)
		case key.NameF3:
			if d.find.what.Text() != "" {
				d.findNext(shift)
			} else {
				d.openFind(findOnly)
			}
			continue
		case "C":
			d.fmtClip = d.ed.CopyFormat()
			d.notice = "Formatting copied"
		case "V":
			if alt {
				d.pasteSpec.open = true
			} else if d.fmtClip != nil {
				d.ed.PasteFormat(d.fmtClip)
				d.edited()
			}
		case "S":
			d.pane.open = !d.pane.open
		case ".", ",":
			// Ctrl+Shift+> and < (the shifted keys).
			dir := 1
			if e.Name == "," {
				dir = -1
			}
			d.ed.GrowFont(dir)
			d.edited()
		}
	}
}

// handleEscape closes what Esc closes among these features (the Format
// Painter, the find bar, header editing) and reports whether it did.
func (d *window) handleEscape() bool {
	switch {
	case d.pasteSpec.open:
		d.pasteSpec.open = false
	case d.newStyle.open:
		d.newStyle.open = false
	case d.painter.armed:
		d.painterOff()
	case d.find.open:
		d.closeFind()
	case d.hf.on:
		d.exitHF()
	default:
		return false
	}
	return true
}

// extraModal reports whether one of these features' dialogs is open.
func (d *window) extraModal() bool { return d.pasteSpec.open || d.newStyle.open }

// extraDialogs draws these features' dialogs.
func (d *window) extraDialogs(gtx layout.Context) {
	if d.pasteSpec.open {
		d.pasteSpec.layout(gtx, d)
	}
	if d.newStyle.open {
		d.newStyle.layout(gtx, d)
	}
}

// Paste options.

// pastePlainText pastes the clipboard as text in the caret's formatting
// (Keep Text Only).
func (d *window) pastePlainText(gtx layout.Context) {
	d.pastePlain = true
	gtx.Execute(clipboard.ReadCmd{Tag: &d.v})
}

// pasteSpecial is the Paste Special dialog (Ctrl+Alt+V): paste as
// formatted text (keep source formatting) or unformatted text.
type pasteSpecial struct {
	open       bool
	choice     int
	opts       [2]widget.Clickable
	ok, cancel widget.Clickable
	tag        int
}

func (p *pasteSpecial) layout(gtx layout.Context, d *window) {
	for i := range p.opts {
		for p.opts[i].Clicked(gtx) {
			p.choice = i
		}
	}
	for p.ok.Clicked(gtx) {
		p.open = false
		if p.choice == 1 {
			d.pastePlainText(gtx)
		} else {
			d.paste(gtx)
		}
		d.v.focusPending = true
	}
	for p.cancel.Clicked(gtx) {
		p.open = false
		d.v.focusPending = true
	}
	th := d.th
	modal(gtx, &p.tag, 440, func(gtx layout.Context) layout.Dimensions {
		opt := func(i int, ic *widget.Icon, title, desc string) layout.FlexChild {
			return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.Option(gtx, &p.opts[i], ic, title, desc, p.choice == i)
				})
			})
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(th.Label(18, "Paste Special", gui.ChromeText, font.SemiBold).Layout),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(th.Label(13, "As:", gui.MutedText, font.Normal).Layout),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			opt(0, gui.IconContentPaste, "Formatted Text", "Keep the copied text's fonts, styles and paragraph formatting."),
			opt(1, gui.IconClearFormat, "Unformatted Text", "Paste only the text, in the formatting where it goes."),
			layout.Rigid(layout.Spacer{Height: 14}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &p.ok, "OK") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return plainButton(gtx, th, &p.cancel, "Cancel") }),
				)
			}),
		)
	})
}

// Format Painter.

// painterState is the Format Painter: armed after a click on its button
// (once) or a double-click (sticky, until Esc or another click).
type painterState struct {
	armed, sticky bool
}

// painterClick runs a click on the Format Painter button.
func (d *window) painterClick(clicks int) {
	p := &d.painter
	if p.armed && clicks < 2 {
		d.painterOff()
		return
	}
	d.fmtClip = d.ed.CopyFormat()
	p.armed, p.sticky = true, clicks >= 2
	d.notice = "Format Painter: select the text to format"
	if p.sticky {
		d.notice += " (Esc to stop)"
	}
}

func (d *window) painterOff() {
	if d.painter.armed {
		d.painter.armed, d.painter.sticky = false, false
		d.notice = ""
	}
}

// painterRelease applies the painter after a mouse selection.
func (d *window) painterRelease() {
	if !d.painter.armed || d.fmtClip == nil {
		return
	}
	d.ed.PasteFormat(d.fmtClip)
	if !d.painter.sticky {
		d.painterOff()
	}
	d.edited()
}

// painterCursor shows the painter cursor over the page view.
func (d *window) painterCursor(gtx layout.Context) {
	if d.painter.armed {
		pointer.CursorCrosshair.Add(gtx.Ops)
	}
}

// previewHome sets up the wordshot preview of these features: the find
// bar (with "the" searched), header or footer editing, the Styles pane,
// New Style or Paste Special.
func (d *window) previewHome(what string) {
	d.relayout()
	switch what {
	case "find", "replace":
		mode := findOnly
		if what == "replace" {
			mode = findReplace
		}
		d.openFind(mode)
		d.find.what.SetText("the")
		d.find.with.SetText("a")
		d.findNext(false)
	case "goto":
		d.openFind(findGoTo)
	case "header", "footer":
		d.enterHF(0, what == "footer")
		if len(d.ed.Para(0).Runs) == 0 {
			d.ed.InsertText("Quarterly report\tDraft\tPage ")
			d.hfInsertField(model.FieldPage)
		}
		d.relayout()
	case "styles":
		d.pane.open = true
	case "newstyle":
		d.newStyle.openFor(d)
	case "paste":
		d.pasteSpec.open = true
	}
}
