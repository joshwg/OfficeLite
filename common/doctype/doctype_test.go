// doctype_test.go tests detection by content and by extension, including
// iWork files and a corrupt compound file.

package doctype

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"officelite/common/ole"
)

// zipWith makes a zip archive holding a small part for each of names.
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

// TestDetect checks Detect on zip, compound and plain files, by content
// first and by extension otherwise.
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
		{"old.numbers", zipWith(t, "index.xml.gz"), Spreadsheet}, // iWork '09: by extension
		{"old.pages", zipWith(t, "index.xml.gz"), Document},
	} {
		if got := Detect(c.name, c.data); got != c.want {
			t.Errorf("Detect(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestDetectIWork checks that Numbers and Pages samples are told apart by
// content (skipped when the samples are missing).
func TestDetectIWork(t *testing.T) {
	for name, want := range map[string]Kind{
		"Sample2.numbers":    Spreadsheet,
		"PagesInvoice.pages": Document,
	} {
		data, err := os.ReadFile("../../Samples/" + name)
		if err != nil {
			t.Skip(err)
		}
		// By content: the extension says nothing.
		if got := Detect("renamed.zip", data); got != want {
			t.Errorf("Detect(%s) = %v, want %v", name, got, want)
		}
	}
}

// TestDetectCorruptOLE checks that a compound file whose header claims
// 0xFFFFFFFF directory sectors (which used to exhaust memory in mscfb) is
// rejected and detected by its extension.
func TestDetectCorruptOLE(t *testing.T) {
	var b bytes.Buffer
	if err := ole.Write(&b, []ole.Stream{{Name: "WordDocument", Data: []byte("x")}}); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	if k := Detect("a.bin", data); k != Document {
		t.Fatalf("valid file = %v", k)
	}
	binary.LittleEndian.PutUint32(data[40:], 0xFFFFFFFF)
	if k := Detect("a.xls", data); k != Spreadsheet {
		t.Errorf("corrupt file = %v, want extension fallback", k)
	}
}

// TestKindString checks the names of the kinds, and that a value outside
// them reads as unknown instead of panicking.
func TestKindString(t *testing.T) {
	for k, want := range map[Kind]string{Unknown: "unknown", Spreadsheet: "spreadsheet", Document: "document", Kind(7): "unknown", Kind(-1): "unknown"} {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
}
