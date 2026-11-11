// macros_test.go checks that a workbook with a VBA project or an Excel 4
// macro sheet is read as having macros, with the load warning.

package xlsxfile

import (
	"bytes"
	"slices"
	"testing"

	"officelite/common/macrotest"
	"officelite/excel/model"
)

// TestReadMacros reads a plain workbook, then the same one with a VBA
// project and with an Excel 4 macro sheet.
func TestReadMacros(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sh := model.NewSheet("Data")
	sh.Set(0, 0, model.Cell{Kind: model.KindText, Text: "x"})
	wb.Sheets = append(wb.Sheets, sh)
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	plain := buf.Bytes()
	vba, err := macrotest.AddToPackage(plain, "xl/_rels/workbook.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	xlm, err := macrotest.AddRel(plain, "xl/_rels/workbook.xml.rels",
		"http://schemas.microsoft.com/office/2006/relationships/xlMacrosheet", "macrosheets/sheet1.xml", "xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{{"plain", plain, false}, {"VBA project", vba, true}, {"Excel 4 macro sheet", xlm, true}} {
		got, err := Read(tc.data)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.HasMacros != tc.want || slices.Contains(got.Warnings, model.MacroWarning) != tc.want {
			t.Errorf("%s: HasMacros %v, warnings %q; want %v", tc.name, got.HasMacros, got.Warnings, tc.want)
		}
		if len(got.Sheets) != 1 || got.Sheets[0].Get(0, 0).Text != "x" {
			t.Errorf("%s: the sheet didn't come through", tc.name)
		}
	}
}
