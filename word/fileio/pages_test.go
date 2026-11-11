// Tests for opening Pages documents through the format-sniffing Read.

package fileio

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestOpenPages checks that a .pages file opens through Read (by
// content) but can't be saved back — Save goes to Save As — and that an
// iWork '09 package is refused with a "not supported" error.
func TestOpenPages(t *testing.T) {
	data, err := os.ReadFile("../../Samples/word-tests/PagesInvoice.pages")
	if err != nil {
		t.Skip(err)
	}
	doc, err := Read("PagesInvoice.pages", data)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "PagesInvoice.pages" || len(doc.Body) < 10 {
		t.Errorf("name %q, %d blocks", doc.Name, len(doc.Body))
	}
	if _, ok := FormatFor("PagesInvoice.pages"); ok {
		t.Error("FormatFor offers to write .pages")
	}
	var b bytes.Buffer
	if err := Write(doc, &b, DOCX); err != nil {
		t.Errorf("saving the import as .docx: %v", err)
	}

	var old bytes.Buffer
	zw := zip.NewWriter(&old)
	if _, err := zw.Create("index.xml.gz"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("old.pages", old.Bytes()); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("iWork '09 file: %v", err)
	}
}
