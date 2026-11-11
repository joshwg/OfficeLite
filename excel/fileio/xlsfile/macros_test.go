// macros_test.go checks that an .xls with a VBA project is read as
// having macros, with the load warning.

package xlsfile

import (
	"bytes"
	"slices"
	"testing"

	"officelite/common/macrotest"
	"officelite/excel/model"
)

// TestReadMacros reads a plain .xls, then the same one with a VBA
// project stream.
func TestReadMacros(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sh := model.NewSheet("Data")
	sh.Set(0, 0, model.Cell{Kind: model.KindText, Text: "x"})
	wb.Sheets = append(wb.Sheets, sh)
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	vba, err := macrotest.AddToCompound(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{{"plain", buf.Bytes(), false}, {"VBA project", vba, true}} {
		got, err := Read(tc.data)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.HasMacros != tc.want || slices.Contains(got.Warnings, model.MacroWarning) != tc.want {
			t.Errorf("%s: HasMacros %v, warnings %q; want %v", tc.name, got.HasMacros, got.Warnings, tc.want)
		}
	}
}
