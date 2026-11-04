// fuzz_test.go fuzzes the .docx reader by patching bytes of one part of
// the testdata documents; a reader panic fails the test.

package docxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// fuzzPart is one entry of a package.
type fuzzPart struct {
	name string
	data []byte
}

// fuzzParts loads the entries of a testdata package.
func fuzzParts(tb testing.TB, name string) []fuzzPart {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		tb.Fatal(err)
	}
	var out []fuzzPart
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			tb.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			tb.Fatal(err)
		}
		if err := rc.Close(); err != nil {
			tb.Fatal(err)
		}
		out = append(out, fuzzPart{f.Name, b})
	}
	return out
}

// FuzzRead patches the bytes of one part of a testdata package (part
// picks it, off the position) and checks Read and a save never panic.
func FuzzRead(f *testing.F) {
	files := []string{"word.docx", "word2.docx", "pydocx.docx"}
	base := make([][]fuzzPart, len(files))
	for i, n := range files {
		base[i] = fuzzParts(f, n)
	}
	f.Add(uint8(0), uint8(0), uint32(0), []byte{0})
	f.Fuzz(func(t *testing.T, file, part uint8, off uint32, patch []byte) {
		parts := base[int(file)%len(base)]
		i := int(part) % len(parts)
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for j, p := range parts {
			d := p.data
			if j == i && len(d) > 0 {
				d = append([]byte(nil), d...)
				o := int(off % uint32(len(d)))
				copy(d[o:], patch)
			}
			w, err := zw.Create(p.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(d); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		doc, err := Read(buf.Bytes())
		if err != nil {
			return
		}
		if err := Write(doc, io.Discard); err != nil {
			t.Logf("write: %v", err)
		}
	})
}
