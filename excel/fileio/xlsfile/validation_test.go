package xlsfile

import (
	"bytes"
	"reflect"
	"testing"

	"officelite/excel/model"
)

func TestValidationRoundTrip(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	sh.Set(0, 0, model.Cell{Kind: model.KindText, Text: "Color"})
	sh.Validations = []model.Validation{
		{Ranges: []model.Range{{R1: 1, C1: 0, R2: 9, C2: 0}}, Type: model.ValidList, Formula1: `"Red,Green,Blue"`,
			AllowBlank: true, ShowInput: true, ShowError: true, PromptTitle: "Color", Prompt: "Pick a color"},
		{Ranges: []model.Range{{R1: 1, C1: 1, R2: 9, C2: 1}, {R1: 12, C1: 1, R2: 12, C2: 1}}, Type: model.ValidWhole,
			Op: model.ValidBetween, Formula1: "1", Formula2: "$H$1", ShowError: true, ErrorStyle: model.ErrorWarning,
			ErrorTitle: "Quantity", Error: "From 1 to H1"},
		{Ranges: []model.Range{{R1: 1, C1: 2, R2: 1, C2: 2}}, Type: model.ValidDate, Op: model.ValidGreaterEqual,
			Formula1: "45292", ShowError: true, ErrorStyle: model.ErrorInformation},
		{Ranges: []model.Range{{R1: 1, C1: 3, R2: 9, C2: 3}}, Type: model.ValidCustom, Formula1: `AND(LEN(D2)<=5,D2<>"x")`, ShowError: true},
		{Ranges: []model.Range{{R1: 1, C1: 4, R2: 9, C2: 4}}, Type: model.ValidList, Formula1: "Lists!$A$1:$A$3", NoDropDown: true},
		{Ranges: []model.Range{{R1: 1, C1: 5, R2: 9, C2: 5}}, Type: model.ValidList, Formula1: "Colors"},
		{Ranges: []model.Range{{R1: 1, C1: 6, R2: 1, C2: 6}}, Type: model.ValidTextLength, Op: model.ValidLess, Formula1: "G1+2"},
		{Ranges: []model.Range{{R1: 1, C1: 7, R2: 1, C2: 7}}, Type: model.ValidAny, ShowInput: true, PromptTitle: "Note", Prompt: "Anything goes"},
	}
	wb := &model.Workbook{Sheets: []*model.Sheet{sh, model.NewSheet("Lists")}, Styles: []model.Style{model.DefaultStyle()},
		Names: []model.DefinedName{{Name: "Colors", Sheet: -1, Formula: "Lists!$A$1:$A$3"}}}
	if issues := Fits(wb); len(issues) != 0 {
		t.Errorf("Fits = %q", issues)
	}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Sheets[0].Validations, sh.Validations) {
		t.Errorf("rules\n got %+v\nwant %+v", got.Sheets[0].Validations, sh.Validations)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("warnings %q", got.Warnings)
	}
}
