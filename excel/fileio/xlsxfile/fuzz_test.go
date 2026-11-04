// Fuzz test: sample packages with one part replaced by arbitrary bytes
// must read without panicking.

package xlsxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

// fuzzPackage is one sample workbook split into its parts.
type fuzzPackage struct {
	names []string
	data  [][]byte
}

// fuzzSamples are the sample workbooks the fuzz cases are built from.
var fuzzSamples = []string{
	"../../../Samples/excel/Notes-and-Hyperlinks.xlsx",
	"../../../Samples/excel-data/Data-Validation-and-Drop-Down-Lists.xlsx",
	"../../../Samples/excel-data/Data-AutoFilter.xlsx",
	"../../../Samples/excel-data/Data-Outline.xlsx",
	"../../../Samples/excel-pivots/Pivot-Tables-and-Charts.xlsx",
	"../../../Samples/excel-charts/Charts.xlsx",
	"../../../Samples/excel-functions/Functions-Arrays.xlsx",
}

// loadFuzzPackage reads the parts of the package at path.
func loadFuzzPackage(path string) (fuzzPackage, error) {
	var p fuzzPackage
	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return p, err
	}
	sort.Slice(zr.File, func(i, j int) bool { return zr.File[i].Name < zr.File[j].Name })
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return p, err
		}
		b, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return p, err
		}
		p.names = append(p.names, f.Name)
		p.data = append(p.data, b)
	}
	return p, nil
}

// FuzzRead replaces one part of a sample workbook with fuzzed bytes and
// reads the result, which must return rather than panic.
func FuzzRead(f *testing.F) {
	var pkgs []fuzzPackage
	for _, s := range fuzzSamples {
		if p, err := loadFuzzPackage(s); err == nil {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		f.Skip("no sample workbooks")
	}
	// One seed per sample keeps plain test runs quick; the fuzzer varies
	// the part.
	for i, p := range pkgs {
		for j, d := range p.data {
			if strings.HasPrefix(p.names[j], "xl/worksheets/sheet") {
				f.Add(uint8(i), uint8(j), d)
				break
			}
		}
	}
	f.Fuzz(func(t *testing.T, pi, part uint8, content []byte) {
		p := pkgs[int(pi)%len(pkgs)]
		k := int(part) % len(p.data)
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for j, name := range p.names {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			d := p.data[j]
			if j == k {
				d = content
			}
			if _, err := w.Write(d); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		wb, err := Read(buf.Bytes())
		if err == nil && wb == nil {
			t.Fatal("nil workbook without an error")
		}
	})
}
