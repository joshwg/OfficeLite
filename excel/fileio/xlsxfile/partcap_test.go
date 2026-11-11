// Tests for the per-part size cap: a part over it fails the open (a
// required part) or is dropped with a warning (a chart), never cut short.

package xlsxfile

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"officelite/excel/internal/chartsample"
	"officelite/excel/model"
)

// padPart returns data with an XML comment of n bytes put after part
// name's XML declaration, and the part's new size.
func padPart(t *testing.T, data []byte, name string, n int) ([]byte, int) {
	t.Helper()
	size := 0
	out := rewritePart(t, data, name, func(s string) string {
		i := strings.Index(s, "?>") + 2
		s = s[:i] + "<!--" + strings.Repeat("x", n) + "-->" + s[i:]
		size = len(s)
		return s
	})
	if size == 0 {
		t.Fatalf("no part %s", name)
	}
	return out, size
}

// The cap reader hands out exactly limit bytes, then fails; an entry of
// exactly limit bytes reads whole.
func TestCappedReader(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, n := range map[string]int{"fits": 100, "over": 101} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(bytes.Repeat([]byte("a"), n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		noted := 0
		rc, err := openPart(f, 100, func(*partTooLargeError) { noted++ })
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		var tooBig *partTooLargeError
		switch f.Name {
		case "fits":
			if err != nil || len(b) != 100 || noted != 0 {
				t.Errorf("fits: %d bytes, %v, noted %d", len(b), err, noted)
			}
		case "over":
			if !errors.As(err, &tooBig) || len(b) > 100 || noted != 1 {
				t.Errorf("over: %d bytes, %v, noted %d", len(b), err, noted)
			}
			if want := "file is damaged or too large: part over is larger than 100 bytes"; err.Error() != want {
				t.Errorf("message %q, want %q", err, want)
			}
		}
	}
}

// A worksheet over the cap fails the open with the cap's message; at the
// cap it opens.
func TestSheetOverPartCap(t *testing.T) {
	data, size := padPart(t, mustWrite(t, buildModel()), "xl/worksheets/sheet1.xml", 4096)
	_, err := read(data, int64(size-1))
	if err == nil || !strings.Contains(err.Error(), "file is damaged or too large: part xl/worksheets/sheet1.xml is larger than") {
		t.Fatalf("err = %v", err)
	}
	if _, err := read(data, int64(size)); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
}

// A chart part over the cap drops that chart with a warning; the rest of
// the workbook opens.
func TestChartOverPartCap(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(chartsample.Created(), &buf); err != nil {
		t.Fatal(err)
	}
	full, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, size := padPart(t, buf.Bytes(), "xl/charts/chart1.xml", 1<<16)
	wb, err := read(data, int64(size-1))
	if err != nil {
		t.Fatal(err)
	}
	nFull, nGot := countCharts(full), countCharts(wb)
	if nGot != nFull-1 {
		t.Errorf("%d charts, want %d", nGot, nFull-1)
	}
	found := false
	for _, w := range wb.Warnings {
		if strings.Contains(w, "part xl/charts/chart1.xml is larger than") && strings.Contains(w, "a chart was dropped") {
			found = true
		}
	}
	if !found {
		t.Errorf("no chart warning in %q", wb.Warnings)
	}
}

// countCharts counts a workbook's charts, chart sheets' included.
func countCharts(wb *model.Workbook) int {
	n := 0
	for _, sh := range wb.Sheets {
		n += len(sh.Charts)
		if sh.ChartSheet != nil {
			n++
		}
	}
	return n
}
