package fileio

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

// A .numbers file opens through Read (by content) but can't be saved
// back: Save goes to Save As.
func TestOpenNumbers(t *testing.T) {
	data, err := os.ReadFile("../../Samples/Sample3.numbers")
	if err != nil {
		t.Skip(err)
	}
	wb, err := Read("Sample3.numbers", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Sheets) != 3 || wb.Name != "Sample3.numbers" {
		t.Errorf("%d sheets, name %q", len(wb.Sheets), wb.Name)
	}
	if _, ok := FormatFor("Sample3.numbers"); ok {
		t.Error("FormatFor offers to write .numbers")
	}
	var b bytes.Buffer
	if err := Write(wb, &b, XLSX, 0); err != nil {
		t.Errorf("saving the import as .xlsx: %v", err)
	}

	var old bytes.Buffer // an iWork '09 package: gzipped XML
	zw := zip.NewWriter(&old)
	zw.Create("index.xml.gz")
	zw.Close()
	if _, err := Read("old.numbers", old.Bytes()); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("iWork '09 file: %v", err)
	}
}
