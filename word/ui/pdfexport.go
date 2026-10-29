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
)

// pdfState holds the PDF export setting.
type pdfState struct {
	noOpen  bool // don't open the exported PDF
	openBtn widget.Clickable
}

// buildPDF renders the document's pages, as the page view lays them out,
// to a PDF in memory. It runs on the UI goroutine (the layout engine's
// shaper belongs to it).
func (d *window) buildPDF() (b []byte, pages int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	d.relayout()
	p := pdf.New()
	p.Title = d.baseName()
	p.Producer = version.String()
	p.Fonts, p.Fallbacks = d.cfg.Fonts, gui.FallbackFamilies
	d.eng.ExportPDF(p)
	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), p.Pages(), nil
}

// exportPDF asks for a file name and writes the PDF there, then opens it
// in the default viewer unless that option is off.
func (d *window) exportPDF() {
	if d.saving.Load() || d.expl == nil {
		return
	}
	gui.SavePDF(gui.PDFSave{Name: d.baseName(), Files: &d.files, Create: d.expl.CreateFile,
		Declined: explorer.ErrUserDecline, Calls: d.calls, Busy: &d.saving, Build: d.buildPDF,
		Open: !d.bs.pdf.noOpen, Report: d.pdfDone})
}

// printViaPDF opens the PDF in the default PDF viewer, to be printed from
// there.
func (d *window) printViaPDF() { d.pdfDone(gui.PrintViaPDF(d.baseName(), d.buildPDF)) }

// pdfDone reports an export or print on the UI goroutine.
func (d *window) pdfDone(notice, errMsg string) {
	d.errMsg = errMsg
	if errMsg != "" {
		return
	}
	d.back.Hide()
	d.v.focusPending = true
	d.notice = notice
}

// pdfExportRows are the Export page's PDF section.
func (d *window) pdfExportRows() []layout.Widget {
	th, bs := d.th, &d.bs
	for bs.pdfOpt.Clicked(d.gtx) {
		d.exportPDF()
	}
	for bs.pdf.openBtn.Clicked(d.gtx) {
		bs.pdf.noOpen = !bs.pdf.noOpen
	}
	return []layout.Widget{
		th.Heading("Create PDF"),
		func(gtx layout.Context) layout.Dimensions {
			if d.saving.Load() {
				gtx = gtx.Disabled()
			}
			return th.Option(gtx, &bs.pdfOpt, gui.IconFile, "PDF", "The pages as laid out here, with fonts embedded; text stays searchable", false)
		},
		func(gtx layout.Context) layout.Dimensions {
			return th.Check(gtx, &bs.pdf.openBtn, "Open the PDF after exporting", !bs.pdf.noOpen)
		},
	}
}

// printPageRows is the Print page: printing goes through the PDF viewer.
func (d *window) printPageRows() []layout.Widget {
	th, bs := d.th, &d.bs
	for bs.printBtn.Clicked(d.gtx) {
		d.printViaPDF()
	}
	rows := []layout.Widget{
		th.Paragraph("Direct printing isn't available in "+version.String()+" yet. "+
			"Print via PDF opens the document as a PDF in your default PDF viewer; print it from there.", gui.MutedText),
		layout.Spacer{Height: 12}.Layout,
		th.Property("Pages", fmt.Sprint(len(d.eng.Pages))),
		layout.Spacer{Height: 16}.Layout,
		func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &bs.printBtn, "Print via PDF") },
	}
	if d.errMsg != "" {
		rows = append(rows, layout.Spacer{Height: 12}.Layout, th.Paragraph(d.errMsg, gui.ErrorText))
	}
	return rows
}
