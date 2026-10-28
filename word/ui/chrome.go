package ui

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/word/model"
)

// Menu bar entries.
const (
	menuFile = iota
	menuHome
	menuInsert
	menuView
)

// Home menu items (indexes into its Items).
const (
	homeUndo = iota
	homeRedo
	_
	homeCut
	homeCopy
	homePaste
	homeSelectAll
	_
	homeBold
	homeItalic
	homeUnderline
	_
	homeNormal
	homeH1
	homeH2
	homeH3
	homeClear
	homeModify
)

func (d *window) menus() []gui.Menu {
	sel := !d.ed.Sel.Empty()
	return []gui.Menu{
		menuFile: {Label: "File", Primary: true},
		menuHome: {Label: "Home", Items: append([]gui.MenuItem{
			{Label: "Undo", Shortcut: "Ctrl+Z", Disabled: !d.ed.CanUndo(), Tip: "Undo your last action."},
			{Label: "Redo", Shortcut: "Ctrl+Y", Disabled: !d.ed.CanRedo(), Tip: "Redo your last action."},
			{Separator: true},
			{Label: "Cut", Shortcut: "Ctrl+X", Disabled: !sel, Tip: "Move the selection to the Clipboard."},
			{Label: "Copy", Shortcut: "Ctrl+C", Disabled: !sel, Tip: "Copy the selection to the Clipboard."},
			{Label: "Paste", Shortcut: "Ctrl+V", Tip: "Add content from the Clipboard."},
			{Label: "Select All", Shortcut: "Ctrl+A", Tip: "Select the whole document."},
			{Separator: true},
			{Label: "Bold", Shortcut: "Ctrl+B", Tip: "Make your text bold."},
			{Label: "Italic", Shortcut: "Ctrl+I", Tip: "Italicize your text."},
			{Label: "Underline", Shortcut: "Ctrl+U", Tip: "Underline your text."},
			{Separator: true},
			{Label: "Normal", Shortcut: "Ctrl+Shift+N", Tip: "Apply the Normal paragraph style."},
			{Label: "Heading 1", Shortcut: "Ctrl+Alt+1", Tip: "Apply the Heading 1 style."},
			{Label: "Heading 2", Shortcut: "Ctrl+Alt+2", Tip: "Apply the Heading 2 style."},
			{Label: "Heading 3", Shortcut: "Ctrl+Alt+3", Tip: "Apply the Heading 3 style."},
			{Label: "Clear Formatting", Shortcut: "Ctrl+Space", Tip: "Remove character formatting from the selection."},
			{Label: "Modify Style…", Tip: "Change the formatting of the current style."},
		}, d.homeExtraItems()...)},
		menuInsert: {Label: "Insert", Items: d.insertMenu()},
		menuView: {Label: "View", Items: []gui.MenuItem{
			{Label: "Zoom In", Shortcut: "Ctrl++", Tip: "Make the page bigger on screen."},
			{Label: "Zoom Out", Shortcut: "Ctrl+-", Tip: "Make the page smaller on screen."},
			{Label: "100%", Shortcut: "Ctrl+0", Tip: "Zoom to the page's actual size."},
		}},
	}
}

func (d *window) menuChosen(ev gui.MenuEvent) {
	switch ev.Menu {
	case menuFile:
		d.back.Show(gui.PageHome)
		return
	case menuHome:
		switch ev.Item {
		case homeUndo:
			d.ed.Undo()
		case homeRedo:
			d.ed.Redo()
		case homeCut:
			d.copy(d.gtx, true)
		case homeCopy:
			d.copy(d.gtx, false)
		case homePaste:
			d.paste(d.gtx)
		case homeSelectAll:
			d.ed.SelectAll()
		case homeBold:
			d.ed.ToggleBold()
		case homeItalic:
			d.ed.ToggleItalic()
		case homeUnderline:
			d.ed.ToggleUnderline()
		case homeNormal:
			d.applyStyle(d.normalID())
		case homeH1, homeH2, homeH3:
			d.applyHeading(ev.Item - homeH1 + 1)
		case homeClear:
			d.ed.ClearCharFormatting()
		case homeModify:
			d.dlg.openFor(d, d.ed.ParaStyleID(d.ed.Sel.Caret.Para))
		default:
			d.homeExtra(ev.Item)
		}
	case menuInsert:
		d.insertChosen(ev.Item)
	case menuView:
		switch ev.Item {
		case 0:
			d.setZoom(d.v.zoom + 0.1)
		case 1:
			d.setZoom(d.v.zoom - 0.1)
		case 2:
			d.setZoom(1)
		}
	}
	d.edited()
}

