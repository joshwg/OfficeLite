package calc

import (
	"testing"
	"time"

	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

var now = time.Date(2026, 10, 15, 13, 30, 0, 0, time.UTC)

func book(cells map[string]model.Cell) *model.Workbook {
	sh := model.NewSheet("Sheet1")
	for ref, c := range cells {
		row, col := cellPos(ref)
		sh.Set(row, col, c)
	}
	dates := model.DefaultStyle()
	dates.NumFmtCode = "yyyy-mm-dd"
	return &model.Workbook{Sheets: []*model.Sheet{sh, model.NewSheet("Data")},
		Styles: []model.Style{model.DefaultStyle(), dates}}
}

func cellPos(ref string) (row, col int) {
	i := 0
	for ref[i] >= 'A' {
		i++
	}
	n := 0
	for _, d := range ref[i:] {
		n = n*10 + int(d-'0')
	}
	return n - 1, model.ParseColName(ref[:i])
}

func get(wb *model.Workbook, ref string) *model.Cell {
	row, col := cellPos(ref)
	return wb.Sheets[0].Get(row, col)
}

func num(v float64) model.Cell { return model.Cell{Kind: model.KindNumber, Num: v, Text: "x"} }
func fx(f string) model.Cell   { return model.Cell{Formula: f} }

func TestRecalculate(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(2),
		"A2": num(3),
		"B1": fx("A1*A2"),
		"B2": fx("B1+1"), // depends on B1
		"B3": fx("IF(B2>5,\"big\",\"small\")"),
		"B4": fx("1/0"),
		"B5": fx("A9"), // empty cell -> 0
		"B6": {Formula: "45000", Style: 1},
		"B7": fx("Data!A1&\"!\""),
		"B8": fx("TaxRate*100"),
		"C1": fx("C2+1"), // circular
		"C2": fx("C1+1"),
	})
	wb.Names = []model.DefinedName{{Name: "TaxRate", Sheet: -1, Formula: "0.07"}}
	res := Recalculate(wb, functions.Default(), Full, now)
	for ref, want := range map[string]string{
		"B1": "6", "B2": "7", "B3": "big", "B4": "#DIV/0!", "B5": "0", "B6": "2023-03-15",
		"B7": "!", "B8": "7",
	} {
		if got := get(wb, ref).Text; got != want {
			t.Errorf("%s = %q, want %q", ref, got, want)
		}
	}
	if get(wb, "B4").Kind != model.KindError || get(wb, "B3").Kind != model.KindText {
		t.Errorf("kinds: B4 %d, B3 %d", get(wb, "B4").Kind, get(wb, "B3").Kind)
	}
	if len(res.Circular) == 0 {
		t.Error("circular reference not reported")
	}
	if res.Calculated != 10 {
		t.Errorf("calculated %d formulas, want 10", res.Calculated)
	}
}

func TestMissingModeKeepsCachedValues(t *testing.T) {
	cached := model.Cell{Formula: "1+1", Kind: model.KindNumber, Num: 99, Text: "99"}
	wb := book(map[string]model.Cell{"A1": cached, "A2": fx("A1*2")})
	Recalculate(wb, functions.Default(), Missing, now)
	if got := get(wb, "A1").Text; got != "99" {
		t.Errorf("A1 = %q, want the cached 99", got)
	}
	if got := get(wb, "A2").Text; got != "198" {
		t.Errorf("A2 = %q, want 198", got)
	}
}

func TestSpill(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": {Formula: "{1,2;3,4}*10", Dynamic: true},
		"D1": fx("B2+1"),     // reads a spilled cell, calculated first in row order
		"D2": fx("SUM(A1#)"), // no SUM in the logical-only registry
		"D3": fx("A1#*2"),
		"F1": {Formula: "{1;2;3}", Dynamic: true},
		"F2": num(5),      // blocks the spill
		"H1": fx("{7,8}"), // not dynamic: shows the first element only
	})
	wb.Sheets[0].Get(2, 3).Dynamic = true // D3 spills too
	Recalculate(wb, functions.Default(), Full, now)
	for ref, want := range map[string]string{
		"A1": "10", "B1": "20", "A2": "30", "B2": "40", "D1": "41",
		"F1": "#SPILL!", "H1": "7", "I1": "",
		"D3": "20", "E3": "40", "D4": "60", "E4": "80",
	} {
		if got := get(wb, ref).Text; got != want {
			t.Errorf("%s = %q, want %q", ref, got, want)
		}
	}
	if rng := wb.Sheets[0].Spills[[2]int{0, 0}]; rng != (model.Range{R2: 1, C2: 1}) {
		t.Errorf("spill range = %+v", rng)
	}
	// Recalculating a smaller result clears the old spill area.
	wb.Sheets[0].Get(0, 0).Formula = "{5}"
	Recalculate(wb, functions.Default(), Full, now)
	if got := get(wb, "B2").Kind; got != model.KindEmpty {
		t.Errorf("B2 not cleared after the spill shrank: kind %d", got)
	}
}
