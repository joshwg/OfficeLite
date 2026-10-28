package ui

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

// insertKeys handles the shortcuts of the Insert menu, Find & Select,
// Fill and notes, wherever the focus is (except in dialogs).
func (a *App) insertKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: "F", Required: key.ModShortcut},
			key.Filter{Name: "H", Required: key.ModShortcut},
			key.Filter{Name: "G", Required: key.ModShortcut},
			key.Filter{Name: "K", Required: key.ModShortcut},
			key.Filter{Name: "D", Required: key.ModShortcut},
			key.Filter{Name: "R", Required: key.ModShortcut},
			key.Filter{Name: key.NameF5},
			key.Filter{Name: key.NameF2, Required: key.ModShift},
			key.Filter{Name: key.NameF3, Required: key.ModShift},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press || a.back.Visible || a.dialog.Visible {
			continue
		}
		if a.ins.kind != dlgNone && a.ins.kind != dlgFind {
			continue // a dialog has the keyboard
		}
		if a.ed.active && e.Name != key.NameF3 {
			continue // shortcuts that change cells wait for the entry
		}
		a.menu.Close()
		switch e.Name {
		case "F":
			a.showFind()
		case "H":
			a.showReplace()
		case "G", key.NameF5:
			a.showGoTo()
		case "K":
			a.insertHyperlinkDialog()
		case "D":
			a.fillDown()
		case "R":
			a.fillRight()
		case key.NameF2:
			a.newNote()
		case key.NameF3:
			a.insertFunctionDialog()
		}
	}
}
