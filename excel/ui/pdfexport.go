package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/x/explorer"

	"officelite/common/gui"
	"officelite/common/pdf"
	"officelite/common/version"
	"officelite/excel/render"
)

// pdfState holds the PDF settings shared by the Export and Print pages.
type pdfState struct {
	workbook  bool // all visible sheets, not just the active one
	fit       bool // fit all columns on one page
	gridlines bool
	noOpen    bool // don't open the exported PDF

	workbookBtn, fitBtn, gridBtn, openBtn widget.Clickable
}

// pdfSheets lists the sheets to export: the active one, or every visible
// sheet.
func (a *App) pdfSheets() []int {
	if !a.bs.pdf.workbook {
		return []int{a.active}
	}
	var out []int
	for i, sh := range a.wb.Sheets {
		if !sh.Hidden {
			out = append(out, i)
		}
	}
	return out
}

// buildPDF renders the sheets to a PDF in memory. It runs on the UI
// goroutine, so the workbook can't change meanwhile.
func (a *App) buildPDF() (b []byte, pages int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	d := pdf.New()
	d.Title = a.baseName()
	d.Producer = version.String()
	d.Fonts, d.Fallbacks = a.fonts, gui.FallbackFamilies
	o := render.PDFOptions{
		Sheets:    a.pdfSheets(),
		Gridlines: a.bs.pdf.gridlines,
		FitWidth:  a.bs.pdf.fit,
	}
	if a.shaper != nil && len(a.wb.Styles) > 0 {
		o.DigitWidth = render.DigitWidth(a.shaper, a.wb.Styles[0].Font)
	}
	if err := render.ExportPDF(d, a.wb, o); err != nil {
		return nil, 0, err
	}
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), d.Pages(), nil
}

// exportPDF asks for a file name and writes the PDF there, then opens it
// in the default viewer unless that option is off.
func (a *App) exportPDF() {
	if a.saving.Load() {
		return
	}
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	a.saving.Store(true)
	name := a.baseName() + ".pdf"
	go func() {
		path, err := a.files.Save(name, gui.PDFFilter, gui.AllFilter)
		switch {
		case err == nil:
			a.post(func() {
				defer a.saving.Store(false)
				b, pages, err := a.buildPDF()
				if err == nil {
					err = os.WriteFile(path, b, 0o644)
				}
				a.pdfDone(path, pages, err, !a.bs.pdf.noOpen)
			})
			return
		case errors.Is(err, gui.ErrCancelled):
			a.saving.Store(false)
			return
		}
		// No native dialog: the explorer's Save dialog.
		wc, err := a.expl.CreateFile(name)
		a.post(func() {
			defer a.saving.Store(false)
			if errors.Is(err, explorer.ErrUserDecline) {
				return
			}
			var pages int
			path := name
			if err == nil {
				var b []byte
				b, pages, err = a.buildPDF()
				if err == nil {
					_, err = wc.Write(b)
				}
				if cerr := wc.Close(); err == nil {
					err = cerr
				}
				if f, ok := wc.(*os.File); ok {
					path = f.Name()
				}
			}
			a.pdfDone(path, pages, err, !a.bs.pdf.noOpen && filepath.IsAbs(path))
		})
	}()
}

// printViaPDF writes the PDF to a temporary file and opens it in the
// default PDF viewer, to be printed from there.
func (a *App) printViaPDF() {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	b, pages, err := a.buildPDF()
	path := ""
	if err == nil {
		dir := filepath.Join(os.TempDir(), "OfficeLite")
		err = os.MkdirAll(dir, 0o755)
		// A new name each time: the viewer may still hold the last one.
		path = filepath.Join(dir, fmt.Sprintf("%s (print %s).pdf", a.baseName(), time.Now().Format("150405")))
		if err == nil {
			err = os.WriteFile(path, b, 0o644)
		}
	}
	if err != nil {
		a.errMsg = "Print failed: " + err.Error()
		return
	}
	if err := gui.OpenWithDefault(path); err != nil {
		a.errMsg = "Can't open the PDF viewer: " + err.Error() + " (the PDF is " + path + ")"
		return
	}
	a.errMsg = ""
	a.back.Hide()
	a.grid.focusPending = true
	a.notice = fmt.Sprintf("Opened %d page(s) in your PDF viewer; print from there", pages)
}

