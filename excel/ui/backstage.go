package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/x/explorer"

	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/common/version"
	"officelite/excel/fileio"
	"officelite/excel/model"
)

// Tiles in the New section.
var templates = []gui.Template{
	{Name: "Blank workbook", Kind: recent.Spreadsheet},
	{Name: "Blank document", Kind: recent.Document},
}

// Formats offered by Save As, and by Export (text formats only).
var (
	saveFormats   = []fileio.Format{fileio.XLSX, fileio.XLS, fileio.CSV, fileio.TSV}
	exportFormats = []fileio.Format{fileio.CSV, fileio.TSV}
)

// backstageState holds the widgets of ExcelLite's own backstage pages.
type backstageState struct {
	saveFmt    int // index into saveFormats
	saveOpts   [4]widget.Clickable
	saveBtn    widget.Clickable
	exportOpts [2]widget.Clickable
	pdfOpt     widget.Clickable
	printBtn   widget.Clickable
	pdf        pdfState // PDF export and Print via PDF (pdfexport.go)
	info       infoCache
}

// infoCache keeps the Info page's counts, which need a pass over the cells.
type infoCache struct {
	wb       *model.Workbook
	formulas int
	cells    int
}

func (a *App) backstageConfig() gui.BackstageConfig {
	return gui.BackstageConfig{
		Templates: templates,
		Recent:    a.recent.Entries(),
		Content:   a.backstagePage,
	}
}

func (a *App) handleBackstage(ev gui.BackstageEvent) {
	switch ev.Action {
	case gui.ActBack:
		a.closing = false
		a.grid.focusPending = true
	case gui.ActNew:
		a.back.Hide()
		kind := doctype.Spreadsheet
		if templates[ev.Template].Kind == recent.Document {
			kind = doctype.Document
		}
		a.launch(kind, "")
	case gui.ActOpenRecent:
		a.openFile(ev.Entry.Path)
	case gui.ActBrowse:
		a.openDialog()
	case gui.ActSave:
		a.save()
	case gui.ActClose:
		a.win.Perform(system.ActionClose)
	}
}

// pristine reports whether the window holds an untouched new workbook,
// which Excel replaces when a file is opened.
func (a *App) pristine() bool {
	return a.path == "" && !a.modified && !a.loading.Load()
}

// openFile opens path in this window if it is pristine, otherwise in a new
// window; documents go to a WordLite window.
func (a *App) openFile(path string) {
	if _, err := os.Stat(path); err != nil {
		a.recent.Remove(path)
		a.errMsg = fmt.Sprintf("Can't open %s: %v", filepath.Base(path), err)
		a.back.Hide()
		return
	}
	a.back.Hide()
	kind, _ := doctype.DetectFile(path)
	switch {
	case kind == doctype.Document:
		a.launch(doctype.Document, path)
	case a.pristine():
		a.openPath(path)
	default:
		a.launch(doctype.Spreadsheet, path)
	}
}

// save writes the workbook back to its file in the file's format, or
// asks where to save it (a new workbook).
func (a *App) save() {
	if a.ed.active && !a.commitEdit(0, 0) {
		a.closing = false
		return
	}
	f, ok := fileio.FormatFor(a.path)
	if a.path == "" || !ok {
		a.back.Show(gui.PageSaveAs)
		return
	}
	a.back.Hide()
	a.grid.focusPending = true
	if !a.modified {
		a.notice = "No changes to save"
		a.closeIfClosing()
		return
	}
	path := a.path
	a.confirmFormat(f, path, func() {
		if err := fileio.SaveFile(a.wb, path, f, a.active); err != nil {
			a.errMsg = "Save failed: " + err.Error()
			a.closing = false
			return
		}
		a.modified = false
		a.fileInfo, _ = os.Stat(path)
		a.errMsg = ""
		a.notice = "Saved " + filepath.Base(path) + lossNote(a.wb, f, a.active)
		a.closeIfClosing()
	})
}

// confirmFormat runs save, first asking (once per file) whether to go on
// when format f can't hold everything in the workbook, as Excel warns
// before saving a CSV.
func (a *App) confirmFormat(f fileio.Format, path string, save func()) {
	if path != "" && a.warned == path {
		save()
		return
	}
	warnings := saveWarnings(a.wb, f, a.active)
	if len(warnings) == 0 {
		save()
		return
	}
	msg := "Some features of this workbook can't be saved in " + f.String() + ":\n\n• " +
		strings.Join(warnings, "\n• ") + "\n\nTo keep them, save as an Excel Workbook (*.xlsx)."
	a.dialog.Show("Save as "+strings.TrimPrefix(f.Ext(), ".")+"?", msg, "Save", "Cancel")
	a.answer = func(i int) {
		if i != 0 {
			a.closing = false
			return
		}
		a.warned = path
		save()
	}
}

