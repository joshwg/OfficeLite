package xlsxfile

import (
	"archive/zip"
	"bytes"
	"reflect"
	"strings"
	"testing"

	"officelite/excel/model"
)

// validationRules is a rule of each kind, as Samples/Data-Tools has.
func validationRules() []model.Validation {
	return []model.Validation{
		{Ranges: []model.Range{{R1: 1, C1: 0, R2: 9, C2: 0}}, Type: model.ValidList, Formula1: `"Red,Green,Blue"`,
			AllowBlank: true, ShowInput: true, ShowError: true, PromptTitle: "Color", Prompt: "Pick a color"},
		{Ranges: []model.Range{{R1: 1, C1: 1, R2: 9, C2: 1}, {R1: 12, C1: 1, R2: 12, C2: 1}}, Type: model.ValidWhole,
			Op: model.ValidBetween, Formula1: "1", Formula2: "$H$1", ShowError: true, ErrorStyle: model.ErrorWarning,
			ErrorTitle: "Quantity", Error: "1 to 100 & no more < >"},
		{Ranges: []model.Range{{R1: 1, C1: 2, R2: 1, C2: 2}}, Type: model.ValidDate, Op: model.ValidGreaterEqual,
			Formula1: "45292", ShowError: true, ErrorStyle: model.ErrorInformation},
		{Ranges: []model.Range{{R1: 1, C1: 3, R2: 9, C2: 3}}, Type: model.ValidCustom, Formula1: `AND(LEN(D2)<=5,D2<>"x")`, ShowError: true},
		{Ranges: []model.Range{{R1: 1, C1: 4, R2: 9, C2: 4}}, Type: model.ValidList, Formula1: "Lists!$A$1:$A$3", NoDropDown: true},
		{Ranges: []model.Range{{R1: 1, C1: 5, R2: 1, C2: 5}}, Type: model.ValidAny, ShowInput: true, PromptTitle: "Note", Prompt: "Anything goes"},
	}
}

func TestValidationRoundTrip(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sh := model.NewSheet("Sheet1")
	sh.Set(0, 0, model.Cell{Kind: model.KindText, Text: "Color"})
	sh.Validations = validationRules()
	wb.Sheets = []*model.Sheet{sh, model.NewSheet("Lists")}
	got, data := roundTrip(t, wb)
	if !reflect.DeepEqual(got.Sheets[0].Validations, sh.Validations) {
		t.Errorf("rules\n got %+v\nwant %+v", got.Sheets[0].Validations, sh.Validations)
	}
	x := zipParts(t, data)["xl/worksheets/sheet1.xml"]
	for _, want := range []string{`showDropDown="true"`, `errorStyle="warning"`, `sqref="B2:B10 B13"`, `<formula1>AND(LEN(D2)&lt;=5,D2&lt;&gt;&quot;x&quot;)</formula1>`} {
		if !strings.Contains(x, want) {
			t.Errorf("sheet XML lacks %s", want)
		}
	}
	if i, j := strings.Index(x, "<dataValidations"), strings.Index(x, "</sheetData>"); i < j {
		t.Error("dataValidations before sheetData's end")
	}
}

// TestValidationExtLst reads the x14 rules Excel 2010+ writes for lists
// from another sheet.
func TestValidationExtLst(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	wb.Sheets = []*model.Sheet{model.NewSheet("Sheet1"), model.NewSheet("Lists")}
	wb.Sheets[0].Set(0, 0, model.Cell{Kind: model.KindText, Text: "x"})
	_, data := roundTrip(t, wb)
	ext := `<extLst><ext uri="{CCE6A557-97BC-4b89-ADB6-D9C93CAAB3DF}" xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main">` +
		`<x14:dataValidations count="1" xmlns:xm="http://schemas.microsoft.com/office/excel/2006/main">` +
		`<x14:dataValidation type="list" allowBlank="1" showInputMessage="1" showErrorMessage="1" prompt="From Lists">` +
		`<x14:formula1><xm:f>Lists!$A$1:$A$4</xm:f></x14:formula1><xm:sqref>C3:C8 E1</xm:sqref></x14:dataValidation>` +
		`</x14:dataValidations></ext></extLst></worksheet>`
	got, err := Read(rewritePart(t, data, "xl/worksheets/sheet1.xml", func(s string) string {
		return strings.Replace(s, "</worksheet>", ext, 1)
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Validation{{Ranges: []model.Range{{R1: 2, C1: 2, R2: 7, C2: 2}, {R1: 0, C1: 4, R2: 0, C2: 4}},
		Type: model.ValidList, Formula1: "Lists!$A$1:$A$4", AllowBlank: true, ShowInput: true, ShowError: true, Prompt: "From Lists"}}
	if !reflect.DeepEqual(got.Sheets[0].Validations, want) {
		t.Errorf("x14 rules %+v", got.Sheets[0].Validations)
	}
	if got.Sheets[0].Get(0, 0).Text != "x" {
		t.Error("cells lost")
	}
}

// rewritePart returns the package data with one part changed by f.
func rewritePart(t *testing.T, data []byte, name string, f func(string) string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for k, v := range zipParts(t, data) {
		if k == name {
			v = f(v)
		}
		w, err := zw.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(v))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
