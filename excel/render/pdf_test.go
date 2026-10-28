package render

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/common/pdf"
	"officelite/excel/fileio"
	"officelite/excel/model"
)

func testBook(rows, cols int) *model.Workbook {
	wb := &model.Workbook{Name: "t", Styles: []model.Style{{Font: model.Font{Family: "Calibri", SizePt: 11}}}}
	bordered := wb.Styles[0]
	bordered.Fill = colors.RGB(0xFFFF00)
	bordered.Border[model.Bottom] = model.Edge{Style: model.BorderThin, Color: colors.RGB(0)}
	wb.Styles = append(wb.Styles, bordered)
	sh := model.NewSheet("Sheet1")
	for r := range rows {
		for c := range cols {
			sh.Set(r, c, model.Cell{Text: fmt.Sprintf("R%dC%d", r+1, c+1), Kind: model.KindText, Style: uint32(r % 2)})
		}
	}
	wb.Sheets = []*model.Sheet{sh}
	return wb
}

func exportPages(t *testing.T, wb *model.Workbook, o PDFOptions) (int, []byte) {
	t.Helper()
	d := pdf.New()
	d.NoCompress = true
	if err := ExportPDF(d, wb, o); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := d.Write(&out); err != nil {
		t.Fatal(err)
	}
	return d.Pages(), out.Bytes()
}

func TestExportPDFPagination(t *testing.T) {
	// 100 rows of 20px (15pt) and 20 columns of 64px (48pt): 44 rows and
	// 10 columns fit a Letter page inside the margins.
	wb := testBook(100, 20)
	n, out := exportPages(t, wb, PDFOptions{Sheets: []int{0}, DigitWidth: 7})
	if n != 3*2 {
		t.Fatalf("%d pages, want 6 (3 down × 2 across)", n)
	}
	if !strings.Contains(string(out), "1 1 0 rg") {
		t.Error("no yellow fill")
	}
	// Down, then over: page 2 continues the first columns.
	n, _ = exportPages(t, wb, PDFOptions{Sheets: []int{0}, DigitWidth: 7, FitWidth: true})
	if n >= 6 {
		t.Fatalf("fit to width: %d pages", n)
	}
	cols := bands(NewGeometry(wb.Sheets[0], 7).Cols, 0, 19, 612-2*marginLR, 1)
	if len(cols) != 2 || cols[0] != [2]int{0, 9} {
		t.Fatalf("column bands %v", cols)
	}
}

func TestExportPDFEmpty(t *testing.T) {
	wb := testBook(0, 0)
	if err := ExportPDF(pdf.New(), wb, PDFOptions{Sheets: []int{0}}); !errors.Is(err, ErrNothingToPrint) {
		t.Fatalf("err %v", err)
	}
}

func TestWrapText(t *testing.T) {
	d := pdf.New()
	f := d.Font(pdf.FontSpec{Family: "Go"})
	lines := wrapText(f, 10, "one two three\nfour", f.Measure("one two", 10)+0.1)
	if strings.Join(lines, "|") != "one two|three|four" {
		t.Fatalf("%q", lines)
	}
	if lines := wrapText(f, 10, "abcdefgh", f.Measure("abc", 10)); len(lines) != 3 {
		t.Fatalf("long word: %q", lines)
	}
}

// TestExportPDFSamples writes the sample workbooks as PDFs into the folder
// named by PDF_SAMPLES, for checking them by eye.
func TestExportPDFSamples(t *testing.T) {
	dir := os.Getenv("PDF_SAMPLES")
	if dir == "" {
		t.Skip("PDF_SAMPLES not set")
	}
	files, _ := filepath.Glob("../../Samples/*.xls*")
	lib := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	for _, path := range files {
		wb, err := fileio.Open(path)
		if err != nil {
			t.Logf("%s: %v", path, err)
			continue
		}
		var sheets []int
		for i, sh := range wb.Sheets {
			if !sh.Hidden {
				sheets = append(sheets, i)
			}
		}
		for _, grid := range []bool{false, true} {
			d := pdf.New()
			d.Title, d.Producer = wb.Name, "OfficeLite test"
			d.Fonts, d.Fallbacks = lib, gui.FallbackFamilies
			if err := ExportPDF(d, wb, PDFOptions{Sheets: sheets, Gridlines: grid, FitWidth: grid}); err != nil {
				t.Logf("%s: %v", path, err)
				continue
			}
			var out bytes.Buffer
			if err := d.Write(&out); err != nil {
				t.Fatal(err)
			}
			suffix := ""
			if grid {
				suffix = "-grid"
			}
			name := filepath.Join(dir, "excel-"+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+suffix+".pdf")
			if err := os.WriteFile(name, out.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d pages, %d bytes", name, d.Pages(), out.Len())
		}
	}
}