// saveWarnings lists what saving wb in format f would lose.
func saveWarnings(wb *model.Workbook, f fileio.Format, sheet int) []string {
	var out []string
	if !f.MultiSheet() {
		if len(wb.Sheets) > 1 {
			out = append(out, fmt.Sprintf("Only the active sheet (%q) is saved", wb.Sheets[sheet].Name))
		}
		out = append(out, "Formulas are saved as their values, and formatting is lost")
	}
	return append(out, fileio.Fits(wb, f)...)
}

// lossNote reminds what a single-sheet text format left out.
func lossNote(wb *model.Workbook, f fileio.Format, sheet int) string {
	if f.MultiSheet() {
		return ""
	}
	if len(wb.Sheets) > 1 {
		return fmt.Sprintf(" (values of sheet %q only)", wb.Sheets[sheet].Name)
	}
	return " (values only)"
}

// askSaveBeforeClose asks what to do with unsaved changes when the window
// is being closed.
func (a *App) askSaveBeforeClose() {
	a.menu.Close()
	a.dialog.Show(appName, "Want to save your changes to "+a.wb.Name+"?", "Save", "Don't Save", "Cancel")
	a.answer = func(i int) {
		switch i {
		case 0:
			a.closing = true
			a.save()
		case 1:
			a.mayClose = true
			a.win.Perform(system.ActionClose)
		default:
			a.closing = false
		}
	}
}

// closeIfClosing closes the window after the save that a close started.
func (a *App) closeIfClosing() {
	if a.closing && !a.modified && a.win != nil {
		a.win.Perform(system.ActionClose)
	}
}

// baseName is the workbook name without its extension.
func (a *App) baseName() string {
	return strings.TrimSuffix(a.wb.Name, filepath.Ext(a.wb.Name))
}

// saveAs asks for a file name and writes the workbook in format f. A Save
// As makes the new file the window's document; an export does not.
func (a *App) saveAs(f fileio.Format, export bool) {
	if a.saving.Load() {
		return
	}
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	if !export {
		a.confirmFormat(f, "", func() { a.writeAs(f, false) })
		return
	}
	a.writeAs(f, true)
}

// writeAs asks for a file name and writes the workbook. The dialog runs
// on its own goroutine; the writing happens on the UI goroutine, so the
// workbook can't change while it is written.
func (a *App) writeAs(f fileio.Format, export bool) {
	a.saving.Store(true)
	name := a.baseName() + f.Ext()
	go func() {
		wc, err := a.expl.CreateFile(name)
		a.post(func() {
			defer a.saving.Store(false)
			if errors.Is(err, explorer.ErrUserDecline) {
				a.closing = false
				return
			}
			wb := a.wb
			if err == nil {
				err = fileio.Write(wb, wc, f, a.active)
				if cerr := wc.Close(); err == nil {
					err = cerr
				}
			}
			path := name
			if file, ok := wc.(*os.File); ok && err == nil {
				path = file.Name()
			}
			a.saved(wb, f, path, export, err)
		})
	}()
}

// saved runs on the UI goroutine after a Save As or export finishes.
func (a *App) saved(wb *model.Workbook, f fileio.Format, path string, export bool, err error) {
	if err != nil {
		a.errMsg = "Save failed: " + err.Error()
		a.closing = false
		return
	}
	a.errMsg = ""
	a.back.Hide()
	a.grid.focusPending = true
	what := "Saved "
	if export {
		what = "Exported "
	}
	a.notice = what + filepath.Base(path) + lossNote(wb, f, a.active)
	if export || wb != a.wb {
		return
	}
	a.modified = false
	if filepath.IsAbs(path) {
		a.path = path
		a.fileInfo, _ = os.Stat(path)
		a.recent.Add(path, recent.Spreadsheet)
	}
	wb.Name = filepath.Base(path)
	a.warned = path
	a.win.Option(windowTitle(wb.Name))
	a.closeIfClosing()
}

