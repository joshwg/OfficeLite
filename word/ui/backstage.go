// The File backstage for documents: New, Open, Info, Save, Save As and
// Export, and the saving behind them.

package ui

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/x/explorer"

	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/recent"
	"officelite/common/templates"
	"officelite/common/version"
	"officelite/word"
	"officelite/word/fileio"
	"officelite/word/model"
)

// Formats offered by Save As, and by Export.
var (
	saveFormats   = [...]fileio.Format{fileio.DOCX, fileio.DOC, fileio.DOTX, fileio.DOT, fileio.TXT}
	exportFormats = [...]fileio.Format{fileio.TXT, fileio.DOC}
)

// backstageState holds the widgets of WordLite's own backstage pages.
type backstageState struct {
	saveFmt    int // index into saveFormats
	saveOpts   [len(saveFormats)]widget.Clickable
	saveBtn    widget.Clickable
	exportOpts [len(exportFormats)]widget.Clickable
	pdfOpt     widget.Clickable
	printBtn   widget.Clickable
	pdf        pdfState // PDF export (pdfexport.go)
	info       infoCache
	tmpl       templates.Lister // the user's templates, for the New page (template.go)
	tiles      []gui.Template   // the New tiles shown this frame: blankTiles, then templates
}

// infoCache keeps the Info page's style list, which needs a pass over the
// document.
type infoCache struct {
	version int
	doc     *model.Document
	styles  []string
}

// backstageConfig is the backstage's New tiles, recent files and page
// content.
func (d *window) backstageConfig() gui.BackstageConfig {
	return gui.BackstageConfig{
		Templates:    d.newTiles(), // template.go
		TemplateNote: templateNote(),
		Recent:       d.cfg.Recent.Entries(),
		Content:      d.backstagePage,
	}
}

// handleBackstage carries out a backstage action: back, new, open, save
// or close.
func (d *window) handleBackstage(ev gui.BackstageEvent) {
	switch ev.Action {
	case gui.ActBack:
		d.v.focusPending = true
		d.closeAfterSave = false
	case gui.ActNew:
		d.closeAfterSave = false // leaving the Save As page abandons a close waiting for it
		if ev.Template >= 0 && ev.Template < len(d.bs.tiles) {
			d.newFromTile(d.bs.tiles[ev.Template]) // template.go
		}
	case gui.ActOpenRecent:
		d.closeAfterSave = false
		d.openFile(ev.Entry.Path)
	case gui.ActBrowse:
		d.closeAfterSave = false
		d.openDialog()
	case gui.ActSave:
		d.save()
	case gui.ActClose:
		d.close()
	}
}

// saveFormat is the format Save writes to the current file, if it has one
// WordLite writes.
func (d *window) saveFormat() (fileio.Format, bool) {
	if d.path == "" || d.lk.readOnly {
		return 0, false
	}
	switch strings.ToLower(filepath.Ext(d.path)) {
	case ".docx", ".doc", ".txt", ".dotx", ".dot":
		return fileio.FormatFor(d.path)
	}
	return 0, false // .docm, .dotm and others: Save As
}

// save writes the document back to its file, or asks where to save it.
func (d *window) save() {
	if d.opening() {
		d.closeAfterSave = false
		return // nothing to save until the document arrives
	}
	f, ok := d.saveFormat()
	if !ok {
		d.back.Show(gui.PageSaveAs)
		return
	}
	d.back.Hide()
	d.v.focusPending = true
	if !d.modified() {
		d.notice = "No changes to save"
		d.afterSave()
		return
	}
	d.checkMacros(true, func() { d.checkLossy(f, func() { d.saveTo(f) }) })
}