func (d *window) layout(gtx layout.Context) {
	d.gtx = gtx
	d.globalKeys(gtx)
	d.extraKeys(gtx)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, gui.ChromeBG)
	d.relayout()
	if d.back.Visible {
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(d.titleBar),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return d.back.Layout(gtx, d.th, d.backstageConfig(), d.handleBackstage)
			}),
		)
	} else {
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(d.titleBar),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return d.menu.Layout(gtx, d.th, d.menus(), d.menuChosen)
			}),
			layout.Rigid(d.toolbar),
			layout.Rigid(d.hfBar),
			layout.Rigid(d.findBarLayout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, d.layoutDoc),
					layout.Rigid(d.vScrollbar),
					layout.Rigid(d.stylesPaneLayout),
				)
			}),
			layout.Rigid(d.statusBar),
		)
	}
	if d.dlg.open {
		d.dlg.layout(gtx, d)
	}
	d.ins.layout(gtx, d)
	d.extraDialogs(gtx)
	if d.confirm.show {
		d.confirm.layout(gtx, d)
	}
	if d.lossy.Visible {
		d.lossyLayout(gtx)
	}
	d.updateTitle()
}

// relayout brings the page layout up to date with the document.
func (d *window) relayout() {
	if d.laidOut == d.ed.Version {
		return
	}
	ps, all := d.ed.TakeDirty()
	d.hfSync()
	d.eng.Invalidate(ps, all)
	d.eng.Layout()
	d.laidOut = d.ed.Version
	d.hfAfterLayout()
}

// edited runs after any command: the layout catches up, the caret is
// shown and kept in view.
func (d *window) edited() {
	d.relayout()
	d.v.ensureCaret = true
	d.v.focusPending = !d.back.Visible
	d.v.resetBlink()
}

// globalKeys handles shortcuts that work regardless of focus.
func (d *window) globalKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "O", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "P", Required: key.ModShortcut},
			key.Filter{Name: "W", Required: key.ModShortcut},
			key.Filter{Name: key.NameF12, Optional: key.ModShortcut},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "+", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Name: "=", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Name: "-", Required: key.ModShortcut},
			key.Filter{Name: "0", Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch e.Name {
		case "N":
			d.cfg.Launch(doctype.Document, "")
		case "O":
			d.menu.Close()
			d.back.Show(gui.PageOpen)
		case "S":
			d.menu.Close()
			d.save()
		case "P":
			d.menu.Close()
			d.back.Show(gui.PagePrint)
		case "W":
			d.close()
		case key.NameF12:
			d.menu.Close()
			if e.Modifiers.Contain(key.ModShortcut) {
				d.openDialog() // Ctrl+F12, as in Word
			} else {
				d.back.Show(gui.PageSaveAs)
			}
		case key.NameEscape:
			switch {
			case d.lossy.Visible:
				d.lossy.Hide()
				d.lossyGo = nil
				d.closeAfterSave = false
			case d.confirm.show:
				d.confirm.show = false
				d.closeAfterSave = false
			case d.dlg.open:
				d.dlg.open = false
			case d.ins.modal():
				d.ins.close(d)
			case d.menu.IsOpen():
				d.menu.Close()
			case d.back.Visible:
				d.back.Hide()
			case d.handleEscape():
			}
			d.v.focusPending = true
		case "+", "=":
			d.setZoom(d.v.zoom + 0.1)
		case "-":
			d.setZoom(d.v.zoom - 0.1)
		case "0":
			d.setZoom(1)
		}
	}
}