// backstagePage returns the rows of the Info, Save As, Print and Export
// pages.
func (a *App) backstagePage(p gui.Page) []layout.Widget {
	th := a.ui
	bs := &a.bs
	switch p {
	case gui.PageInfo:
		return a.infoPage()
	case gui.PageSaveAs:
		for i := range saveFormats {
			for bs.saveOpts[i].Clicked(a.gtx) {
				bs.saveFmt = i
			}
		}
		for bs.saveBtn.Clicked(a.gtx) {
			a.saveAs(saveFormats[bs.saveFmt], false)
		}
		f := saveFormats[bs.saveFmt]
		rows := []layout.Widget{
			th.Property("File name", a.baseName()+f.Ext()),
			th.Property("Current file", orNone(a.path, "Not saved yet")),
			th.Heading("Save as type"),
		}
		for i, sf := range saveFormats {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return th.Option(gtx, &bs.saveOpts[i], formatIcon(sf), sf.String(), formatDesc(sf), i == bs.saveFmt)
			})
		}
		rows = append(rows, layout.Spacer{Height: 16}.Layout, func(gtx layout.Context) layout.Dimensions {
			if a.saving.Load() {
				gtx = gtx.Disabled()
			}
			return th.Button(gtx, &bs.saveBtn, "Save")
		})
		return rows
	case gui.PagePrint:
		return a.printPageRows()
	case gui.PageExport:
		for i := range exportFormats {
			for bs.exportOpts[i].Clicked(a.gtx) {
				a.saveAs(exportFormats[i], true)
			}
		}
		rows := []layout.Widget{th.Heading("Change file type")}
		for i, ef := range exportFormats {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return th.Option(gtx, &bs.exportOpts[i], formatIcon(ef), ef.String(), formatDesc(ef), false)
			})
		}
		return append(rows, a.pdfExportRows()...)
	}
	return nil
}

func (a *App) infoPage() []layout.Widget {
	th := a.ui
	wb := a.wb
	if a.bs.info.wb != wb {
		a.bs.info = infoCache{wb: wb}
		for _, sh := range wb.Sheets {
			for r := 0; r < sh.UsedRows(); r++ {
				for c := 0; c < sh.RowLen(r); c++ {
					cell := sh.Get(r, c)
					if cell.Formula != "" {
						a.bs.info.formulas++
					}
					if cell.Formula != "" || cell.Kind != model.KindEmpty {
						a.bs.info.cells++
					}
				}
			}
		}
	}
	sheets := fmt.Sprint(len(wb.Sheets))
	hidden := 0
	for _, sh := range wb.Sheets {
		if sh.Hidden {
			hidden++
		}
	}
	if hidden > 0 {
		sheets += fmt.Sprintf(" (%d hidden)", hidden)
	}
	sh := wb.Sheets[a.active]
	used := "Empty"
	if rows, cols := sh.UsedRows(), sh.UsedCols(); rows > 0 && cols > 0 {
		used = "A1:" + model.CellName(rows-1, cols-1)
	}
	dates := "1900"
	if wb.Date1904 {
		dates = "1904"
	}
	rows := []layout.Widget{
		th.Heading(wb.Name),
		th.Paragraph(orNone(a.path, "This workbook hasn't been saved yet."), gui.MutedText),
		th.Heading("Properties"),
		th.Property("Type", fileType(a.path)),
	}
	if fi := a.fileInfo; fi != nil {
		rows = append(rows,
			th.Property("Size", byteSize(fi.Size())),
			th.Property("Modified", fi.ModTime().Format("January 2, 2006 3:04 PM")))
	}
	rows = append(rows,
		th.Property("Sheets", sheets),
		th.Property("Used range", fmt.Sprintf("%s (%s)", used, sh.Name)),
		th.Property("Cells", fmt.Sprint(a.bs.info.cells)),
		th.Property("Formulas", fmt.Sprint(a.bs.info.formulas)),
		th.Property("Defined names", fmt.Sprint(len(wb.Names))),
		th.Property("Date system", dates),
	)
	if len(wb.Warnings) > 0 {
		rows = append(rows, th.Heading("Not shown"))
		for _, w := range wb.Warnings {
			rows = append(rows, th.Paragraph("• "+w, gui.ChromeText))
		}
	}
	rows = append(rows, th.Heading("About"),
		th.Paragraph(version.String()+" · "+appName+" · MIT License", gui.MutedText))
	return rows
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}

func fileType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case "":
		return "Excel Workbook (not saved)"
	case ".xlsx":
		return "Excel Workbook"
	case ".xlsm":
		return "Excel Macro-Enabled Workbook (macros are not run)"
	case ".xls":
		return "Excel 97-2003 Workbook"
	case ".csv":
		return "CSV (Comma delimited)"
	case ".tsv", ".txt":
		return "Text (Tab delimited)"
	}
	return "Text"
}

func byteSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func formatIcon(f fileio.Format) *widget.Icon {
	if f.MultiSheet() {
		return gui.IconGrid
	}
	return gui.IconText
}

func formatDesc(f fileio.Format) string {
	switch f {
	case fileio.XLSX:
		return "All sheets, formulas and formatting"
	case fileio.XLS:
		return "For Excel 97-2003: up to 65,536 rows and 256 columns per sheet"
	case fileio.CSV:
		return "Values of the active sheet, separated by commas"
	case fileio.TSV:
		return "Values of the active sheet, separated by tabs"
	}
	return ""
}

func windowTitle(name string) app.Option { return app.Title(name + " - " + appName) }