// saveTo writes the document to its file in format f.
func (d *window) saveTo(f fileio.Format) {
	// After an AutoSave still writing, never under it (autosave.go).
	if err := d.as.Saver.Save(func() error { return fileio.SaveFile(d.doc, d.path, f) }); err != nil {
		d.errMsg = lockfile.SaveError(d.path, err)
		d.closeAfterSave = false
		return
	}
	d.errMsg = ""
	d.saved = d.bodyEd().Version
	d.doc.DropMacros() // the file has none now
	d.dropRecovery()   // recover.go
	d.fileInfo, _ = os.Stat(d.path)
	d.notice = "Saved " + filepath.Base(d.path)
	d.afterSave()
}

// afterSave closes the window when saving was asked for by the close
// prompt.
func (d *window) afterSave() {
	if d.closeAfterSave {
		d.closeAfterSave = false
		d.allowClose = true
		d.close()
	}
}

// baseName is the document name without its extension.
func (d *window) baseName() string {
	return strings.TrimSuffix(d.doc.Name, filepath.Ext(d.doc.Name))
}

// saveAs asks for a file name and writes a copy of the document in format
// f. A Save As makes the new file the window's document; an export does
// not.
func (d *window) saveAs(f fileio.Format, export bool) {
	if d.saving.Load() || d.expl == nil || d.opening() {
		return
	}
	check := func() { d.checkLossy(f, func() { d.writeAs(f, export) }) }
	if export {
		check() // the window's file, and its macros, stay as they are
		return
	}
	d.checkMacros(false, check)
}

// writeAs asks for a file name and writes a clone of the document there
// in the background (gui.SaveAs). d.saving stays set until the result is
// handled on the UI goroutine, so the window isn't pristine (and its
// document can't be replaced) in between.
func (d *window) writeAs(f fileio.Format, export bool) {
	d.saveAsJob(f, export).Start()
}

// saveSnap is a Save As or export in progress: doc, a clone of owner
// taken at the body editor's version, written in format f.
type saveSnap struct {
	owner, doc *model.Document
	version    int
	f          fileio.Format
	export     bool
}

// saveAsJob is the Save As (or export, which leaves the window's file as
// it is) of the document in format f.
func (d *window) saveAsJob(f fileio.Format, export bool) gui.SaveAs[saveSnap] {
	j := gui.SaveAs[saveSnap]{
		Name:     d.saveAsName(f), // recover.go
		Save:     d.files.Save,
		Filters:  []gui.FileFilter{gui.FormatFilter(f.String(), f.Ext()), gui.AllFilter},
		Create:   d.expl.CreateFile,
		Declined: explorer.ErrUserDecline,
		Calls:    d.calls,
		Busy:     &d.saving,
		Snapshot: func() saveSnap {
			return saveSnap{owner: d.doc, doc: d.doc.Clone(), version: d.bodyEd().Version, f: f, export: export}
		},
		Write:  func(s saveSnap, w io.Writer) error { return fileio.Write(s.doc, w, s.f) },
		Done:   d.writeAsDone,
		Cancel: func() { d.closeAfterSave = false },
	}
	if f.IsTemplate() {
		j.Locate = templateSaveName // template.go
	}
	if !export {
		j.Ask = d.askSaveLocked // lock.go
	}
	return j
}

// writeAsDone runs on the UI goroutine when a Save As or export of
// snapshot s has ended as r says: when the window still shows s's
// document it becomes the saved file (savedAs). A document that replaced
// it meanwhile keeps its own path and name.
func (d *window) writeAsDone(s saveSnap, r gui.SaveAsResult) {
	if s.owner != d.doc && r.Err == nil && !s.export {
		d.errMsg = ""
		d.notice = "Saved " + filepath.Base(r.Path)
		d.closeAfterSave = false
		return
	}
	d.savedAs(s, r)
}

