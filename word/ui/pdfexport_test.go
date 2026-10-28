package ui

import (
	"bytes"
	"testing"

	"officelite/common/gui"
)

func TestPDFExportBuild(t *testing.T) {
	h := newHarness(t)
	h.typeText("Hello PDF")
	b, pages, err := h.d.buildPDF()
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || !bytes.HasPrefix(b, []byte("%PDF-1.7")) || !bytes.Contains(b, []byte("/Producer (OfficeLite ")) {
		t.Fatalf("%d pages, starts %q", pages, b[:min(len(b), 8)])
	}
	for _, p := range []gui.Page{gui.PageExport, gui.PagePrint} {
		h.d.back.Show(p)
		h.frame()
		h.frame()
	}
}