func (d *window) close() {
	if d.win != nil {
		d.win.Perform(system.ActionClose)
	}
}

func (d *window) setZoom(z float64) {
	d.v.zoom = min(max(z, 0.1), 5)
	d.v.ensureCaret = true
}

func (d *window) titleBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(36)
	gui.FillRect(gtx.Ops, image.Rect(0, 0, gtx.Constraints.Max.X, h), accent)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	name := d.doc.Name
	if d.modified() {
		name += " •"
	}
	return layout.Inset{Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(d.th.Label(15, appName, colors.White, font.Bold).Layout),
				layout.Rigid(layout.Spacer{Width: 16}.Layout),
				layout.Flexed(1, d.th.Label(13, name, colors.White, font.Normal).Layout),
			)
		})
	})
}

// statsCache keeps the word count, which needs a pass over the text.
type statsCache struct {
	version             int
	valid               bool
	paras, words, chars int
}

func (d *window) docStats() *statsCache {
	if !d.stats.valid || d.stats.version != d.ed.Version {
		d.stats.paras, d.stats.words, d.stats.chars = d.doc.Stats()
		d.stats.version, d.stats.valid = d.ed.Version, true
	}
	return &d.stats
}

func (d *window) statusBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(24)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	w := gtx.Constraints.Max.X
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), gui.ChromeBG)
	gui.HLine(gtx.Ops, 0, w, 0, 1, gui.ChromeLine)

	left := fmt.Sprintf("Page %d of %d", d.ceng().PageOfPos(d.ed.Sel.Caret)+1, max(len(d.eng.Pages), 1))
	words := d.docStats().words
	left += fmt.Sprintf("     %d words", words)
	if !d.ed.Sel.Empty() {
		left = fmt.Sprintf("Page %d of %d     %d of %d words", d.ceng().PageOfPos(d.ed.Sel.Caret)+1, len(d.eng.Pages),
			countWords(d.ed.Copy()), words)
	}
	msg, msgColor := "", gui.MutedText
	switch {
	case d.loading.Load():
		msg = "Loading…"
	case d.saving.Load():
		msg = "Saving…"
	case d.errMsg != "":
		msg, msgColor = d.errMsg, gui.ErrorText
	case d.notice != "":
		msg = d.notice
	case len(d.doc.Warnings) > 0:
		msg = d.doc.Warnings[0]
		if n := len(d.doc.Warnings); n > 1 {
			msg += fmt.Sprintf(" (+%d more)", n-1)
		}
	}
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(d.th.Label(12, left, gui.ChromeText, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Width: 24}.Layout),
				layout.Flexed(1, d.th.Label(12, msg, msgColor, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Width: 16}.Layout),
				layout.Rigid(d.zoomControl),
			)
		})
	})
}

func countWords(f *model.Fragment) int {
	n := 0
	for _, p := range f.Paras {
		_, w, _ := (&model.Document{Body: []model.Block{p}}).Stats()
		n += w
	}
	return n
}

// zoomControl is the status bar's "−  100%  +".
func (d *window) zoomControl(gtx layout.Context) layout.Dimensions {
	v := &d.v
	for v.zoomInBtn.Clicked(gtx) {
		d.setZoom(v.zoom + 0.1)
	}
	for v.zoomOutBtn.Clicked(gtx) {
		d.setZoom(v.zoom - 0.1)
	}
	for v.zoomBtn.Clicked(gtx) {
		d.setZoom(1)
	}
	btn := func(c *widget.Clickable, s string, w int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.Hoverable(gtx, c, colors.Transparent, gui.HoverBG, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(w)), gtx.Constraints.Max.Y))
				l := d.th.Label(12, s, gui.ChromeText, font.Normal)
				l.Alignment = text.Middle
				return layout.Center.Layout(gtx, l.Layout)
			})
		})
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		btn(&v.zoomOutBtn, "−", 22),
		btn(&v.zoomBtn, fmt.Sprintf("%d%%", int(v.zoom*100+0.5)), 44),
		btn(&v.zoomInBtn, "+", 22),
	)
}
