package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/x/explorer"

	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/recent"
	"officelite/common/version"
	"officelite/word/fileio"
	"officelite/word/model"
)

// Formats offered by Save As, and by Export.
var (
	saveFormats   = []fileio.Format{fileio.DOCX, fileio.DOC, fileio.TXT}
	exportFormats = []fileio.Format{fileio.TXT, fileio.DOC}
)

// backstageState holds the widgets of WordLite's own backstage pages.
type backstageState struct {
	saveFmt    int // index into saveFormats
	saveOpts   [3]widget.Clickable
	saveBtn    widget.Clickable
	exportOpts [2]widget.Clickable
	pdfOpt     widget.Clickable
	printBtn   widget.Clickable
	pdf        pdfState // PDF export (pdfexport.go)
	info       infoCache
}

// infoCache keeps the Info page's style list, which needs a pass over the
// document.
type infoCache struct {
	version int
	doc     *model.Document
	styles  []string
}

func (d *window) backstageConfig() gui.BackstageConfig {
	return gui.BackstageConfig{
		Templates: templates,
		Recent:    d.cfg.Recent.Entries(),
		Content:   d.backstagePage,
	}
}

func (d *window) handleBackstage(ev gui.BackstageEvent) {
	switch ev.Action {
	case gui.ActBack:
		d.v.focusPending = true
		d.closeAfterSave = false
	case gui.ActNew:
		d.back.Hide()
		kind := doctype.Document
		if templates[ev.Template].Kind == recent.Spreadsheet {
			kind = doctype.Spreadsheet
		}
		d.cfg.Launch(kind, "")
	case gui.ActOpenRecent:
		d.openFile(ev.Entry.Path)
	case gui.ActBrowse:
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
	case ".docx":
		return fileio.DOCX, true
	case ".doc":
		return fileio.DOC, true
	case ".txt":
		return fileio.TXT, true
	}
	return 0, false // .docm and others: Save As
}

// save writes the document back to its file, or asks where to save it.
func (d *window) save() {
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
	d.checkLossy(f, func() { d.saveTo(f) })
}

// saveTo writes the document to its file in format f.
func (d *window) saveTo(f fileio.Format) {
	// After an AutoSave still writing, never under it (autosave.go).
	if err := d.as.saver.Save(func() error { return fileio.SaveFile(d.doc, d.path, f) }); err != nil {
		d.errMsg = lockfile.SaveError(d.path, err)
		d.closeAfterSave = false
		return
	}
	d.errMsg = ""
	d.saved = d.bodyEd().Version
	d.dropRecovery() // recover.go
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
	if d.saving.Load() || d.expl == nil {
		return
	}
	d.checkLossy(f, func() { d.writeAs(f, export) })
}

func (d *window) writeAs(f fileio.Format, export bool) {
	d.saving.Store(true)
	doc, version := d.doc.Clone(), d.bodyEd().Version
	name := d.saveAsName(f) // recover.go
	go func() {
		defer d.saving.Store(false)
		wc, err := d.expl.CreateFile(name)
		if errors.Is(err, explorer.ErrUserDecline) {
			d.calls.Post(func() { d.closeAfterSave = false })
			return
		}
		if err == nil {
			err = fileio.Write(doc, wc, f)
			if cerr := wc.Close(); err == nil {
				err = cerr
			}
		}
		path := name
		if file, ok := wc.(*os.File); ok && err == nil {
			path = file.Name()
		}
		d.calls.Post(func() { d.savedAs(f, path, export, version, err) })
	}()
}

// savedAs runs on the UI goroutine after a Save As or export finishes.
func (d *window) savedAs(f fileio.Format, path string, export bool, version int, err error) {
	if err != nil {
		d.errMsg = "Save failed: " + err.Error()
		d.closeAfterSave = false
		return
	}
	d.errMsg = ""
	d.back.Hide()
	d.v.focusPending = true
	what := "Saved "
	if export {
		what = "Exported "
	}
	d.notice = what + filepath.Base(path)
	if f == fileio.TXT {
		d.notice += " (text only)"
	}
	if export {
		return
	}
	d.saved = version
	d.dropRecovery()
	if filepath.IsAbs(path) {
		d.path = path
		d.fileInfo, _ = os.Stat(path)
		d.cfg.Recent.Add(path, recent.Document)
		d.lk.readOnly = false
		d.lk.doc.Set(path, true) // the new file's registration and owner file
		d.as.elsewhere = false
	}
	d.doc.Name = filepath.Base(path)
	d.updateTitle()
	if !d.as.saver.On() {
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
	styleText := strings.Join(styles[:min(len(styles), 12)], ", ")
	if len(styles) > 12 {
		styleText += fmt.Sprintf(" and %d more", len(styles)-12)
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
	case ".txt":
		return "Plain Text"
	case ".pages":
		return "Apple Pages Document (imported; Save As to keep changes)"
	}
	return "Document"
}

func formatIcon(f fileio.Format) *gui.Icon {
	if f == fileio.TXT {
		return gui.IconText
	}
	return gui.IconFile
}

func formatDesc(f fileio.Format) string {
	switch f {
	case fileio.DOCX:
		return "Text, formatting, styles and lists"
	case fileio.DOC:
		return "For Word 97 to 2003"
	case fileio.TXT:
		return "Text only, one line per paragraph"
	}
	return ""
}
