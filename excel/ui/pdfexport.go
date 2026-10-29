// pdfexport.go builds the workbook's PDF and lays out the backstage Export and Print pages' PDF settings.

package ui

import (
	"bytes"
	"fmt"

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
	gui.SavePDF(gui.PDFSave{Name: a.baseName(), Files: &a.files, Create: a.expl.CreateFile,
		Declined: explorer.ErrUserDecline, Calls: a.calls, Busy: &a.saving, Build: a.buildPDF,
		Open: !a.bs.pdf.noOpen, Report: a.pdfDone})
}

// printViaPDF opens the PDF in the default PDF viewer, to be printed from
// there.
func (a *App) printViaPDF() {
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	a.pdfDone(gui.PrintViaPDF(a.baseName(), a.buildPDF))
}

// pdfDone reports an export or print on the UI goroutine.
func (a *App) pdfDone(notice, errMsg string) {
	a.errMsg = errMsg
	if errMsg != "" {
		return
	}
	a.back.Hide()
	a.grid.focusPending = true
	a.notice = notice
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
