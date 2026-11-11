// partsize_test.go checks that a part larger than the size cap fails the open
// (instead of being silently truncated), whether the part is required
// (document.xml) or optional (a header the reader could do without).

package docxfile

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPartTooLarge lowers the part cap to the largest part of a fixture and
// pads one part past it: the open must fail with a "too large" error naming
// the part.
func TestPartTooLarge(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "word.docx"))
	if err != nil {
		t.Fatal(err)
	}
	parts := zipParts(t, data)
	var biggest int64
	for _, b := range parts {
		biggest = max(biggest, int64(len(b)))
	}
	old := maxPartSize
	t.Cleanup(func() { maxPartSize = old })
	maxPartSize = biggest
	if _, err := Read(data); err != nil {
		t.Fatalf("every part fits the cap, yet: %v", err)
	}
	for _, name := range []string{"word/document.xml", "word/header1.xml", "word/media/image1.png"} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for n, b := range parts {
			if n == name {
				// Trailing whitespace keeps the XML valid.
				b = append(append([]byte{}, b...), bytes.Repeat([]byte(" "), int(biggest))...)
			}
			writeZipFile(t, zw, n, b)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		_, err := Read(buf.Bytes())
		if err == nil || !strings.Contains(err.Error(), "too large") || !strings.Contains(err.Error(), name) {
			t.Errorf("%s past the cap: got error %v, want a \"too large\" error naming the part", name, err)
		}
	}
}
