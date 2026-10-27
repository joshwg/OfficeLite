package doctype

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func zipWith(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("<x/>"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDetect(t *testing.T) {
	xls, err := os.ReadFile("../../excel/fileio/xlsfile/testdata/sample.xls")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		data []byte
		want Kind
	}{
		{"a.xlsx", zipWith(t, "[Content_Types].xml", "xl/workbook.xml"), Spreadsheet},
		{"renamed.zip", zipWith(t, "word/document.xml"), Document},
		{"wrong-ext.docx", zipWith(t, "xl/workbook.xml"), Spreadsheet},
		{"sample.xls", xls, Spreadsheet},
		{"data.csv", []byte("a,b\n1,2\n"), Spreadsheet},
		{"notes.txt", []byte("hello"), Document},
		{"mystery", []byte("hello"), Unknown},
	} {
		if got := Detect(c.name, c.data); got != c.want {
			t.Errorf("Detect(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