// pdfDone reports an export on the UI goroutine.
func (a *App) pdfDone(path string, pages int, err error, open bool) {
	if err != nil {
		a.errMsg = "PDF export failed: " + err.Error()
		return
	}
	a.errMsg = ""
	a.back.Hide()
	a.grid.focusPending = true
	a.notice = fmt.Sprintf("Exported %s (%d page(s))", filepath.Base(path), pages)
	if open {
		if err := gui.OpenWithDefault(path); err != nil {
			a.notice += "; can't open it: " + err.Error()
		}
	}
}

// pdfOptionRows are the PDF settings, shared by the Export and Print
// pages; withOpen adds "Open the PDF after exporting".
func (a *App) pdfOptionRows(withOpen bool) []layout.Widget {
	th, st := a.ui, &a.bs.pdf
	for st.workbookBtn.Clicked(a.gtx) {
		st.workbook = !st.workbook
	}
	for st.fitBtn.Clicked(a.gtx) {
		st.fit = !st.fit
	}
	for st.gridBtn.Clicked(a.gtx) {
		st.gridlines = !st.gridlines
	}
	for st.openBtn.Clicked(a.gtx) {
		st.noOpen = !st.noOpen
	}
	check := func(btn *widget.Clickable, label string, on *bool, invert bool) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return th.Check(gtx, btn, label, *on != invert)
		}
	}
	rows := []layout.Widget{
		check(&st.workbookBtn, "Entire workbook (every visible sheet), not just the active sheet", &st.workbook, false),
		check(&st.fitBtn, "Fit all columns on one page", &st.fit, false),
		check(&st.gridBtn, "Print gridlines", &st.gridlines, false),
	}
	if withOpen {
		rows = append(rows, check(&st.openBtn, "Open the PDF after exporting", &st.noOpen, true))
	}
	return rows
}

// pdfExportRows are the Export page's PDF section.
func (a *App) pdfExportRows() []layout.Widget {
	th, bs := a.ui, &a.bs
	for bs.pdfOpt.Clicked(a.gtx) {
		a.exportPDF()
	}
	rows := []layout.Widget{th.Heading("Create PDF"), func(gtx layout.Context) layout.Dimensions {
		if a.saving.Load() {
			gtx = gtx.Disabled()
		}
		return th.Option(gtx, &bs.pdfOpt, gui.IconFile, "PDF", "Letter pages as Excel prints them: the used range, page by page down then across", false)
	}}
	rows = append(rows, a.pdfOptionRows(true)...)
	if a.errMsg != "" {
		rows = append(rows, layout.Spacer{Height: 12}.Layout, th.Paragraph(a.errMsg, gui.ErrorText))
	}
	return rows
}

// printPageRows is the Print page: printing goes through the PDF viewer.
func (a *App) printPageRows() []layout.Widget {
	th, bs := a.ui, &a.bs
	for bs.printBtn.Clicked(a.gtx) {
		a.printViaPDF()
	}
	rows := []layout.Widget{
		th.Paragraph("Direct printing isn't available in "+version.String()+" yet. "+
			"Print via PDF opens the sheet as a PDF in your default PDF viewer; print it from there.", gui.MutedText),
		th.Heading("Settings"),
	}
	rows = append(rows, a.pdfOptionRows(false)...)
	rows = append(rows, layout.Spacer{Height: 16}.Layout, func(gtx layout.Context) layout.Dimensions {
		return th.Button(gtx, &bs.printBtn, "Print via PDF")
	})
	if a.errMsg != "" {
		rows = append(rows, layout.Spacer{Height: 12}.Layout, th.Paragraph(a.errMsg, gui.ErrorText))
	}
	return rows
}
