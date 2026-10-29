// pdfexport_test.go tests building the PDF export and laying out the Export and Print pages.

package ui

import (
	"bytes"
	"testing"

	"gioui.org/io/key"

	"officelite/common/gui"
)

// TestPDFExportBuild checks a one-cell sheet builds a one-page PDF, the Export
// and Print pages lay out, and a workbook export covers every sheet.
func TestPDFExportBuild(t *testing.T) {
	h := newHarness(t)
	h.typeText("Hello PDF")
	h.press(key.NameReturn, 0)
	b, pages, err := h.a.buildPDF()
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || !bytes.HasPrefix(b, []byte("%PDF-1.7")) {
		t.Fatalf("%d pages, starts %q", pages, b[:min(len(b), 8)])
	}
	// The Export and Print pages lay out with the PDF settings.
	for _, p := range []gui.Page{gui.PageExport, gui.PagePrint} {
		h.a.back.Show(p)
		h.frame()
		h.frame()
	}
	h.a.bs.pdf.workbook = true
	if got := h.a.pdfSheets(); len(got) != len(h.a.wb.Sheets) {
		t.Fatalf("workbook export sheets %v", got)
	}
}