// savedAs runs on the UI goroutine after a Save As or export of snapshot
// s finishes as r says.
func (d *window) savedAs(s saveSnap, r gui.SaveAsResult) {
	if r.Err != nil {
		d.errMsg = "Save failed: " + r.Err.Error()
		d.closeAfterSave = false
		return
	}
	path := r.Path
	d.errMsg = ""
	d.back.Hide()
	d.v.focusPending = true
	what := "Saved "
	if s.export {
		what = "Exported "
	}
	d.notice = what + filepath.Base(path)
	if s.f == fileio.TXT {
		d.notice += " (text only)"
	}
	if s.export {
		d.closeAfterSave = false // an export isn't the save the close waits for
		return
	}
	d.saved = s.version
	d.doc.DropMacros() // the window's file is the new one, without them
	d.dropRecovery()
	if filepath.IsAbs(path) {
		d.path = path
		d.fileInfo, _ = os.Stat(path)
		if err := d.cfg.Recent.Add(path, recent.Document); err != nil {
			log.Printf("add %s to recent documents: %v", path, err)
		}
		d.lk.readOnly = false
		d.lk.doc.Set(path, true)  // the new file's registration and owner file
		d.as.Elsewhere = r.Shared // saved over a file open elsewhere: no AutoSave
	}
	d.doc.Name = filepath.Base(path)
	d.updateTitle()
	if !d.as.Saver.On() {
		d.autoSaveLoaded() // on for a .docx when that's the default (autosave.go)
	}
	d.afterSave()
}

// backstagePage returns the rows of the Info, Save As, Print and Export
// pages.
func (d *window) backstagePage(p gui.Page) []layout.Widget {
	th := d.th
	bs := &d.bs
	switch p {
	case gui.PageInfo:
		return d.infoPage()
	case gui.PageSaveAs:
		for i := range saveFormats {
			for bs.saveOpts[i].Clicked(d.gtx) {
				bs.saveFmt = i
			}
		}
		for bs.saveBtn.Clicked(d.gtx) {
			d.saveAs(saveFormats[bs.saveFmt], false)
		}
		f := saveFormats[bs.saveFmt]
		rows := []layout.Widget{
			th.Property("File name", d.baseName()+f.Ext()),
			th.Property("Current file", gui.OrNone(d.path, "Not saved yet")),
			th.Heading("Save as type"),
		}
		for i, sf := range saveFormats {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return th.Option(gtx, &bs.saveOpts[i], formatIcon(sf), sf.String(), formatDesc(sf), i == bs.saveFmt)
			})
		}
		rows = append(rows, layout.Spacer{Height: 16}.Layout, func(gtx layout.Context) layout.Dimensions {
			if d.saving.Load() {
				gtx = gtx.Disabled()
			}
			return th.Button(gtx, &bs.saveBtn, "Save")
		})
		if d.errMsg != "" {
			rows = append(rows, layout.Spacer{Height: 12}.Layout, th.Paragraph(d.errMsg, gui.ErrorText))
		}
		return rows
	case gui.PagePrint:
		return d.printPageRows()
	case gui.PageExport:
		for i := range exportFormats {
			for bs.exportOpts[i].Clicked(d.gtx) {
				d.saveAs(exportFormats[i], true)
			}
		}
		rows := []layout.Widget{th.Heading("Change file type")}
		for i, ef := range exportFormats {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return th.Option(gtx, &bs.exportOpts[i], formatIcon(ef), ef.String(), formatDesc(ef), false)
			})
		}
		rows = append(rows, d.pdfExportRows()...)
		if d.errMsg != "" {
			rows = append(rows, layout.Spacer{Height: 12}.Layout, th.Paragraph(d.errMsg, gui.ErrorText))
		}
		return rows
	}
	return nil
}

