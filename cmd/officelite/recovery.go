// Document Recovery: the window listing copies a crashed OfficeLite left,
// and saving one of them under a new name.

package main

import (
	"log"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/x/explorer"

	"officelite/common/autorecover"
	"officelite/common/gui"
	excelio "officelite/excel/fileio"
	wordio "officelite/word/fileio"
)

// runRecovery shows the Document Recovery window for the copies a crashed
// OfficeLite left. They are this process's while it is open (so another
// OfficeLite started meanwhile doesn't list them too); Close lets the rest
// go for the next start.
func runRecovery(w *app.Window, fonts *gui.FontLibrary, entries []autorecover.Entry, open func(autorecover.Entry)) {
	for _, e := range entries {
		autorecover.Claim(e)
	}
	w.Option(app.Title("Document Recovery - OfficeLite"), app.Size(unit.Dp(680), unit.Dp(480)))
	expl := explorer.NewExplorer(w)
	r := gui.NewRecovery(gui.RecoveryConfig{
		Fonts:    fonts,
		Entries:  entries,
		Open:     open,
		SaveAs:   saveRecovered,
		Close:    func() { w.Perform(system.ActionClose) },
		Wake:     w.Invalidate,
		Create:   expl.CreateFile,
		Declined: explorer.ErrUserDecline,
	})
	defer r.Release()
	var ops op.Ops
	for {
		e := w.Event()
		expl.ListenEvents(e)
		r.Files.ListenEvents(e)
		switch e := e.(type) {
		case app.DestroyEvent:
			if e.Err != nil {
				log.Print(e.Err)
			}
			return
		case app.FrameEvent:
			r.Run()
			gtx := app.NewContext(&ops, e)
			r.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// saveRecovered writes a recovered copy to path in the format of path's
// extension (.xlsx / .docx when it names none OfficeLite writes).
func saveRecovered(e autorecover.Entry, path string) error {
	if e.App == autorecover.Word {
		doc, err := wordio.Open(e.Copy)
		if err != nil {
			return err
		}
		f, ok := wordio.FormatFor(path)
		if !ok {
			f = wordio.DOCX
		}
		return wordio.SaveFile(doc, path, f)
	}
	wb, err := excelio.Open(e.Copy)
	if err != nil {
		return err
	}
	f, ok := excelio.FormatFor(path)
	if !ok {
		f = excelio.XLSX
	}
	return excelio.SaveFile(wb, path, f, 0)
}
