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
	d.saving.Store(true)
	name := d.baseName() + ".pdf"
	go func() {
		path, err := d.files.Save(name, gui.PDFFilter, gui.AllFilter)
		switch {
		case err == nil:
			d.post(func() {
				defer d.saving.Store(false)
				b, pages, err := d.buildPDF()
				if err == nil {
					err = os.WriteFile(path, b, 0o644)
				}
				d.pdfDone(path, pages, err, !d.bs.pdf.noOpen)
			})
			return
		case errors.Is(err, gui.ErrCancelled):
			d.saving.Store(false)
			return
		}
		// No native dialog: the explorer's Save dialog.
		wc, err := d.expl.CreateFile(name)
		d.post(func() {
			defer d.saving.Store(false)
			if errors.Is(err, explorer.ErrUserDecline) {
				return
			}
			var pages int
			path := name
			if err == nil {
				var b []byte
				b, pages, err = d.buildPDF()
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
			d.pdfDone(path, pages, err, !d.bs.pdf.noOpen && filepath.IsAbs(path))
		})
	}()
}

// printViaPDF writes the PDF to a temporary file and opens it in the
// default PDF viewer, to be printed from there.
func (d *window) printViaPDF() {
	b, pages, err := d.buildPDF()
	path := ""
	if err == nil {
		dir := filepath.Join(os.TempDir(), "OfficeLite")
		err = os.MkdirAll(dir, 0o755)
		// A new name each time: the viewer may still hold the last one.
		path = filepath.Join(dir, fmt.Sprintf("%s (print %s).pdf", d.baseName(), time.Now().Format("150405")))
		if err == nil {
			err = os.WriteFile(path, b, 0o644)
		}
	}
	if err != nil {
		d.errMsg = "Print failed: " + err.Error()
		return
	}
	if err := gui.OpenWithDefault(path); err != nil {
		d.errMsg = "Can't open the PDF viewer: " + err.Error() + " (the PDF is " + path + ")"
		return
	}
	d.errMsg = ""
	d.back.Hide()
	d.v.focusPending = true
	d.notice = fmt.Sprintf("Opened %d page(s) in your PDF viewer; print from there", pages)
}

// pdfDone reports an export on the UI goroutine.
func (d *window) pdfDone(path string, pages int, err error, open bool) {
	if err != nil {
		d.errMsg = "PDF export failed: " + err.Error()
		return
	}
	d.errMsg = ""
	d.back.Hide()
	d.v.focusPending = true
	d.notice = fmt.Sprintf("Exported %s (%d page(s))", filepath.Base(path), pages)
	if open {
		if err := gui.OpenWithDefault(path); err != nil {
			d.notice += "; can't open it: " + err.Error()
		}
	}
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