// infoPage returns the rows of the Info page: the file's properties and
// counts, what isn't shown, AutoRecover, AutoSave and About.
func (d *window) infoPage() []layout.Widget {
	th := d.th
	doc := d.doc
	if c := &d.bs.info; c.doc != doc || c.version != d.ed.Version {
		*c = infoCache{doc: doc, version: d.ed.Version, styles: d.stylesUsed()}
	}
	st := d.docStats()
	rows := []layout.Widget{
		th.Heading(doc.Name),
		th.Paragraph(gui.OrNone(d.path, "This document hasn't been saved yet."), gui.MutedText),
		th.Heading("Properties"),
		th.Property("Type", fileType(d.path)),
	}
	if fi := d.fileInfo; fi != nil {
		rows = append(rows,
			th.Property("Size", gui.ByteSize(fi.Size())),
			th.Property("Modified", fi.ModTime().Format("January 2, 2006 3:04 PM")))
	}
	styles := d.bs.info.styles
	styleText := strings.Join(styles[:min(len(styles), word.UIMaxInfoStyles)], ", ")
	if len(styles) > word.UIMaxInfoStyles {
		styleText += fmt.Sprintf(" and %d more", len(styles)-word.UIMaxInfoStyles)
	}
	rows = append(rows,
		th.Property("Pages", fmt.Sprint(len(d.eng.Pages))),
		th.Property("Words", fmt.Sprint(st.words)),
		th.Property("Characters", fmt.Sprint(st.chars)),
		th.Property("Paragraphs", fmt.Sprint(st.paras)),
		th.Property("Styles used", gui.OrNone(styleText, "None")),
	)
	if d.modified() {
		rows = append(rows, th.Property("Changes", "Not saved"))
	}
	if len(doc.Warnings) > 0 {
		rows = append(rows, th.Heading("Not shown or not kept"))
		for _, w := range doc.Warnings {
			rows = append(rows, th.Paragraph("• "+w, gui.ChromeText))
		}
	}
	rows = append(rows, d.back.AutoRecoverRows(th)...)
	rows = append(rows, d.back.AutoSaveRows(th, ".docx")...)
	rows = append(rows, th.Heading("About"),
		th.Paragraph(version.String()+" · "+appName+" · MIT License", gui.MutedText))
	return rows
}

// stylesUsed lists the display names of the paragraph and character
// styles the document's text uses.
func (d *window) stylesUsed() []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string, t model.StyleType) {
		if id == "" && t == model.ParagraphStyle {
			id = d.normalID()
		}
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, styleName(d.doc.Styles.Find(id)))
	}
	d.doc.Paragraphs(func(p *model.Paragraph) bool {
		add(p.Style, model.ParagraphStyle)
		for i := range p.Runs {
			add(p.Runs[i].Style, model.CharacterStyle)
		}
		return true
	})
	slices.Sort(out)
	return out
}

// fileType is the Info page's description of path's file type, by its
// extension.
func fileType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case "":
		return "Word Document (not saved)"
	case ".docx":
		return "Word Document"
	case ".docm":
		return "Word Macro-Enabled Document (macros are not run)"
	case ".doc":
		return "Word 97-2003 Document"
	case ".dotx":
		return "Word Template"
	case ".dotm":
		return "Word Macro-Enabled Template (macros are not run)"
	case ".dot":
		return "Word 97-2003 Template"
	case ".txt":
		return "Plain Text"
	case ".pages":
		return "Apple Pages Document (imported; Save As to keep changes)"
	}
	return "Document"
}

// formatIcon is format f's icon: text for plain text, a page otherwise.
func formatIcon(f fileio.Format) *gui.Icon {
	if f == fileio.TXT {
		return gui.IconText
	}
	return gui.IconFile
}

// formatDesc is the line under format f's name in Save As and Export.
func formatDesc(f fileio.Format) string {
	switch f {
	case fileio.DOCX:
		return "Text, formatting, styles and lists"
	case fileio.DOC:
		return "For Word 97 to 2003"
	case fileio.DOTX:
		return "A starting point for new documents, listed on the New page"
	case fileio.DOT:
		return "A template for Word 97 to 2003"
	case fileio.TXT:
		return "Text only, one line per paragraph"
	}
	return ""
}
