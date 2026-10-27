package xlsxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/excel/model"
)

func buildSample(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	const sh = "Sheet1"
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	style := func(s *excelize.Style) int {
		id, err := f.NewStyle(s)
		must(err)
		return id
	}

	must(f.SetCellValue(sh, "A1", "Title"))
	must(f.SetCellStyle(sh, "A1", "A1", style(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "FF0000", Family: "Arial"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})))
	must(f.SetCellValue(sh, "A2", 1234.5))
	must(f.SetCellStyle(sh, "A2", "A2", style(&excelize.Style{NumFmt: 4}))) // #,##0.00
	must(f.SetCellValue(sh, "B2", 0.25))
	must(f.SetCellStyle(sh, "B2", "B2", style(&excelize.Style{NumFmt: 9}))) // 0%
	must(f.SetCellValue(sh, "C2", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)))
	must(f.SetCellStyle(sh, "C2", "C2", style(&excelize.Style{CustomNumFmt: ptr("yyyy-mm-dd")})))
	must(f.SetCellValue(sh, "D2", 1.0/3))
	must(f.SetCellValue(sh, "E2", 1234.5))
	// excelize cannot write built-in id 7 (locale currency); write a custom
	// format and patch its id to 7 below.
	must(f.SetCellStyle(sh, "E2", "E2", style(&excelize.Style{CustomNumFmt: ptr("0.000")})))
	must(f.SetCellValue(sh, "A3", true))
	must(f.SetCellStyle(sh, "B3", "B3", style(&excelize.Style{
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFF00"}},
		Border: []excelize.Border{{Type: "bottom", Style: 2, Color: "0000FF"}},
	})))
	must(f.MergeCell(sh, "A5", "C6"))
	must(f.SetColWidth(sh, "A", "A", 20))
	must(f.SetRowHeight(sh, 8, 30))
	must(f.SetPanes(sh, &excelize.Panes{Freeze: true, XSplit: 1, YSplit: 1, TopLeftCell: "B2", ActivePane: "bottomRight"}))

	for r := 10; r <= 12; r++ {
		must(f.SetCellValue(sh, cell("A", r), r))
	}
	shared := excelize.STCellFormulaTypeShared
	ref := "B10:B12"
	must(f.SetCellFormula(sh, "B10", "A10*2", excelize.FormulaOpts{Type: &shared, Ref: &ref}))

	_, err := f.NewSheet("Hidden")
	must(err)
	must(f.SetSheetVisible("Hidden", false))

	buf, err := f.WriteToBuffer()
	must(err)
	return patchZip(t, buf.Bytes(), "xl/styles.xml", `<xf numFmtId="165"`, `<xf numFmtId="7"`) // 164 is the date format
}

// patchZip returns data with one string replaced in one zip entry.
func patchZip(t *testing.T, data []byte, name, old, new string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == name {
			if !bytes.Contains(b, []byte(old)) {
				t.Fatalf("%s does not contain %q", name, old)
			}
			b = bytes.ReplaceAll(b, []byte(old), []byte(new))
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func ptr[T any](v T) *T { return &v }

func cell(col string, row int) string {
	n, _ := excelize.CoordinatesToCellName(model.ParseColName(col)+1, row)
	return n
}

func TestReadSample(t *testing.T) {
	wb, err := Read(buildSample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Sheets) != 2 || !wb.Sheets[1].Hidden {
		t.Fatalf("sheets: %d, second hidden=%v", len(wb.Sheets), len(wb.Sheets) > 1 && wb.Sheets[1].Hidden)
	}
	sh := wb.Sheets[0]
	for _, c := range []struct {
		ref, text string
		kind      model.Kind
	}{
		{"A1", "Title", model.KindText},
		{"A2", "1,234.50", model.KindNumber},
		{"B2", "25%", model.KindNumber},
		{"C2", "2026-10-15", model.KindNumber},
		{"D2", "0.333333333", model.KindNumber},
		{"E2", "$1,234.50 ", model.KindNumber},
		{"A3", "TRUE", model.KindBool},
	} {
		row, col, _ := parseCellRef(c.ref)
		got := sh.Get(row, col)
		if got.Text != c.text || got.Kind != c.kind {
			t.Errorf("%s = %q (kind %d), want %q (kind %d)", c.ref, got.Text, got.Kind, c.text, c.kind)
		}
	}

	title := wb.Styles[sh.Get(0, 0).Style]
	if !title.Font.Bold || title.Font.SizePt != 14 || title.Font.Family != "Arial" ||
		title.Font.Color != colors.RGB(0xFF0000) || title.HAlign != model.HAlignCenter {
		t.Errorf("title style = %+v", title)
	}
	b3 := sh.Get(2, 1)
	if b3.Kind != model.KindEmpty {
		t.Errorf("B3 should be an empty styled cell, got %+v", b3)
	}
	if st := wb.Styles[b3.Style]; st.Fill != colors.RGB(0xFFFF00) ||
		st.Border[model.Bottom] != (model.Edge{Style: model.BorderMedium, Color: colors.RGB(0x0000FF)}) {
		t.Errorf("B3 style = %+v", st)
	}

	if len(sh.Merges) != 1 || sh.Merges[0] != (model.Range{R1: 4, C1: 0, R2: 5, C2: 2}) {
		t.Errorf("merges = %v", sh.Merges)
	}
	if w := sh.ColWidth[0]; w < 19.9 || w > 20.8 { // stored in characters
		t.Errorf("col A width = %v chars, want ~20", w)
	}
	if h := sh.RowHeightPx(7); h != 40 {
		t.Errorf("row 8 height = %v, want 40", h)
	}
	if sh.FreezeRows != 1 || sh.FreezeCols != 1 {
		t.Errorf("freeze = %d,%d", sh.FreezeRows, sh.FreezeCols)
	}
	for r, want := range map[int]string{9: "A10*2", 10: "A11*2", 11: "A12*2"} {
		if got := sh.Get(r, 1).Formula; got != want {
			t.Errorf("B%d formula = %q, want %q", r+1, got, want)
		}
	}
}
